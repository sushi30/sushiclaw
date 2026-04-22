package agent_test

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/internal/agent"
)

func TestInMemoryMemory_AddAndGet(t *testing.T) {
	mem := agent.NewInMemoryMemory()
	ctx := context.Background()

	err := mem.AddMessage(ctx, interfaces.Message{Role: interfaces.MessageRoleUser, Content: "hello"})
	require.NoError(t, err)

	err = mem.AddMessage(ctx, interfaces.Message{Role: interfaces.MessageRoleAssistant, Content: "hi"})
	require.NoError(t, err)

	msgs, err := mem.GetMessages(ctx)
	require.NoError(t, err)
	assert.Len(t, msgs, 2)
	assert.Equal(t, "hello", msgs[0].Content)
	assert.Equal(t, "hi", msgs[1].Content)
}

func TestInMemoryMemory_GetWithLimit(t *testing.T) {
	mem := agent.NewInMemoryMemory()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_ = mem.AddMessage(ctx, interfaces.Message{Role: interfaces.MessageRoleUser, Content: "msg"})
	}

	msgs, err := mem.GetMessages(ctx, interfaces.WithLimit(2))
	require.NoError(t, err)
	assert.Len(t, msgs, 2)
}

func TestInMemoryMemory_GetWithRoles(t *testing.T) {
	mem := agent.NewInMemoryMemory()
	ctx := context.Background()

	_ = mem.AddMessage(ctx, interfaces.Message{Role: interfaces.MessageRoleUser, Content: "user msg"})
	_ = mem.AddMessage(ctx, interfaces.Message{Role: interfaces.MessageRoleAssistant, Content: "assistant msg"})

	msgs, err := mem.GetMessages(ctx, interfaces.WithRoles(string(interfaces.MessageRoleUser)))
	require.NoError(t, err)
	assert.Len(t, msgs, 1)
	assert.Equal(t, "user msg", msgs[0].Content)
}

func TestInMemoryMemory_Clear(t *testing.T) {
	mem := agent.NewInMemoryMemory()
	ctx := context.Background()

	_ = mem.AddMessage(ctx, interfaces.Message{Role: interfaces.MessageRoleUser, Content: "hello"})
	_ = mem.Clear(ctx)

	msgs, err := mem.GetMessages(ctx)
	require.NoError(t, err)
	assert.Len(t, msgs, 0)
}
