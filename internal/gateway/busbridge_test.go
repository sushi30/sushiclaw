package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/internal/commandfilter"
)

func TestBusBridge_UnknownCommandBlocked(t *testing.T) {
	realBus := bus.NewMessageBus()
	agentBus := bus.NewMessageBus()
	filter := commandfilter.NewCommandFilter()
	bridge := NewBusBridge(realBus, agentBus, filter)
	bridge.Start()
	defer bridge.Stop()

	ctx := context.Background()

	err := realBus.PublishInbound(ctx, bus.InboundMessage{
		Channel: "telegram",
		ChatID:  "123",
		Content: "/nonexistent",
	})
	if err != nil {
		t.Fatalf("PublishInbound: %v", err)
	}

	select {
	case msg := <-realBus.OutboundChan():
		if msg.Content != "Unknown command: /nonexistent" {
			t.Errorf("expected error message, got %q", msg.Content)
		}
		if msg.Channel != "telegram" || msg.ChatID != "123" {
			t.Errorf("expected telegram/123, got %s/%s", msg.Channel, msg.ChatID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for error reply on outbound")
	}

	select {
	case <-agentBus.InboundChan():
		t.Fatal("blocked message should not be forwarded to agent bus")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestBusBridge_KnownCommandPassesThrough(t *testing.T) {
	realBus := bus.NewMessageBus()
	agentBus := bus.NewMessageBus()
	filter := commandfilter.NewCommandFilter()
	bridge := NewBusBridge(realBus, agentBus, filter)
	bridge.Start()
	defer bridge.Stop()

	ctx := context.Background()

	err := realBus.PublishInbound(ctx, bus.InboundMessage{
		Channel: "telegram",
		ChatID:  "456",
		Content: "/help",
	})
	if err != nil {
		t.Fatalf("PublishInbound: %v", err)
	}

	select {
	case msg := <-agentBus.InboundChan():
		if msg.Content != "/help" {
			t.Errorf("expected /help, got %q", msg.Content)
		}
		if msg.Channel != "telegram" || msg.ChatID != "456" {
			t.Errorf("expected telegram/456, got %s/%s", msg.Channel, msg.ChatID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message on agent bus")
	}
}

func TestBusBridge_PlainTextPassesThrough(t *testing.T) {
	realBus := bus.NewMessageBus()
	agentBus := bus.NewMessageBus()
	filter := commandfilter.NewCommandFilter()
	bridge := NewBusBridge(realBus, agentBus, filter)
	bridge.Start()
	defer bridge.Stop()

	ctx := context.Background()

	err := realBus.PublishInbound(ctx, bus.InboundMessage{
		Channel: "telegram",
		ChatID:  "789",
		Content: "hello world",
	})
	if err != nil {
		t.Fatalf("PublishInbound: %v", err)
	}

	select {
	case msg := <-agentBus.InboundChan():
		if msg.Content != "hello world" {
			t.Errorf("expected 'hello world', got %q", msg.Content)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message on agent bus")
	}
}

func TestBusBridge_SystemMessagePassesThrough(t *testing.T) {
	realBus := bus.NewMessageBus()
	agentBus := bus.NewMessageBus()
	filter := commandfilter.NewCommandFilter()
	bridge := NewBusBridge(realBus, agentBus, filter)
	bridge.Start()
	defer bridge.Stop()

	ctx := context.Background()

	err := realBus.PublishInbound(ctx, bus.InboundMessage{
		Channel: "system",
		ChatID:  "sys:123",
		Content: "/nonexistent",
	})
	if err != nil {
		t.Fatalf("PublishInbound: %v", err)
	}

	select {
	case msg := <-agentBus.InboundChan():
		if msg.Channel != "system" {
			t.Errorf("expected system, got %s", msg.Channel)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for system message on agent bus")
	}
}

func TestBusBridge_OutboundForwarded(t *testing.T) {
	realBus := bus.NewMessageBus()
	agentBus := bus.NewMessageBus()
	filter := commandfilter.NewCommandFilter()
	bridge := NewBusBridge(realBus, agentBus, filter)
	bridge.Start()
	defer bridge.Stop()

	ctx := context.Background()

	err := agentBus.PublishOutbound(ctx, bus.OutboundMessage{
		Channel: "telegram",
		ChatID:  "123",
		Content: "response text",
	})
	if err != nil {
		t.Fatalf("PublishOutbound: %v", err)
	}

	select {
	case msg := <-realBus.OutboundChan():
		if msg.Content != "response text" {
			t.Errorf("expected 'response text', got %q", msg.Content)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for outbound message on real bus")
	}
}

func TestBusBridge_OutboundMediaForwarded(t *testing.T) {
	realBus := bus.NewMessageBus()
	agentBus := bus.NewMessageBus()
	filter := commandfilter.NewCommandFilter()
	bridge := NewBusBridge(realBus, agentBus, filter)
	bridge.Start()
	defer bridge.Stop()

	ctx := context.Background()

	err := agentBus.PublishOutboundMedia(ctx, bus.OutboundMediaMessage{
		Channel: "telegram",
		ChatID:  "123",
		Parts: []bus.MediaPart{
			{Type: "image", Ref: "media://abc"},
		},
	})
	if err != nil {
		t.Fatalf("PublishOutboundMedia: %v", err)
	}

	select {
	case msg := <-realBus.OutboundMediaChan():
		if len(msg.Parts) != 1 || msg.Parts[0].Ref != "media://abc" {
			t.Errorf("unexpected media message: %+v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for outbound media on real bus")
	}
}
