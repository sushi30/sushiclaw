package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
	pictools "github.com/sipeed/picoclaw/pkg/tools"

	sushitools "github.com/sushi30/sushiclaw/pkg/tools"
)

func newRestrictedCfg() *config.Config {
	cfg := &config.Config{}
	cfg.Tools.Exec.EnableDenyPatterns = true
	cfg.Tools.Exec.AllowRemote = false
	return cfg
}

// TestTrustedExec_TrustedChatIDAllowed verifies that a chatID in the allowlist
// can run exec from a remote channel.
func TestTrustedExec_TrustedChatIDAllowed(t *testing.T) {
	tool, err := sushitools.NewTrustedExecTool(newRestrictedCfg(), "", false, []string{"+1234567890"})
	if err != nil {
		t.Fatalf("NewTrustedExecTool: %v", err)
	}

	ctx := pictools.WithToolContext(context.Background(), "telegram", "+1234567890")
	result := tool.Execute(ctx, map[string]any{"action": "run", "command": "echo hi"})

	if result.IsError {
		t.Fatalf("trusted chatID should be allowed, got error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "hi") {
		t.Errorf("expected output to contain 'hi', got: %s", result.ForLLM)
	}
}

// TestTrustedExec_UntrustedChatIDBlocked verifies that a chatID not in the
// allowlist is blocked on a remote channel.
func TestTrustedExec_UntrustedChatIDBlocked(t *testing.T) {
	tool, err := sushitools.NewTrustedExecTool(newRestrictedCfg(), "", false, []string{"+1234567890"})
	if err != nil {
		t.Fatalf("NewTrustedExecTool: %v", err)
	}

	ctx := pictools.WithToolContext(context.Background(), "telegram", "+9999999999")
	result := tool.Execute(ctx, map[string]any{"action": "run", "command": "echo hi"})

	if !result.IsError {
		t.Fatal("untrusted chatID on remote channel should be blocked")
	}
	if !strings.Contains(result.ForLLM, "restricted to internal channels") {
		t.Errorf("expected 'restricted to internal channels', got: %s", result.ForLLM)
	}
}

// TestTrustedExec_EmptyAllowlistBlocksAll verifies that an empty allowlist
// falls through to the restricted inner tool for all remote senders.
func TestTrustedExec_EmptyAllowlistBlocksAll(t *testing.T) {
	tool, err := sushitools.NewTrustedExecTool(newRestrictedCfg(), "", false, nil)
	if err != nil {
		t.Fatalf("NewTrustedExecTool: %v", err)
	}

	ctx := pictools.WithToolContext(context.Background(), "telegram", "+1234567890")
	result := tool.Execute(ctx, map[string]any{"action": "run", "command": "echo hi"})

	if !result.IsError {
		t.Fatal("empty allowlist should block all remote senders")
	}
	if !strings.Contains(result.ForLLM, "restricted to internal channels") {
		t.Errorf("expected 'restricted to internal channels', got: %s", result.ForLLM)
	}
}

// TestTrustedExec_InternalChannelAlwaysWorks verifies that internal channels
// (cli) are unaffected by the allowlist.
func TestTrustedExec_InternalChannelAlwaysWorks(t *testing.T) {
	tool, err := sushitools.NewTrustedExecTool(newRestrictedCfg(), "", false, nil)
	if err != nil {
		t.Fatalf("NewTrustedExecTool: %v", err)
	}

	ctx := pictools.WithToolContext(context.Background(), "cli", "direct")
	result := tool.Execute(ctx, map[string]any{"action": "run", "command": "echo hi"})

	if result.IsError {
		t.Fatalf("internal channel should always work, got error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "hi") {
		t.Errorf("expected output to contain 'hi', got: %s", result.ForLLM)
	}
}

// TestParseAllowedSenders_Parsing verifies trimming and splitting behaviour.
func TestParseAllowedSenders_Parsing(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"single", "+1234567890", []string{"+1234567890"}},
		{"multiple", "+1234567890,telegram:123456", []string{"+1234567890", "telegram:123456"}},
		{"spaces", " +1234567890 , telegram:123456 ", []string{"+1234567890", "telegram:123456"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SUSHICLAW_EXEC_ALLOWED_SENDERS", tc.input)
			got := sushitools.ParseAllowedSenders()
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i, v := range got {
				if v != tc.want[i] {
					t.Errorf("index %d: got %q, want %q", i, v, tc.want[i])
				}
			}
		})
	}
}
