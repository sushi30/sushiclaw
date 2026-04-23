// Package openrouter provides an LLM client for OpenRouter.
// It wraps the agent-sdk-go OpenAI client with OpenRouter-specific defaults:
//   - Base URL defaults to https://openrouter.ai/api/v1
//   - The "openrouter/" prefix is stripped from model names
package openrouter

import (
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

// defaultBaseURL is the OpenRouter API endpoint.
const defaultBaseURL = "https://openrouter.ai/api/v1"

// OpenRouterClient wraps openai.OpenAIClient with OpenRouter defaults.
type OpenRouterClient struct {
	*openai.OpenAIClient
	modelName string
}

// Option configures an OpenRouterClient.
type Option func(*OpenRouterClient)

// WithModel sets the model (strips "openrouter/" prefix if present).
func WithModel(model string) Option {
	return func(c *OpenRouterClient) {
		c.modelName = model
	}
}

// NewClient creates an OpenRouter LLM client.
//
// apiKey is the OpenRouter API key. The model can include the "openrouter/"
// prefix (e.g. "openrouter/z-ai/glm-4.5"); it will be stripped before
// sending to the OpenRouter API.
func NewClient(apiKey string, opts ...Option) *OpenRouterClient {
	c := &OpenRouterClient{}
	for _, opt := range opts {
		opt(c)
	}

	openAIOpts := []openai.Option{openai.WithBaseURL(defaultBaseURL)}
	model := stripOpenRouterPrefix(c.modelName)
	if model != "" {
		openAIOpts = append(openAIOpts, openai.WithModel(model))
	}

	c.OpenAIClient = openai.NewClient(apiKey, openAIOpts...)

	return c
}

// Name returns the provider name.
func (c *OpenRouterClient) Name() string {
	return "openrouter"
}

// stripOpenRouterPrefix removes the "openrouter/" prefix from model names.
func stripOpenRouterPrefix(model string) string {
	return strings.TrimPrefix(model, "openrouter/")
}

// Compile-time check that OpenRouterClient implements interfaces.LLM.
var _ interfaces.LLM = (*OpenRouterClient)(nil)
