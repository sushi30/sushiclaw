package cron

import (
	"fmt"
	"strings"
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
		}
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n")
}
