package email

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sushi30/sushiclaw/internal/envresolve"
)

func init() {
	channels.RegisterFactory("email", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return nil, nil
	})
}

func LoadEmailConfig() (EmailConfig, error) {
	return loadEmailConfig()
}

func loadEmailConfig() (EmailConfig, error) {
	path := configFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return EmailConfig{}, nil // no config file = not enabled
	}

	var raw struct {
		Channels struct {
			Email EmailConfig `json:"email"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return EmailConfig{}, err
	}
	cfg := raw.Channels.Email
	envresolve.SecureString(&cfg.SMTPFrom)
	envresolve.SecureString(&cfg.SMTPUser)
	envresolve.SecureString(&cfg.SMTPPassword)
	envresolve.SecureString(&cfg.IMAPUser)
	envresolve.SecureString(&cfg.IMAPPassword)
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
