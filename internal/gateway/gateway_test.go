package gateway_test

import (
	"context"
	"testing"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
	pictools "github.com/sipeed/picoclaw/pkg/tools"

	sushitools "github.com/sushi30/sushiclaw/pkg/tools"
)

// stubProvider satisfies providers.LLMProvider without making real API calls.
type stubProvider struct{}

func (s *stubProvider) Chat(_ context.Context, _ []providers.Message, _ []providers.ToolDefinition, _ string, _ map[string]any) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{}, nil
}
func (s *stubProvider) GetDefaultModel() string { return "stub" }

// newTestLoop creates a minimal AgentLoop suitable for registration tests.
func newTestLoop(cfg *config.Config) *agent.AgentLoop {
	return agent.NewAgentLoop(cfg, bus.NewMessageBus(), &stubProvider{})
}

// TestGateway_TrustedExecRegistered verifies that when
// SUSHICLAW_EXEC_ALLOWED_SENDERS is set and exec is enabled, the registered
// "exec" tool is a TrustedExecTool that allows the listed chatID.
func TestGateway_TrustedExecRegistered(t *testing.T) {
	t.Setenv("SUSHICLAW_EXEC_ALLOWED_SENDERS", "+1234567890")

	cfg := config.DefaultConfig()
	cfg.Tools.Exec.AllowRemote = false

	loop := newTestLoop(cfg)

	allowedSenders := sushitools.ParseAllowedSenders()
	if len(allowedSenders) == 0 {
		t.Fatal("expected ParseAllowedSenders to return entries")
	}

	workingDir := cfg.Agents.Defaults.Workspace
	restrict := cfg.Agents.Defaults.RestrictToWorkspace
	trustedExec, err := sushitools.NewTrustedExecTool(cfg, workingDir, restrict, allowedSenders)
	if err != nil {
		t.Fatalf("NewTrustedExecTool: %v", err)
	}
	loop.RegisterTool(trustedExec)

	// Retrieve the registered tool from the default agent and confirm it
	// allows the trusted chatID on a remote channel.
	defaultAgent := loop.GetRegistry().GetDefaultAgent()
	tool, ok := defaultAgent.Tools.Get("exec")
	if !ok {
		t.Fatal("exec tool not found in registry after registration")
	}

	ctx := pictools.WithToolContext(context.Background(), "telegram", "+1234567890")
	result := tool.Execute(ctx, map[string]any{"action": "run", "command": "echo hi"})
	if result.IsError {
		t.Fatalf("trusted chatID should be allowed after registration, got: %s", result.ForLLM)
	}
}

// TestGateway_NoRegistrationWhenEnvUnset verifies that when the env var is
// absent the exec tool in the registry is not a TrustedExecTool (i.e. the
// picoclaw default is left in place).
func TestGateway_NoRegistrationWhenEnvUnset(t *testing.T) {
	t.Setenv("SUSHICLAW_EXEC_ALLOWED_SENDERS", "")

	cfg := config.DefaultConfig()
	loop := newTestLoop(cfg)

	allowedSenders := sushitools.ParseAllowedSenders()
	if len(allowedSenders) > 0 {
		t.Fatal("expected ParseAllowedSenders to return nil when env var is empty")
	}

	// No registration — the tool in the registry should be picoclaw's default,
	// not a TrustedExecTool.
	defaultAgent := loop.GetRegistry().GetDefaultAgent()
	tool, ok := defaultAgent.Tools.Get("exec")
	if !ok {
		t.Fatal("exec tool not found in registry")
	}
	if _, isTrusted := tool.(*sushitools.TrustedExecTool); isTrusted {
		t.Fatal("expected picoclaw default exec, got TrustedExecTool")
	}
}
