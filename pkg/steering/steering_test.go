package steering_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/pkg/steering"
)

func TestPushDequeue(t *testing.T) {
	q := steering.NewQueue()
	require.NoError(t, q.Push("sess1", "hello"))
	require.NoError(t, q.Push("sess1", "world"))

	msgs := q.Dequeue("sess1")
	assert.Len(t, msgs, 2)
	assert.Equal(t, "hello", msgs[0].Content)
	assert.Equal(t, "world", msgs[1].Content)

	// After dequeue, scope is empty
	assert.Empty(t, q.Dequeue("sess1"))
}

func TestMaxSizeReturnsError(t *testing.T) {
	q := steering.NewQueue()
	for i := range 10 {
		require.NoError(t, q.Push("scope", "msg"), "push %d should succeed", i)
	}
	err := q.Push("scope", "overflow")
	assert.ErrorIs(t, err, steering.ErrQueueFull)
}

func TestScopeIsolation(t *testing.T) {
	q := steering.NewQueue()
	require.NoError(t, q.Push("a", "msg-a"))
	require.NoError(t, q.Push("b", "msg-b"))

	msgsA := q.Dequeue("a")
	assert.Len(t, msgsA, 1)
	assert.Equal(t, "msg-a", msgsA[0].Content)

	msgsB := q.Dequeue("b")
	assert.Len(t, msgsB, 1)
	assert.Equal(t, "msg-b", msgsB[0].Content)
}
