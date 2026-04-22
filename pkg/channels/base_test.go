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
)

// fakeChannel is a minimal Channel implementation for testing.
type fakeChannel struct {
	*channels.BaseChannel
}

func (f *fakeChannel) Start(_ context.Context) error { f.SetRunning(true); return nil }
func (f *fakeChannel) Stop(_ context.Context) error  { f.SetRunning(false); return nil }
func (f *fakeChannel) Send(_ context.Context, _ bus.OutboundMessage) ([]string, error) {
	return nil, nil
}

func newFake(t *testing.T, allowList []string, opts ...channels.BaseChannelOption) (*fakeChannel, *bus.MessageBus) {
	t.Helper()
	mb := bus.NewMessageBus()
	t.Cleanup(mb.Close)
	bc := channels.NewBaseChannel("test", nil, mb, allowList, opts...)
	ch := &fakeChannel{BaseChannel: bc}
	bc.SetOwner(ch)
	return ch, mb
}

func TestBaseChannelHandleMessagePublishesToBus(t *testing.T) {
	ch, mb := newFake(t, []string{"*"})

	ctx := context.Background()
	ch.HandleMessageWithContext(ctx, "chat1", "hello", nil, bus.InboundContext{
		Channel:  "test",
		ChatID:   "chat1",
		SenderID: "user1",
	})

	select {
	case msg := <-mb.InboundChan():
		assert.Equal(t, "hello", msg.Content)
		assert.Equal(t, "chat1", msg.ChatID)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for inbound message")
	}
}

func TestAllowList(t *testing.T) {
	ch, _ := newFake(t, []string{"allowed123"})
	assert.True(t, ch.IsAllowed("allowed123"))
	assert.False(t, ch.IsAllowed("blocked456"))
}

func TestWildcardAllowList(t *testing.T) {
	ch, _ := newFake(t, []string{"*"})
	assert.True(t, ch.IsAllowed("anyone"))
}

func TestFactoryRegistration(t *testing.T) {
	channels.RegisterFactory("__test_factory__", func(name, _ string, _ *config.Config, _ *bus.MessageBus) (channels.Channel, error) {
		mb := bus.NewMessageBus()
		bc := channels.NewBaseChannel(name, nil, mb, []string{"*"})
		return &fakeChannel{BaseChannel: bc}, nil
	})

	names := channels.GetRegisteredFactoryNames()
	found := false
	for _, n := range names {
		if n == "__test_factory__" {
			found = true
			break
		}
	}
	require.True(t, found, "factory should be registered")
}
