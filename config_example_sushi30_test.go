package main_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sushi30/sushiclaw/pkg/config"
)

func TestExampleConfigLoadsAsV2(t *testing.T) {
	_, callerFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Dir(callerFile)
	src := filepath.Join(repoRoot, "config.example.json")

	// Copy to tmpDir so LoadConfig can't mutate the source file
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "config.json")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read config.example.json: %v", err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := config.LoadConfig(dst)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Version != 2 {
		t.Errorf("Version = %d, want 2", cfg.Version)
	}

	if len(cfg.ModelList) == 0 {
		t.Error("model_list is empty")
	}

	// WhatsApp: use_native should be set (channel registered under "whatsapp_native" key)
	waCh := cfg.Channels["whatsapp_native"]
	if waCh == nil {
		t.Fatal("whatsapp_native channel config missing")
	}
	var waCfg config.WhatsAppSettings
	if err := waCh.Decode(&waCfg); err != nil {
		t.Fatalf("decode whatsapp_native settings: %v", err)
	}
	if !waCfg.UseNative {
		t.Error("whatsapp use_native should be true in example config")
	}

	// Telegram: streaming should be configured
	tgCh := cfg.Channels["telegram"]
	if tgCh == nil {
		t.Fatal("telegram channel config missing")
	}
	var tgCfg config.TelegramSettings
	if err := tgCh.Decode(&tgCfg); err != nil {
		t.Fatalf("decode telegram settings: %v", err)
	}
	if !tgCfg.Streaming.Enabled {
		t.Error("telegram streaming.enabled should be true in example config")
	}

	emailCh := cfg.Channels["email"]
	if emailCh == nil {
		t.Fatal("email channel config missing")
	}
	if emailCh.Type != config.ChannelEmail {
		t.Errorf("email type = %q, want %q", emailCh.Type, config.ChannelEmail)
	}
	var emailCfg config.EmailSettings
	if err := emailCh.Decode(&emailCfg); err != nil {
		t.Fatalf("decode email settings: %v", err)
	}
	if emailCfg.SMTPHost == "" {
		t.Error("channels.email.smtp_host missing from example config")
	}
	if emailCfg.IMAPPassword.String() == "" {
		t.Error("channels.email.imap_password missing from example config")
	}
}
