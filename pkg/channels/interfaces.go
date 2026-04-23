package channels

import (
	"context"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/commands"
)

// TypingCapable channels can show a typing/thinking indicator.
type TypingCapable interface {
	StartTyping(ctx context.Context, chatID string) (stop func(), err error)
}

// MessageEditor channels can edit an existing message.
type MessageEditor interface {
	EditMessage(ctx context.Context, chatID string, messageID string, content string) error
}

// MessageDeleter channels can delete a message by ID.
type MessageDeleter interface {
	DeleteMessage(ctx context.Context, chatID string, messageID string) error
}

// ReactionCapable channels can add a reaction to an inbound message.
type ReactionCapable interface {
	ReactToMessage(ctx context.Context, chatID, messageID string) (undo func(), err error)
}

// PlaceholderCapable channels can send a placeholder message to be edited later.
type PlaceholderCapable interface {
	SendPlaceholder(ctx context.Context, chatID string) (messageID string, err error)
}

// StreamingCapable channels can show partial LLM output in real-time.
type StreamingCapable interface {
	BeginStream(ctx context.Context, chatID string) (Streamer, error)
}

// Streamer pushes incremental content to a streaming-capable channel.
// Aliased from bus to avoid circular imports.
type Streamer = bus.Streamer

// PlaceholderRecorder is injected into channels by Manager.
type PlaceholderRecorder interface {
	RecordPlaceholder(channel, chatID, placeholderID string)
	RecordTypingStop(channel, chatID string, stop func())
	RecordReactionUndo(channel, chatID string, undo func())
}

// CommandRegistrarCapable channels can register command menus with their platform.
type CommandRegistrarCapable interface {
	RegisterCommands(ctx context.Context, defs []commands.Definition) error
}
