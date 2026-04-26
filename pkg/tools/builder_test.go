package tools_test

import (
	"context"
	"testing"

	agentpkg "github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/tools"
)

func TestNewChatTools_RegistersEnabledTools(t *testing.T) {
	cfg := newToolsConfig(t)
	cfg.Tools.Exec.Enabled = true
	cfg.Tools.ReadFile.Enabled = true
	cfg.Tools.WriteFile.Enabled = true
	cfg.Tools.ListDir.Enabled = true

	got := toolNames(tools.NewChatTools(cfg))
	want := []string{"read_file", "write_file", "list_dir", "exec"}
	if !equalStrings(got, want) {
		t.Fatalf("tool names = %v, want %v", got, want)
	}
}

func TestNewGatewayTools_RegistersFileToolsWithoutExecAllowlist(t *testing.T) {
	cfg := newToolsConfig(t)
	cfg.Tools.Exec.Enabled = true
	cfg.Tools.ReadFile.Enabled = true
	cfg.Tools.ListDir.Enabled = true

	built, err := tools.NewGatewayTools(cfg, nil)
	if err != nil {
		t.Fatalf("NewGatewayTools: %v", err)
	}

	got := toolNames(built)
	want := []string{"read_file", "list_dir"}
	if !equalStrings(got, want) {
		t.Fatalf("tool names = %v, want %v", got, want)
	}
}

func TestNewGatewayTools_RegistersTrustedExecWithAllowlist(t *testing.T) {
	cfg := newToolsConfig(t)
	cfg.Tools.Exec.Enabled = true

	built, err := tools.NewGatewayTools(cfg, []string{"chat-1"})
	if err != nil {
		t.Fatalf("NewGatewayTools: %v", err)
	}

	got := toolNames(built)
	want := []string{"exec"}
	if !equalStrings(got, want) {
		t.Fatalf("tool names = %v, want %v", got, want)
	}
}

func TestMaybeAppendSubagentTaskTool_GatewayOnly(t *testing.T) {
	cfg := newToolsConfig(t)
	cfg.Tools.SubagentTask.Enabled = true
	cfg.SubAgents = map[string]config.SubAgentConfig{
		"coder": {Description: "Code tasks"},
	}
	factory := func(_ *config.Config, _, _, _, _ string, _ []interfaces.Tool) (*agentpkg.Agent, error) {
		t.Fatal("factory should not run during registration")
		return nil, nil
	}

	chatTools := tools.NewChatTools(cfg)
	if contains(toolNames(chatTools), "subagent_task") {
		t.Fatalf("chat tools included subagent_task: %v", toolNames(chatTools))
	}

	gatewayTools := tools.MaybeAppendSubagentTaskTool(nil, cfg, bus.NewMessageBus(), factory)
	if got := toolNames(gatewayTools); !equalStrings(got, []string{"subagent_task"}) {
		t.Fatalf("gateway tools = %v, want [subagent_task]", got)
	}
}

func TestToolsWithoutSubagentTask(t *testing.T) {
	input := []interfaces.Tool{
		&mockBuilderTool{name: "read_file"},
		&mockBuilderTool{name: "subagent_task"},
	}

	got := toolNames(tools.ToolsWithoutSubagentTask(input))
	want := []string{"read_file"}
	if !equalStrings(got, want) {
		t.Fatalf("tool names = %v, want %v", got, want)
	}
}

func newToolsConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:           t.TempDir(),
				RestrictToWorkspace: true,
			},
		},
	}
}

type mockBuilderTool struct {
	name string
}

func (m *mockBuilderTool) Name() string                                    { return m.name }
func (m *mockBuilderTool) Description() string                             { return "" }
func (m *mockBuilderTool) Parameters() map[string]interfaces.ParameterSpec { return nil }
func (m *mockBuilderTool) Run(_ context.Context, _ string) (string, error) { return "", nil }
func (m *mockBuilderTool) Execute(_ context.Context, _ string) (string, error) {
	return "", nil
}

func toolNames(tools []interfaces.Tool) []string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name()
	}
	return names
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
