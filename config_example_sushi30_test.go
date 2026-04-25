package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sushi30/sushiclaw/pkg/channels/email"
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
	if cfg.Onboarding.Auto.Enabled {
		t.Error("onboarding.auto.enabled should default to false in example config")
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

	// Email is wired separately via email.InitChannel (not through ChannelsConfig).
	// Decode email_channel directly into email.EmailConfig so json tag renames break this test.
	var rawTop struct {
		EmailChannel email.EmailConfig `json:"email_channel"`
	}
	if err := json.Unmarshal(data, &rawTop); err != nil {
		t.Fatalf("parse email_channel section: %v", err)
	}
	if rawTop.EmailChannel.SMTPHost == "" {
		t.Error("email_channel.smtp_host missing from example config")
	}
}
