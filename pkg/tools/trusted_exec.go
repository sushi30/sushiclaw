package tools

import (
	"context"
	"os"
	"slices"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/tools/exec"
	"github.com/sushi30/sushiclaw/pkg/tools/toolctx"
)

// TrustedExecTool wraps exec.ExecTool to allow specific chat IDs to
// bypass the remote-channel exec restriction. Configured via
// SUSHICLAW_EXEC_ALLOWED_SENDERS (comma-separated chatIDs).
type TrustedExecTool struct {
	allowedChatIDs []string
	trusted        *exec.ExecTool // AllowRemote=true
	restricted     *exec.ExecTool // respects config allow_remote
}

// NewTrustedExecTool creates a TrustedExecTool.
func NewTrustedExecTool(
	_ *config.Config,
	workingDir string,
	restrict bool,
	allowedChatIDs []string,
) (*TrustedExecTool, error) {
	restricted := exec.NewExecTool(workingDir, restrict, false)
	trusted := exec.NewExecTool(workingDir, restrict, true)

	return &TrustedExecTool{
		allowedChatIDs: allowedChatIDs,
		trusted:        trusted,
		restricted:     restricted,
	}, nil
}

func (t *TrustedExecTool) Name() string        { return "exec" }
func (t *TrustedExecTool) Description() string { return t.restricted.Description() }
func (t *TrustedExecTool) Parameters() map[string]interfaces.ParameterSpec {
	return t.restricted.Parameters()
}

// Run executes the tool, dispatching to trusted or restricted based on chatID.
func (t *TrustedExecTool) Run(ctx context.Context, input string) (string, error) {
	chatID := toolctx.ChatIDFromContext(ctx)
	if slices.Contains(t.allowedChatIDs, chatID) {
		return t.trusted.Run(ctx, input)
	}
	return t.restricted.Run(ctx, input)
}

// Execute executes the tool with args JSON string.
func (t *TrustedExecTool) Execute(ctx context.Context, args string) (string, error) {
	chatID := toolctx.ChatIDFromContext(ctx)
	if slices.Contains(t.allowedChatIDs, chatID) {
		return t.trusted.Execute(ctx, args)
	}
	return t.restricted.Execute(ctx, args)
}

// ParseAllowedSenders reads SUSHICLAW_EXEC_ALLOWED_SENDERS and returns the
// trimmed, non-empty entries. Returns nil when the env var is unset or empty.
func ParseAllowedSenders() []string {
	raw := os.Getenv("SUSHICLAW_EXEC_ALLOWED_SENDERS")
	if raw == "" {
		return nil
	}
	var out []string
	for s := range strings.SplitSeq(raw, ",") {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}
