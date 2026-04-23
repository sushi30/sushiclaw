package openrouter_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/pkg/llm/openrouter"
)

func TestNewClient_StripsPrefix(t *testing.T) {
	client := openrouter.NewClient("test-key", openrouter.WithModel("openrouter/z-ai/glm-4.5"))
	require.NotNil(t, client)

	assert.Equal(t, "z-ai/glm-4.5", client.GetModel(),
		"openrouter/ prefix should be stripped from model name")
}

func TestNewClient_NoPrefix(t *testing.T) {
	client := openrouter.NewClient("test-key", openrouter.WithModel("anthropic/claude-3.5-sonnet"))
	require.NotNil(t, client)

	assert.Equal(t, "anthropic/claude-3.5-sonnet", client.GetModel(),
		"model name without openrouter/ prefix should be preserved")
}

func TestNewClient_Name(t *testing.T) {
	client := openrouter.NewClient("test-key", openrouter.WithModel("openrouter/gpt-4o"))
	require.NotNil(t, client)

	assert.Equal(t, "openrouter", client.Name())
}

func TestNewClient_SupportsStreaming(t *testing.T) {
	client := openrouter.NewClient("test-key", openrouter.WithModel("openrouter/gpt-4o"))
	require.NotNil(t, client)

	assert.True(t, client.SupportsStreaming())
}

func TestNewClient_NoOptions(t *testing.T) {
	// Should not panic when no options are provided.
	client := openrouter.NewClient("test-key")
	require.NotNil(t, client)

	// Default model from openai client is "gpt-4o-mini".
	assert.Equal(t, "gpt-4o-mini", client.GetModel())
}

func TestNewClient_EmptyModel(t *testing.T) {
	client := openrouter.NewClient("test-key", openrouter.WithModel(""))
	require.NotNil(t, client)

	// Empty model falls back to openai default.
	assert.Equal(t, "gpt-4o-mini", client.GetModel())
}
