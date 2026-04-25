package tools

import (
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/tools/exec"
	fstools "github.com/sushi30/sushiclaw/pkg/tools/fs"
	"github.com/sushi30/sushiclaw/pkg/tools/secureinput"
)

var defaultSecureInputStore = secureinput.NewStore()

// NewChatTools returns tools available to the local terminal chat.
func NewChatTools(cfg *config.Config) []interfaces.Tool {
	out := newFileTools(cfg)
	if cfg.Tools.IsToolEnabled("exec") {
		out = append(out, exec.NewExecTool(workspacePath(cfg), restrictToWorkspace(cfg), true))
	}
	return withSecureInput(cfg, out)
}

// NewGatewayTools returns tools available to remote gateway sessions.
func NewGatewayTools(cfg *config.Config, execAllowedSenders []string) ([]interfaces.Tool, error) {
	out := newFileTools(cfg)
	if cfg.Tools.IsToolEnabled("exec") && len(execAllowedSenders) > 0 {
		trustedExec, err := NewTrustedExecTool(cfg, workspacePath(cfg), restrictToWorkspace(cfg), execAllowedSenders)
		if err != nil {
			return out, err
		}
		out = append(out, trustedExec)
	}
	return withSecureInput(cfg, out), nil
}

// ClearSecureInputs clears captured secure values for one chat/session.
func ClearSecureInputs(chatID string) {
	defaultSecureInputStore.ClearSession(chatID)
}

// ClearAllSecureInputs clears all captured secure values.
func ClearAllSecureInputs() {
	defaultSecureInputStore.ClearAll()
}

func newFileTools(cfg *config.Config) []interfaces.Tool {
	workspace := workspacePath(cfg)
	restrict := restrictToWorkspace(cfg)

	var out []interfaces.Tool
	if cfg.Tools.IsToolEnabled("read_file") {
		out = append(out, fstools.NewReadFileTool(workspace, restrict, 0))
	}
	if cfg.Tools.IsToolEnabled("write_file") {
		out = append(out, fstools.NewWriteFileTool(workspace, restrict))
	}
	if cfg.Tools.IsToolEnabled("list_dir") {
		out = append(out, fstools.NewListDirTool(workspace, restrict))
	}
	return out
}

func withSecureInput(cfg *config.Config, out []interfaces.Tool) []interfaces.Tool {
	if cfg == nil || !cfg.Tools.IsToolEnabled("secure_input") {
		return out
	}
	out = secureinput.WrapAll(out, defaultSecureInputStore)
	return append([]interfaces.Tool{secureinput.NewTool(defaultSecureInputStore, nil)}, out...)
}

func workspacePath(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.WorkspacePath()
}

func restrictToWorkspace(cfg *config.Config) bool {
	return cfg != nil && cfg.Agents.Defaults.RestrictToWorkspace
}
