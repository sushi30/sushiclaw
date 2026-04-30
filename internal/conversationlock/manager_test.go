package conversationlock

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeSender struct {
	to   string
	code string
	err  error
}

func (s *fakeSender) SendUnlockCode(_ context.Context, to, code string, _ time.Duration) error {
	s.to = to
	s.code = code
	return s.err
}

func newTestManager(now time.Time, sender *fakeSender) *Manager {
	m := NewManager(Options{
		Enabled:        true,
		StandbyTimeout: 10 * time.Minute,
		GlobalTimeout:  time.Hour,
		OTAExpiry:      5 * time.Minute,
		UnlockEmail:    "user@example.com",
	}, sender)
	m.now = func() time.Time { return now }
	m.code = func() (string, error) { return "123456", nil }
	return m
}

func TestManagerLockAndUnlockWithSingleUseCode(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	sender := &fakeSender{}
	m := newTestManager(now, sender)

	if err := m.Lock("telegram:1"); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if !m.IsLocked("telegram:1") {
		t.Fatal("expected session locked")
	}
	if err := m.RequestUnlock(context.Background(), "telegram:1"); err != nil {
		t.Fatalf("RequestUnlock: %v", err)
	}
	if sender.to != "user@example.com" || sender.code != "123456" {
		t.Fatalf("sent to/code = %q/%q", sender.to, sender.code)
	}
	if err := m.VerifyUnlock("telegram:1", "123456"); err != nil {
		t.Fatalf("VerifyUnlock: %v", err)
	}
	if m.IsLocked("telegram:1") {
		t.Fatal("expected session unlocked")
	}
	if err := m.VerifyUnlock("telegram:1", "123456"); !errors.Is(err, ErrAlreadyUnlocked) {
		t.Fatalf("reused code error = %v, want ErrAlreadyUnlocked", err)
	}
}

func TestManagerInvalidCodeKeepsLocked(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	m := newTestManager(now, &fakeSender{})

	_ = m.Lock("telegram:1")
	_ = m.RequestUnlock(context.Background(), "telegram:1")
	if err := m.VerifyUnlock("telegram:1", "000000"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("VerifyUnlock error = %v, want ErrInvalidCode", err)
	}
	if !m.IsLocked("telegram:1") {
		t.Fatal("expected session to remain locked")
	}
}

func TestManagerExpiredCodeKeepsLocked(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	m := newTestManager(now, &fakeSender{})
	current := now
	m.now = func() time.Time { return current }

	_ = m.Lock("telegram:1")
	_ = m.RequestUnlock(context.Background(), "telegram:1")
	current = now.Add(6 * time.Minute)
	if err := m.VerifyUnlock("telegram:1", "123456"); !errors.Is(err, ErrExpiredCode) {
		t.Fatalf("VerifyUnlock error = %v, want ErrExpiredCode", err)
	}
	if !m.IsLocked("telegram:1") {
		t.Fatal("expected session to remain locked")
	}
}

func TestManagerAutoLocksOnStandbyTimeout(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	current := now
	m := newTestManager(now, &fakeSender{})
	m.now = func() time.Time { return current }

	m.RecordActivity("telegram:1")
	current = now.Add(9 * time.Minute)
	if m.IsLocked("telegram:1") {
		t.Fatal("expected session unlocked before standby timeout")
	}
	current = now.Add(10 * time.Minute)
	if !m.IsLocked("telegram:1") {
		t.Fatal("expected session locked at standby timeout")
	}
}

func TestManagerAutoLocksOnGlobalTimeoutDespiteActivity(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	current := now
	m := newTestManager(now, &fakeSender{})
	m.now = func() time.Time { return current }
	m.opts.StandbyTimeout = 10 * time.Minute
	m.opts.GlobalTimeout = 30 * time.Minute

	m.RecordActivity("telegram:1")
	for i := 1; i <= 3; i++ {
		current = now.Add(time.Duration(i*9) * time.Minute)
		m.RecordActivity("telegram:1")
		if m.IsLocked("telegram:1") {
			t.Fatalf("unexpected lock before global timeout at iteration %d", i)
		}
	}
	current = now.Add(30 * time.Minute)
	if !m.IsLocked("telegram:1") {
		t.Fatal("expected session locked at global timeout")
	}
}

func TestManagerDisabledTimers(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	current := now
	m := newTestManager(now, &fakeSender{})
	m.now = func() time.Time { return current }
	m.opts.StandbyTimeout = 0
	m.opts.GlobalTimeout = 0

	m.RecordActivity("telegram:1")
	current = now.Add(24 * time.Hour)
	if m.IsLocked("telegram:1") {
		t.Fatal("expected disabled timers not to lock session")
	}
}

func TestManagerDisabledFeature(t *testing.T) {
	m := NewManager(Options{}, &fakeSender{})
	if m.IsLocked("telegram:1") {
		t.Fatal("disabled manager should not lock")
	}
	if err := m.Lock("telegram:1"); !errors.Is(err, ErrDisabled) {
		t.Fatalf("Lock error = %v, want ErrDisabled", err)
	}
}

func TestManagerPersistsSupportLockout(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	path := t.TempDir() + "/.lock"
	sender := &fakeSender{}
	m := NewManager(Options{
		Enabled:     true,
		OTAExpiry:   5 * time.Minute,
		MaxAttempts: 2,
		UnlockEmail: "user@example.com",
		LockoutPath: path,
	}, sender)
	m.now = func() time.Time { return now }
	m.code = func() (string, error) { return "123456", nil }

	_ = m.Lock("telegram:1")
	_ = m.RequestUnlock(context.Background(), "telegram:1")
	if err := m.VerifyUnlock("telegram:1", "000000"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("first VerifyUnlock error = %v, want ErrInvalidCode", err)
	}
	if err := m.VerifyUnlock("telegram:1", "111111"); !errors.Is(err, ErrSupportRequired) {
		t.Fatalf("second VerifyUnlock error = %v, want ErrSupportRequired", err)
	}

	reloaded := NewManager(Options{
		Enabled:     true,
		OTAExpiry:   5 * time.Minute,
		MaxAttempts: 2,
		UnlockEmail: "user@example.com",
		LockoutPath: path,
	}, sender)
	reloaded.now = func() time.Time { return now }
	if !reloaded.IsLocked("telegram:1") {
		t.Fatal("expected persisted lockout to be locked after reload")
	}
	if err := reloaded.RequestUnlock(context.Background(), "telegram:1"); !errors.Is(err, ErrSupportRequired) {
		t.Fatalf("RequestUnlock after reload error = %v, want ErrSupportRequired", err)
	}
}

func TestManagerLockoutExpiresAfterTwentyFourHours(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	current := now
	path := t.TempDir() + "/.lock"
	m := NewManager(Options{
		Enabled:         true,
		OTAExpiry:       5 * time.Minute,
		MaxAttempts:     2,
		LockoutDuration: 24 * time.Hour,
		UnlockEmail:     "user@example.com",
		LockoutPath:     path,
	}, &fakeSender{})
	m.now = func() time.Time { return current }
	m.code = func() (string, error) { return "123456", nil }

	_ = m.Lock("telegram:1")
	_ = m.RequestUnlock(context.Background(), "telegram:1")
	_ = m.VerifyUnlock("telegram:1", "000000")
	if err := m.VerifyUnlock("telegram:1", "111111"); !errors.Is(err, ErrSupportRequired) {
		t.Fatalf("VerifyUnlock error = %v, want ErrSupportRequired", err)
	}

	current = now.Add(24*time.Hour + time.Second)
	if !m.IsLocked("telegram:1") {
		t.Fatal("session should remain locked after support lockout expires")
	}
	if err := m.RequestUnlock(context.Background(), "telegram:1"); err != nil {
		t.Fatalf("RequestUnlock after lockout expiry: %v", err)
	}
	if err := m.VerifyUnlock("telegram:1", "000000"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("VerifyUnlock after lockout expiry error = %v, want ErrInvalidCode", err)
	}
}

func TestManagerAttemptCounterResetsAfterTwentyFourHours(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	current := now
	m := NewManager(Options{
		Enabled:         true,
		OTAExpiry:       5 * time.Minute,
		MaxAttempts:     2,
		LockoutDuration: 24 * time.Hour,
		UnlockEmail:     "user@example.com",
	}, &fakeSender{})
	m.now = func() time.Time { return current }
	m.code = func() (string, error) { return "123456", nil }

	_ = m.Lock("telegram:1")
	_ = m.RequestUnlock(context.Background(), "telegram:1")
	if err := m.VerifyUnlock("telegram:1", "000000"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("first VerifyUnlock error = %v, want ErrInvalidCode", err)
	}

	current = now.Add(24*time.Hour + time.Second)
	_ = m.RequestUnlock(context.Background(), "telegram:1")
	if err := m.VerifyUnlock("telegram:1", "111111"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("second VerifyUnlock error = %v, want ErrInvalidCode", err)
	}
	if err := m.RequestUnlock(context.Background(), "telegram:1"); err != nil {
		t.Fatalf("RequestUnlock after reset: %v", err)
	}
}

func TestManagerMissingUnlockEmail(t *testing.T) {
	m := NewManager(Options{Enabled: true, OTAExpiry: time.Minute}, &fakeSender{})
	_ = m.Lock("telegram:1")
	if err := m.RequestUnlock(context.Background(), "telegram:1"); !errors.Is(err, ErrMissingUnlockEmail) {
		t.Fatalf("RequestUnlock error = %v, want ErrMissingUnlockEmail", err)
	}
}
