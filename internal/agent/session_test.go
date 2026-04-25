package agent_test

import (
	"context"
	"os"
	"reflect"
	"testing"

	agentsdk "github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/internal/agent"
	"github.com/sushi30/sushiclaw/pkg/config"
)

type mockTool struct{ name string }

func (m *mockTool) Name() string                                        { return m.name }
func (m *mockTool) Description() string                                 { return "" }
func (m *mockTool) Run(_ context.Context, _ string) (string, error)     { return "", nil }
func (m *mockTool) Parameters() map[string]interfaces.ParameterSpec     { return nil }
func (m *mockTool) Execute(_ context.Context, _ string) (string, error) { return "", nil }

func agentMaxIterations(t *testing.T, a *agentsdk.Agent) int {
	t.Helper()

	v := reflect.ValueOf(a).Elem().FieldByName("maxIterations")
	require.True(t, v.IsValid(), "expected maxIterations field to exist")
	return int(v.Int())
}

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

func TestBuildAgent_DefaultOpenAI(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{ModelName: "test-model"},
		},
		ModelList: []config.ModelConfig{
			{
				ModelName: "test-model",
				Model:     "gpt-4o",
				APIKey:    config.NewSecureString("test-key"),
			},
		},
	}

	// BuildAgent should succeed for default (non-OpenRouter) models.
	_, err := agent.BuildAgent(cfg, nil)
	require.NoError(t, err, "expected BuildAgent to succeed with default OpenAI provider")
}

func TestBuildAgent_MaxToolIterationsApplied(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				ModelName:         "test-model",
				MaxToolIterations: 7,
			},
		},
		ModelList: []config.ModelConfig{
			{
				ModelName: "test-model",
				Model:     "gpt-4o",
				APIKey:    config.NewSecureString("test-key"),
			},
		},
	}

	a, err := agent.BuildAgent(cfg, nil)
	require.NoError(t, err)
	assert.Equal(t, 7, agentMaxIterations(t, a))
}

func TestBuildAgent_MaxToolIterationsZeroUsesDefault(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				ModelName:         "test-model",
				MaxToolIterations: 0,
			},
		},
		ModelList: []config.ModelConfig{
			{
				ModelName: "test-model",
				Model:     "gpt-4o",
				APIKey:    config.NewSecureString("test-key"),
			},
		},
	}

	a, err := agent.BuildAgent(cfg, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, agentMaxIterations(t, a))
}

func TestBuildAgent_MaxToolIterationsNegative(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				ModelName:         "test-model",
				MaxToolIterations: -1,
			},
		},
		ModelList: []config.ModelConfig{
			{
				ModelName: "test-model",
				Model:     "gpt-4o",
				APIKey:    config.NewSecureString("test-key"),
			},
		},
	}

	_, err := agent.BuildAgent(cfg, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_tool_iterations")
	assert.Contains(t, err.Error(), "must be >= 0")
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

func TestNewSessionManager_ToolsRegistered(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{ModelName: "test-model"},
		},
		ModelList: []config.ModelConfig{
			{
				ModelName: "test-model",
				Model:     "gpt-4o",
				APIKey:    config.NewSecureString("test-key"),
			},
		},
	}

	tool := &mockTool{name: "test-tool"}
	sm, err := agent.NewSessionManager(cfg, nil, []interfaces.Tool{tool})
	require.NoError(t, err)
	assert.Equal(t, []string{"test-tool"}, sm.ToolNames())
}

func TestBuildAgent_WithMCPConfig(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{ModelName: "test-model"},
		},
		ModelList: []config.ModelConfig{
			{
				ModelName: "test-model",
				Model:     "gpt-4o",
				APIKey:    config.NewSecureString("test-key"),
			},
		},
		MCP: config.MCPConfig{
			MCPServers: map[string]config.MCPServerConfig{
				"filesystem": {
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
				},
				"remote": {
					URL:   "http://localhost:3000/mcp",
					Token: config.NewSecureString("secret-token"),
				},
			},
		},
	}

	// BuildAgent should succeed with MCP config; agent-sdk-go uses lazy
	// initialization so no actual server connection is attempted.
	_, err := agent.BuildAgent(cfg, nil)
	require.NoError(t, err, "expected BuildAgent to succeed with MCP config")
}
