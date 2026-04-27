package exec

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/sushi30/sushiclaw/pkg/tools/toolctx"
)

// WithChatID returns a context with the chat ID set.
// Deprecated: use toolctx.WithChatID instead.
func WithChatID(ctx context.Context, chatID string) context.Context {
	return toolctx.WithChatID(ctx, chatID)
}

// ChatIDFromContext returns the chat ID from the context, if any.
// Deprecated: use toolctx.ChatIDFromContext instead.
func ChatIDFromContext(ctx context.Context) string {
	return toolctx.ChatIDFromContext(ctx)
}

// ExecTool runs shell commands.
type ExecTool struct {
	workingDir          string
	restrictToWorkspace bool
	allowRemote         bool
}

// NewExecTool creates a new exec tool.
func NewExecTool(workingDir string, restrictToWorkspace, allowRemote bool) *ExecTool {
	return &ExecTool{
		workingDir:          workingDir,
		restrictToWorkspace: restrictToWorkspace,
		allowRemote:         allowRemote,
	}
}

func (e *ExecTool) Name() string        { return "exec" }
func (e *ExecTool) Description() string { return "Execute a shell command. Use with caution." }

func (e *ExecTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"command": {
			Type:        "string",
			Description: "The shell command to execute",
			Required:    true,
		},
	}
}

// Run executes the tool with the given input string.
func (e *ExecTool) Run(ctx context.Context, input string) (string, error) {
	return e.Execute(ctx, input)
}

// Execute executes the tool with the given arguments JSON string.
func (e *ExecTool) Execute(ctx context.Context, args string) (string, error) {
	cmdStr := parseCommand(args)
	if cmdStr == "" {
		return "", fmt.Errorf("no command provided")
	}

	// Check remote restriction.
	if !e.allowRemote {
		chatID := ChatIDFromContext(ctx)
		if chatID != "" && !isLocalChat(chatID) {
			return "", fmt.Errorf("remote exec is disabled for this chat")
		}
	}

	// Check workspace restriction.
	wd := e.workingDir
	if wd == "" {
		wd, _ = os.Getwd()
	}
	if e.restrictToWorkspace {
		// Ensure the command doesn't escape the workspace.
		cmdStr = restrictCommand(cmdStr, wd)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	cmd.Dir = wd
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("exec error: %w", err)
	}
	return string(out), nil
}

func parseCommand(args string) string {
	args = strings.TrimSpace(args)
	if args == "" {
		return ""
	}

	var payload struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(args), &payload); err == nil && payload.Command != "" {
		return strings.TrimSpace(payload.Command)
	}

	var raw string
	if err := json.Unmarshal([]byte(args), &raw); err == nil {
		return strings.TrimSpace(raw)
	}

	return args
}

func isLocalChat(chatID string) bool {
	// Known local identifiers.
	switch chatID {
	case "cli", "direct", "internal", "localhost", "":
		return true
	}
	// Phone numbers, emails, and canonical IDs are remote.
	if strings.HasPrefix(chatID, "+") {
		return false
	}
	if strings.Contains(chatID, "@") || strings.Contains(chatID, ":") {
		return false
	}
	return true
}

func restrictCommand(cmd, workspace string) string {
	// Basic restriction: block cd commands that try to escape.
	if strings.Contains(cmd, "cd /") || strings.Contains(cmd, "cd ~") {
		return "echo 'Workspace restriction: cd outside workspace blocked'"
	}
	return cmd
}
