package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
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

	if cfg.Version != config.CurrentVersion {
		t.Errorf("Version = %d, want %d", cfg.Version, config.CurrentVersion)
	}

	if len(cfg.ModelList) == 0 {
		t.Error("model_list is empty")
	}

	// WhatsApp: use_native should be set
	if !cfg.Channels.WhatsApp.UseNative {
		t.Error("whatsapp use_native should be true in example config")
	}

	// Telegram: streaming should be configured
	if !cfg.Channels.Telegram.Streaming.Enabled {
		t.Error("telegram streaming.enabled should be true in example config")
	}

	// Email channel is owned by sushiclaw and not part of picoclaw's ChannelsConfig.
	// Verify the section is present and parseable via raw JSON.
	var raw struct {
		Channels struct {
			Email struct {
				SMTPHost string `json:"smtp_host"`
			} `json:"email"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse email section: %v", err)
	}
	if raw.Channels.Email.SMTPHost == "" {
		t.Error("email smtp_host missing from example config")
	}
}
