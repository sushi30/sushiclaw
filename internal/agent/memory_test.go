package agent_test

import (
	"context"
	"path/filepath"
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

func TestSQLiteMemory_PersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sessions.db")

	mem, err := agent.NewSQLiteSessionMemory(ctx, dbPath, "telegram:chat1")
	require.NoError(t, err)
	defer swallowError(mem.Close)

	err = mem.AddMessage(ctx, interfaces.Message{
		Role:       interfaces.MessageRoleAssistant,
		Content:    "using a tool",
		Metadata:   map[string]interface{}{"retry_count": 1},
		ToolCallID: "tool-call-result",
		ToolCalls: []interfaces.ToolCall{{
			ID:        "call-1",
			Name:      "exec",
			Arguments: `{"cmd":"date"}`,
		}},
	})
	require.NoError(t, err)
	require.NoError(t, mem.Close())

	reopened, err := agent.NewSQLiteSessionMemory(ctx, dbPath, "telegram:chat1")
	require.NoError(t, err)
	defer swallowError(reopened.Close)

	msgs, err := reopened.GetMessages(ctx)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, interfaces.MessageRoleAssistant, msgs[0].Role)
	assert.Equal(t, "using a tool", msgs[0].Content)
	assert.Equal(t, "tool-call-result", msgs[0].ToolCallID)
	require.Len(t, msgs[0].ToolCalls, 1)
	assert.Equal(t, "call-1", msgs[0].ToolCalls[0].ID)
	assert.Equal(t, float64(1), msgs[0].Metadata["retry_count"])
}

func TestSQLiteMemory_IsolatesSessionsAndFilters(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sessions.db")

	sessionA, err := agent.NewSQLiteSessionMemory(ctx, dbPath, "telegram:chat-a")
	require.NoError(t, err)
	defer swallowError(sessionA.Close)
	sessionB, err := agent.NewSQLiteSessionMemory(ctx, dbPath, "telegram:chat-b")
	require.NoError(t, err)
	defer swallowError(sessionB.Close)

	require.NoError(t, sessionA.AddMessage(ctx, interfaces.Message{Role: interfaces.MessageRoleUser, Content: "a1"}))
	require.NoError(t, sessionA.AddMessage(ctx, interfaces.Message{Role: interfaces.MessageRoleAssistant, Content: "a2"}))
	require.NoError(t, sessionB.AddMessage(ctx, interfaces.Message{Role: interfaces.MessageRoleUser, Content: "b1"}))

	userMsgs, err := sessionA.GetMessages(ctx, interfaces.WithRoles(string(interfaces.MessageRoleUser)))
	require.NoError(t, err)
	require.Len(t, userMsgs, 1)
	assert.Equal(t, "a1", userMsgs[0].Content)

	limited, err := sessionA.GetMessages(ctx, interfaces.WithLimit(1))
	require.NoError(t, err)
	require.Len(t, limited, 1)
	assert.Equal(t, "a2", limited[0].Content)

	msgsB, err := sessionB.GetMessages(ctx)
	require.NoError(t, err)
	require.Len(t, msgsB, 1)
	assert.Equal(t, "b1", msgsB[0].Content)
}

func TestSQLiteMemory_ClearDeletesPersistedRows(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sessions.db")

	mem, err := agent.NewSQLiteSessionMemory(ctx, dbPath, "telegram:chat1")
	require.NoError(t, err)
	require.NoError(t, mem.AddMessage(ctx, interfaces.Message{Role: interfaces.MessageRoleUser, Content: "hello"}))
	require.NoError(t, mem.Clear(ctx))
	require.NoError(t, mem.Close())

	reopened, err := agent.NewSQLiteSessionMemory(ctx, dbPath, "telegram:chat1")
	require.NoError(t, err)
	defer swallowError(reopened.Close)

	msgs, err := reopened.GetMessages(ctx)
	require.NoError(t, err)
	assert.Empty(t, msgs)
}

func swallowError(fn func() error) {
	_ = fn()
}
