package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/logger"
)

type mockRunner struct {
	stream      <-chan interfaces.AgentStreamEvent
	streamErr   error
	detailed    *interfaces.AgentResponse
	detailedErr error
	runResult   string
	runErr      error
}

func (m *mockRunner) Run(context.Context, string) (string, error) {
	if m.runErr != nil {
		return "", m.runErr
	}
	if m.runResult != "" {
		return m.runResult, nil
	}
	return "", errors.New("Run should not be called")
}

func (m *mockRunner) RunStream(context.Context, string) (<-chan interfaces.AgentStreamEvent, error) {
	return m.stream, m.streamErr
}

func (m *mockRunner) RunDetailed(context.Context, string) (*interfaces.AgentResponse, error) {
	if m.detailedErr != nil {
		return nil, m.detailedErr
	}
	if m.detailed != nil {
		return m.detailed, nil
	}
	if m.runErr != nil {
		return nil, m.runErr
	}
	if m.runResult != "" {
		return &interfaces.AgentResponse{Content: m.runResult}, nil
	}
	return m.detailed, m.detailedErr
}

type collectingProgress struct {
	heartbeat time.Duration
	mu        sync.Mutex
	events    []ProgressEvent
	summaries []ProgressSummary
}

func (c *collectingProgress) HeartbeatInterval() time.Duration {
	if c.heartbeat > 0 {
		return c.heartbeat
	}
	return time.Hour
}

func (c *collectingProgress) Progress(_ context.Context, event ProgressEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}

func (c *collectingProgress) Summary(_ context.Context, summary ProgressSummary) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.summaries = append(c.summaries, summary)
}

func TestSessionManagerDebugStartCompletionAndSummary(t *testing.T) {
	extBus := bus.NewMessageBus()
	progress := &collectingProgress{}
	sm := &SessionManager{bus: extBus, progress: progress}
	session := &Session{
		agent: &mockRunner{
			detailed: &interfaces.AgentResponse{
				Content: "hello",
				Usage:   &interfaces.TokenUsage{InputTokens: 7, OutputTokens: 8, TotalTokens: 15},
				ExecutionSummary: interfaces.ExecutionSummary{
					LLMCalls:  1,
					ToolCalls: 2,
				},
			},
		},
		mgr: sm,
	}

	session.handleInbound(t.Context(), inbound("telegram", "chat1", "hi"), "telegram:chat1")

	msg := requireOutboundMessage(t, extBus)
	assert.Equal(t, "hello", msg.Content)
	assertEventKinds(t, progress.events, ProgressTurnStarted, ProgressCompleted)
	require.Len(t, progress.summaries, 1)
	assert.True(t, progress.summaries[0].Success)
	assert.Equal(t, 2, progress.summaries[0].ToolCalls)
	require.NotNil(t, progress.summaries[0].Usage)
	assert.Equal(t, 15, progress.summaries[0].Usage.TotalTokens)
	assert.Equal(t, len("hello"), progress.summaries[0].ResponseBytes)
	assert.GreaterOrEqual(t, progress.summaries[0].Duration, time.Duration(0))
}

func TestSessionManagerDebugToolEventsUseOnlyNames(t *testing.T) {
	events := streamEvents(
		interfaces.AgentStreamEvent{
			Type: interfaces.AgentEventToolCall,
			ToolCall: &interfaces.ToolCallEvent{
				Name:      "exec",
				Arguments: `{"cmd":"secret"}`,
			},
		},
		interfaces.AgentStreamEvent{
			Type: interfaces.AgentEventToolResult,
			ToolCall: &interfaces.ToolCallEvent{
				Name:   "exec",
				Result: "secret result",
			},
		},
		interfaces.AgentStreamEvent{Type: interfaces.AgentEventContent, Content: "done"},
	)
	progress := &collectingProgress{}
	sm := &SessionManager{bus: bus.NewMessageBus(), progress: progress}
	session := &Session{agent: &mockRunner{stream: events}, mgr: sm}

	response, _, toolCalls, err := session.runStreamingTurn(t.Context(), t.Context(), "telegram", "chat1", "run", time.Now())

	require.NoError(t, err)
	assert.Equal(t, "done", response)
	assert.Equal(t, 1, toolCalls)

	var toolEvents []ProgressEvent
	for _, event := range progress.events {
		if event.Kind == ProgressToolCallStarted || event.Kind == ProgressToolCallFinished {
			toolEvents = append(toolEvents, event)
		}
	}
	require.Len(t, toolEvents, 2)
	for _, event := range toolEvents {
		assert.Equal(t, "exec", event.ToolName)
		assert.NotContains(t, event.ToolName, "secret")
	}
}

func TestSessionManagerDebugTokenSummaryFromStreamMetadata(t *testing.T) {
	events := streamEvents(
		interfaces.AgentStreamEvent{Type: interfaces.AgentEventContent, Content: "ok"},
		interfaces.AgentStreamEvent{
			Type: interfaces.AgentEventContent,
			Metadata: map[string]interface{}{
				"usage": map[string]interface{}{
					"prompt_tokens":     4,
					"completion_tokens": 6,
					"total_tokens":      10,
				},
			},
		},
	)
	progress := &collectingProgress{}
	sm := &SessionManager{bus: bus.NewMessageBus(), progress: progress}
	session := &Session{agent: &mockRunner{stream: events}, mgr: sm}

	_, usage, _, err := session.runStreamingTurn(t.Context(), t.Context(), "telegram", "chat1", "tokens", time.Now())

	require.NoError(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, 4, usage.InputTokens)
	assert.Equal(t, 6, usage.OutputTokens)
	assert.Equal(t, 10, usage.TotalTokens)
}

func TestSessionManagerDebugHeartbeatAfterSilence(t *testing.T) {
	ch := make(chan interfaces.AgentStreamEvent, 2)
	go func() {
		time.Sleep(30 * time.Millisecond)
		ch <- interfaces.AgentStreamEvent{Type: interfaces.AgentEventContent, Content: "late"}
		close(ch)
	}()
	progress := &collectingProgress{heartbeat: 10 * time.Millisecond}
	sm := &SessionManager{bus: bus.NewMessageBus(), progress: progress}
	session := &Session{agent: &mockRunner{stream: ch}, mgr: sm}

	_, _, _, err := session.runStreamingTurn(t.Context(), t.Context(), "telegram", "chat1", "slow", time.Now())

	require.NoError(t, err)
	assertHasEvent(t, progress.events, ProgressHeartbeat)
}

func TestSessionManagerRunErrorPublishesOneUserErrorAndFailureSummary(t *testing.T) {
	runErr := errors.New("run failed")
	extBus := bus.NewMessageBus()
	progress := &collectingProgress{}
	sm := &SessionManager{bus: extBus, progress: progress}
	session := &Session{agent: &mockRunner{detailedErr: runErr}, mgr: sm}

	session.handleInbound(t.Context(), inbound("telegram", "chat1", "bad"), "telegram:chat1")

	msg := requireOutboundMessage(t, extBus)
	assert.Equal(t, "Error: run failed", msg.Content)
	assertNoOutboundMessage(t, extBus)
	require.Len(t, progress.summaries, 1)
	assert.False(t, progress.summaries[0].Success)
	assert.ErrorIs(t, progress.summaries[0].Error, runErr)
	assert.Equal(t, 0, progress.summaries[0].ResponseBytes)
	assertHasEvent(t, progress.events, ProgressFailed)
}

func TestSessionManagerTurnSummaryLogsUsageAndDuration(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "debug.log")

	prevLevel := logger.GetLevel()
	logger.SetLevel(logger.DEBUG)
	require.NoError(t, logger.EnableFileLogging(logFile))
	t.Cleanup(func() {
		logger.DisableFileLogging()
		logger.SetLevel(prevLevel)
	})

	extBus := bus.NewMessageBus()
	progress := &collectingProgress{}
	sm := &SessionManager{bus: extBus, progress: progress}
	session := &Session{
		agent: &mockRunner{
			detailed: &interfaces.AgentResponse{
				Content: "hello",
				Usage:   &interfaces.TokenUsage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3},
				ExecutionSummary: interfaces.ExecutionSummary{
					LLMCalls:  1,
					ToolCalls: 1,
				},
			},
		},
		mgr: sm,
	}

	session.handleInbound(t.Context(), inbound("telegram", "chat1", "hi"), "telegram:chat1")

	data, err := os.ReadFile(logFile)
	require.NoError(t, err)
	logs := string(data)
	assert.Contains(t, logs, "Turn summary")
	assert.Contains(t, logs, "total_tokens")
	assert.Contains(t, logs, "duration")
	assert.Contains(t, logs, "tool_calls")
	assert.Contains(t, logs, "response_bytes")
}

func TestSessionManagerStreamingStartupFallbackUsesDetailedUsage(t *testing.T) {
	startErr := errors.New("no streaming")
	progress := &collectingProgress{}
	sm := &SessionManager{bus: bus.NewMessageBus(), progress: progress}
	session := &Session{
		agent: &mockRunner{
			streamErr: startErr,
			detailed: &interfaces.AgentResponse{
				Content: "fallback",
				Usage:   &interfaces.TokenUsage{InputTokens: 7, OutputTokens: 8, TotalTokens: 15},
				ExecutionSummary: interfaces.ExecutionSummary{
					LLMCalls:  1,
					ToolCalls: 2,
				},
			},
		},
		mgr: sm,
	}

	response, usage, toolCalls, err := session.runStreamingTurn(t.Context(), t.Context(), "telegram", "chat1", "fallback", time.Now())

	require.NoError(t, err)
	assert.Equal(t, "fallback", response)
	assertHasEvent(t, progress.events, ProgressFallback)
	assert.Equal(t, 2, toolCalls)
	require.NotNil(t, usage)
	assert.Equal(t, 15, usage.TotalTokens)
}

func streamEvents(events ...interfaces.AgentStreamEvent) <-chan interfaces.AgentStreamEvent {
	ch := make(chan interfaces.AgentStreamEvent, len(events))
	for _, event := range events {
		ch <- event
	}
	close(ch)
	return ch
}

func inbound(channel, chatID, content string) bus.InboundMessage {
	return bus.InboundMessage{Channel: channel, ChatID: chatID, Content: content}
}

func requireOutboundMessage(t *testing.T, extBus *bus.MessageBus) bus.OutboundMessage {
	t.Helper()
	select {
	case msg := <-extBus.OutboundChan():
		return msg
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected outbound message")
		return bus.OutboundMessage{}
	}
}

func assertNoOutboundMessage(t *testing.T, extBus *bus.MessageBus) {
	t.Helper()
	select {
	case msg := <-extBus.OutboundChan():
		t.Fatalf("unexpected outbound message: %#v", msg)
	case <-time.After(20 * time.Millisecond):
	}
}

func assertEventKinds(t *testing.T, events []ProgressEvent, want ...ProgressKind) {
	t.Helper()
	require.GreaterOrEqual(t, len(events), len(want))
	for i, kind := range want {
		assert.Equal(t, kind, events[i].Kind)
	}
}

func assertHasEvent(t *testing.T, events []ProgressEvent, kind ProgressKind) {
	t.Helper()
	for _, event := range events {
		if event.Kind == kind {
			return
		}
	}
	t.Fatalf("expected event kind %s in %#v", kind, events)
}
