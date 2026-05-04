package cron

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/tools/toolctx"
)

func TestCronToolAddStoresInboundContext(t *testing.T) {
	t.Parallel()

	scheduler := newTestScheduler(t)
	tool := NewCronTool(scheduler, scheduler.cfg)

	inboundCtx := bus.InboundContext{
		Channel:          "telegram",
		ChatID:           "-100123/42",
		ChatType:         "group",
		SenderID:         "user-1",
		MessageID:        "99",
		ReplyToMessageID: "77",
		Raw: map[string]string{
			"parent_peer_kind": "topic",
			"parent_peer_id":   "42",
		},
	}

	ctx := context.Background()
	ctx = toolctx.WithChatID(ctx, inboundCtx.ChatID)
	ctx = toolctx.WithChannel(ctx, inboundCtx.Channel)
	ctx = toolctx.WithSenderID(ctx, inboundCtx.SenderID)
	ctx = toolctx.WithInboundContext(ctx, inboundCtx)

	_, err := tool.Execute(ctx, `{"action":"add","name":"daily","message":"hello","cron_expr":"0 9 * * *","deliver":true}`)
	require.NoError(t, err)

	jobs, err := scheduler.ListJobs()
	require.NoError(t, err)
	require.Len(t, jobs, 1)

	require.Equal(t, inboundCtx.Channel, jobs[0].Channel)
	require.Equal(t, inboundCtx.ChatID, jobs[0].ChatID)
	require.Equal(t, inboundCtx.SenderID, jobs[0].SenderID)
	require.Equal(t, inboundCtx, jobs[0].Context)
}

func newTestScheduler(t *testing.T) *Scheduler {
	t.Helper()

	tmp := t.TempDir()
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace: filepath.Join(tmp, "workspace"),
			},
		},
	}

	scheduler, err := NewScheduler(cfg, bus.NewMessageBus())
	require.NoError(t, err)
	t.Cleanup(scheduler.Stop)
	return scheduler
}
