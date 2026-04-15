package gateway

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
)

// debugStubProvider satisfies providers.LLMProvider without real API calls.
type debugStubProvider struct{}

func (s *debugStubProvider) Chat(_ context.Context, _ []providers.Message, _ []providers.ToolDefinition, _ string, _ map[string]any) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{}, nil
}
func (s *debugStubProvider) GetDefaultModel() string { return "stub" }

func newDebugTestLoop() (*agent.AgentLoop, *bus.MessageBus) {
	agentBus := bus.NewMessageBus()
	loop := agent.NewAgentLoop(config.DefaultConfig(), agentBus, &debugStubProvider{})
	return loop, agentBus
}

// TestDebugManager_ToggleOnOff verifies that the first Toggle enables debug
// mode and the second disables it, with distinct reply strings each time.
func TestDebugManager_ToggleOnOff(t *testing.T) {
	loop, _ := newDebugTestLoop()
	extBus := bus.NewMessageBus()
	mgr := &DebugManager{agentLoop: loop, externalBus: extBus}

	ctx := t.Context()

	reply1 := mgr.Toggle(ctx, "telegram", "chat1")
	if !mgr.active {
		t.Fatal("expected active=true after first Toggle")
	}
	if !strings.Contains(reply1, "enabled") {
		t.Fatalf("first Toggle reply should contain 'enabled', got %q", reply1)
	}

	reply2 := mgr.Toggle(ctx, "telegram", "chat1")
	if mgr.active {
		t.Fatal("expected active=false after second Toggle")
	}
	if !strings.Contains(reply2, "disabled") {
		t.Fatalf("second Toggle reply should contain 'disabled', got %q", reply2)
	}
}

// TestDebugManager_FormatEvent checks that each interesting EventKind produces
// a [debug]-prefixed string, and that suppressed kinds return "".
func TestDebugManager_FormatEvent(t *testing.T) {
	mgr := &DebugManager{}

	interesting := []struct {
		name    string
		evt     agent.Event
		wantPfx string
	}{
		{
			name:    "turn_start",
			evt:     agent.Event{Kind: agent.EventKindTurnStart, Payload: agent.TurnStartPayload{Channel: "tg", ChatID: "c1"}},
			wantPfx: "[debug] turn start",
		},
		{
			name:    "turn_end",
			evt:     agent.Event{Kind: agent.EventKindTurnEnd, Payload: agent.TurnEndPayload{Status: agent.TurnEndStatusCompleted, Iterations: 2, Duration: 500 * time.Millisecond}},
			wantPfx: "[debug] turn end",
		},
		{
			name:    "llm_request",
			evt:     agent.Event{Kind: agent.EventKindLLMRequest, Payload: agent.LLMRequestPayload{Model: "claude", MessagesCount: 4, ToolsCount: 2}},
			wantPfx: "[debug] llm→",
		},
		{
			name:    "llm_response",
			evt:     agent.Event{Kind: agent.EventKindLLMResponse, Payload: agent.LLMResponsePayload{ToolCalls: 1, ContentLen: 200}},
			wantPfx: "[debug] llm←",
		},
		{
			name:    "llm_retry",
			evt:     agent.Event{Kind: agent.EventKindLLMRetry, Payload: agent.LLMRetryPayload{Attempt: 1, MaxRetries: 3, Reason: "timeout", Backoff: time.Second}},
			wantPfx: "[debug] llm retry",
		},
		{
			name:    "tool_exec_start",
			evt:     agent.Event{Kind: agent.EventKindToolExecStart, Payload: agent.ToolExecStartPayload{Tool: "bash", Arguments: map[string]any{"cmd": "ls"}}},
			wantPfx: "[debug] tool↓ bash",
		},
		{
			name:    "tool_exec_end",
			evt:     agent.Event{Kind: agent.EventKindToolExecEnd, Payload: agent.ToolExecEndPayload{Tool: "bash", Duration: 100 * time.Millisecond}},
			wantPfx: "[debug] tool↑ bash",
		},
		{
			name:    "tool_exec_skipped",
			evt:     agent.Event{Kind: agent.EventKindToolExecSkipped, Payload: agent.ToolExecSkippedPayload{Tool: "bash", Reason: "hard_abort"}},
			wantPfx: "[debug] tool skipped",
		},
		{
			name:    "context_compress",
			evt:     agent.Event{Kind: agent.EventKindContextCompress, Payload: agent.ContextCompressPayload{Reason: "proactive_budget", DroppedMessages: 5, RemainingMessages: 10}},
			wantPfx: "[debug] context compressed",
		},
		{
			name:    "error",
			evt:     agent.Event{Kind: agent.EventKindError, Payload: agent.ErrorPayload{Stage: "tool", Message: "exec failed"}},
			wantPfx: "[debug] error",
		},
	}

	for _, tc := range interesting {
		t.Run(tc.name, func(t *testing.T) {
			got := mgr.formatEvent(tc.evt)
			if got == "" {
				t.Fatalf("formatEvent(%s) returned empty string, want %q prefix", tc.name, tc.wantPfx)
			}
			if !strings.HasPrefix(got, tc.wantPfx) {
				t.Fatalf("formatEvent(%s) = %q, want prefix %q", tc.name, got, tc.wantPfx)
			}
		})
	}

	suppressed := []struct {
		name string
		kind agent.EventKind
	}{
		{"llm_delta", agent.EventKindLLMDelta},
		{"session_summarize", agent.EventKindSessionSummarize},
		{"steering_injected", agent.EventKindSteeringInjected},
		{"follow_up_queued", agent.EventKindFollowUpQueued},
		{"interrupt_received", agent.EventKindInterruptReceived},
		{"subturn_spawn", agent.EventKindSubTurnSpawn},
		{"subturn_end", agent.EventKindSubTurnEnd},
		{"subturn_result_delivered", agent.EventKindSubTurnResultDelivered},
		{"subturn_orphan", agent.EventKindSubTurnOrphan},
	}

	for _, tc := range suppressed {
		t.Run(tc.name+"_suppressed", func(t *testing.T) {
			got := mgr.formatEvent(agent.Event{Kind: tc.kind})
			if got != "" {
				t.Fatalf("formatEvent(%s) = %q, want empty (suppressed)", tc.name, got)
			}
		})
	}
}

// TestDebugManager_EventsForwardedWhenActive verifies that after Toggle(on),
// events from the agent loop appear as outbound messages on externalBus.
func TestDebugManager_EventsForwardedWhenActive(t *testing.T) {
	loop, _ := newDebugTestLoop()
	extBus := bus.NewMessageBus()
	mgr := &DebugManager{agentLoop: loop, externalBus: extBus}

	ctx := t.Context()

	mgr.Toggle(ctx, "telegram", "chat42")
	// Give the subscriber goroutine a moment to start.
	time.Sleep(20 * time.Millisecond)

	// Trigger a turn via ProcessDirect — emits TurnStart + LLMRequest + TurnEnd events.
	go func() {
		_, _ = loop.ProcessDirect(ctx, "hello", "debug-test-session")
	}()

	select {
	case msg := <-extBus.OutboundChan():
		if msg.ChatID != "chat42" {
			t.Fatalf("expected ChatID chat42, got %q", msg.ChatID)
		}
		if msg.Channel != "telegram" {
			t.Fatalf("expected Channel telegram, got %q", msg.Channel)
		}
		if !strings.HasPrefix(msg.Content, "[debug]") {
			t.Fatalf("expected [debug] prefix, got %q", msg.Content)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for debug event on externalBus")
	}
}

// TestDebugManager_NoEventsWhenInactive verifies that when debug mode has not
// been toggled on, no messages appear on externalBus from agent events.
func TestDebugManager_NoEventsWhenInactive(t *testing.T) {
	loop, _ := newDebugTestLoop()
	extBus := bus.NewMessageBus()
	// No DebugManager created / Toggle not called — no subscriber registered.

	ctx := t.Context()

	go func() {
		_, _ = loop.ProcessDirect(ctx, "hello", "debug-inactive-session")
	}()

	select {
	case msg := <-extBus.OutboundChan():
		t.Fatalf("unexpected message on externalBus when debug inactive: %+v", msg)
	case <-time.After(300 * time.Millisecond):
		// Nothing arrived — correct.
	}
}
