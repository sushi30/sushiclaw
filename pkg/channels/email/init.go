package email

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/channels"
)

// InitChannel loads email config, resolves env vars, and returns an initialized
// EmailChannel. Returns (nil, nil) if email is disabled in config.
// Returns an error if email is enabled but a required env var is unset.
func InitChannel(b *bus.MessageBus) (channels.Channel, error) {
	emailCfg, err := loadEmailConfig()
	if err != nil {
		return nil, err
	}
	if !emailCfg.Enabled {
		return nil, nil
	}
	return NewEmailChannel(emailCfg, b)
}

// loadEmailConfig reads the "channels.email" section from the config file and
// resolves env:// references. Required fields return an error if unresolved.
func loadEmailConfig() (EmailConfig, error) {
	path := configFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return EmailConfig{}, nil // no config file = not enabled
	}

	// Try the new top-level "email_channel" key first (V3+ configs and new example format).
	// Fall back to the legacy "channels"."email" location for backwards compat.
	var raw struct {
		EmailChannel EmailConfig `json:"email_channel"`
		Channels     struct {
			Email EmailConfig `json:"email"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return EmailConfig{}, err
	}
	cfg := raw.EmailChannel
	if !cfg.Enabled && raw.Channels.Email.Enabled {
		cfg = raw.Channels.Email
	}
	if !cfg.Enabled {
		return cfg, nil
	}

	// Required fields — return an error if not set or unresolved env://.
	if cfg.SMTPFrom.String() == "" || cfg.SMTPFrom.IsUnresolvedEnv() {
		return EmailConfig{}, fmt.Errorf("smtp_from is required (unresolved: %s)", cfg.SMTPFrom.String())
	}
	if cfg.IMAPUser.String() == "" || cfg.IMAPUser.IsUnresolvedEnv() {
		return EmailConfig{}, fmt.Errorf("imap_user is required (unresolved: %s)", cfg.IMAPUser.String())
	}
	if cfg.IMAPPassword.String() == "" || cfg.IMAPPassword.IsUnresolvedEnv() {
		return EmailConfig{}, fmt.Errorf("imap_password is required (unresolved: %s)", cfg.IMAPPassword.String())
	}

	return cfg, nil
}

func configFilePath() string {
	if p := os.Getenv("SUSHICLAW_CONFIG"); p != "" {
		return p
	}
	if p := os.Getenv("PICOCLAW_CONFIG"); p != "" {
		return p
	}
	home := os.Getenv("SUSHICLAW_HOME")
	if home == "" {
		home = os.Getenv("PICOCLAW_HOME")
	}
	if home == "" {
		if h, err := os.UserHomeDir(); err == nil {
			home = filepath.Join(h, ".picoclaw")
		}
	}
	return filepath.Join(home, "config.json")
}
