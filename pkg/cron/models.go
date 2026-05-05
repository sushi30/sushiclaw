package cron

import (
	"time"

	"github.com/sushi30/sushiclaw/pkg/bus"
)

// Job represents a scheduled cron job.
type Job struct {
	Name         string             `json:"name"`
	Message      string             `json:"message"`
	Channel      string             `json:"channel"`
	ChatID       string             `json:"chat_id"`
	SenderID     string             `json:"sender_id"`
	Context      bus.InboundContext `json:"context,omitempty"`
	AtSeconds    *int               `json:"at_seconds,omitempty"`
	EverySeconds *int               `json:"every_seconds,omitempty"`
	CronExpr     string             `json:"cron_expr,omitempty"`
	Timezone     string             `json:"timezone,omitempty"`
	Deliver      bool               `json:"deliver"`
	Command      string             `json:"command,omitempty"`
	Enabled      bool               `json:"enabled"`
	CreatedAt    time.Time          `json:"created_at"`
	State        JobState           `json:"state,omitempty"`
}

// JobState is persisted scheduler bookkeeping. It makes cron observable across
// restarts instead of relying on process-local timers.
type JobState struct {
	NextRunAt         *time.Time `json:"next_run_at,omitempty"`
	RunningAt         *time.Time `json:"running_at,omitempty"`
	LastRunAt         *time.Time `json:"last_run_at,omitempty"`
	LastStatus        string     `json:"last_status,omitempty"`
	LastError         string     `json:"last_error,omitempty"`
	LastDurationMS    int64      `json:"last_duration_ms,omitempty"`
	ConsecutiveErrors int        `json:"consecutive_errors,omitempty"`
}

type StoreFile struct {
	Version int   `json:"version"`
	Jobs    []Job `json:"jobs"`
}

const (
	StatusOK      = "ok"
	StatusError   = "error"
	StatusSkipped = "skipped"
)
