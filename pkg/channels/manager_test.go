package channels_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/channels"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/media"
)

func TestManager_RegisterChannel(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()
	ms := media.NewFileMediaStore()

	m, err := channels.NewManager(&config.Config{}, mb, ms)
	require.NoError(t, err)

	ch, _ := newFake(t, []string{"*"})
	m.RegisterChannel("email", ch)

	enabled := m.GetEnabledChannels()
	found := false
	for _, name := range enabled {
		if name == "email" {
			found = true
			break
		}
	}
	assert.True(t, found, "email should be in enabled channels")
}

func TestManager_StartAll_StopAll(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()
	ms := media.NewFileMediaStore()

	m, err := channels.NewManager(&config.Config{}, mb, ms)
	require.NoError(t, err)

	ch, _ := newFake(t, []string{"*"})
	m.RegisterChannel("test", ch)

	ctx := context.Background()
	err = m.StartAll(ctx)
	require.NoError(t, err)
	assert.True(t, ch.IsRunning())

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err = m.StopAll(stopCtx)
	require.NoError(t, err)
	assert.False(t, ch.IsRunning())
}

func TestManager_PlaceholderRecorder(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()
	ms := media.NewFileMediaStore()

	m, err := channels.NewManager(&config.Config{}, mb, ms)
	require.NoError(t, err)

	// Should not panic
	m.RecordPlaceholder("telegram", "chat1", "ph1")
	m.RecordTypingStop("telegram", "chat1", func() {})
	m.RecordReactionUndo("telegram", "chat1", func() {})
}

func TestManager_GetStreamer(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()
	ms := media.NewFileMediaStore()

	m, err := channels.NewManager(&config.Config{}, mb, ms)
	require.NoError(t, err)

	mb.SetStreamDelegate(m)

	// Channel doesn't implement StreamingCapable, so should return false
	_, ok := mb.GetStreamer(context.Background(), "telegram", "123")
	assert.False(t, ok)
}
