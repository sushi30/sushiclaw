package exec

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// Context key for chat ID.
type chatIDKey struct{}

// WithChatID returns a context with the chat ID set.
func WithChatID(ctx context.Context, chatID string) context.Context {
	return context.WithValue(ctx, chatIDKey{}, chatID)
}

// ChatIDFromContext returns the chat ID from the context, if any.
func ChatIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(chatIDKey{}).(string)
	return v
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
	// Simple parsing: if args looks like a JSON object with "command", extract it.
	cmdStr := strings.Trim(args, `{} `)
	if strings.HasPrefix(cmdStr, `"command":`) {
		parts := strings.SplitN(cmdStr, `"command":`, 2)
		if len(parts) == 2 {
			cmdStr = strings.Trim(parts[1], ` "{},`)
		}
	}
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
