package cron

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/config"
)

func TestSchedulerDeliverMessageUsesStoredContext(t *testing.T) {
	t.Parallel()

	scheduler := newExecutionTestScheduler(t, false)
	job := Job{
		Name:    "deliver",
		Message: "drink water",
		Context: bus.InboundContext{
			Channel:          "telegram",
			ChatID:           "-100123/42",
			ReplyToMessageID: "77",
			Raw: map[string]string{
				"parent_peer_kind": "topic",
				"parent_peer_id":   "42",
			},
		},
		Deliver: true,
		Enabled: true,
	}
	require.NoError(t, scheduler.store.Save([]Job{job}))

	scheduler.executeJob(job)

	msg := requireOutboundMessage(t, scheduler.bus.OutboundChan())
	expectedCtx := job.Context
	if expectedCtx.Raw == nil {
		expectedCtx.Raw = map[string]string{}
	}
	expectedCtx.Raw["message_kind"] = bus.MessageKindSystem
	require.Equal(t, bus.MessageKindSystem, msg.Context.Raw["message_kind"])
	require.Equal(t, "[system] drink water", msg.Content)
	require.Equal(t, expectedCtx, msg.Context)
	require.Equal(t, job.Context.Channel, msg.Channel)
	require.Equal(t, job.Context.ChatID, msg.ChatID)
	require.Equal(t, job.Context.ReplyToMessageID, msg.ReplyToMessageID)
}

func TestSchedulerAgentTurnUsesStoredContext(t *testing.T) {
	t.Parallel()

	scheduler := newExecutionTestScheduler(t, false)
	job := Job{
		Name:    "agent-turn",
		Message: "check in",
		Context: bus.InboundContext{
			Channel:   "telegram",
			ChatID:    "-100123/42",
			ChatType:  "group",
			SenderID:  "user-1",
			MessageID: "99",
		},
		Enabled: true,
	}
	require.NoError(t, scheduler.store.Save([]Job{job}))

	scheduler.executeJob(job)

	msg := requireInboundMessage(t, scheduler.bus.InboundChan())
	require.Equal(t, "check in", msg.Content)
	require.Equal(t, job.Context, msg.Context)
	require.Equal(t, "cron:agent-turn:telegram:-100123/42", msg.SessionKey)
}

func TestSchedulerCommandJobUsesStoredContextForOutput(t *testing.T) {
	t.Parallel()

	scheduler := newExecutionTestScheduler(t, true)
	job := Job{
		Name:    "command",
		Command: "printf hello",
		Context: bus.InboundContext{
			Channel: "telegram",
			ChatID:  "-100123/42",
			Raw: map[string]string{
				"parent_peer_kind": "topic",
				"parent_peer_id":   "42",
			},
		},
		Enabled: true,
	}
	require.NoError(t, scheduler.store.Save([]Job{job}))

	scheduler.executeJob(job)

	msg := requireOutboundMessage(t, scheduler.bus.OutboundChan())
	expectedCtx := job.Context
	if expectedCtx.Raw == nil {
		expectedCtx.Raw = map[string]string{}
	}
	expectedCtx.Raw["message_kind"] = bus.MessageKindSystem
	require.Equal(t, bus.MessageKindSystem, msg.Context.Raw["message_kind"])
	require.Equal(t, "[system] hello", msg.Content)
	require.Equal(t, expectedCtx, msg.Context)
	require.Equal(t, job.Context.Channel, msg.Channel)
	require.Equal(t, job.Context.ChatID, msg.ChatID)
}

func newExecutionTestScheduler(t *testing.T, execEnabled bool) *Scheduler {
	t.Helper()

	tmp := t.TempDir()
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace: filepath.Join(tmp, "workspace"),
			},
		},
		Tools: config.ToolsConfig{
			Exec: config.ExecToolConfig{Enabled: execEnabled},
		},
	}

	messageBus := bus.NewMessageBus()
	scheduler, err := NewScheduler(cfg, messageBus)
	require.NoError(t, err)
	t.Cleanup(scheduler.Stop)
	return scheduler
}

func requireOutboundMessage(t *testing.T, ch <-chan bus.OutboundMessage) bus.OutboundMessage {
	t.Helper()

	select {
	case msg := <-ch:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for outbound message")
		return bus.OutboundMessage{}
	}
}

func requireInboundMessage(t *testing.T, ch <-chan bus.InboundMessage) bus.InboundMessage {
	t.Helper()

	select {
	case msg := <-ch:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for inbound message")
		return bus.InboundMessage{}
	}
}

func TestSchedulerSkipsLegacyJobsWithoutContext(t *testing.T) {
	t.Parallel()

	scheduler := newExecutionTestScheduler(t, false)
	job := Job{
		Name:    "legacy",
		Message: "hello",
		Enabled: true,
	}
	require.NoError(t, scheduler.store.Save([]Job{job}))

	scheduler.executeJob(job)

	select {
	case msg := <-scheduler.bus.OutboundChan():
		t.Fatalf("unexpected outbound message: %#v", msg)
	case msg := <-scheduler.bus.InboundChan():
		t.Fatalf("unexpected inbound message: %#v", msg)
	case <-time.After(200 * time.Millisecond):
	}
}
