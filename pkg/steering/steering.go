package steering

import (
	"errors"
	"sync"
)

// ErrQueueFull is returned when a scope's queue exceeds maxQueueSize.
var ErrQueueFull = errors.New("steering queue full")

const maxQueueSize = 10

// Message is a queued steering message.
type Message struct {
	Content string
}

// Queue is a per-scope FIFO steering queue, safe for concurrent use.
type Queue struct {
	mu    sync.Mutex
	items map[string][]Message
}

// NewQueue creates an empty Queue.
func NewQueue() *Queue {
	return &Queue{items: make(map[string][]Message)}
}

// Push adds a message to the given scope's queue.
// Returns ErrQueueFull if the scope already has maxQueueSize messages.
func (q *Queue) Push(scope, content string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items[scope]) >= maxQueueSize {
		return ErrQueueFull
	}
	q.items[scope] = append(q.items[scope], Message{Content: content})
	return nil
}

// Dequeue removes and returns all messages for the given scope.
func (q *Queue) Dequeue(scope string) []Message {
	q.mu.Lock()
	defer q.mu.Unlock()
	msgs := q.items[scope]
	delete(q.items, scope)
	return msgs
}

// Len returns the number of queued messages for the given scope.
func (q *Queue) Len(scope string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items[scope])
}
