package subagenttask_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	agentpkg "github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/tools/subagenttask"
)

type mockLLM struct{}

func (m mockLLM) Generate(_ context.Context, _ string, _ ...interfaces.GenerateOption) (string, error) {
	return "", nil
}
func (m mockLLM) GenerateWithTools(_ context.Context, _ string, _ []interfaces.Tool, _ ...interfaces.GenerateOption) (string, error) {
	return "", nil
}
func (m mockLLM) GenerateDetailed(_ context.Context, _ string, _ ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	return &interfaces.LLMResponse{}, nil
}
func (m mockLLM) GenerateWithToolsDetailed(_ context.Context, _ string, _ []interfaces.Tool, _ ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	return &interfaces.LLMResponse{}, nil
}
func (m mockLLM) Name() string            { return "mock" }
func (m mockLLM) SupportsStreaming() bool { return false }

func TestSubagentTask_RejectsMissingFieldsAndUnknownAgent(t *testing.T) {
	tool := newTestTool(t, func(_ context.Context, input string) (string, error) {
		return input, nil
	})

	_, err := tool.Execute(context.Background(), `{"task":"do it"}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent_type is required")

	_, err = tool.Execute(context.Background(), `{"agent_type":"coder"}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task is required")

	_, err = tool.Execute(context.Background(), `{"agent_type":"missing","task":"do it"}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown subagent "missing"`)
}

func TestSubagentTask_ReturnsStartedImmediately(t *testing.T) {
	release := make(chan struct{})
	tool := newTestTool(t, func(ctx context.Context, input string) (string, error) {
		select {
		case <-release:
			return input, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})
	defer close(release)

	start := time.Now()
	got, err := tool.Execute(context.Background(), `{"agent_type":"coder","task":"do it"}`)
	require.NoError(t, err)

	assert.Equal(t, "Task 1 started.", got)
	assert.Less(t, time.Since(start), 200*time.Millisecond)
}

func TestSubagentTask_PublishesSuccess(t *testing.T) {
	messageBus := bus.NewMessageBus()
	tool := newTestToolWithBus(t, messageBus, func(_ context.Context, input string) (string, error) {
		return "done: " + input, nil
	})
	ctx := subagenttask.WithContext(context.Background(), "telegram", "chat-1", "msg-1")

	got, err := tool.Execute(ctx, `{"agent_type":"coder","task":"do it"}`)
	require.NoError(t, err)
	assert.Equal(t, "Task 1 started.", got)

	out := waitOutbound(t, messageBus)
	assert.Equal(t, "telegram", out.Channel)
	assert.Equal(t, "chat-1", out.ChatID)
	assert.Equal(t, "msg-1", out.ReplyToMessageID)
	assert.Equal(t, "Task 1 completed:\ndone: do it", out.Content)
}

func TestSubagentTask_PublishesFailure(t *testing.T) {
	messageBus := bus.NewMessageBus()
	tool := newTestToolWithBus(t, messageBus, func(_ context.Context, _ string) (string, error) {
		return "", errors.New("boom")
	})
	ctx := subagenttask.WithContext(context.Background(), "telegram", "chat-1", "")

	_, err := tool.Execute(ctx, `{"agent_type":"coder","task":"do it"}`)
	require.NoError(t, err)

	out := waitOutbound(t, messageBus)
	assert.True(t, strings.HasPrefix(out.Content, "Task 1 failed: boom"))
}

func newTestTool(t *testing.T, run func(context.Context, string) (string, error)) *subagenttask.Tool {
	t.Helper()
	return newTestToolWithBus(t, bus.NewMessageBus(), run)
}

func newTestToolWithBus(t *testing.T, messageBus *bus.MessageBus, run func(context.Context, string) (string, error)) *subagenttask.Tool {
	t.Helper()
	cfg := &config.Config{
		SubAgents: map[string]config.SubAgentConfig{
			"coder": {Description: "Code tasks"},
		},
	}
	factory := func(_ *config.Config, name, description, _, _ string, _ []interfaces.Tool) (*agentpkg.Agent, error) {
		return agentpkg.NewAgent(
			agentpkg.WithName(name),
			agentpkg.WithDescription(description),
			agentpkg.WithLLM(mockLLM{}),
			agentpkg.WithCustomRunFunction(func(ctx context.Context, input string, _ *agentpkg.Agent) (string, error) {
				return run(ctx, input)
			}),
		)
	}
	return subagenttask.NewTool(cfg, messageBus, nil, factory)
}

func waitOutbound(t *testing.T, messageBus *bus.MessageBus) bus.OutboundMessage {
	t.Helper()
	select {
	case msg := <-messageBus.OutboundChan():
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for outbound message")
		return bus.OutboundMessage{}
	}
}
