package exec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sushi30/sushiclaw/pkg/logger"
)

func TestWithChatID(t *testing.T) {
	ctx := WithChatID(context.Background(), "chat123")
	got := ChatIDFromContext(ctx)
	if got != "chat123" {
		t.Errorf("ChatIDFromContext = %q, want chat123", got)
	}
}

func TestChatIDFromContext_Missing(t *testing.T) {
	got := ChatIDFromContext(context.Background())
	if got != "" {
		t.Errorf("ChatIDFromContext = %q, want empty", got)
	}
}

func TestExecTool_NameAndDescription(t *testing.T) {
	tool := NewExecTool("", false, false)
	if tool.Name() != "exec" {
		t.Errorf("Name = %q, want exec", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestExecTool_Parameters(t *testing.T) {
	tool := NewExecTool("", false, false)
	params := tool.Parameters()
	if len(params) != 1 {
		t.Fatalf("expected 1 parameter, got %d", len(params))
	}
	p, ok := params["command"]
	if !ok {
		t.Fatal("expected 'command' parameter")
	}
	if p.Type != "string" {
		t.Errorf("command type = %v, want string", p.Type)
	}
	if !p.Required {
		t.Error("expected command to be required")
	}
}

func TestExecTool_Execute_Success(t *testing.T) {
	tool := NewExecTool("", false, true)
	ctx := WithChatID(context.Background(), "cli")

	out, err := tool.Execute(ctx, `{"command":"echo hello"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("output = %q, want to contain 'hello'", out)
	}
}

func TestExecTool_Execute_RawCommand(t *testing.T) {
	tool := NewExecTool("", false, true)
	ctx := WithChatID(context.Background(), "cli")

	out, err := tool.Execute(ctx, `echo hello world`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "hello world") {
		t.Errorf("output = %q, want to contain 'hello world'", out)
	}
}

func TestExecTool_Execute_JSONCommandPreservesQuoting(t *testing.T) {
	tool := NewExecTool("", false, true)
	ctx := WithChatID(context.Background(), "cli")

	out, err := tool.Execute(ctx, `{"command":"printf '%s\n' \"hello world\""}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "hello world\n" {
		t.Errorf("output = %q, want %q", out, "hello world\n")
	}
}

func TestExecTool_Execute_JSONStringCommand(t *testing.T) {
	tool := NewExecTool("", false, true)
	ctx := WithChatID(context.Background(), "cli")

	out, err := tool.Execute(ctx, `"echo string-command"`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "string-command") {
		t.Errorf("output = %q, want to contain 'string-command'", out)
	}
}

func TestExecTool_Execute_Empty(t *testing.T) {
	tool := NewExecTool("", false, true)
	_, err := tool.Execute(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty command")
	}
}

func TestExecTool_Execute_RemoteBlocked(t *testing.T) {
	tool := NewExecTool("", false, false) // allowRemote=false
	ctx := WithChatID(context.Background(), "+1234567890")

	_, err := tool.Execute(ctx, `{"command":"echo hello"}`)
	if err == nil {
		t.Fatal("expected error for remote chat")
	}
	if !strings.Contains(err.Error(), "remote exec is disabled") {
		t.Errorf("error = %v, want 'remote exec is disabled'", err)
	}
}

func TestExecTool_Execute_RemoteAllowed(t *testing.T) {
	tool := NewExecTool("", false, true) // allowRemote=true
	ctx := WithChatID(context.Background(), "+1234567890")

	out, err := tool.Execute(ctx, `{"command":"echo hello"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("output = %q, want to contain 'hello'", out)
	}
}

func TestExecTool_Execute_RestrictToWorkspace(t *testing.T) {
	tool := NewExecTool("", true, true)
	ctx := WithChatID(context.Background(), "cli")

	// This command should be restricted
	out, err := tool.Execute(ctx, `cd / && echo escaped`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Workspace restriction") {
		t.Errorf("output = %q, want workspace restriction message", out)
	}
}

func TestExecTool_Run(t *testing.T) {
	tool := NewExecTool("", false, true)
	ctx := WithChatID(context.Background(), "cli")

	out, err := tool.Run(ctx, `{"command":"echo run-test"}`)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out, "run-test") {
		t.Errorf("output = %q, want to contain 'run-test'", out)
	}
}

func TestExecTool_LogsExecutedCommand(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "debug.log")

	prevLevel := logger.GetLevel()
	logger.SetLevel(logger.DEBUG)
	if err := logger.EnableFileLogging(logFile); err != nil {
		t.Fatalf("EnableFileLogging: %v", err)
	}
	t.Cleanup(func() {
		logger.DisableFileLogging()
		logger.SetLevel(prevLevel)
	})

	tool := NewExecTool("", false, true)
	ctx := WithChatID(context.Background(), "cli")

	_, err := tool.Execute(ctx, `{"command":"echo log-me"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	logs := string(data)
	if !strings.Contains(logs, "Executing command") {
		t.Fatalf("logs = %q, want to contain executing command entry", logs)
	}
	if !strings.Contains(logs, "log-me") {
		t.Fatalf("logs = %q, want to contain the command", logs)
	}
	if !strings.Contains(logs, "duration") {
		t.Fatalf("logs = %q, want to contain duration", logs)
	}
}

func TestIsLocalChat(t *testing.T) {
	tests := []struct {
		chatID string
		want   bool
	}{
		{"cli", true},
		{"direct", true},
		{"internal", true},
		{"localhost", true},
		{"", true},
		{"+1234567890", false},
		{"user@example.com", false},
		{"telegram:123456", false},
		{"someuser", true},
	}
	for _, tc := range tests {
		got := isLocalChat(tc.chatID)
		if got != tc.want {
			t.Errorf("isLocalChat(%q) = %v, want %v", tc.chatID, got, tc.want)
		}
	}
}

func TestRestrictCommand(t *testing.T) {
	tests := []struct {
		cmd, want string
	}{
		{"echo hello", "echo hello"},
		{"cd / && ls", "echo 'Workspace restriction: cd outside workspace blocked'"},
		{"cd ~ && ls", "echo 'Workspace restriction: cd outside workspace blocked'"},
		{"pwd", "pwd"},
	}
	for _, tc := range tests {
		got := restrictCommand(tc.cmd, "/tmp")
		if got != tc.want {
			t.Errorf("restrictCommand(%q) = %q, want %q", tc.cmd, got, tc.want)
		}
	}
}
