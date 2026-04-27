package message

import (
	"context"
	"testing"
	"time"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/tools/toolctx"
)

func TestMessageTool_NameAndDescription(t *testing.T) {
	mt := NewTool(bus.NewMessageBus(), 0)
	if mt.Name() != "message_tool" {
		t.Errorf("expected name message_tool, got %s", mt.Name())
	}
	if mt.Description() == "" {
		t.Error("expected non-empty description")
	}
	params := mt.Parameters()
	if len(params) != 1 {
		t.Fatalf("expected 1 parameter, got %d", len(params))
	}
	if _, ok := params["content"]; !ok {
		t.Error("expected 'content' parameter")
	}
}

func TestMessageTool_Success(t *testing.T) {
	b := bus.NewMessageBus()
	mt := NewTool(b, 0)

	ctx := toolctx.WithChannel(toolctx.WithChatID(context.Background(), "chat123"), config.ChannelTelegram)

	result, err := mt.Execute(ctx, `{"content":"Hello update"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Message sent." {
		t.Errorf("unexpected result: %s", result)
	}

	select {
	case msg := <-b.OutboundChan():
		if msg.Channel != config.ChannelTelegram {
			t.Errorf("expected channel %s, got %s", config.ChannelTelegram, msg.Channel)
		}
		if msg.ChatID != "chat123" {
			t.Errorf("expected chatID chat123, got %s", msg.ChatID)
		}
		if msg.Content != "Hello update" {
			t.Errorf("expected content 'Hello update', got %s", msg.Content)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for outbound message")
	}
}

func TestMessageTool_EmailRejection(t *testing.T) {
	b := bus.NewMessageBus()
	mt := NewTool(b, 0)

	ctx := toolctx.WithChannel(toolctx.WithChatID(context.Background(), "chat@example.com"), config.ChannelEmail)

	_, err := mt.Execute(ctx, `{"content":"Blocked"}`)
	if err == nil {
		t.Fatal("expected error for email channel")
	}
	if err.Error() != "message_tool: not available for email channel" {
		t.Errorf("unexpected error message: %v", err)
	}

	select {
	case <-b.OutboundChan():
		t.Fatal("expected no outbound message for email")
	case <-time.After(100 * time.Millisecond):
		// expected
	}
}

func TestMessageTool_MissingContext(t *testing.T) {
	b := bus.NewMessageBus()
	mt := NewTool(b, 0)

	// Missing channel
	ctx := toolctx.WithChatID(context.Background(), "chat123")
	_, err := mt.Execute(ctx, `{"content":"Test"}`)
	if err == nil {
		t.Fatal("expected error for missing channel")
	}

	// Missing chatID
	ctx = toolctx.WithChannel(context.Background(), config.ChannelTelegram)
	_, err = mt.Execute(ctx, `{"content":"Test"}`)
	if err == nil {
		t.Fatal("expected error for missing chatID")
	}
}

func TestMessageTool_Throttling(t *testing.T) {
	b := bus.NewMessageBus()
	mt := NewTool(b, 1) // 1 second min interval

	ctx := toolctx.WithChannel(toolctx.WithChatID(context.Background(), "chat456"), config.ChannelTelegram)

	// First call should succeed
	_, err := mt.Execute(ctx, `{"content":"First"}`)
	if err != nil {
		t.Fatalf("unexpected error on first call: %v", err)
	}
	<-b.OutboundChan() // drain

	// Second call immediately should fail
	_, err = mt.Execute(ctx, `{"content":"Second"}`)
	if err == nil {
		t.Fatal("expected throttle error on second call")
	}

	// Wait for throttle to reset
	time.Sleep(1100 * time.Millisecond)

	_, err = mt.Execute(ctx, `{"content":"Third"}`)
	if err != nil {
		t.Fatalf("unexpected error after throttle reset: %v", err)
	}

	select {
	case msg := <-b.OutboundChan():
		if msg.Content != "Third" {
			t.Errorf("expected 'Third', got %s", msg.Content)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for third message")
	}
}

func TestMessageTool_EmptyContent(t *testing.T) {
	b := bus.NewMessageBus()
	mt := NewTool(b, 0)

	ctx := toolctx.WithChannel(toolctx.WithChatID(context.Background(), "chat123"), config.ChannelTelegram)

	_, err := mt.Execute(ctx, `{"content":""}`)
	if err == nil {
		t.Fatal("expected error for empty content")
	}
	if err.Error() != "message_tool: content is required" {
		t.Errorf("unexpected error message: %v", err)
	}
}
