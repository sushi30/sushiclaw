package bus_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/pkg/bus"
)

func TestMessageBus_OutboundMedia(t *testing.T) {
	b := bus.NewMessageBus()
	defer b.Close()

	ctx := context.Background()
	msg := bus.OutboundMediaMessage{
		Channel: "telegram",
		ChatID:  "123",
		Parts:   []bus.MediaPart{{Type: "image", Ref: "ref1"}},
	}

	err := b.PublishOutboundMedia(ctx, msg)
	require.NoError(t, err)

	select {
	case got := <-b.OutboundMediaChan():
		assert.Equal(t, "telegram", got.Channel)
		assert.Len(t, got.Parts, 1)
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestMessageBus_AudioChunk(t *testing.T) {
	b := bus.NewMessageBus()
	defer b.Close()

	ctx := context.Background()
	chunk := bus.AudioChunk{SessionID: "sess1", Data: []byte{1, 2, 3}}

	err := b.PublishAudioChunk(ctx, chunk)
	require.NoError(t, err)

	select {
	case got := <-b.AudioChunksChan():
		assert.Equal(t, "sess1", got.SessionID)
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestMessageBus_VoiceControl(t *testing.T) {
	b := bus.NewMessageBus()
	defer b.Close()

	ctx := context.Background()
	vc := bus.VoiceControl{SessionID: "sess1", Action: "start"}

	err := b.PublishVoiceControl(ctx, vc)
	require.NoError(t, err)

	select {
	case got := <-b.VoiceControlsChan():
		assert.Equal(t, "start", got.Action)
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestMessageBus_SetStreamDelegate(t *testing.T) {
	b := bus.NewMessageBus()
	defer b.Close()

	delegate := &mockStreamDelegate{}
	b.SetStreamDelegate(delegate)

	streamer, ok := b.GetStreamer(context.Background(), "telegram", "123")
	require.True(t, ok)
	assert.NotNil(t, streamer)
}

func TestMessageBus_GetStreamer_NoDelegate(t *testing.T) {
	b := bus.NewMessageBus()
	defer b.Close()

	_, ok := b.GetStreamer(context.Background(), "telegram", "123")
	assert.False(t, ok)
}

func TestMessageBus_Close(t *testing.T) {
	b := bus.NewMessageBus()
	b.Close()

	// Second close should not panic
	b.Close()
}

func TestMessageBus_MissingContext(t *testing.T) {
	b := bus.NewMessageBus()
	defer b.Close()

	ctx := context.Background()
	// Inbound without context
	err := b.PublishInbound(ctx, bus.InboundMessage{Content: "test"})
	assert.Error(t, err)

	// Outbound without context
	err = b.PublishOutbound(ctx, bus.OutboundMessage{Content: "test"})
	assert.Error(t, err)
}

type mockStreamDelegate struct{}

func (m *mockStreamDelegate) GetStreamer(ctx context.Context, channel, chatID string) (bus.Streamer, bool) {
	return &mockStreamer{}, true
}

type mockStreamer struct{}

func (m *mockStreamer) Update(ctx context.Context, content string) error   { return nil }
func (m *mockStreamer) Finalize(ctx context.Context, content string) error { return nil }
func (m *mockStreamer) Cancel(ctx context.Context)                         {}
