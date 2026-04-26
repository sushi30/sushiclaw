package cron

import "time"

// Job represents a scheduled cron job.
type Job struct {
	Name         string    `json:"name"`
	Message      string    `json:"message"`
	Channel      string    `json:"channel"`
	ChatID       string    `json:"chat_id"`
	SenderID     string    `json:"sender_id"`
	AtSeconds    *int      `json:"at_seconds,omitempty"`
	EverySeconds *int      `json:"every_seconds,omitempty"`
	CronExpr     string    `json:"cron_expr,omitempty"`
	Deliver      bool      `json:"deliver"`
	Command      string    `json:"command,omitempty"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
}
