package gateway

import (
	"context"
	"sync"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/logger"
)

// DebugManager toggles per-chat debug event forwarding.
// Currently stubbed: agent-sdk-go integration does not yet expose event streaming.
type DebugManager struct {
	mu      sync.Mutex
	active  bool
	cancel  context.CancelFunc
	bus     *bus.MessageBus
	channel string
	chatID  string
}

// NewDebugManager creates a new DebugManager.
func NewDebugManager(messageBus *bus.MessageBus) *DebugManager {
	return &DebugManager{bus: messageBus}
}

// Toggle flips debug mode and returns a status string to send back to the user.
func (d *DebugManager) Toggle(ctx context.Context, channel, chatID string) string {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.active {
		if d.cancel != nil {
			d.cancel()
		}
		d.cancel = nil
		d.active = false
		d.channel = ""
		d.chatID = ""
		logger.InfoCF("debug", "Debug mode disabled", map[string]any{"chat_id": chatID})
		return "Debug mode disabled."
	}

	d.channel = channel
	d.chatID = chatID
	d.active = true

	logger.InfoCF("debug", "Debug mode enabled", map[string]any{"chat_id": chatID})
	return "Debug mode enabled. (Event forwarding not yet implemented with agent-sdk-go.)"
}
