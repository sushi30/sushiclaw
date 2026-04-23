package logger

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetLevel(t *testing.T) {
	SetLevel(DEBUG)
	if GetLevel() != DEBUG {
		t.Errorf("GetLevel() = %v, want DEBUG", GetLevel())
	}

	SetLevel(INFO)
	if GetLevel() != INFO {
		t.Errorf("GetLevel() = %v, want INFO", GetLevel())
	}

	SetLevel(WARN)
	if GetLevel() != WARN {
		t.Errorf("GetLevel() = %v, want WARN", GetLevel())
	}

	SetLevel(ERROR)
	if GetLevel() != ERROR {
		t.Errorf("GetLevel() = %v, want ERROR", GetLevel())
	}
}

func TestSetLevelFromString(t *testing.T) {
	tests := []struct {
		input string
		want  LogLevel
	}{
		{"debug", DEBUG},
		{"info", INFO},
		{"warn", WARN},
		{"error", ERROR},
		{"invalid", INFO}, // should keep current or no-op
	}

	for _, tc := range tests {
		SetLevel(INFO) // reset
		SetLevelFromString(tc.input)
		got := GetLevel()
		if got != tc.want {
			t.Errorf("SetLevelFromString(%q): got %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestEnableDisableFileLogging(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	err := EnableFileLogging(logFile)
	if err != nil {
		t.Fatalf("EnableFileLogging: %v", err)
	}

	// Log something
	Info("test message")

	DisableFileLogging()

	// Verify file exists and has content
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected log file to have content")
	}
}

func TestInitPanic(t *testing.T) {
	tmpDir := t.TempDir()
	panicFile := filepath.Join(tmpDir, "panic.log")

	cleanup, err := InitPanic(panicFile)
	if err != nil {
		t.Fatalf("InitPanic: %v", err)
	}
	defer cleanup()

	// Verify directory was created
	if _, err := os.Stat(filepath.Dir(panicFile)); err != nil {
		t.Errorf("panic log dir not created: %v", err)
	}
}

func TestLoggerMethods(t *testing.T) {
	l := NewLogger("test-component")

	// These should not panic
	l.Debugf("debug %s", "msg")
	l.Infof("info %s", "msg")
	l.Warnf("warn %s", "msg")
	l.Errorf("error %s", "msg")
}

func TestPackageLevelLoggers(t *testing.T) {
	// These should not panic
	Debug("debug")
	DebugC("comp", "debug")
	DebugCF("comp", "debug", map[string]any{"k": "v"})

	Info("info")
	InfoC("comp", "info")
	InfoCF("comp", "info", map[string]any{"k": "v"})

	Warn("warn")
	WarnC("comp", "warn")
	WarnCF("comp", "warn", map[string]any{"k": "v"})

	Error("error")
	ErrorCF("comp", "error", map[string]any{"k": "v"})
}
