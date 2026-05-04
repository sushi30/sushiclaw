package conversationlock

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sushi30/sushiclaw/pkg/commands"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/logger"
)

type Controller struct {
	manager *Manager
}

func NewControllerFromConfig(cfg config.ConversationLockConfig, lockoutPath string) (*Controller, error) {
	if !cfg.Enabled {
		return &Controller{}, nil
	}
	apiKey := ""
	if cfg.ResendAPIKey != nil {
		apiKey = cfg.ResendAPIKey.String()
	}
	sender, err := NewResendSender(apiKey, cfg.FromEmail)
	if err != nil {
		return nil, fmt.Errorf("conversation lock email sender: %w", err)
	}
	opts := OptionsFromConfig(cfg)
	opts.LockoutPath = lockoutPath
	return &Controller{manager: NewManager(opts, sender)}, nil
}

func (c *Controller) Enabled() bool {
	return c != nil && c.manager != nil && c.manager.Enabled()
}

func (c *Controller) Register(rt *commands.Runtime) {
	if rt == nil {
		return
	}
	rt.LockConversation = c.lockConversation
	rt.RequestUnlock = c.requestUnlock
	rt.VerifyUnlockCode = c.verifyUnlockCode
}

func (c *Controller) IsLocked(sessionKey string) bool {
	return c.Enabled() && c.manager.IsLocked(sessionKey)
}

func (c *Controller) RecordActivity(sessionKey string) {
	if c.Enabled() {
		c.manager.RecordActivity(sessionKey)
	}
}

func (c *Controller) LockedReply() string {
	return "Conversation is locked. Use /unlock to request an unlock code."
}

func (c *Controller) LogEnabled(cfg config.ConversationLockConfig) {
	if !c.Enabled() {
		return
	}
	logger.InfoCF("gateway", "Conversation lock enabled", map[string]any{
		"standby_auto_lock_minutes": cfg.StandbyAutoLockMinutes,
		"global_auto_lock_minutes":  cfg.GlobalAutoLockMinutes,
		"ota_expiry_minutes":        cfg.OTAExpiryMinutes,
		"max_unlock_attempts":       c.manager.opts.MaxAttempts,
		"lockout_path":              c.manager.opts.LockoutPath,
	})
}

func (c *Controller) lockConversation(_ context.Context, req commands.Request) string {
	if !c.Enabled() {
		return "Conversation lock is disabled."
	}
	if err := c.manager.Lock(req.SessionKey); err != nil {
		return replyForError(err)
	}
	return "Conversation locked."
}

func (c *Controller) requestUnlock(ctx context.Context, req commands.Request) string {
	if !c.Enabled() {
		return "Conversation lock is disabled."
	}
	if err := c.manager.RequestUnlock(ctx, req.SessionKey); err != nil {
		return replyForError(err)
	}
	return "Unlock code sent."
}

func (c *Controller) verifyUnlockCode(_ context.Context, req commands.Request, code string) string {
	if !c.Enabled() {
		return "Conversation lock is disabled."
	}
	if err := c.manager.VerifyUnlock(req.SessionKey, code); err != nil {
		return replyForError(err)
	}
	return "Conversation unlocked."
}

func replyForError(err error) string {
	switch {
	case errors.Is(err, ErrDisabled):
		return "Conversation lock is disabled."
	case errors.Is(err, ErrAlreadyUnlocked):
		return "Conversation is already unlocked."
	case errors.Is(err, ErrMissingUnlockEmail):
		return "Unlock email is not configured."
	case errors.Is(err, ErrMissingSender):
		return "Unlock email sender is not configured."
	case errors.Is(err, ErrInvalidCode):
		return "Invalid unlock code."
	case errors.Is(err, ErrExpiredCode):
		return "Unlock code expired. Use /unlock to request a new code."
	case errors.Is(err, ErrSupportRequired):
		return "Too many invalid unlock attempts. Please reach out for support using the email in the bot description."
	default:
		return "Unlock failed: " + err.Error()
	}
}

func IsUnlockCommand(input string) bool {
	name, ok := commands.CommandName(input)
	return ok && name == "unlock"
}

func RedactCommand(input string) string {
	if !IsUnlockCommand(input) {
		return input
	}
	fields := strings.Fields(input)
	if len(fields) <= 1 {
		return input
	}
	return fields[0] + " [redacted]"
}
