package message

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/tools/toolctx"
)

const defaultMinInterval = 5 * time.Second

// Tool sends user-facing progress updates to the current chat via the message bus.
type Tool struct {
	bus         *bus.MessageBus
	minInterval time.Duration
	lastSent    map[string]time.Time
	mu          sync.RWMutex
}

// NewTool creates a new message tool.
// If minIntervalSeconds <= 0, a 5-second default is used.
func NewTool(b *bus.MessageBus, minIntervalSeconds int) *Tool {
	interval := time.Duration(minIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = defaultMinInterval
	}
	return &Tool{
		bus:         b,
		minInterval: interval,
		lastSent:    make(map[string]time.Time),
	}
}

func (t *Tool) Name() string        { return "message_tool" }
func (t *Tool) Description() string { return description }

func (t *Tool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"content": {
			Type:        "string",
			Description: "The message to send to the user. Keep it concise and human-readable.",
			Required:    true,
		},
	}
}

// Run executes the tool with the given input string.
func (t *Tool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

// Execute parses args and sends the message to the current chat.
func (t *Tool) Execute(ctx context.Context, args string) (string, error) {
	channel := toolctx.ChannelFromContext(ctx)
	chatID := toolctx.ChatIDFromContext(ctx)

	if channel == "" || chatID == "" {
		return "", fmt.Errorf("message_tool: missing chat context (no active conversation)")
	}

	if channel == config.ChannelEmail {
		return "", fmt.Errorf("message_tool: not available for email channel")
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(args), &req); err != nil {
		return "", fmt.Errorf("message_tool: invalid arguments: %w", err)
	}
	if req.Content == "" {
		return "", fmt.Errorf("message_tool: content is required")
	}

	key := channel + ":" + chatID
	t.mu.Lock()
	last := t.lastSent[key]
	if time.Since(last) < t.minInterval {
		remaining := t.minInterval - time.Since(last)
		t.mu.Unlock()
		return "", fmt.Errorf("message_tool: please wait %v before sending another update", remaining.Round(time.Second))
	}
	t.lastSent[key] = time.Now()
	t.mu.Unlock()

	msg := bus.OutboundMessage{
		Channel: channel,
		ChatID:  chatID,
		Context: bus.NewOutboundContext(channel, chatID, ""),
		Content: req.Content,
	}

	if err := t.bus.PublishOutbound(ctx, msg); err != nil {
		return "", fmt.Errorf("message_tool: failed to send message: %w", err)
	}

	return "Message sent.", nil
}

const description = `Send a short progress update to the user in the current chat.

Use this tool when:
- The user asked to be kept updated during a long-running task.
- You have reached a meaningful milestone and want to share progress.
- You are blocked and need to inform the user before continuing.

Do not overuse this tool. Wait for meaningful progress before sending an update.
This tool is not available for email conversations.`
