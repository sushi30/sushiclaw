package vision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/media"
)

const defaultPrompt = "Describe this image in detail."
const defaultMaxTokens = 1024

// VisionTool calls a vision-capable LLM to describe an image.
type VisionTool struct {
	model         string
	apiKey        string
	apiBase       string
	defaultPrompt string
	client        *http.Client
	mediaStore    media.MediaStore
}

// Args is the expected JSON shape from the agent.
type Args struct {
	ImagePath string `json:"image_path"`
	Prompt    string `json:"prompt"`
}

// NewTool creates a VisionTool from config.
func NewTool(cfg config.VisionToolConfig, defaultModelCfg *config.ModelConfig, store media.MediaStore) (*VisionTool, error) {
	apiKey := cfg.APIKeyString()
	if apiKey == "" && defaultModelCfg != nil {
		apiKey = defaultModelCfg.APIKeyString()
	}
	if apiKey == "" {
		return nil, fmt.Errorf("vision tool requires api_key (set VISION_API_KEY env var or config, or ensure a default model has an api_key)")
	}

	model := cfg.Model
	if model == "" && defaultModelCfg != nil {
		model = defaultModelCfg.Model
		if model == "" {
			model = defaultModelCfg.ModelName
		}
	}
	if model == "" {
		return nil, fmt.Errorf("vision tool requires model (set vision.model in config, or ensure a default model is configured)")
	}

	// Strip openrouter/ prefix if present — the API endpoint expects the raw model ID.
	model = strings.TrimPrefix(model, "openrouter/")

	apiBase := cfg.APIBase
	if apiBase == "" && defaultModelCfg != nil {
		apiBase = defaultModelCfg.APIBase
	}
	if apiBase == "" {
		apiBase = "https://openrouter.ai/api/v1"
	}

	prompt := cfg.Prompt
	if prompt == "" {
		prompt = defaultPrompt
	}

	return &VisionTool{
		model:         model,
		apiKey:        apiKey,
		apiBase:       strings.TrimSuffix(apiBase, "/"),
		defaultPrompt: prompt,
		client:        &http.Client{Timeout: 60 * time.Second},
		mediaStore:    store,
	}, nil
}

func (t *VisionTool) Name() string { return "vision" }
func (t *VisionTool) Description() string {
	return "Analyze an image using a vision-capable LLM and return a text description."
}

func (t *VisionTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"image_path": {
			Type:        "string",
			Description: "Local file path or media:// reference to the image to analyze",
			Required:    true,
		},
		"prompt": {
			Type:        "string",
			Description: "Optional custom prompt for the vision analysis (default: describe the image)",
			Required:    false,
		},
	}
}

// Run executes the tool with the given input string.
func (t *VisionTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

// Execute executes the tool with the given arguments JSON string.
func (t *VisionTool) Execute(ctx context.Context, args string) (string, error) {
	var a Args
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		// Fallback: treat whole input as image_path.
		a.ImagePath = strings.TrimSpace(args)
	}
	if a.ImagePath == "" {
		return "", fmt.Errorf("image_path is required")
	}

	localPath := a.ImagePath
	if strings.HasPrefix(a.ImagePath, "media://") {
		if t.mediaStore == nil {
			return "", fmt.Errorf("cannot resolve media ref: no media store available")
		}
		resolved, err := t.mediaStore.Resolve(a.ImagePath)
		if err != nil {
			return "", fmt.Errorf("resolve media ref %q: %w", a.ImagePath, err)
		}
		localPath = resolved
	}

	data, err := os.ReadFile(localPath)
	if err != nil {
		return "", fmt.Errorf("read image %q: %w", localPath, err)
	}

	contentType := mime.TypeByExtension(filepath.Ext(localPath))
	if contentType == "" {
		contentType = "image/jpeg"
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	dataURI := fmt.Sprintf("data:%s;base64,%s", contentType, b64)

	prompt := a.Prompt
	if prompt == "" {
		prompt = t.defaultPrompt
	}

	return t.describeImage(ctx, prompt, dataURI)
}

func (t *VisionTool) describeImage(ctx context.Context, prompt, dataURI string) (string, error) {
	payload := map[string]interface{}{
		"model": t.model,
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{"type": "text", "text": prompt},
					{"type": "image_url", "image_url": map[string]string{"url": dataURI}},
				},
			},
		},
		"max_tokens": defaultMaxTokens,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.apiBase+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("vision API request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vision API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse vision API response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("vision API returned no choices")
	}
	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}
