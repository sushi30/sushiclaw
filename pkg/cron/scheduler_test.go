package cron

import (
	"context"
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

func TestSchedulerCronExpressionUsesJobTimezone(t *testing.T) {
	t.Parallel()

	scheduler := newExecutionTestScheduler(t, false)
	now := time.Date(2026, time.May, 5, 5, 59, 0, 0, time.UTC)
	job := Job{
		Name:      "morning",
		CronExpr:  "5 8 * * *",
		Timezone:  "Europe/Amsterdam",
		Enabled:   true,
		CreatedAt: now,
	}

	next, err := scheduler.computeNextRun(job, now)
	require.NoError(t, err)
	require.NotNil(t, next)
	require.Equal(t, time.Date(2026, time.May, 5, 6, 5, 0, 0, time.UTC), *next)
}

func TestSchedulerFinishJobPersistsStatusAndNextRun(t *testing.T) {
	t.Parallel()

	scheduler := newExecutionTestScheduler(t, false)
	every := 60
	start := time.Date(2026, time.May, 5, 6, 0, 0, 0, time.UTC)
	job := Job{
		Name:         "heartbeat",
		Message:      "tick",
		Channel:      "telegram",
		ChatID:       "123",
		SenderID:     "123",
		EverySeconds: &every,
		Enabled:      true,
		CreatedAt:    start,
	}
	require.NoError(t, scheduler.store.Save([]Job{job}))

	scheduler.finishJob(job.Name, StatusOK, "", start, start.Add(2*time.Second))

	jobs, err := scheduler.store.Load()
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.Equal(t, StatusOK, jobs[0].State.LastStatus)
	require.NotNil(t, jobs[0].State.LastRunAt)
	require.NotNil(t, jobs[0].State.NextRunAt)
	require.Equal(t, start.Add(62*time.Second), *jobs[0].State.NextRunAt)
}

func TestSchedulerRunJobUsesStartContext(t *testing.T) {
	t.Parallel()

	scheduler := newExecutionTestScheduler(t, false)
	started := make(chan struct{})
	done := make(chan struct{})
	scheduler.SetAgentRunner(blockingAgentRunner{
		started: started,
		done:    done,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scheduler.Start(ctx)

	job := Job{
		Name:    "agent",
		Message: "check",
		Context: bus.InboundContext{
			Channel:  "telegram",
			ChatID:   "123",
			SenderID: "123",
		},
		Enabled:   true,
		CreatedAt: time.Now(),
	}
	require.NoError(t, scheduler.AddJob(job))
	require.NoError(t, scheduler.RunJob(job.Name, true))

	require.Eventually(t, func() bool {
		select {
		case <-started:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
	cancel()
	require.Eventually(t, func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
}

type blockingAgentRunner struct {
	started chan struct{}
	done    chan struct{}
}

func (r blockingAgentRunner) RunCronAgentTurn(ctx context.Context, _ bus.InboundMessage) (AgentRunResult, error) {
	close(r.started)
	<-ctx.Done()
	close(r.done)
	return AgentRunResult{}, ctx.Err()
}
