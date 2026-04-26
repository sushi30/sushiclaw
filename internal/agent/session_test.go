package agent_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	agentsdk "github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/internal/agent"
	"github.com/sushi30/sushiclaw/pkg/commands"
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

func TestNewSessionManager_ListSubAgentsSorted(t *testing.T) {
	cfg := newAgentTestConfig()
	cfg.SubAgents = map[string]config.SubAgentConfig{
		"researcher": {Description: "Research tasks"},
		"coder":      {Description: "Code tasks"},
	}

	sm, err := agent.NewSessionManager(cfg, nil, nil)
	require.NoError(t, err)

	assert.Equal(t, []string{"coder", "researcher"}, sm.ListSubAgents())
}

func TestBuildSubagent_PassesDescription(t *testing.T) {
	cfg := newAgentTestConfig()

	sub, err := agent.BuildSubagent(cfg, "coder", "Writes and reviews code", "", "Code carefully.", nil)
	require.NoError(t, err)

	assert.Equal(t, "coder", sub.GetName())
	assert.Equal(t, "Writes and reviews code", sub.GetDescription())
}

func TestBuildAgent_RegistersSDKSubagentTools(t *testing.T) {
	cfg := newAgentTestConfig()
	cfg.SubAgents = map[string]config.SubAgentConfig{
		"coder": {Description: "Writes and reviews code"},
	}

	a, err := agent.BuildAgent(cfg, nil)
	require.NoError(t, err)

	assert.Contains(t, sdkToolNames(a), "coder_agent")
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

func newAgentTestConfig() *config.Config {
	return &config.Config{
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
}

func sdkToolNames(a *agentsdk.Agent) []string {
	tools := a.GetTools()
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name()
	}
	return names
}

func TestSessionManager_ActivateSkill(t *testing.T) {
	ws := t.TempDir()
	skillsDir := filepath.Join(ws, "skills", "python")
	require.NoError(t, os.MkdirAll(skillsDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte("You are a Python expert."), 0644))

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
	cfg.Agents.Defaults.Workspace = ws

	sm, err := agent.NewSessionManager(cfg, nil, nil)
	require.NoError(t, err)

	// First activation should succeed.
	err = sm.ActivateSkill("python")
	require.NoError(t, err)

	// Verify the skill content is in memory.
	msgs, err := sm.GetMessages(context.Background())
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, interfaces.MessageRoleSystem, msgs[0].Role)
	assert.Equal(t, "You are a Python expert.", msgs[0].Content)
}

func TestSessionManager_ActivateSkill_AlreadyLoaded(t *testing.T) {
	ws := t.TempDir()
	skillsDir := filepath.Join(ws, "skills", "python")
	require.NoError(t, os.MkdirAll(skillsDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte("You are a Python expert."), 0644))

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
	cfg.Agents.Defaults.Workspace = ws

	sm, err := agent.NewSessionManager(cfg, nil, nil)
	require.NoError(t, err)

	require.NoError(t, sm.ActivateSkill("python"))
	err = sm.ActivateSkill("python")
	require.Error(t, err)
	assert.ErrorIs(t, err, commands.ErrSkillAlreadyLoaded)
}

func TestSessionManager_ActivateSkill_NotFound(t *testing.T) {
	ws := t.TempDir()

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
	cfg.Agents.Defaults.Workspace = ws

	sm, err := agent.NewSessionManager(cfg, nil, nil)
	require.NoError(t, err)

	err = sm.ActivateSkill("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSessionManager_ListSkills(t *testing.T) {
	ws := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, "skills", "python"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(ws, "skills", "review"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(ws, "skills", "missing-file"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, "skills", "ignored.txt"), []byte("not a skill"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(ws, "skills", "python", "SKILL.md"), []byte("---\nname: python\ndescription: Python coding help\n---\n\n# Python\n\nBody fallback."), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(ws, "skills", "review", "SKILL.md"), []byte("# Review\n\n- Review code carefully."), 0644))

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
	cfg.Agents.Defaults.Workspace = ws

	sm, err := agent.NewSessionManager(cfg, nil, nil)
	require.NoError(t, err)

	skills := sm.ListSkills()
	require.Len(t, skills, 2)
	assert.Equal(t, "python", skills[0].Name)
	assert.Equal(t, "Python coding help", skills[0].Description)
	assert.Equal(t, "review", skills[1].Name)
	assert.Equal(t, "Review code carefully.", skills[1].Description)
}

func TestSessionManager_ListSkillsMissingDirectory(t *testing.T) {
	ws := t.TempDir()
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
	cfg.Agents.Defaults.Workspace = ws

	sm, err := agent.NewSessionManager(cfg, nil, nil)
	require.NoError(t, err)

	assert.Empty(t, sm.ListSkills())
}
