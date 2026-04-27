package asr

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sushi30/sushiclaw/pkg/config"
)

func TestDetectTranscriber_Disabled(t *testing.T) {
	cfg := voiceTestConfig(false)

	assert.Nil(t, DetectTranscriber(cfg))
}

func TestDetectTranscriber_Enabled(t *testing.T) {
	cfg := voiceTestConfig(true)

	assert.NotNil(t, DetectTranscriber(cfg))
}

func voiceTestConfig(enabled bool) *config.Config {
	return &config.Config{
		VoiceConfig: config.VoiceConfig{
			Enabled:   enabled,
			ModelName: "whisper-1",
		},
		ModelList: []config.ModelConfig{
			{
				ModelName: "whisper-1",
				Model:     "whisper-1",
				APIKey:    config.NewSecureString("test-key"),
			},
		},
	}
}
