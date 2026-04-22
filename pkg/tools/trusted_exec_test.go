package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/tools"
	"github.com/sushi30/sushiclaw/pkg/tools/exec"
)

func newRestrictedCfg() *config.Config {
	return &config.Config{}
}

// TestTrustedExec_TrustedChatIDAllowed verifies that a chatID in the allowlist
// can run exec from a remote channel.
func TestTrustedExec_TrustedChatIDAllowed(t *testing.T) {
	tool, err := tools.NewTrustedExecTool(newRestrictedCfg(), "", false, []string{"+1234567890"})
	if err != nil {
		t.Fatalf("NewTrustedExecTool: %v", err)
	}

	ctx := exec.WithChatID(context.Background(), "+1234567890")
	result, err := tool.Execute(ctx, `{"command":"echo hi"}`)

	if err != nil {
		t.Fatalf("trusted chatID should be allowed, got error: %v", err)
	}
	if !strings.Contains(result, "hi") {
		t.Errorf("expected output to contain 'hi', got: %s", result)
	}
}

// TestTrustedExec_UntrustedChatIDBlocked verifies that a chatID not in the
// allowlist is blocked on a remote channel.
func TestTrustedExec_UntrustedChatIDBlocked(t *testing.T) {
	tool, err := tools.NewTrustedExecTool(newRestrictedCfg(), "", false, []string{"+1234567890"})
	if err != nil {
		t.Fatalf("NewTrustedExecTool: %v", err)
	}

	ctx := exec.WithChatID(context.Background(), "+9999999999")
	_, err = tool.Execute(ctx, `{"command":"echo hi"}`)

	if err == nil {
		t.Fatal("untrusted chatID on remote channel should be blocked")
	}
	if !strings.Contains(err.Error(), "remote exec is disabled") {
		t.Errorf("expected 'remote exec is disabled', got: %v", err)
	}
}

// TestTrustedExec_EmptyAllowlistBlocksAll verifies that an empty allowlist
// falls through to the restricted inner tool for all remote senders.
func TestTrustedExec_EmptyAllowlistBlocksAll(t *testing.T) {
	tool, err := tools.NewTrustedExecTool(newRestrictedCfg(), "", false, nil)
	if err != nil {
		t.Fatalf("NewTrustedExecTool: %v", err)
	}

	ctx := exec.WithChatID(context.Background(), "+1234567890")
	_, err = tool.Execute(ctx, `{"command":"echo hi"}`)

	if err == nil {
		t.Fatal("empty allowlist should block all remote senders")
	}
	if !strings.Contains(err.Error(), "remote exec is disabled") {
		t.Errorf("expected 'remote exec is disabled', got: %v", err)
	}
}

// TestTrustedExec_InternalChannelAlwaysWorks verifies that internal channels
// (cli) are unaffected by the allowlist.
func TestTrustedExec_InternalChannelAlwaysWorks(t *testing.T) {
	tool, err := tools.NewTrustedExecTool(newRestrictedCfg(), "", false, nil)
	if err != nil {
		t.Fatalf("NewTrustedExecTool: %v", err)
	}

	ctx := exec.WithChatID(context.Background(), "cli")
	result, err := tool.Execute(ctx, `{"command":"echo hi"}`)

	if err != nil {
		t.Fatalf("internal channel should always work, got error: %v", err)
	}
	if !strings.Contains(result, "hi") {
		t.Errorf("expected output to contain 'hi', got: %s", result)
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
			got := tools.ParseAllowedSenders()
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
