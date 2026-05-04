package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveActiveSessionKeyDefaultsToStableKey(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sessions.db")

	store, err := NewSQLiteSessionMemory(ctx, dbPath, "telegram:chat1")
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	activeKey, err := resolveActiveSessionKey(ctx, store.db, "telegram:chat1")
	require.NoError(t, err)
	assert.Equal(t, "telegram:chat1", activeKey)
}

func TestSetActiveSessionKeyPersistsHeadAndLineage(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sessions.db")

	store, err := NewSQLiteSessionMemory(ctx, dbPath, "telegram:chat1")
	require.NoError(t, err)
	require.NoError(t, setActiveSessionKey(ctx, store.db, "telegram:chat1", "telegram:chat1#summary-1"))
	require.NoError(t, store.Close())

	reopened, err := NewSQLiteSessionMemory(ctx, dbPath, "telegram:chat1")
	require.NoError(t, err)
	defer func() { _ = reopened.Close() }()

	activeKey, err := resolveActiveSessionKey(ctx, reopened.db, "telegram:chat1")
	require.NoError(t, err)
	assert.Equal(t, "telegram:chat1#summary-1", activeKey)

	lineage, err := listSessionLineageKeys(ctx, reopened.db, "telegram:chat1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"telegram:chat1", "telegram:chat1#summary-1"}, lineage)
}

func TestClearSessionLineageRemovesHeadAndTrackedBackingKeys(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sessions.db")

	store, err := NewSQLiteSessionMemory(ctx, dbPath, "telegram:chat1")
	require.NoError(t, err)
	require.NoError(t, ensureSessionLineage(ctx, store.db, "telegram:chat1", "telegram:chat1"))
	require.NoError(t, ensureSessionLineage(ctx, store.db, "telegram:chat1", "telegram:chat1#summary-1"))
	require.NoError(t, setActiveSessionKey(ctx, store.db, "telegram:chat1", "telegram:chat1#summary-1"))

	require.NoError(t, clearSessionLineage(ctx, store.db, "telegram:chat1"))

	activeKey, err := resolveActiveSessionKey(ctx, store.db, "telegram:chat1")
	require.NoError(t, err)
	assert.Equal(t, "telegram:chat1", activeKey)

	lineage, err := listSessionLineageKeys(ctx, store.db, "telegram:chat1")
	require.NoError(t, err)
	assert.Equal(t, []string{"telegram:chat1"}, lineage)
}
