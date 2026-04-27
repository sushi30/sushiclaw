package tools_test

import (
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
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

	built, err := tools.NewGatewayTools(cfg, nil, nil)
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

	built, err := tools.NewGatewayTools(cfg, []string{"chat-1"}, nil)
	if err != nil {
		t.Fatalf("NewGatewayTools: %v", err)
	}

	got := toolNames(built)
	want := []string{"exec"}
	if !equalStrings(got, want) {
		t.Fatalf("tool names = %v, want %v", got, want)
	}
}

func TestNewGatewayTools_RegistersVisionFromModelListReference(t *testing.T) {
	cfg := newToolsConfig(t)
	cfg.Agents.Defaults.ModelName = "text-model"
	cfg.ModelList = []config.ModelConfig{
		{ModelName: "text-model", Model: "openrouter/z-ai/glm-4.5"},
		{
			ModelName: "vision-model",
			Model:     "openrouter/z-ai/glm-5v-turbo",
			APIKey:    config.NewSecureString("test-key"),
		},
	}
	cfg.Tools.Vision.Enabled = true
	cfg.Tools.Vision.ModelName = "vision-model"

	built, err := tools.NewGatewayTools(cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewGatewayTools: %v", err)
	}

	got := toolNames(built)
	want := []string{"vision"}
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
