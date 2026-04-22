package bus_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/pkg/bus"
)

func TestPublishReceiveInbound(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	ctx := context.Background()
	msg := bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "123",
		SenderID: "456",
		Content:  "hello",
	}

	err := mb.PublishInbound(ctx, msg)
	require.NoError(t, err)

	select {
	case got := <-mb.InboundChan():
		assert.Equal(t, "telegram", got.Channel)
		assert.Equal(t, "123", got.ChatID)
		assert.Equal(t, "hello", got.Content)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for inbound message")
	}
}

func TestPublishReceiveOutbound(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	ctx := context.Background()
	msg := bus.OutboundMessage{
		Channel: "telegram",
		ChatID:  "123",
		Content: "reply",
	}

	err := mb.PublishOutbound(ctx, msg)
	require.NoError(t, err)

	select {
	case got := <-mb.OutboundChan():
		assert.Equal(t, "telegram", got.Channel)
		assert.Equal(t, "reply", got.Content)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for outbound message")
	}
}

func TestCloseDropsPublish(t *testing.T) {
	mb := bus.NewMessageBus()
	mb.Close()

	ctx := context.Background()
	err := mb.PublishInbound(ctx, bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "123",
		SenderID: "456",
	})
	assert.ErrorIs(t, err, bus.ErrBusClosed)
}

func TestPublishInboundMissingContext(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	err := mb.PublishInbound(context.Background(), bus.InboundMessage{})
	assert.ErrorIs(t, err, bus.ErrMissingInboundContext)
}
