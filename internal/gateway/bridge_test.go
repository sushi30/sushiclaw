package gateway

import (
	"strings"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sushi30/sushiclaw/internal/commandfilter"
)

func newBridgeTestLoop() *agent.AgentLoop {
	return agent.NewAgentLoop(config.DefaultConfig(), bus.NewMessageBus(), &debugStubProvider{})
}

// TestHandleInbound_DebugCommand verifies that a /debug message is intercepted:
// a reply is sent to externalBus and nothing is forwarded to agentBus.
func TestHandleInbound_DebugCommand(t *testing.T) {
	agentBus := bus.NewMessageBus()
	extBus := bus.NewMessageBus()
	cmdFilter := commandfilter.NewCommandFilter()
	mgr := &DebugManager{agentLoop: newBridgeTestLoop(), externalBus: extBus}

	ctx := t.Context()

	handleInbound(ctx, bus.InboundMessage{
		Channel: "telegram",
		ChatID:  "chat99",
		Content: "/debug",
	}, cmdFilter, mgr, agentBus, extBus)

	// externalBus should have the toggle reply.
	select {
	case out := <-extBus.OutboundChan():
		if out.Channel != "telegram" {
			t.Fatalf("reply Channel = %q, want telegram", out.Channel)
		}
		if out.ChatID != "chat99" {
			t.Fatalf("reply ChatID = %q, want chat99", out.ChatID)
		}
		if out.Content == "" {
			t.Fatal("reply Content is empty")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for /debug reply on externalBus")
	}

	// agentBus must NOT have received the /debug message.
	select {
	case fwd := <-agentBus.InboundChan():
		t.Fatalf("unexpected message forwarded to agentBus: %+v", fwd)
	default:
		// Nothing there — correct.
	}
}

// TestHandleInbound_NormalMessage verifies that a non-debug, non-command message is
// forwarded to agentBus and no reply appears on externalBus.
func TestHandleInbound_NormalMessage(t *testing.T) {
	agentBus := bus.NewMessageBus()
	extBus := bus.NewMessageBus()
	cmdFilter := commandfilter.NewCommandFilter()
	mgr := &DebugManager{agentLoop: newBridgeTestLoop(), externalBus: extBus}

	ctx := t.Context()

	handleInbound(ctx, bus.InboundMessage{
		Channel: "telegram",
		ChatID:  "chat88",
		Content: "hello world",
	}, cmdFilter, mgr, agentBus, extBus)

	// agentBus should receive the forwarded message.
	select {
	case fwd := <-agentBus.InboundChan():
		if fwd.Content != "hello world" {
			t.Fatalf("forwarded Content = %q, want hello world", fwd.Content)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for forwarded message on agentBus")
	}

	// externalBus must NOT have received a reply.
	select {
	case out := <-extBus.OutboundChan():
		t.Fatalf("unexpected reply on externalBus: %+v", out)
	default:
		// Nothing there — correct.
	}
}

// TestHandleInbound_KnownCommand verifies that a known slash command like /help
// is forwarded to agentBus (not blocked).
func TestHandleInbound_KnownCommand(t *testing.T) {
	agentBus := bus.NewMessageBus()
	extBus := bus.NewMessageBus()
	cmdFilter := commandfilter.NewCommandFilter()
	mgr := &DebugManager{agentLoop: newBridgeTestLoop(), externalBus: extBus}

	ctx := t.Context()

	handleInbound(ctx, bus.InboundMessage{
		Channel: "telegram",
		ChatID:  "chat77",
		Content: "/help",
	}, cmdFilter, mgr, agentBus, extBus)

	select {
	case fwd := <-agentBus.InboundChan():
		if fwd.Content != "/help" {
			t.Fatalf("forwarded Content = %q, want /help", fwd.Content)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for /help on agentBus")
	}
}

// TestHandleInbound_UnknownCommandBlocked verifies that an unrecognized slash
// command is blocked: an error reply appears on externalBus and nothing is
// forwarded to agentBus.
func TestHandleInbound_UnknownCommandBlocked(t *testing.T) {
	agentBus := bus.NewMessageBus()
	extBus := bus.NewMessageBus()
	cmdFilter := commandfilter.NewCommandFilter()
	mgr := &DebugManager{agentLoop: newBridgeTestLoop(), externalBus: extBus}

	ctx := t.Context()

	handleInbound(ctx, bus.InboundMessage{
		Channel: "telegram",
		ChatID:  "chat55",
		Content: "/nonexistent",
	}, cmdFilter, mgr, agentBus, extBus)

	select {
	case out := <-extBus.OutboundChan():
		if out.Content != "Unknown command: /nonexistent" {
			t.Fatalf("expected 'Unknown command: /nonexistent', got %q", out.Content)
		}
		if out.Channel != "telegram" || out.ChatID != "chat55" {
			t.Errorf("expected telegram/chat55, got %s/%s", out.Channel, out.ChatID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for error reply on externalBus")
	}

	// agentBus must NOT have received the blocked command.
	select {
	case fwd := <-agentBus.InboundChan():
		t.Fatalf("unexpected message forwarded to agentBus: %+v", fwd)
	default:
		// Nothing there — correct.
	}
}

// TestHandleInbound_SystemMessagePassesThrough verifies that system channel
// messages bypass command filtering.
func TestHandleInbound_SystemMessagePassesThrough(t *testing.T) {
	agentBus := bus.NewMessageBus()
	extBus := bus.NewMessageBus()
	cmdFilter := commandfilter.NewCommandFilter()
	mgr := &DebugManager{agentLoop: newBridgeTestLoop(), externalBus: extBus}

	ctx := t.Context()

	handleInbound(ctx, bus.InboundMessage{
		Channel: "system",
		ChatID:  "sys:123",
		Content: "/nonexistent",
	}, cmdFilter, mgr, agentBus, extBus)

	select {
	case fwd := <-agentBus.InboundChan():
		if fwd.Channel != "system" {
			t.Errorf("expected system, got %s", fwd.Channel)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for system message on agentBus")
	}
}

// TestHandleInbound_UnknownCommandBeforeDebug verifies that /debug is NOT
// reachable via an unrecognized command name — i.e., /debuq should be blocked,
// not treated as /debug.
func TestHandleInbound_UnknownCommandBeforeDebug(t *testing.T) {
	agentBus := bus.NewMessageBus()
	extBus := bus.NewMessageBus()
	cmdFilter := commandfilter.NewCommandFilter()
	mgr := &DebugManager{agentLoop: newBridgeTestLoop(), externalBus: extBus}

	ctx := t.Context()

	handleInbound(ctx, bus.InboundMessage{
		Channel: "telegram",
		ChatID:  "chat44",
		Content: "/debuq",
	}, cmdFilter, mgr, agentBus, extBus)

	// Should get the "Unknown command" error, not a debug toggle reply.
	select {
	case out := <-extBus.OutboundChan():
		if out.Content != "Unknown command: /debuq" {
			t.Fatalf("expected 'Unknown command: /debuq', got %q", out.Content)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for error reply on externalBus")
	}
}

// TestInjectDebugIntoHelp_HelpResponse verifies that a /help response gets
// the /debug line appended.
func TestInjectDebugIntoHelp_HelpResponse(t *testing.T) {
	helpContent := "/start - Start the agent\n/help - Show this help message\n/clear - Clear history"
	msg := bus.OutboundMessage{Channel: "telegram", ChatID: "c1", Content: helpContent}
	got := injectDebugIntoHelp(msg)
	if got.Content == helpContent {
		t.Fatal("expected /debug line to be appended, content unchanged")
	}
	const want = "\n/debug - Toggle debug event forwarding to this chat"
	if !strings.HasSuffix(got.Content, want) {
		t.Fatalf("expected content to end with %q, got %q", want, got.Content)
	}
}

// TestInjectDebugIntoHelp_NonHelpResponse verifies that non-help content is
// returned unchanged.
func TestInjectDebugIntoHelp_NonHelpResponse(t *testing.T) {
	content := "Hello, world!"
	msg := bus.OutboundMessage{Channel: "telegram", ChatID: "c2", Content: content}
	got := injectDebugIntoHelp(msg)
	if got.Content != content {
		t.Fatalf("expected content unchanged, got %q", got.Content)
	}
}
