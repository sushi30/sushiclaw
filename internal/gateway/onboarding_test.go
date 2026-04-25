package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/sushi30/sushiclaw/internal/commandfilter"
	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/commands"
	"github.com/sushi30/sushiclaw/pkg/config"
)

func TestInboundRouter_AutoOnboardsFirstQualifyingMessage(t *testing.T) {
	messageBus := bus.NewMessageBus()
	defer messageBus.Close()

	dispatched := false
	router := newTestInboundRouter(messageBus, true, func(context.Context, bus.InboundMessage) {
		dispatched = true
	})

	router.handleMessage(context.Background(), bus.InboundMessage{
		Channel: config.ChannelTelegram,
		ChatID:  "123",
		Content: "hello there",
		Context: bus.InboundContext{
			Channel:  config.ChannelTelegram,
			ChatID:   "123",
			ChatType: "direct",
		},
	})

	msg := mustReadOutbound(t, messageBus)
	if msg.Content != commands.OnboardingMessage() {
		t.Fatalf("got onboarding %q, want shared onboarding message", msg.Content)
	}
	if dispatched {
		t.Fatal("expected first qualifying message to be consumed before agent dispatch")
	}
}

func TestInboundRouter_AutoOnboardsOnlyOncePerProcess(t *testing.T) {
	messageBus := bus.NewMessageBus()
	defer messageBus.Close()

	dispatches := 0
	router := newTestInboundRouter(messageBus, true, func(context.Context, bus.InboundMessage) {
		dispatches++
	})

	first := bus.InboundMessage{
		Channel: config.ChannelTelegram,
		ChatID:  "123",
		Content: "hello",
		Context: bus.InboundContext{
			Channel:  config.ChannelTelegram,
			ChatID:   "123",
			ChatType: "direct",
		},
	}
	second := bus.InboundMessage{
		Channel: config.ChannelWhatsAppNative,
		ChatID:  "456",
		Content: "hello again",
		Context: bus.InboundContext{
			Channel:  config.ChannelWhatsAppNative,
			ChatID:   "456",
			ChatType: "direct",
		},
	}

	router.handleMessage(context.Background(), first)
	_ = mustReadOutbound(t, messageBus)

	router.handleMessage(context.Background(), second)
	assertNoOutbound(t, messageBus)
	if dispatches != 1 {
		t.Fatalf("dispatches = %d, want 1", dispatches)
	}
}

func TestInboundRouter_DoesNotAutoOnboardEmailOrGroupMessages(t *testing.T) {
	tests := []struct {
		name string
		msg  bus.InboundMessage
	}{
		{
			name: "email direct",
			msg: bus.InboundMessage{
				Channel: config.ChannelEmail,
				ChatID:  "person@example.com",
				Content: "hello",
				Context: bus.InboundContext{
					Channel:  config.ChannelEmail,
					ChatID:   "person@example.com",
					ChatType: "direct",
				},
			},
		},
		{
			name: "telegram group",
			msg: bus.InboundMessage{
				Channel: config.ChannelTelegram,
				ChatID:  "-100123",
				Content: "hello",
				Context: bus.InboundContext{
					Channel:  config.ChannelTelegram,
					ChatID:   "-100123",
					ChatType: "group",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			messageBus := bus.NewMessageBus()
			defer messageBus.Close()

			dispatches := 0
			router := newTestInboundRouter(messageBus, true, func(context.Context, bus.InboundMessage) {
				dispatches++
			})

			router.handleMessage(context.Background(), tc.msg)

			assertNoOutbound(t, messageBus)
			if dispatches != 1 {
				t.Fatalf("dispatches = %d, want 1", dispatches)
			}
		})
	}
}

func TestInboundRouter_UnknownCommandStaysBlocked(t *testing.T) {
	messageBus := bus.NewMessageBus()
	defer messageBus.Close()

	dispatched := false
	router := newTestInboundRouter(messageBus, true, func(context.Context, bus.InboundMessage) {
		dispatched = true
	})

	router.handleMessage(context.Background(), bus.InboundMessage{
		Channel: config.ChannelTelegram,
		ChatID:  "123",
		Content: "/nope",
		Context: bus.InboundContext{
			Channel:  config.ChannelTelegram,
			ChatID:   "123",
			ChatType: "direct",
		},
	})

	msg := mustReadOutbound(t, messageBus)
	if msg.Content != "Unknown command: /nope" {
		t.Fatalf("got %q, want unknown-command reply", msg.Content)
	}
	if dispatched {
		t.Fatal("expected blocked unknown command not to dispatch to agent")
	}
}

func TestInboundRouter_WelcomeCommandWorksWhenAutoOnboardingDisabled(t *testing.T) {
	messageBus := bus.NewMessageBus()
	defer messageBus.Close()

	dispatched := false
	router := newTestInboundRouter(messageBus, false, func(context.Context, bus.InboundMessage) {
		dispatched = true
	})

	router.handleMessage(context.Background(), bus.InboundMessage{
		Channel: config.ChannelTelegram,
		ChatID:  "123",
		Content: "/welcome",
		Context: bus.InboundContext{
			Channel:  config.ChannelTelegram,
			ChatID:   "123",
			ChatType: "direct",
		},
	})

	msg := mustReadOutbound(t, messageBus)
	if msg.Content != commands.OnboardingMessage() {
		t.Fatalf("got welcome reply %q, want shared onboarding message", msg.Content)
	}
	if dispatched {
		t.Fatal("expected /welcome to be handled locally")
	}
}

func newTestInboundRouter(messageBus *bus.MessageBus, autoEnabled bool, dispatch func(context.Context, bus.InboundMessage)) *inboundRouter {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	rt := &commands.Runtime{ListDefinitions: reg.Definitions}
	return &inboundRouter{
		messageBus:        messageBus,
		cmdFilter:         commandfilter.NewCommandFilter(),
		executor:          commands.NewExecutor(reg, rt),
		sessionDispatcher: dispatch,
		autoOnboarding:    newOnboardingState(config.OnboardingConfig{Auto: config.AutoOnboardingConfig{Enabled: autoEnabled}}),
	}
}

func mustReadOutbound(t *testing.T, messageBus *bus.MessageBus) bus.OutboundMessage {
	t.Helper()
	select {
	case msg := <-messageBus.OutboundChan():
		return msg
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for outbound message")
		return bus.OutboundMessage{}
	}
}

func assertNoOutbound(t *testing.T, messageBus *bus.MessageBus) {
	t.Helper()
	select {
	case msg := <-messageBus.OutboundChan():
		t.Fatalf("unexpected outbound message: %q", msg.Content)
	default:
	}
}
