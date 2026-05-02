package agent

import (
	"context"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

const DefaultDebugHeartbeatInterval = 30 * time.Second

type ProgressKind string

const (
	ProgressTurnStarted      ProgressKind = "turn_started"
	ProgressFirstActivity    ProgressKind = "first_activity"
	ProgressToolCallStarted  ProgressKind = "tool_call_started"
	ProgressToolCallFinished ProgressKind = "tool_call_finished"
	ProgressFallback         ProgressKind = "fallback"
	ProgressHeartbeat        ProgressKind = "heartbeat"
	ProgressCompleted        ProgressKind = "completed"
	ProgressFailed           ProgressKind = "failed"
)

type ProgressEvent struct {
	Channel string
	ChatID  string
	Kind    ProgressKind

	ToolName string
	Error    error
	Elapsed  time.Duration
}

type ProgressSummary struct {
	Channel string
	ChatID  string
	Success bool

	ToolCalls     int
	Usage         *interfaces.TokenUsage
	Duration      time.Duration
	ResponseBytes int
	Error         error
}

type ProgressSink interface {
	HeartbeatInterval() time.Duration
	Progress(ctx context.Context, event ProgressEvent)
	Summary(ctx context.Context, summary ProgressSummary)
}
