package agent

import (
	"context"
	"sync"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// InMemoryMemory is a simple in-memory conversation store.
type InMemoryMemory struct {
	mu       sync.RWMutex
	messages []interfaces.Message
}

// NewInMemoryMemory creates a new in-memory memory store.
func NewInMemoryMemory() *InMemoryMemory {
	return &InMemoryMemory{
		messages: make([]interfaces.Message, 0),
	}
}

// AddMessage appends a message to memory.
func (m *InMemoryMemory) AddMessage(_ context.Context, message interfaces.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, message)
	return nil
}

// GetMessages returns all messages (or filtered by options).
func (m *InMemoryMemory) GetMessages(_ context.Context, options ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	opts := &interfaces.GetMessagesOptions{}
	for _, o := range options {
		o(opts)
	}

	result := make([]interfaces.Message, len(m.messages))
	copy(result, m.messages)

	// Apply role filter
	if len(opts.Roles) > 0 {
		var filtered []interfaces.Message
		roleSet := make(map[string]bool, len(opts.Roles))
		for _, r := range opts.Roles {
			roleSet[r] = true
		}
		for _, msg := range result {
			if roleSet[string(msg.Role)] {
				filtered = append(filtered, msg)
			}
		}
		result = filtered
	}

	// Apply limit
	if opts.Limit > 0 && opts.Limit < len(result) {
		result = result[len(result)-opts.Limit:]
	}

	return result, nil
}

// Clear removes all messages from memory.
func (m *InMemoryMemory) Clear(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = m.messages[:0]
	return nil
}
