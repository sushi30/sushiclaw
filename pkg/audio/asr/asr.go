// Package asr provides automatic speech recognition (transcription) for voice messages.
package asr

import (
	"context"
	"fmt"
	"strings"

	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/logger"
)

// Transcriber transcribes audio files to text.
type Transcriber interface {
	Name() string
	Transcribe(ctx context.Context, audioFilePath string) (*TranscriptionResponse, error)
}

// TranscriptionResponse is the result of transcribing an audio file.
type TranscriptionResponse struct {
	Text     string  `json:"text"`
	Language string  `json:"language,omitempty"`
	Duration float64 `json:"duration,omitempty"`
}

// DetectTranscriber creates a Transcriber from config if voice transcription is configured.
// It looks up cfg.Voice.ModelName in cfg.ModelList and creates the appropriate implementation.
func DetectTranscriber(cfg *config.Config) Transcriber {
	voiceCfg := cfg.Voice()
	if voiceCfg.ModelName == "" {
		return nil
	}

	var modelCfg *config.ModelConfig
	for i := range cfg.ModelList {
		if cfg.ModelList[i].ModelName == voiceCfg.ModelName {
			modelCfg = &cfg.ModelList[i]
			break
		}
	}
	if modelCfg == nil {
		logger.WarnCF("voice", "Voice model not found in model_list", map[string]any{
			"model_name": voiceCfg.ModelName,
		})
		return nil
	}

	apiKey := modelCfg.APIKeyString()
	if apiKey == "" {
		logger.WarnCF("voice", "Voice model has no API key", map[string]any{
			"model_name": voiceCfg.ModelName,
		})
		return nil
	}
	if strings.HasPrefix(apiKey, "env://") {
		logger.WarnCF("voice", "Voice model API key env var is not set", map[string]any{
			"model_name": voiceCfg.ModelName,
			"api_key":    apiKey,
		})
		return nil
	}

	model := modelCfg.Model
	if model == "" {
		model = voiceCfg.ModelName
	}

	// Use WhisperTranscriber for any OpenAI-compatible endpoint.
	// This covers Groq, OpenAI, OpenRouter, and any other provider
	// that exposes /audio/transcriptions.
	return NewWhisperTranscriber(apiKey, model, modelCfg.APIBase)
}

// IsAudioFile checks whether a file is audio based on its metadata.
func IsAudioFile(filename, contentType string) bool {
	if strings.HasPrefix(contentType, "audio/") {
		return true
	}
	ext := strings.ToLower(filename)
	for _, audioExt := range []string{".ogg", ".oga", ".mp3", ".wav", ".m4a", ".aac", ".flac", ".opus", ".webm"} {
		if strings.HasSuffix(ext, audioExt) {
			return true
		}
	}
	return false
}

// WrapTranscription wraps transcribed text in <transcription> tags.
func WrapTranscription(text string) string {
	if text == "" {
		return ""
	}
	return fmt.Sprintf("<transcription>%s</transcription>", text)
}
