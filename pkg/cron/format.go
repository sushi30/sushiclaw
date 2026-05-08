package cron

import (
	"fmt"
	"strings"
	"time"
)

// FormatJobs renders cron jobs for chat and CLI surfaces.
func FormatJobs(jobs []Job) string {
	if len(jobs) == 0 {
		return "No cron jobs scheduled."
	}
	var sb strings.Builder
	sb.WriteString("Scheduled jobs:\n")
	for _, j := range jobs {
		fmt.Fprintf(&sb, "- %s", j.Name)
		if !j.Enabled {
			sb.WriteString(" [disabled]")
		}
		if j.Command != "" {
			fmt.Fprintf(&sb, " (command: %s)", j.Command)
		} else if j.Deliver {
			sb.WriteString(" (direct delivery)")
		} else {
			sb.WriteString(" (agent turn)")
		}
		if j.AtSeconds != nil {
			fmt.Fprintf(&sb, " at %ds", *j.AtSeconds)
		} else if j.EverySeconds != nil {
			fmt.Fprintf(&sb, " every %ds", *j.EverySeconds)
		} else if j.CronExpr != "" {
			fmt.Fprintf(&sb, " cron: %s", j.CronExpr)
			if j.Timezone != "" {
				fmt.Fprintf(&sb, " tz: %s", j.Timezone)
			}
		}
		if j.State.NextRunAt != nil {
			fmt.Fprintf(&sb, " next: %s", formatJobTime(*j.State.NextRunAt, j.Timezone))
		}
		if j.State.LastStatus != "" {
			fmt.Fprintf(&sb, " last: %s", j.State.LastStatus)
		}
		if j.State.LastError != "" {
			fmt.Fprintf(&sb, " error: %s", j.State.LastError)
		}
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatJobTime(t time.Time, timezone string) string {
	if timezone != "" {
		if loc, err := time.LoadLocation(timezone); err == nil {
			return t.In(loc).Format(time.RFC3339)
		}
	}
	return t.UTC().Format(time.RFC3339)
}
