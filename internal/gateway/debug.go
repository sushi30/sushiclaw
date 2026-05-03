package gateway

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sushi30/sushiclaw/internal/agent"
	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/logger"
)

// DebugManager owns per-chat operational debug progress.
type DebugManager struct {
	mu        sync.RWMutex
	active    map[string]bool
	bus       *bus.MessageBus
	heartbeat time.Duration
}

// NewDebugManager creates a new DebugManager.
func NewDebugManager(messageBus *bus.MessageBus, heartbeat ...time.Duration) *DebugManager {
	interval := agent.DefaultDebugHeartbeatInterval
	if len(heartbeat) > 0 && heartbeat[0] > 0 {
		interval = heartbeat[0]
	}
	return &DebugManager{
		active:    make(map[string]bool),
		bus:       messageBus,
		heartbeat: interval,
	}
}

func (d *DebugManager) HeartbeatInterval() time.Duration {
	if d == nil || d.heartbeat <= 0 {
		return agent.DefaultDebugHeartbeatInterval
	}
	return d.heartbeat
}

// Set updates debug mode for one channel/chat pair and returns a user-facing status.
func (d *DebugManager) Set(ctx context.Context, channel, chatID, mode string) string {
	if d == nil {
		return "Debug toggle unavailable."
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "", "toggle":
		if d.isActive(channel, chatID) {
			return d.disable(channel, chatID)
		}
		return d.enable(channel, chatID)
	case "on":
		if d.isActive(channel, chatID) {
			return "Debug mode already enabled."
		}
		return d.enable(channel, chatID)
	case "off":
		if !d.isActive(channel, chatID) {
			return "Debug mode already disabled."
		}
		return d.disable(channel, chatID)
	default:
		return "Usage: /debug [on|off]"
	}
}

// Toggle flips debug mode and returns a status string to send back to the user.
func (d *DebugManager) Toggle(ctx context.Context, channel, chatID string) string {
	return d.Set(ctx, channel, chatID, "toggle")
}

func (d *DebugManager) Progress(ctx context.Context, event agent.ProgressEvent) {
	if d == nil || !d.isActive(event.Channel, event.ChatID) {
		return
	}
	d.publish(ctx, event.Channel, event.ChatID, debugProgressText(event))
}

func (d *DebugManager) Summary(ctx context.Context, summary agent.ProgressSummary) {
	if d == nil || !d.isActive(summary.Channel, summary.ChatID) {
		return
	}
	d.publish(ctx, summary.Channel, summary.ChatID, debugSummaryText(summary))
}

func (d *DebugManager) enable(channel, chatID string) string {
	d.mu.Lock()
	d.active[stateKey(channel, chatID)] = true
	d.mu.Unlock()
	logger.InfoCF("debug", "Debug mode enabled", map[string]any{"channel": channel, "chat_id": chatID})
	return "Debug mode enabled."
}

func (d *DebugManager) disable(channel, chatID string) string {
	d.mu.Lock()
	delete(d.active, stateKey(channel, chatID))
	d.mu.Unlock()
	logger.InfoCF("debug", "Debug mode disabled", map[string]any{"channel": channel, "chat_id": chatID})
	return "Debug mode disabled."
}

func (d *DebugManager) isActive(channel, chatID string) bool {
	if d == nil {
		return false
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.active[stateKey(channel, chatID)]
}

func (d *DebugManager) publish(ctx context.Context, channel, chatID, content string) {
	if d.bus == nil || strings.TrimSpace(content) == "" {
		return
	}
	if err := d.bus.PublishOutbound(ctx, bus.OutboundMessage{
		Channel: channel,
		ChatID:  chatID,
		Content: content,
	}); err != nil {
		logger.WarnCF("debug", "Failed to publish debug message", map[string]any{
			"channel": channel,
			"chat_id": chatID,
			"error":   err.Error(),
		})
	}
}

func stateKey(channel, chatID string) string {
	return strings.TrimSpace(channel) + "\x00" + strings.TrimSpace(chatID)
}

func debugProgressText(event agent.ProgressEvent) string {
	switch event.Kind {
	case agent.ProgressTurnStarted:
		return "[debug] Turn started."
	case agent.ProgressFirstActivity:
		return "[debug] Model activity started."
	case agent.ProgressToolCallStarted:
		return fmt.Sprintf("[debug] Tool call started: %s", event.ToolName)
	case agent.ProgressToolCallFinished:
		return fmt.Sprintf("[debug] Tool call completed: %s", event.ToolName)
	case agent.ProgressFallback:
		if event.Error != nil {
			return fmt.Sprintf("[debug] Streaming unavailable, falling back to detailed run: %v", event.Error)
		}
		return "[debug] Streaming unavailable, falling back to detailed run."
	case agent.ProgressHeartbeat:
		return fmt.Sprintf("[debug] Still working. No agent events for %s.", formatDuration(event.Elapsed))
	case agent.ProgressCompleted:
		return "[debug] Turn completed."
	case agent.ProgressFailed:
		if event.Error != nil {
			return fmt.Sprintf("[debug] Turn failed: %v", event.Error)
		}
		return "[debug] Turn failed."
	default:
		return ""
	}
}

func debugSummaryText(summary agent.ProgressSummary) string {
	var sb strings.Builder
	if summary.Success {
		sb.WriteString("[debug] Summary")
	} else {
		sb.WriteString("[debug] Failure summary")
	}
	if summary.Error != nil {
		sb.WriteString("\nError: ")
		sb.WriteString(summary.Error.Error())
	}
	fmt.Fprintf(&sb, "\nTool calls: %d", summary.ToolCalls)
	if summary.Usage != nil {
		fmt.Fprintf(&sb, "\nTokens: total=%d input=%d output=%d",
			summary.Usage.TotalTokens,
			summary.Usage.InputTokens,
			summary.Usage.OutputTokens,
		)
	} else {
		sb.WriteString("\nTokens: unavailable")
	}
	sb.WriteString("\nTask time: ")
	sb.WriteString(formatDuration(summary.Duration))
	fmt.Fprintf(&sb, "\nResponse size: %d bytes", summary.ResponseBytes)
	return sb.String()
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}
	return d.Round(time.Second).String()
}
