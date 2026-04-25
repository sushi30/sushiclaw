package tools

import (
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/tools/exec"
	fstools "github.com/sushi30/sushiclaw/pkg/tools/fs"
	"github.com/sushi30/sushiclaw/pkg/tools/spawn"
	"github.com/sushi30/sushiclaw/pkg/tools/websearch"
)

// SpawnFactory is the constructor signature for a sub-agent factory.
type SpawnFactory = spawn.SubAgentFactory

// NewChatTools returns tools available to the local terminal chat.
func NewChatTools(cfg *config.Config) []interfaces.Tool {
	out := newFileTools(cfg)
	if cfg.Tools.IsToolEnabled("exec") {
		out = append(out, exec.NewExecTool(workspacePath(cfg), restrictToWorkspace(cfg), true))
	}
	if cfg.Tools.IsToolEnabled("web_search") {
		if tool, err := websearch.NewTool(cfg.Tools.WebSearch); err == nil {
			out = append(out, tool)
		}
	}
	return out
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
	return out, nil
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

func workspacePath(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.WorkspacePath()
}

func restrictToWorkspace(cfg *config.Config) bool {
	return cfg != nil && cfg.Agents.Defaults.RestrictToWorkspace
}

// ToolsWithoutSpawn returns a copy of the tool slice with the spawn tool removed.
// Use this when building sub-agents to prevent infinite recursion.
func ToolsWithoutSpawn(tools []interfaces.Tool) []interfaces.Tool {
	out := make([]interfaces.Tool, 0, len(tools))
	for _, t := range tools {
		if t.Name() == "spawn" {
			continue
		}
		out = append(out, t)
	}
	return out
}

// MaybeAppendSpawnTool appends the spawn tool if enabled in config.
func MaybeAppendSpawnTool(tools []interfaces.Tool, cfg *config.Config, factory SpawnFactory) []interfaces.Tool {
	if !cfg.Tools.IsToolEnabled("spawn") {
		return tools
	}
	return append(tools, spawn.NewSpawnTool(cfg, ToolsWithoutSpawn(tools), factory))
}
