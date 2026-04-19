package gateway

import (
	"context"
	"errors"
	"testing"
)

type stubProcessor struct {
	response string
	err      error
	// captures what was passed in
	gotChannel string
	gotChatID  string
}

func (s *stubProcessor) ProcessHeartbeat(_ context.Context, _, channel, chatID string) (string, error) {
	s.gotChannel = channel
	s.gotChatID = chatID
	return s.response, s.err
}

func TestCreateHeartbeatHandler_DefaultsEmptyChannelToCLI(t *testing.T) {
	proc := &stubProcessor{response: "HEARTBEAT_OK"}
	handler := createHeartbeatHandler(proc)

	handler("check time", "", "")

	if proc.gotChannel != "cli" {
		t.Errorf("channel = %q, want %q", proc.gotChannel, "cli")
	}
	if proc.gotChatID != "direct" {
		t.Errorf("chatID = %q, want %q", proc.gotChatID, "direct")
	}
}

func TestCreateHeartbeatHandler_PreservesNonEmptyChannel(t *testing.T) {
	proc := &stubProcessor{response: "HEARTBEAT_OK"}
	handler := createHeartbeatHandler(proc)

	handler("check time", "telegram", "123456")

	if proc.gotChannel != "telegram" {
		t.Errorf("channel = %q, want %q", proc.gotChannel, "telegram")
	}
	if proc.gotChatID != "123456" {
		t.Errorf("chatID = %q, want %q", proc.gotChatID, "123456")
	}
}

func TestCreateHeartbeatHandler_HeartbeatOK_ReturnsSilent(t *testing.T) {
	proc := &stubProcessor{response: "HEARTBEAT_OK"}
	handler := createHeartbeatHandler(proc)

	result := handler("check", "telegram", "123")

	if result.IsError {
		t.Errorf("expected no error, got: %s", result.ForLLM)
	}
	if !result.Silent {
		t.Error("expected Silent=true for HEARTBEAT_OK")
	}
}

func TestCreateHeartbeatHandler_OtherResponse_ReturnsSilentWithContent(t *testing.T) {
	proc := &stubProcessor{response: "Rain expected tomorrow"}
	handler := createHeartbeatHandler(proc)

	result := handler("weather", "telegram", "123")

	if result.IsError {
		t.Errorf("unexpected error: %s", result.ForLLM)
	}
	if !result.Silent {
		t.Error("expected Silent=true")
	}
	if result.ForLLM != "Rain expected tomorrow" {
		t.Errorf("ForLLM = %q, want %q", result.ForLLM, "Rain expected tomorrow")
	}
}

func TestCreateHeartbeatHandler_ProcessError_ReturnsError(t *testing.T) {
	proc := &stubProcessor{err: errors.New("provider offline")}
	handler := createHeartbeatHandler(proc)

	result := handler("check", "telegram", "123")

	if !result.IsError {
		t.Error("expected IsError=true on processor error")
	}
	if result.ForLLM == "" {
		t.Error("expected non-empty error message")
	}
}
