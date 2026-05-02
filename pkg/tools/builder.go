package tools

import (
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/media"
	"github.com/sushi30/sushiclaw/pkg/tools/exec"
	fstools "github.com/sushi30/sushiclaw/pkg/tools/fs"
	"github.com/sushi30/sushiclaw/pkg/tools/message"
	"github.com/sushi30/sushiclaw/pkg/tools/vision"
	"github.com/sushi30/sushiclaw/pkg/tools/websearch"
)

// NewChatTools returns tools available to the local terminal chat.
func NewChatTools(cfg *config.Config) []interfaces.Tool {
	out := newFileTools(cfg)
	if cfg.Tools.IsToolEnabled("exec") {
		out = append(out, withDebugLogging(exec.NewExecTool(workspacePath(cfg), restrictToWorkspace(cfg), true)))
	}
	if cfg.Tools.IsToolEnabled("web_search") {
		if tool, err := websearch.NewTool(cfg.Tools.WebSearch); err == nil {
			out = append(out, withDebugLogging(tool))
		}
	}
	if cfg.Tools.IsToolEnabled("vision") {
		if tool, err := vision.NewTool(cfg.Tools.Vision, visionModel(cfg), nil); err == nil {
			out = append(out, withDebugLogging(tool))
		}
	}
	return out
}

// NewGatewayTools returns tools available to remote gateway sessions.
func NewGatewayTools(cfg *config.Config, execAllowedSenders []string, store media.MediaStore, messageBus *bus.MessageBus) ([]interfaces.Tool, error) {
	out := newFileTools(cfg)
	if cfg.Tools.IsToolEnabled("exec") && len(execAllowedSenders) > 0 {
		trustedExec, err := NewTrustedExecTool(cfg, workspacePath(cfg), restrictToWorkspace(cfg), execAllowedSenders)
		if err != nil {
			return out, err
		}
		out = append(out, withDebugLogging(trustedExec))
	}
	if cfg.Tools.IsToolEnabled("vision") {
		if tool, err := vision.NewTool(cfg.Tools.Vision, visionModel(cfg), store); err == nil {
			out = append(out, withDebugLogging(tool))
		}
	}
	if cfg.Tools.IsToolEnabled("message") && messageBus != nil {
		out = append(out, withDebugLogging(message.NewTool(messageBus, cfg.Tools.Message.MinInterval)))
	}
	return out, nil
}

func newFileTools(cfg *config.Config) []interfaces.Tool {
	workspace := workspacePath(cfg)
	restrict := restrictToWorkspace(cfg)

	var out []interfaces.Tool
	if cfg.Tools.IsToolEnabled("read_file") {
		out = append(out, withDebugLogging(fstools.NewReadFileTool(workspace, restrict, 0)))
	}
	if cfg.Tools.IsToolEnabled("write_file") {
		out = append(out, withDebugLogging(fstools.NewWriteFileTool(workspace, restrict)))
	}
	if cfg.Tools.IsToolEnabled("list_dir") {
		out = append(out, withDebugLogging(fstools.NewListDirTool(workspace, restrict)))
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

func defaultModel(cfg *config.Config) *config.ModelConfig {
	if cfg == nil {
		return nil
	}
	name := cfg.Agents.Defaults.ModelName
	for i := range cfg.ModelList {
		if cfg.ModelList[i].ModelName == name {
			return &cfg.ModelList[i]
		}
	}
	return nil
}

func visionModel(cfg *config.Config) *config.ModelConfig {
	if cfg == nil {
		return nil
	}
	name := cfg.Tools.Vision.ModelName
	if name == "" {
		return defaultModel(cfg)
	}
	for i := range cfg.ModelList {
		if cfg.ModelList[i].ModelName == name {
			return &cfg.ModelList[i]
		}
	}
	return nil
}
