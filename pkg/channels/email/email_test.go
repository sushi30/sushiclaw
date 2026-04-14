package email_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/channels/email"
)

func TestLoadEmailConfig(t *testing.T) {
	tests := []struct {
		name        string
		configJSON  string
		wantEnabled bool
		wantErr     bool
	}{
		{
			name: "disabled",
			configJSON: `{
				"channels": {
					"email": {
						"enabled": false,
						"smtp_host": "smtp.example.com",
						"imap_host": "imap.example.com"
					}
				}
			}`,
			wantEnabled: false,
			wantErr:     false,
		},
		{
			name: "enabled with required fields",
			configJSON: `{
				"channels": {
					"email": {
						"enabled": true,
						"smtp_host": "smtp.gmail.com",
						"smtp_port": 587,
						"smtp_from": "bot@example.com",
						"imap_host": "imap.gmail.com",
						"imap_port": 993,
						"imap_user": "bot@example.com",
						"poll_interval_secs": 30
					}
				}
			}`,
			wantEnabled: true,
			wantErr:     false,
		},
		{
			name: "no email section",
			configJSON: `{
				"channels": {}
			}`,
			wantEnabled: false,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.json")
			if err := os.WriteFile(configPath, []byte(tt.configJSON), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			t.Setenv("SUSHICLAW_CONFIG", configPath)

			cfg, err := email.LoadEmailConfig()
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadEmailConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if cfg.Enabled != tt.wantEnabled {
				t.Errorf("LoadEmailConfig().Enabled = %v, want %v", cfg.Enabled, tt.wantEnabled)
			}
		})
	}
}

func TestNewEmailChannelValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     email.EmailConfig
		wantErr bool
	}{
		{
			name: "missing smtp_host",
			cfg: email.EmailConfig{
				Enabled:  true,
				SMTPFrom: mustSecureString("bot@example.com"),
				IMAPHost: "imap.example.com",
				IMAPUser: mustSecureString("bot@example.com"),
			},
			wantErr: true,
		},
		{
			name: "missing smtp_from",
			cfg: email.EmailConfig{
				Enabled:  true,
				SMTPHost: "smtp.example.com",
				IMAPHost: "imap.example.com",
				IMAPUser: mustSecureString("bot@example.com"),
			},
			wantErr: true,
		},
		{
			name: "missing imap_host",
			cfg: email.EmailConfig{
				Enabled:  true,
				SMTPHost: "smtp.example.com",
				SMTPFrom: mustSecureString("bot@example.com"),
				IMAPUser: mustSecureString("bot@example.com"),
			},
			wantErr: true,
		},
		{
			name: "missing imap_user",
			cfg: email.EmailConfig{
				Enabled:  true,
				SMTPHost: "smtp.example.com",
				SMTPFrom: mustSecureString("bot@example.com"),
				IMAPHost: "imap.example.com",
			},
			wantErr: true,
		},
		{
			name: "valid config",
			cfg: email.EmailConfig{
				Enabled:  true,
				SMTPHost: "smtp.gmail.com",
				SMTPPort: 587,
				SMTPFrom: mustSecureString("bot@example.com"),
				IMAPHost: "imap.gmail.com",
				IMAPPort: 993,
				IMAPUser: mustSecureString("bot@example.com"),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgBus := bus.NewMessageBus()
			_, err := email.NewEmailChannel(tt.cfg, msgBus)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewEmailChannel() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadEmailConfigFromExampleConfig(t *testing.T) {
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "config.json")

	src, err := os.Open("../../../config.example.json")
	if err != nil {
		t.Fatalf("open example config: %v", err)
	}
	data, err := os.ReadFile(src.Name())
	if closeErr := src.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("read example config: %v", err)
	}
	if err = os.WriteFile(dst, data, 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	t.Setenv("SUSHICLAW_CONFIG", dst)

	cfg, err := email.LoadEmailConfig()
	if err != nil {
		t.Fatalf("LoadEmailConfig: %v", err)
	}

	if cfg.Enabled != false {
		t.Errorf("email.enabled = %v, want false", cfg.Enabled)
	}
	if cfg.SMTPHost != "smtp.example.com" {
		t.Errorf("email.smtp_host = %q, want %q", cfg.SMTPHost, "smtp.example.com")
	}
	if cfg.IMAPHost != "imap.example.com" {
		t.Errorf("email.imap_host = %q, want %q", cfg.IMAPHost, "imap.example.com")
	}
	if cfg.DefaultSubject != "Message from sushiclaw" {
		t.Errorf("email.default_subject = %q, want %q", cfg.DefaultSubject, "Message from sushiclaw")
	}
	if cfg.PollIntervalSecs != 30 {
		t.Errorf("email.poll_interval_secs = %d, want 30", cfg.PollIntervalSecs)
	}
}

func mustSecureString(s string) config.SecureString {
	var ss config.SecureString
	if err := json.Unmarshal([]byte(`"`+s+`"`), &ss); err != nil {
		panic(err)
	}
	return ss
}
