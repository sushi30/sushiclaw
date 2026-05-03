package channels_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/channels"
	"github.com/sushi30/sushiclaw/pkg/config"
)

func TestBaseChannel_MaxMessageLength(t *testing.T) {
	ch, _ := newFake(t, []string{"*"}, channels.WithMaxMessageLength(100))
	assert.Equal(t, 100, ch.MaxMessageLength())
}

func TestBaseChannel_Name(t *testing.T) {
	ch, _ := newFake(t, []string{"*"})
	assert.Equal(t, "test", ch.Name())
	ch.SetName("renamed")
	assert.Equal(t, "renamed", ch.Name())
}

func TestBaseChannel_ReasoningChannelID(t *testing.T) {
	ch, _ := newFake(t, []string{"*"}, channels.WithReasoningChannelID("reasoning1"))
	assert.Equal(t, "reasoning1", ch.ReasoningChannelID())
}

func TestBaseChannel_IsRunning(t *testing.T) {
	ch, _ := newFake(t, []string{"*"})
	assert.False(t, ch.IsRunning())
	ch.SetRunning(true)
	assert.True(t, ch.IsRunning())
	ch.SetRunning(false)
	assert.False(t, ch.IsRunning())
}

func TestBaseChannel_IsAllowedSender(t *testing.T) {
	ch, _ := newFake(t, []string{"telegram:123"})
	sender := bus.SenderInfo{Platform: "telegram", PlatformID: "123", CanonicalID: "telegram:123"}
	assert.True(t, ch.IsAllowedSender(sender))

	sender2 := bus.SenderInfo{Platform: "telegram", PlatformID: "456"}
	assert.False(t, ch.IsAllowedSender(sender2))
}

func TestBaseChannel_ShouldRespondInGroup(t *testing.T) {
	ch, _ := newFake(t, []string{"*"})
	respond, _ := ch.ShouldRespondInGroup(true, "hello")
	assert.True(t, respond)

	// Default (no group trigger config) should respond
	respond, _ = ch.ShouldRespondInGroup(false, "hello")
	assert.True(t, respond)
}

func TestBaseChannel_BuildMediaScope(t *testing.T) {
	scope := channels.BuildMediaScope("telegram", "chat1", "msg1")
	assert.Equal(t, "telegram:chat1:msg1", scope)
}

func TestRegisterSafeFactory(t *testing.T) {
	channels.RegisterSafeFactory[config.TelegramSettings]("__test_safe__", func(bc *config.Channel, settings *config.TelegramSettings, b *bus.MessageBus) (channels.Channel, error) {
		return newFakeChannel("__test_safe__", b), nil
	})

	names := channels.GetRegisteredFactoryNames()
	found := false
	for _, n := range names {
		if n == "__test_safe__" {
			found = true
			break
		}
	}
	require.True(t, found, "safe factory should be registered")
}

func newFakeChannel(name string, b *bus.MessageBus) channels.Channel {
	bc := channels.NewBaseChannel(name, nil, b, []string{"*"})
	ch := &fakeChannel{BaseChannel: bc}
	bc.SetOwner(ch)
	return ch
}
