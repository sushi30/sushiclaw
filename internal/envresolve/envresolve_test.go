package envresolve_test

import (
	"os"
	"testing"

	"github.com/sipeed/picoclaw/pkg/audio/asr"
	"github.com/sipeed/picoclaw/pkg/config"

	"github.com/sushi30/sushiclaw/internal/envresolve"
)

func TestConfig_ResolvesEnvAPIKey(t *testing.T) {
	t.Setenv("TEST_API_KEY", "sk-resolved-value")

	cfg := &config.Config{}
	cfg.ModelList = config.SecureModelList{
		{ModelName: "test-model", Model: "openai/gpt-4o"},
	}
	cfg.ModelList[0].SetAPIKey("env://TEST_API_KEY")

	envresolve.Config(cfg)

	got := cfg.ModelList[0].APIKey()
	if got != "sk-resolved-value" {
		t.Errorf("APIKey() = %q, want %q", got, "sk-resolved-value")
	}
}

func TestConfig_IgnoresNonEnvKeys(t *testing.T) {
	cfg := &config.Config{}
	cfg.ModelList = config.SecureModelList{
		{ModelName: "test-model", Model: "openai/gpt-4o"},
	}
	cfg.ModelList[0].SetAPIKey("sk-plain-key")

	envresolve.Config(cfg)

	got := cfg.ModelList[0].APIKey()
	if got != "sk-plain-key" {
		t.Errorf("APIKey() = %q, want %q", got, "sk-plain-key")
	}
}

func TestConfig_MissingEnvVar_LeavesUnchanged(t *testing.T) {
	_ = os.Unsetenv("MISSING_VAR")

	cfg := &config.Config{}
	cfg.ModelList = config.SecureModelList{
		{ModelName: "test-model", Model: "openai/gpt-4o"},
	}
	cfg.ModelList[0].SetAPIKey("env://MISSING_VAR")

	envresolve.Config(cfg)

	// Should remain unresolved — not blank, not overwritten with empty string.
	got := cfg.ModelList[0].APIKey()
	if got != "env://MISSING_VAR" {
		t.Errorf("APIKey() = %q, want %q (unresolved)", got, "env://MISSING_VAR")
	}
}

func TestDetectTranscriber_WithEnvKey(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "gsk_test_key")

	cfg := &config.Config{}
	cfg.Voice.ModelName = "groq-asr"
	cfg.ModelList = config.SecureModelList{
		{
			ModelName: "groq-asr",
			Model:     "groq/whisper-large-v3-turbo",
			APIBase:   "https://api.groq.com/openai/v1",
		},
	}
	cfg.ModelList[0].SetAPIKey("env://GROQ_API_KEY")

	envresolve.Config(cfg)

	transcriber := asr.DetectTranscriber(cfg)
	if transcriber == nil {
		t.Fatal("DetectTranscriber() = nil, want a transcriber")
	}
	t.Logf("transcriber: %s", transcriber.Name())
}
