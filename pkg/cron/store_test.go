package cron

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStoreRoundtrip(t *testing.T) {
	tmp := t.TempDir()
	store := NewStore(filepath.Join(tmp, "cron", "jobs.json"))

	jobs, err := store.Load()
	require.NoError(t, err)
	require.Empty(t, jobs)

	every := 300
	jobs = []Job{
		{
			Name:         "test-job",
			Message:      "hello",
			Channel:      "telegram",
			ChatID:       "123",
			SenderID:     "user1",
			EverySeconds: &every,
			Enabled:      true,
			CreatedAt:    time.Now().UTC().Truncate(time.Second),
		},
	}
	require.NoError(t, store.Save(jobs))

	loaded, err := store.Load()
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	require.Equal(t, "test-job", loaded[0].Name)
	require.Equal(t, "hello", loaded[0].Message)
	require.Equal(t, "telegram", loaded[0].Channel)
	require.Equal(t, 300, *loaded[0].EverySeconds)
	require.True(t, loaded[0].Enabled)
}

func TestStoreAtomicWrite(t *testing.T) {
	tmp := t.TempDir()
	store := NewStore(filepath.Join(tmp, "jobs.json"))

	require.NoError(t, store.Save([]Job{{Name: "a"}}))

	stat, err := os.Stat(store.path)
	require.NoError(t, err)
	require.False(t, stat.IsDir())
}
