package gateway_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/sushi30/sushiclaw/pkg/config"
)

// TestLoadExampleConfig verifies that config.example.json is a valid config
// that loads without error and has the expected structure.
func TestLoadExampleConfig(t *testing.T) {
	// Copy to temp dir so migration writes (backup + saved v2) go there, not source tree.
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "config.json")

	src, err := os.Open("../../config.example.json")
	if err != nil {
		t.Fatalf("open example config: %v", err)
	}
	data, err := io.ReadAll(src)
	if closeErr := src.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("read example config: %v", err)
	}
	if err = os.WriteFile(dst, data, 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := config.LoadConfig(dst)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Agents.Defaults.ModelName != "claude-sonnet" {
		t.Errorf("model_name = %q, want %q", cfg.Agents.Defaults.ModelName, "claude-sonnet")
	}
	if len(cfg.ModelList) == 0 {
		t.Fatal("model_list is empty")
	}
	if cfg.ModelList[0].ModelName != "claude-sonnet" {
		t.Errorf("model_list[0].model_name = %q, want %q", cfg.ModelList[0].ModelName, "claude-sonnet")
	}
	if cfg.Gateway.Port != 18800 {
		t.Errorf("gateway.port = %d, want 18800", cfg.Gateway.Port)
	}
}
