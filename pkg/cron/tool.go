package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/tools/toolctx"
)

// CronTool is the agent-facing tool for managing cron jobs.
type CronTool struct {
	scheduler *Scheduler
	cfg       *config.Config
}

// NewCronTool creates a new CronTool.
func NewCronTool(scheduler *Scheduler, cfg *config.Config) *CronTool {
	return &CronTool{
		scheduler: scheduler,
		cfg:       cfg,
	}
}

// Name returns the tool name.
func (t *CronTool) Name() string { return "cron" }

// Description returns the tool description.
func (t *CronTool) Description() string {
	return "Manage scheduled cron jobs. Actions: add, list, remove, enable, disable."
}

// Parameters returns the tool parameters.
func (t *CronTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"action": {
			Type:        "string",
			Description: "Action to perform: add, list, remove, enable, disable",
			Required:    true,
		},
		"name": {
			Type:        "string",
			Description: "Job name (required for add, remove, enable, disable)",
			Required:    false,
		},
		"message": {
			Type:        "string",
			Description: "Message content (required for add)",
			Required:    false,
		},
		"at_seconds": {
			Type:        "number",
			Description: "One-time delay in seconds",
			Required:    false,
		},
		"every_seconds": {
			Type:        "number",
			Description: "Recurring interval in seconds",
			Required:    false,
		},
		"cron_expr": {
			Type:        "string",
			Description: "Standard cron expression",
			Required:    false,
		},
		"deliver": {
			Type:        "boolean",
			Description: "Deliver directly without agent processing",
			Required:    false,
		},
		"command": {
			Type:        "string",
			Description: "Shell command to execute instead of sending a message",
			Required:    false,
		},
	}
}

// Run executes the tool.
func (t *CronTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

// Execute executes the tool with a JSON args string.
func (t *CronTool) Execute(ctx context.Context, args string) (string, error) {
	var params map[string]any
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		params = map[string]any{"action": strings.Trim(args, `" `)}
	}

	action, _ := params["action"].(string)
	action = strings.ToLower(action)

	switch action {
	case "add":
		return t.addJob(ctx, params)
	case "list":
		return t.listJobs()
	case "remove":
		return t.removeJob(params)
	case "enable":
		return t.enableJob(params)
	case "disable":
		return t.disableJob(params)
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (t *CronTool) addJob(ctx context.Context, params map[string]any) (string, error) {
	message, _ := params["message"].(string)
	if message == "" {
		return "", fmt.Errorf("message is required for add")
	}

	name, _ := params["name"].(string)
	if name == "" {
		name = fmt.Sprintf("job-%d", time.Now().Unix())
	}

	var atSeconds, everySeconds *int
	if v, ok := params["at_seconds"]; ok {
		switch val := v.(type) {
		case float64:
			i := int(val)
			atSeconds = &i
		case int:
			atSeconds = &val
		case string:
			i, err := strconv.Atoi(val)
			if err != nil {
				return "", fmt.Errorf("invalid at_seconds: %v", val)
			}
			atSeconds = &i
		}
	}
	if v, ok := params["every_seconds"]; ok {
		switch val := v.(type) {
		case float64:
			i := int(val)
			everySeconds = &i
		case int:
			everySeconds = &val
		case string:
			i, err := strconv.Atoi(val)
			if err != nil {
				return "", fmt.Errorf("invalid every_seconds: %v", val)
			}
			everySeconds = &i
		}
	}
	cronExpr, _ := params["cron_expr"].(string)

	if atSeconds == nil && everySeconds == nil && cronExpr == "" {
		return "", fmt.Errorf("one of at_seconds, every_seconds, or cron_expr is required")
	}

	deliver := false
	if v, ok := params["deliver"]; ok {
		switch val := v.(type) {
		case bool:
			deliver = val
		case string:
			deliver = strings.ToLower(val) == "true"
		}
	}

	command, _ := params["command"].(string)
	if command != "" && !t.cfg.Tools.Cron.AllowCommand {
		return "", fmt.Errorf("command jobs are not allowed")
	}

	job := Job{
		Name:         name,
		Message:      message,
		Channel:      toolctx.ChannelFromContext(ctx),
		ChatID:       toolctx.ChatIDFromContext(ctx),
		SenderID:     toolctx.SenderIDFromContext(ctx),
		AtSeconds:    atSeconds,
		EverySeconds: everySeconds,
		CronExpr:     cronExpr,
		Deliver:      deliver,
		Command:      command,
		Enabled:      true,
		CreatedAt:    time.Now(),
	}

	if err := t.scheduler.AddJob(job); err != nil {
		return "", err
	}

	return fmt.Sprintf("Job %q added successfully", name), nil
}

func (t *CronTool) listJobs() (string, error) {
	jobs, err := t.scheduler.ListJobs()
	if err != nil {
		return "", err
	}
	if len(jobs) == 0 {
		return "No cron jobs scheduled.", nil
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
	return strings.TrimRight(sb.String(), "\n"), nil
}

func (t *CronTool) removeJob(params map[string]any) (string, error) {
	name, _ := params["name"].(string)
	if name == "" {
		return "", fmt.Errorf("name is required for remove")
	}
	if err := t.scheduler.RemoveJob(name); err != nil {
		return "", err
	}
	return fmt.Sprintf("Job %q removed", name), nil
}

func (t *CronTool) enableJob(params map[string]any) (string, error) {
	name, _ := params["name"].(string)
	if name == "" {
		return "", fmt.Errorf("name is required for enable")
	}
	if err := t.scheduler.EnableJob(name); err != nil {
		return "", err
	}
	return fmt.Sprintf("Job %q enabled", name), nil
}

func (t *CronTool) disableJob(params map[string]any) (string, error) {
	name, _ := params["name"].(string)
	if name == "" {
		return "", fmt.Errorf("name is required for disable")
	}
	if err := t.scheduler.DisableJob(name); err != nil {
		return "", err
	}
	return fmt.Sprintf("Job %q disabled", name), nil
}
