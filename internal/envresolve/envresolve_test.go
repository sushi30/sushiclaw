package envresolve_test

import (
	"os"
	"strings"
	"testing"

	"github.com/sushi30/sushiclaw/internal/envresolve"
	"github.com/sushi30/sushiclaw/pkg/config"
)

func TestConfig_ResolvesEnvAPIKey(t *testing.T) {
	t.Setenv("TEST_API_KEY", "sk-resolved-value")

	cfg := &config.Config{}
	cfg.ModelList = []config.ModelConfig{
		{ModelName: "test-model", Model: "openai/gpt-4o"},
	}
	cfg.ModelList[0].APIKey = config.NewSecureString("env://TEST_API_KEY")

	envresolve.Config(cfg)

	got := cfg.ModelList[0].APIKeyString()
	if got != "sk-resolved-value" {
		t.Errorf("APIKeyString() = %q, want %q", got, "sk-resolved-value")
	}
}

func TestConfig_IgnoresNonEnvKeys(t *testing.T) {
	cfg := &config.Config{}
	cfg.ModelList = []config.ModelConfig{
		{ModelName: "test-model", Model: "openai/gpt-4o"},
	}
	cfg.ModelList[0].APIKey = config.NewSecureString("sk-plain-key")

	envresolve.Config(cfg)

	got := cfg.ModelList[0].APIKeyString()
	if got != "sk-plain-key" {
		t.Errorf("APIKeyString() = %q, want %q", got, "sk-plain-key")
	}
}

func TestConfig_MissingEnvVar_LeavesUnchanged(t *testing.T) {
	_ = os.Unsetenv("MISSING_VAR")

	cfg := &config.Config{}
	cfg.ModelList = []config.ModelConfig{
		{ModelName: "test-model", Model: "openai/gpt-4o"},
	}
	cfg.ModelList[0].APIKey = config.NewSecureString("env://MISSING_VAR")

	envresolve.Config(cfg)

	// Should remain unresolved — not blank, not overwritten with empty string.
	got := cfg.ModelList[0].APIKeyString()
	if got != "env://MISSING_VAR" {
		t.Errorf("APIKeyString() = %q, want %q (unresolved)", got, "env://MISSING_VAR")
	}
}

func TestSecureStringRequired_MissingVar_ReturnsError(t *testing.T) {
	_ = os.Unsetenv("REQUIRED_MISSING")

	s := config.NewSecureString("env://REQUIRED_MISSING")

	err := envresolve.SecureStringRequired(s)
	if err == nil {
		t.Fatal("SecureStringRequired() = nil, want error")
	}
	if !strings.Contains(err.Error(), "REQUIRED_MISSING") {
		t.Errorf("error %q does not mention var name", err.Error())
	}
}

func TestSecureStringRequired_SetVar_ReturnsNil(t *testing.T) {
	t.Setenv("REQUIRED_PRESENT", "myvalue")

	s := config.NewSecureString("env://REQUIRED_PRESENT")

	if err := envresolve.SecureStringRequired(s); err != nil {
		t.Fatalf("SecureStringRequired() = %v, want nil", err)
	}
	if s.String() != "myvalue" {
		t.Errorf("s.String() = %q, want %q", s.String(), "myvalue")
	}
}
