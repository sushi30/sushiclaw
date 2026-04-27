package asr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sushi30/sushiclaw/pkg/logger"
)

const defaultWhisperBaseURL = "https://api.openai.com/v1"

// WhisperTranscriber transcribes audio using an OpenAI-compatible /audio/transcriptions endpoint.
type WhisperTranscriber struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewWhisperTranscriber creates a new Whisper-compatible transcriber.
func NewWhisperTranscriber(apiKey, model, baseURL string) *WhisperTranscriber {
	if baseURL == "" {
		baseURL = defaultWhisperBaseURL
	}
	return &WhisperTranscriber{
		apiKey:  apiKey,
		model:   model,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

// Name returns the transcriber name.
func (t *WhisperTranscriber) Name() string {
	return "whisper"
}

// Transcribe sends the audio file to the transcription endpoint and returns the text.
func (t *WhisperTranscriber) Transcribe(ctx context.Context, audioFilePath string) (*TranscriptionResponse, error) {
	file, err := os.Open(audioFilePath)
	if err != nil {
		return nil, fmt.Errorf("open audio file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			logger.WarnCF("voice", "Failed to close audio file", map[string]any{
				"error": err.Error(),
			})
		}
	}()

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Add the audio file.
	fileName := filepath.Base(audioFilePath)
	filePart, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(filePart, file); err != nil {
		return nil, fmt.Errorf("copy file to form: %w", err)
	}

	// Add model field.
	if err := writer.WriteField("model", t.model); err != nil {
		return nil, fmt.Errorf("write model field: %w", err)
	}

	// Request JSON response for structured parsing.
	if err := writer.WriteField("response_format", "json"); err != nil {
		return nil, fmt.Errorf("write response_format field: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/audio/transcriptions", &requestBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	logger.DebugCF("voice", "Sending transcription request", map[string]any{
		"model":    t.model,
		"base_url": t.baseURL,
		"file":     fileName,
	})

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("transcription request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.WarnCF("voice", "Failed to close transcription response body", map[string]any{
				"error": err.Error(),
			})
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("transcription API returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Text     string  `json:"text"`
		Language string  `json:"language,omitempty"`
		Duration float64 `json:"duration,omitempty"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse transcription response: %w", err)
	}

	logger.DebugCF("voice", "Transcription received", map[string]any{
		"language": result.Language,
		"duration": result.Duration,
	})

	return &TranscriptionResponse{
		Text:     result.Text,
		Language: result.Language,
		Duration: result.Duration,
	}, nil
}
