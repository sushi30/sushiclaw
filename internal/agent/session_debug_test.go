package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/logger"
)

type mockRunner struct {
	stream      <-chan interfaces.AgentStreamEvent
	streamErr   error
	detailed    *interfaces.AgentResponse
	detailedErr error
	runResult   string
	runErr      error
	runFunc     func(string) (string, error)
}

func (m *mockRunner) Run(_ context.Context, input string) (string, error) {
	if m.runFunc != nil {
		return m.runFunc(input)
	}
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

	turnCtx, turnSeq, turnDone := session.startTurn(t.Context())
	session.handleInbound(turnCtx, inbound("telegram", "chat1", "hi"), "telegram:chat1", turnSeq, turnDone)

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

	turnCtx, turnSeq, turnDone := session.startTurn(t.Context())
	session.handleInbound(turnCtx, inbound("telegram", "chat1", "bad"), "telegram:chat1", turnSeq, turnDone)

	msg := requireOutboundMessage(t, extBus)
	assert.Equal(t, bus.MessageKindSystem, msg.Context.Raw["message_kind"])
	assert.Equal(t, "[system] Error: run failed", msg.Content)
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

	turnCtx, turnSeq, turnDone := session.startTurn(t.Context())
	session.handleInbound(turnCtx, inbound("telegram", "chat1", "hi"), "telegram:chat1", turnSeq, turnDone)

	data, err := os.ReadFile(logFile)
	require.NoError(t, err)
	logs := string(data)
	assert.Contains(t, logs, "Turn summary")
	assert.Contains(t, logs, "total_tokens")
	assert.Contains(t, logs, "duration")
	assert.Contains(t, logs, "tool_calls")
	assert.Contains(t, logs, "response_bytes")
}

func TestSessionManagerOpenRouterLimitErrorsNotifyUser(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "context length exceeded", err: errors.New("openrouter: context_length_exceeded")},
		{name: "max tokens exceeded", err: errors.New("provider returned max_tokens_exceeded while generating response")},
		{name: "token limit exceeded", err: errors.New("token_limit_exceeded")},
		{name: "string too long", err: errors.New("string_too_long")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			extBus := bus.NewMessageBus()
			progress := &collectingProgress{}
			sm := &SessionManager{bus: extBus, progress: progress}
			session := &Session{agent: &mockRunner{detailedErr: tc.err}, mgr: sm}

			turnCtx, turnSeq, turnDone := session.startTurn(t.Context())
			session.handleInbound(turnCtx, inbound("telegram", "chat1", "bad"), "telegram:chat1", turnSeq, turnDone)

			msg := requireOutboundMessage(t, extBus)
			assert.Equal(t, bus.MessageKindSystem, msg.Context.Raw["message_kind"])
			assert.Equal(t, "[system] This request hit OpenRouter's context or token limit. The turn was not completed. Clear or shorten the session history and try again.", msg.Content)
			assertNoOutboundMessage(t, extBus)
			require.Len(t, progress.summaries, 1)
			assert.False(t, progress.summaries[0].Success)
			assert.ErrorIs(t, progress.summaries[0].Error, tc.err)
			assertHasEvent(t, progress.events, ProgressFailed)
		})
	}
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

func TestSessionManagerStopTurnCancelsActiveWorkAndAllowsNextTurn(t *testing.T) {
	extBus := bus.NewMessageBus()
	progress := &collectingProgress{}
	sm := &SessionManager{
		bus:      extBus,
		progress: progress,
		sessions: map[string]*Session{},
	}

	started := make(chan struct{}, 1)
	session := &Session{
		agent: &scriptedRunner{
			run: func(ctx context.Context, input string) (string, error) {
				switch input {
				case "first":
					started <- struct{}{}
					<-ctx.Done()
					return "", ctx.Err()
				case "second":
					return "second reply", nil
				default:
					return "", errors.New("unexpected input")
				}
			},
		},
		mgr: sm,
	}
	sm.sessions["telegram:chat1"] = session

	go sm.Dispatch(t.Context(), inbound("telegram", "chat1", "first"))
	<-started

	assert.True(t, sm.StopTurn("telegram:chat1"))
	assertNoOutboundMessage(t, extBus)
	assertNotHasEvent(t, progress.events, ProgressFailed)

	sm.Dispatch(t.Context(), inbound("telegram", "chat1", "second"))

	msg := requireOutboundMessage(t, extBus)
	assert.Equal(t, "second reply", msg.Content)
	assert.Equal(t, "telegram:chat1", msg.SessionKey)
}

func TestSessionManagerSecondInboundCancelsFirstAndPublishesOnlySecond(t *testing.T) {
	extBus := bus.NewMessageBus()
	progress := &collectingProgress{}
	sm := &SessionManager{
		bus:      extBus,
		progress: progress,
		sessions: map[string]*Session{},
	}

	firstStarted := make(chan struct{}, 1)
	firstRelease := make(chan struct{})
	session := &Session{
		agent: &scriptedRunner{
			run: func(ctx context.Context, input string) (string, error) {
				switch input {
				case "first":
					firstStarted <- struct{}{}
					<-firstRelease
					return "stale first reply", nil
				case "second":
					return "second reply", nil
				default:
					return "", errors.New("unexpected input")
				}
			},
		},
		mgr: sm,
	}
	sm.sessions["telegram:chat1"] = session

	go sm.Dispatch(t.Context(), inbound("telegram", "chat1", "first"))
	<-firstStarted

	sm.Dispatch(t.Context(), inbound("telegram", "chat1", "second"))
	close(firstRelease)

	msg := requireOutboundMessage(t, extBus)
	assert.Equal(t, "second reply", msg.Content)
	assertNoOutboundMessage(t, extBus)
	assertNotHasEvent(t, progress.events, ProgressFailed)
}

func TestSessionManagerCanceledTurnSuppressesContextCanceledReply(t *testing.T) {
	extBus := bus.NewMessageBus()
	progress := &collectingProgress{}
	sm := &SessionManager{
		bus:      extBus,
		progress: progress,
		sessions: map[string]*Session{},
	}

	started := make(chan struct{}, 1)
	session := &Session{
		agent: &scriptedRunner{
			run: func(ctx context.Context, input string) (string, error) {
				started <- struct{}{}
				<-ctx.Done()
				return "", ctx.Err()
			},
		},
		mgr: sm,
	}
	sm.sessions["telegram:chat1"] = session

	go sm.Dispatch(t.Context(), inbound("telegram", "chat1", "first"))
	<-started
	assert.True(t, sm.StopTurn("telegram:chat1"))

	assertNoOutboundMessage(t, extBus)
	assertNotHasEvent(t, progress.events, ProgressFailed)
}

func TestSessionManagerStaleCanceledTurnCompletionIsIgnored(t *testing.T) {
	extBus := bus.NewMessageBus()
	progress := &collectingProgress{}
	sm := &SessionManager{
		bus:      extBus,
		progress: progress,
		sessions: map[string]*Session{},
	}

	firstStarted := make(chan struct{}, 1)
	firstRelease := make(chan struct{})
	session := &Session{
		agent: &scriptedRunner{
			run: func(ctx context.Context, input string) (string, error) {
				switch input {
				case "first":
					firstStarted <- struct{}{}
					<-firstRelease
					return "stale first reply", nil
				case "second":
					return "fresh second reply", nil
				default:
					return "", errors.New("unexpected input")
				}
			},
		},
		mgr: sm,
	}
	sm.sessions["telegram:chat1"] = session

	go sm.Dispatch(t.Context(), inbound("telegram", "chat1", "first"))
	<-firstStarted

	sm.Dispatch(t.Context(), inbound("telegram", "chat1", "second"))
	close(firstRelease)

	msg := requireOutboundMessage(t, extBus)
	assert.Equal(t, "fresh second reply", msg.Content)
	assertNoOutboundMessage(t, extBus)
}

type scriptedRunner struct {
	run         func(context.Context, string) (string, error)
	runDetailed func(context.Context, string) (*interfaces.AgentResponse, error)
	runStream   func(context.Context, string) (<-chan interfaces.AgentStreamEvent, error)
}

func (s *scriptedRunner) Run(ctx context.Context, input string) (string, error) {
	if s.run == nil {
		return "", errors.New("Run should not be called")
	}
	return s.run(ctx, input)
}

func (s *scriptedRunner) RunDetailed(ctx context.Context, input string) (*interfaces.AgentResponse, error) {
	if s.runDetailed != nil {
		return s.runDetailed(ctx, input)
	}
	if s.run != nil {
		content, err := s.run(ctx, input)
		if err != nil {
			return nil, err
		}
		return &interfaces.AgentResponse{Content: content}, nil
	}
	return nil, errors.New("RunDetailed should not be called")
}

func (s *scriptedRunner) RunStream(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error) {
	if s.runStream == nil {
		return nil, errors.New("RunStream should not be called")
	}
	return s.runStream(ctx, input)
}

func TestSessionManagerContextOverflowCreatesNewSummarySession(t *testing.T) {
	ctx := t.Context()
	extBus := bus.NewMessageBus()
	cfg := &config.Config{
		Agents: config.AgentsConfig{Defaults: config.AgentDefaults{
			ModelName: "test-model",
			Summary: config.SummaryConfig{
				Enabled:      true,
				TokenTrigger: 1024,
				Model:        "summary-model",
			},
		}},
		ModelList: []config.ModelConfig{{
			ModelName: "test-model",
			Model:     "gpt-4o",
			APIKey:    config.NewSecureString("test-key"),
		}, {
			ModelName: "summary-model",
			Model:     "gpt-4o-mini",
			APIKey:    config.NewSecureString("test-key"),
		}},
		Sessions: config.SessionsConfig{Directory: t.TempDir()},
	}

	seed, err := NewSQLiteSessionMemory(ctx, filepath.Join(cfg.Sessions.Directory, "sessions.db"), "telegram:chat1")
	require.NoError(t, err)
	require.NoError(t, seed.AddMessage(ctx, interfaces.Message{Role: interfaces.MessageRoleUser, Content: "remember this"}))
	require.NoError(t, seed.Close())

	overflowErr := errors.New("maximum context length exceeded")
	factoryCalls := 0
	sm := &SessionManager{
		cfg:      cfg,
		bus:      extBus,
		sessions: make(map[string]*Session),
		summaryFactory: func() (agentRunner, error) {
			return &mockRunner{runResult: "Compact summary"}, nil
		},
		agentFactory: func(mem interfaces.Memory) (agentRunner, error) {
			factoryCalls++
			if factoryCalls == 1 {
				return &mockRunner{runErr: overflowErr}, nil
			}
			return &mockRunner{runResult: "retried"}, nil
		},
	}

	session, err := sm.getOrCreateSession("telegram:chat1")
	require.NoError(t, err)
	session.activatedSkills["python"] = true

	sm.Dispatch(ctx, bus.InboundMessage{
		Channel: "telegram",
		ChatID:  "chat1",
		Content: "new request",
	})

	notice := requireOutboundMessage(t, extBus)
	assert.Equal(t, "Summarizing the conversation to keep going...", notice.Content)
	reply := requireOutboundMessage(t, extBus)
	assert.Equal(t, "retried", reply.Content)

	dbPath := filepath.Join(cfg.Sessions.Directory, "sessions.db")
	stable, err := NewSQLiteSessionMemory(ctx, dbPath, "telegram:chat1")
	require.NoError(t, err)
	defer func() { _ = stable.Close() }()
	msgs, err := stable.GetMessages(ctx)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "remember this", msgs[0].Content)

	activeKey, err := resolveActiveSessionKey(ctx, stable.db, "telegram:chat1")
	require.NoError(t, err)
	assert.NotEqual(t, "telegram:chat1", activeKey)

	summary, err := NewSQLiteSessionMemory(ctx, dbPath, activeKey)
	require.NoError(t, err)
	defer func() { _ = summary.Close() }()
	msgs, err = summary.GetMessages(ctx)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, interfaces.MessageRoleAssistant, msgs[0].Role)
	assert.Contains(t, msgs[0].Content, "Compact summary")
	assert.Empty(t, session.activatedSkills)
}

func TestSessionManagerOverflowDoesNotSummarizeWhenDisabled(t *testing.T) {
	overflowErr := errors.New("maximum context length exceeded")
	extBus := bus.NewMessageBus()
	sm := &SessionManager{bus: extBus, cfg: &config.Config{}, progress: &collectingProgress{}}
	session := &Session{agent: &mockRunner{runErr: overflowErr}, mgr: sm, stableKey: "telegram:chat1"}

	turnCtx, turnSeq, turnDone := session.startTurn(t.Context())
	session.handleInbound(turnCtx, inbound("telegram", "chat1", "bad"), "telegram:chat1", turnSeq, turnDone)

	msg := requireOutboundMessage(t, extBus)
	assert.Contains(t, msg.Content, "Error: maximum context length exceeded")
	assertNoOutboundMessage(t, extBus)
}

func TestGenerateSummaryFallsBackToChunkedReduction(t *testing.T) {
	ctx := t.Context()
	cfg := &config.Config{
		Agents: config.AgentsConfig{Defaults: config.AgentDefaults{
			ModelName: "test-model",
			Summary:   config.SummaryConfig{Enabled: true, Model: "summary-model"},
		}},
		ModelList: []config.ModelConfig{{
			ModelName: "test-model",
			Model:     "gpt-4o",
			APIKey:    config.NewSecureString("test-key"),
		}, {
			ModelName: "summary-model",
			Model:     "gpt-4o-mini",
			APIKey:    config.NewSecureString("test-key"),
		}},
	}

	sm := &SessionManager{
		cfg: cfg,
		summaryFactory: func() (agentRunner, error) {
			return &mockRunner{runFunc: func(input string) (string, error) {
				if strings.Contains(input, "Conversation:\nuser: one") && strings.Contains(input, "assistant: two") {
					return "", errors.New("maximum context length exceeded")
				}
				if strings.Contains(input, "Chunk 1 summary:") {
					return "## Goals\n- combined\n## Key Facts\n- merged\n## Decisions\n- none\n## Preferences\n- none\n## Open Threads\n- follow up\n## Resources\n- none", nil
				}
				return "## Goals\n- part\n## Key Facts\n- item\n## Decisions\n- none\n## Preferences\n- none\n## Open Threads\n- none\n## Resources\n- none", nil
			}}, nil
		},
	}
	session := &Session{mgr: sm}

	summary, err := session.generateSummary(ctx, "user: one\n\nassistant: two")
	require.NoError(t, err)
	assert.Contains(t, summary, "## Goals")
	assert.Contains(t, summary, "combined")
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

func assertNotHasEvent(t *testing.T, events []ProgressEvent, kind ProgressKind) {
	t.Helper()
	for _, event := range events {
		if event.Kind == kind {
			t.Fatalf("unexpected event kind %s in %#v", kind, events)
		}
	}
}
