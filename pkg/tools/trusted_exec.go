package tools

import (
	"context"
	"os"
	"slices"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	pictools "github.com/sipeed/picoclaw/pkg/tools"
)

// TrustedExecTool wraps picoclaw's ExecTool to allow specific chat IDs to
// bypass the remote-channel exec restriction. Configured via
// SUSHICLAW_EXEC_ALLOWED_SENDERS (comma-separated chatIDs).
//
// When the inbound chatID matches an entry in allowedChatIDs, the command is
// dispatched to an inner ExecTool built with AllowRemote=true. All other
// callers use a restricted inner ExecTool that respects the config's
// allow_remote setting.
type TrustedExecTool struct {
	allowedChatIDs []string
	trusted        *pictools.ExecTool // AllowRemote=true
	restricted     *pictools.ExecTool // respects config allow_remote
}

// NewTrustedExecTool creates a TrustedExecTool. allowedChatIDs is the list of
// chatIDs that are permitted to run exec on remote channels.
func NewTrustedExecTool(
	cfg *config.Config,
	workingDir string,
	restrict bool,
	allowedChatIDs []string,
) (*TrustedExecTool, error) {
	restricted, err := pictools.NewExecToolWithConfig(workingDir, restrict, cfg)
	if err != nil {
		return nil, err
	}

	// Shallow copy is safe: Config.Tools.Exec is embedded by value all the way
	// down. Only AllowRemote is mutated; pointer fields (sensitiveCache, etc.)
	// are shared but only read by NewExecToolWithConfig.
	trustedCfg := *cfg
	trustedCfg.Tools.Exec.AllowRemote = true
	trusted, err := pictools.NewExecToolWithConfig(workingDir, restrict, &trustedCfg)
	if err != nil {
		return nil, err
	}

	return &TrustedExecTool{
		allowedChatIDs: allowedChatIDs,
		trusted:        trusted,
		restricted:     restricted,
	}, nil
}

func (t *TrustedExecTool) Name() string                { return "exec" }
func (t *TrustedExecTool) Description() string         { return t.restricted.Description() }
func (t *TrustedExecTool) Parameters() map[string]any  { return t.restricted.Parameters() }

// Execute dispatches to the trusted inner tool when the caller's chatID is in
// the allowlist, otherwise to the restricted inner tool.
func (t *TrustedExecTool) Execute(ctx context.Context, args map[string]any) *pictools.ToolResult {
	if slices.Contains(t.allowedChatIDs, pictools.ToolChatID(ctx)) {
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
