package channels

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/logger"
	"github.com/sushi30/sushiclaw/pkg/media"
)

// ErrSendFailed is a sentinel error channels can return to indicate a send failure.
var ErrSendFailed = errors.New("channel send failed")

// Manager owns the channel set and dispatches outbound messages from the bus.
type Manager struct {
	mu          sync.RWMutex
	channels    map[string]Channel
	bus         *bus.MessageBus
	mediaStore  media.MediaStore

	placeholders  sync.Map // "channel:chatID" → placeholderID (string)
	typingStops   sync.Map // "channel:chatID" → func()
	reactionUndos sync.Map // "channel:chatID" → func()
	streamActive  sync.Map // "channel:chatID" → bool

	stopDispatch context.CancelFunc
	dispatchDone chan struct{}
}

// NewManager creates a Manager, creates channels from config factories, and sets up media/placeholders.
func NewManager(cfg *config.Config, messageBus *bus.MessageBus, ms media.MediaStore) (*Manager, error) {
	m := &Manager{
		channels:    make(map[string]Channel),
		bus:         messageBus,
		mediaStore:  ms,
		dispatchDone: make(chan struct{}),
	}

	for name, chCfg := range cfg.Channels {
		if !chCfg.Enabled {
			continue
		}
		chType := chCfg.Type
		if chType == "" {
			chType = name // fall back to name as type
		}
		factory, ok := getFactory(chType)
		if !ok {
			logger.WarnCF("channels", "No factory registered for channel type", map[string]any{
				"name": name,
				"type": chType,
			})
			continue
		}
		ch, err := factory(name, chType, cfg, messageBus)
		if err != nil {
			return nil, err
		}
		m.injectChannel(name, ch, ms)
		m.channels[name] = ch
	}

	return m, nil
}

// RegisterChannel adds an externally constructed channel (e.g. email).
func (m *Manager) RegisterChannel(name string, ch Channel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.injectChannel(name, ch, m.mediaStore)
	m.channels[name] = ch
}

func (m *Manager) injectChannel(name string, ch Channel, ms media.MediaStore) {
	if bc, ok := ch.(interface {
		SetMediaStore(media.MediaStore)
		SetPlaceholderRecorder(PlaceholderRecorder)
		SetOwner(Channel)
	}); ok {
		bc.SetMediaStore(ms)
		bc.SetPlaceholderRecorder(m)
		bc.SetOwner(ch)
	}
	_ = name
}

// StartAll starts all registered channels and the outbound dispatch goroutine.
func (m *Manager) StartAll(ctx context.Context) error {
	m.mu.RLock()
	channels := make([]Channel, 0, len(m.channels))
	for _, ch := range m.channels {
		channels = append(channels, ch)
	}
	m.mu.RUnlock()

	for _, ch := range channels {
		if err := ch.Start(ctx); err != nil {
			return err
		}
	}

	dispatchCtx, cancel := context.WithCancel(context.Background())
	m.stopDispatch = cancel
	go m.runOutboundDispatch(dispatchCtx)
	return nil
}

// StopAll stops all channels and the dispatch goroutine.
func (m *Manager) StopAll(ctx context.Context) error {
	if m.stopDispatch != nil {
		m.stopDispatch()
	}
	select {
	case <-m.dispatchDone:
	case <-ctx.Done():
	}

	m.mu.RLock()
	channels := make([]Channel, 0, len(m.channels))
	for _, ch := range m.channels {
		channels = append(channels, ch)
	}
	m.mu.RUnlock()

	for _, ch := range channels {
		if err := ch.Stop(ctx); err != nil {
			logger.WarnCF("channels", "Error stopping channel", map[string]any{
				"channel": ch.Name(),
				"error":   err.Error(),
			})
		}
	}
	return nil
}

// GetEnabledChannels returns names of all registered channels.
func (m *Manager) GetEnabledChannels() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.channels))
	for name := range m.channels {
		names = append(names, name)
	}
	return names
}

// GetStreamer implements bus.StreamDelegate.
func (m *Manager) GetStreamer(ctx context.Context, channel, chatID string) (bus.Streamer, bool) {
	m.mu.RLock()
	ch, ok := m.channels[channel]
	m.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if sc, ok := ch.(StreamingCapable); ok {
		streamer, err := sc.BeginStream(ctx, chatID)
		if err != nil {
			return nil, false
		}
		m.streamActive.Store(stateKey(channel, chatID), true)
		return streamer, true
	}
	return nil, false
}

// PlaceholderRecorder implementation

// RecordPlaceholder stores a placeholder message ID for later editing.
func (m *Manager) RecordPlaceholder(channel, chatID, placeholderID string) {
	m.placeholders.Store(stateKey(channel, chatID), placeholderID)
}

// RecordTypingStop stores a typing stop function.
func (m *Manager) RecordTypingStop(channel, chatID string, stop func()) {
	m.typingStops.Store(stateKey(channel, chatID), stop)
}

// RecordReactionUndo stores a reaction undo function.
func (m *Manager) RecordReactionUndo(channel, chatID string, undo func()) {
	m.reactionUndos.Store(stateKey(channel, chatID), undo)
}

func (m *Manager) runOutboundDispatch(ctx context.Context) {
	defer close(m.dispatchDone)
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-m.bus.OutboundChan():
			if !ok {
				return
			}
			m.handleOutbound(ctx, msg)
		case msg, ok := <-m.bus.OutboundMediaChan():
			if !ok {
				return
			}
			m.handleOutboundMedia(ctx, msg)
		}
	}
}

func (m *Manager) handleOutbound(ctx context.Context, msg bus.OutboundMessage) {
	key := stateKey(msg.Channel, msg.ChatID)

	// Stop typing indicator
	if stopVal, ok := m.typingStops.LoadAndDelete(key); ok {
		if stop, ok := stopVal.(func()); ok {
			stop()
		}
	}

	// Remove reaction
	if undoVal, ok := m.reactionUndos.LoadAndDelete(key); ok {
		if undo, ok := undoVal.(func()); ok {
			undo()
		}
	}

	// Check if streaming took over — if so, skip placeholder edit
	streamActive := false
	if _, active := m.streamActive.LoadAndDelete(key); active {
		streamActive = true
	}

	m.mu.RLock()
	ch, ok := m.channels[msg.Channel]
	m.mu.RUnlock()
	if !ok {
		logger.WarnCF("channels", "No channel found for outbound message", map[string]any{
			"channel": msg.Channel,
		})
		return
	}

	// Edit placeholder if available and streaming didn't handle it
	if !streamActive {
		if phVal, ok := m.placeholders.LoadAndDelete(key); ok {
			if phID, ok := phVal.(string); ok && phID != "" {
				if editor, ok := ch.(MessageEditor); ok {
					if err := editor.EditMessage(ctx, msg.ChatID, phID, msg.Content); err == nil {
						return // successfully edited placeholder
					}
				}
			}
		}
	} else {
		m.placeholders.Delete(key)
	}

	// Normal send
	if _, err := ch.Send(ctx, msg); err != nil {
		logger.ErrorCF("channels", "Failed to send outbound message", map[string]any{
			"channel": msg.Channel,
			"chat_id": msg.ChatID,
			"error":   err.Error(),
		})
	}
}

func (m *Manager) handleOutboundMedia(ctx context.Context, msg bus.OutboundMediaMessage) {
	m.mu.RLock()
	ch, ok := m.channels[msg.Channel]
	m.mu.RUnlock()
	if !ok {
		return
	}
	type MediaSender interface {
		SendMedia(ctx context.Context, msg bus.OutboundMediaMessage) ([]string, error)
	}
	if ms, ok := ch.(MediaSender); ok {
		if _, err := ms.SendMedia(ctx, msg); err != nil {
			logger.ErrorCF("channels", "Failed to send media message", map[string]any{
				"channel": msg.Channel,
				"error":   err.Error(),
			})
		}
	}
}

func stateKey(channel, chatID string) string {
	return strings.Join([]string{channel, chatID}, ":")
}
