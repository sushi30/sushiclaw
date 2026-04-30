package conversationlock

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sushi30/sushiclaw/pkg/config"
)

var (
	ErrDisabled           = errors.New("conversation lock is disabled")
	ErrAlreadyUnlocked    = errors.New("conversation is already unlocked")
	ErrMissingUnlockEmail = errors.New("unlock email is not configured")
	ErrMissingSender      = errors.New("unlock email sender is not configured")
	ErrInvalidCode        = errors.New("invalid unlock code")
	ErrExpiredCode        = errors.New("unlock code expired")
	ErrSupportRequired    = errors.New("too many unlock attempts")
)

type EmailSender interface {
	SendUnlockCode(ctx context.Context, to, code string, expiresIn time.Duration) error
}

type Options struct {
	Enabled         bool
	StandbyTimeout  time.Duration
	GlobalTimeout   time.Duration
	OTAExpiry       time.Duration
	MaxAttempts     int
	LockoutDuration time.Duration
	UnlockEmail     string
	LockoutPath     string
}

type Manager struct {
	mu     sync.Mutex
	opts   Options
	sender EmailSender
	now    func() time.Time
	code   func() (string, error)
	states map[string]*sessionState
}

type sessionState struct {
	Locked       bool
	LastActivity time.Time
	LastUnlock   time.Time
	CodeHash     [32]byte
	CodeExpiry   time.Time
	HasCode      bool
	Attempts     int
	FirstAttempt time.Time
	LockedOut    bool
	LockedUntil  time.Time
}

func OptionsFromConfig(cfg config.ConversationLockConfig) Options {
	expiry := time.Duration(cfg.OTAExpiryMinutes) * time.Minute
	if cfg.Enabled && expiry <= 0 {
		expiry = 10 * time.Minute
	}
	maxAttempts := cfg.MaxUnlockAttempts
	if cfg.Enabled && maxAttempts <= 0 {
		maxAttempts = 5
	}
	return Options{
		Enabled:         cfg.Enabled,
		StandbyTimeout:  time.Duration(cfg.StandbyAutoLockMinutes) * time.Minute,
		GlobalTimeout:   time.Duration(cfg.GlobalAutoLockMinutes) * time.Minute,
		OTAExpiry:       expiry,
		MaxAttempts:     maxAttempts,
		LockoutDuration: 24 * time.Hour,
		UnlockEmail:     strings.TrimSpace(cfg.UnlockEmail),
	}
}

func NewManager(opts Options, sender EmailSender) *Manager {
	m := &Manager{
		opts:   opts,
		sender: sender,
		now:    time.Now,
		code:   generateCode,
		states: make(map[string]*sessionState),
	}
	m.loadLockouts()
	return m
}

func (m *Manager) Enabled() bool {
	return m != nil && m.opts.Enabled
}

func (m *Manager) Lock(sessionKey string) error {
	if !m.Enabled() {
		return ErrDisabled
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.ensureLocked(sessionKey)
	st.Locked = true
	return nil
}

func (m *Manager) RequestUnlock(ctx context.Context, sessionKey string) error {
	if !m.Enabled() {
		return ErrDisabled
	}
	if strings.TrimSpace(m.opts.UnlockEmail) == "" {
		return ErrMissingUnlockEmail
	}
	if m.sender == nil {
		return ErrMissingSender
	}
	now := m.now()
	code, err := m.code()
	if err != nil {
		return err
	}
	codeHash := sha256.Sum256([]byte(code))

	m.mu.Lock()
	st := m.ensure(sessionKey, now)
	m.clearExpiredLockout(st, now)
	if st.LockedOut {
		m.mu.Unlock()
		return ErrSupportRequired
	}
	if !st.Locked && !m.shouldAutoLock(st, now) {
		m.mu.Unlock()
		return ErrAlreadyUnlocked
	}
	st.Locked = true
	st.CodeHash = codeHash
	st.CodeExpiry = now.Add(m.opts.OTAExpiry)
	st.HasCode = true
	expiresIn := m.opts.OTAExpiry
	m.mu.Unlock()

	return m.sender.SendUnlockCode(ctx, m.opts.UnlockEmail, code, expiresIn)
}

func (m *Manager) VerifyUnlock(sessionKey, code string) error {
	if !m.Enabled() {
		return ErrDisabled
	}
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()

	st := m.ensure(sessionKey, now)
	m.clearExpiredLockout(st, now)
	if st.LockedOut {
		return ErrSupportRequired
	}
	if !st.Locked && !m.shouldAutoLock(st, now) {
		return ErrAlreadyUnlocked
	}
	if !st.HasCode {
		m.recordFailedAttempt(st)
		if st.LockedOut {
			return ErrSupportRequired
		}
		return ErrInvalidCode
	}
	if !st.CodeExpiry.IsZero() && !now.Before(st.CodeExpiry) {
		st.HasCode = false
		st.CodeHash = [32]byte{}
		m.recordFailedAttempt(st)
		if st.LockedOut {
			return ErrSupportRequired
		}
		return ErrExpiredCode
	}
	codeHash := sha256.Sum256([]byte(strings.TrimSpace(code)))
	if subtle.ConstantTimeCompare(codeHash[:], st.CodeHash[:]) != 1 {
		m.recordFailedAttempt(st)
		if st.LockedOut {
			return ErrSupportRequired
		}
		return ErrInvalidCode
	}
	st.Locked = false
	st.LastActivity = now
	st.LastUnlock = now
	st.HasCode = false
	st.CodeHash = [32]byte{}
	st.CodeExpiry = time.Time{}
	st.Attempts = 0
	st.FirstAttempt = time.Time{}
	return nil
}

func (m *Manager) IsLocked(sessionKey string) bool {
	if !m.Enabled() {
		return false
	}
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.ensure(sessionKey, now)
	m.clearExpiredLockout(st, now)
	if m.shouldAutoLock(st, now) {
		st.Locked = true
	}
	return st.Locked
}

func (m *Manager) RecordActivity(sessionKey string) {
	if !m.Enabled() {
		return
	}
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.ensure(sessionKey, now)
	m.clearExpiredLockout(st, now)
	if !st.Locked {
		st.LastActivity = now
		if st.LastUnlock.IsZero() {
			st.LastUnlock = now
		}
	}
}

func (m *Manager) ensureLocked(sessionKey string) *sessionState {
	now := m.now()
	st := m.ensure(sessionKey, now)
	if st.LastUnlock.IsZero() {
		st.LastUnlock = now
	}
	return st
}

func (m *Manager) ensure(sessionKey string, now time.Time) *sessionState {
	key := strings.TrimSpace(sessionKey)
	st, ok := m.states[key]
	if ok {
		return st
	}
	st = &sessionState{
		LastActivity: now,
		LastUnlock:   now,
	}
	m.states[key] = st
	return st
}

func (m *Manager) shouldAutoLock(st *sessionState, now time.Time) bool {
	m.clearExpiredLockout(st, now)
	if st.LockedOut {
		return true
	}
	if st.Locked {
		return false
	}
	if m.opts.StandbyTimeout > 0 && !st.LastActivity.IsZero() && !now.Before(st.LastActivity.Add(m.opts.StandbyTimeout)) {
		return true
	}
	if m.opts.GlobalTimeout > 0 && !st.LastUnlock.IsZero() && !now.Before(st.LastUnlock.Add(m.opts.GlobalTimeout)) {
		return true
	}
	return false
}

func (m *Manager) recordFailedAttempt(st *sessionState) {
	now := m.now()
	if !st.FirstAttempt.IsZero() && !now.Before(st.FirstAttempt.Add(m.lockoutDuration())) {
		st.Attempts = 0
		st.FirstAttempt = time.Time{}
	}
	if st.Attempts == 0 {
		st.FirstAttempt = now
	}
	st.Attempts++
	if m.opts.MaxAttempts > 0 && st.Attempts >= m.opts.MaxAttempts {
		st.Locked = true
		st.LockedOut = true
		st.HasCode = false
		st.CodeHash = [32]byte{}
		st.CodeExpiry = time.Time{}
		st.LockedUntil = now.Add(m.lockoutDuration())
		_ = m.saveLockouts()
	}
}

type lockoutFile struct {
	Version  int                  `json:"version"`
	Sessions map[string]time.Time `json:"sessions"`
}

func (m *Manager) loadLockouts() {
	path := strings.TrimSpace(m.opts.LockoutPath)
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var file lockoutFile
	if err := json.Unmarshal(data, &file); err != nil {
		return
	}
	now := m.now()
	for key, lockedUntil := range file.Sessions {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if !lockedUntil.IsZero() && !now.Before(lockedUntil) {
			continue
		}
		m.states[key] = &sessionState{
			Locked:       true,
			LockedOut:    true,
			LockedUntil:  lockedUntil,
			LastActivity: now,
			LastUnlock:   now,
		}
	}
}

func (m *Manager) saveLockouts() error {
	path := strings.TrimSpace(m.opts.LockoutPath)
	if path == "" {
		return nil
	}
	sessions := make(map[string]time.Time)
	now := m.now()
	for key, st := range m.states {
		m.clearExpiredLockout(st, now)
		if st.LockedOut {
			sessions[key] = st.LockedUntil
		}
	}
	data, err := json.MarshalIndent(lockoutFile{Version: 1, Sessions: sessions}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (m *Manager) clearExpiredLockout(st *sessionState, now time.Time) {
	if st == nil || !st.LockedOut || st.LockedUntil.IsZero() || now.Before(st.LockedUntil) {
		return
	}
	st.LockedOut = false
	st.LockedUntil = time.Time{}
	st.Attempts = 0
	st.FirstAttempt = time.Time{}
	st.HasCode = false
	st.CodeHash = [32]byte{}
	st.CodeExpiry = time.Time{}
}

func (m *Manager) lockoutDuration() time.Duration {
	if m.opts.LockoutDuration > 0 {
		return m.opts.LockoutDuration
	}
	return 24 * time.Hour
}

func generateCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", fmt.Errorf("generate unlock code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
