package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/sushi30/sushiclaw/pkg/logger"
)

type stubTool struct {
	name    string
	runFunc func(context.Context, string) (string, error)
}

func (s stubTool) Name() string { return s.name }

func (s stubTool) Description() string { return "stub" }

func (s stubTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"input": {Type: "string", Required: true},
	}
}

func (s stubTool) Run(ctx context.Context, input string) (string, error) {
	if s.runFunc != nil {
		return s.runFunc(ctx, input)
	}
	return "ok", nil
}

func (s stubTool) Execute(ctx context.Context, input string) (string, error) {
	return s.Run(ctx, input)
}

func TestWithDebugLoggingPreservesToolIdentity(t *testing.T) {
	wrapped := WithDebugLogging(stubTool{name: "read_file"})
	if wrapped.Name() != "read_file" {
		t.Fatalf("Name() = %q, want read_file", wrapped.Name())
	}
	if wrapped.Description() != "stub" {
		t.Fatalf("Description() = %q, want stub", wrapped.Description())
	}
	if _, ok := wrapped.Parameters()["input"]; !ok {
		t.Fatal("expected parameters to be forwarded")
	}
}

func TestWithDebugLoggingLogsParamsAndDuration(t *testing.T) {
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

	wrapped := WithDebugLogging(stubTool{
		name: "read_file",
		runFunc: func(context.Context, string) (string, error) {
			return "file contents", nil
		},
	})

	out, err := wrapped.Run(context.Background(), `{"path":"README.md"}`)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "file contents" {
		t.Fatalf("Run output = %q, want file contents", out)
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	logs := string(data)
	if !strings.Contains(logs, "Tool invoked") {
		t.Fatalf("logs = %q, want invocation log", logs)
	}
	if !strings.Contains(logs, "README.md") {
		t.Fatalf("logs = %q, want params", logs)
	}
	if !strings.Contains(logs, "Tool completed") {
		t.Fatalf("logs = %q, want completion log", logs)
	}
	if !strings.Contains(logs, "output_bytes") {
		t.Fatalf("logs = %q, want output size", logs)
	}
}
