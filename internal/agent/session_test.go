package agent_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/internal/agent"
	"github.com/sushi30/sushiclaw/pkg/config"
)

func TestBuildAgent_UnresolvedEnvKey(t *testing.T) {
	_ = os.Unsetenv("MISSING_API_KEY")

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{ModelName: "test-model"},
		},
		ModelList: []config.ModelConfig{
			{
				ModelName: "test-model",
				Model:     "gpt-4",
				APIKey:    config.NewSecureString("env://MISSING_API_KEY"),
			},
		},
	}

	_, err := agent.BuildAgent(cfg, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MISSING_API_KEY")
	assert.Contains(t, err.Error(), "is not set")
}

func TestBuildAgent_APIKeysArray_Unresolved(t *testing.T) {
	_ = os.Unsetenv("MISSING_ARR_KEY")

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{ModelName: "test-model"},
		},
		ModelList: []config.ModelConfig{
			{
				ModelName: "test-model",
				Model:     "gpt-4",
				APIKeys: []*config.SecureString{
					config.NewSecureString("env://MISSING_ARR_KEY"),
				},
			},
		},
	}

	_, err := agent.BuildAgent(cfg, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MISSING_ARR_KEY")
	assert.Contains(t, err.Error(), "is not set")
}

func TestBuildAgent_NoModelConfigured(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{ModelName: ""},
		},
	}

	_, err := agent.BuildAgent(cfg, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no default model configured")
}

func TestBuildAgent_ModelNotFound(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{ModelName: "missing-model"},
		},
		ModelList: []config.ModelConfig{
			{ModelName: "other-model", APIKey: config.NewSecureString("key")},
		},
	}

	_, err := agent.BuildAgent(cfg, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in model_list")
}

func TestBuildAgent_OpenRouterAutoBaseURL(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{ModelName: "test-model"},
		},
		ModelList: []config.ModelConfig{
			{
				ModelName: "test-model",
				Model:     "openrouter/z-ai/glm-4.5",
				APIKey:    config.NewSecureString("test-key"),
			},
		},
	}

	// BuildAgent should succeed with the OpenRouter base URL auto-detected.
	// The agent itself doesn't validate the key until first use.
	_, err := agent.BuildAgent(cfg, nil)
	require.NoError(t, err, "expected BuildAgent to succeed with OpenRouter auto-detected base URL")
}

func TestBuildAgent_NoAPIKey(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{ModelName: "test-model"},
		},
		ModelList: []config.ModelConfig{
			{ModelName: "test-model", Model: "gpt-4"},
		},
	}

	_, err := agent.BuildAgent(cfg, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no API key")
}
