package gateway

import (
	"strings"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/stretchr/testify/assert"

	"github.com/sushi30/sushiclaw/internal/agent"
	"github.com/sushi30/sushiclaw/pkg/bus"
)

func TestDebugManagerToggleOnOff(t *testing.T) {
	mgr := NewDebugManager(bus.NewMessageBus())
	ctx := t.Context()

	reply1 := mgr.Set(ctx, "telegram", "chat1", "toggle")
	assert.True(t, mgr.isActive("telegram", "chat1"))
	assert.Contains(t, reply1, "enabled")

	reply2 := mgr.Set(ctx, "telegram", "chat1", "toggle")
	assert.False(t, mgr.isActive("telegram", "chat1"))
	assert.Contains(t, reply2, "disabled")
}

func TestDebugManagerOnOffIdempotent(t *testing.T) {
	mgr := NewDebugManager(bus.NewMessageBus())
	ctx := t.Context()

	assert.Equal(t, "Debug mode enabled.", mgr.Set(ctx, "telegram", "chat1", "on"))
	assert.Equal(t, "Debug mode already enabled.", mgr.Set(ctx, "telegram", "chat1", "on"))
	assert.Equal(t, "Debug mode disabled.", mgr.Set(ctx, "telegram", "chat1", "off"))
	assert.Equal(t, "Debug mode already disabled.", mgr.Set(ctx, "telegram", "chat1", "off"))
}

func TestDebugManagerHeartbeatDefaultAndOverride(t *testing.T) {
	defaultMgr := NewDebugManager(bus.NewMessageBus(), -1)
	assert.Equal(t, agent.DefaultDebugHeartbeatInterval, defaultMgr.HeartbeatInterval())

	overrideMgr := NewDebugManager(bus.NewMessageBus(), 5*time.Second)
	assert.Equal(t, 5*time.Second, overrideMgr.HeartbeatInterval())
}

func TestDebugManagerPerChatIsolation(t *testing.T) {
	mgr := NewDebugManager(bus.NewMessageBus())
	ctx := t.Context()

	mgr.Set(ctx, "telegram", "chat1", "on")

	assert.True(t, mgr.isActive("telegram", "chat1"))
	assert.False(t, mgr.isActive("telegram", "chat2"))
	assert.False(t, mgr.isActive("email", "chat1"))
}

func TestDebugManagerInactiveSuppressesOutbound(t *testing.T) {
	extBus := bus.NewMessageBus()
	mgr := NewDebugManager(extBus)

	mgr.Progress(t.Context(), agent.ProgressEvent{
		Channel: "telegram",
		ChatID:  "chat1",
		Kind:    agent.ProgressTurnStarted,
	})

	assertNoOutbound(t, extBus)
}

func TestDebugManagerPublishesProgressOnlyToEnabledChat(t *testing.T) {
	extBus := bus.NewMessageBus()
	mgr := NewDebugManager(extBus)
	ctx := t.Context()
	mgr.Set(ctx, "telegram", "chat1", "on")

	mgr.Progress(ctx, agent.ProgressEvent{
		Channel:  "telegram",
		ChatID:   "chat2",
		Kind:     agent.ProgressToolCallStarted,
		ToolName: "exec",
	})
	assertNoOutbound(t, extBus)

	mgr.Progress(ctx, agent.ProgressEvent{
		Channel:  "telegram",
		ChatID:   "chat1",
		Kind:     agent.ProgressToolCallStarted,
		ToolName: "exec",
	})
	msg := requireOutbound(t, extBus)
	assert.Equal(t, "telegram", msg.Channel)
	assert.Equal(t, "chat1", msg.ChatID)
	assert.Contains(t, msg.Content, "exec")
}

func TestDebugManagerSummaryFormatsUsage(t *testing.T) {
	extBus := bus.NewMessageBus()
	mgr := NewDebugManager(extBus)
	ctx := t.Context()
	mgr.Set(ctx, "telegram", "chat1", "on")

	mgr.Summary(ctx, agent.ProgressSummary{
		Channel:   "telegram",
		ChatID:    "chat1",
		Success:   true,
		ToolCalls: 2,
		Usage:     &interfaces.TokenUsage{InputTokens: 3, OutputTokens: 5, TotalTokens: 8},
		Duration:  1200 * time.Millisecond,
	})

	msg := requireOutbound(t, extBus)
	assert.Contains(t, msg.Content, "Tool calls: 2")
	assert.Contains(t, msg.Content, "Tokens: total=8 input=3 output=5")
	assert.True(t, strings.Contains(msg.Content, "Task time: 1s") || strings.Contains(msg.Content, "Task time: 1.2s"))
}

func requireOutbound(t *testing.T, extBus *bus.MessageBus) bus.OutboundMessage {
	t.Helper()
	select {
	case msg := <-extBus.OutboundChan():
		return msg
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected outbound message")
		return bus.OutboundMessage{}
	}
}

func assertNoOutbound(t *testing.T, extBus *bus.MessageBus) {
	t.Helper()
	select {
	case msg := <-extBus.OutboundChan():
		t.Fatalf("unexpected outbound message: %#v", msg)
	case <-time.After(20 * time.Millisecond):
	}
}
