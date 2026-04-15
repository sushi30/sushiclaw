package gateway

import (
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
)

func newBridgeTestLoop() *agent.AgentLoop {
	return agent.NewAgentLoop(config.DefaultConfig(), bus.NewMessageBus(), &debugStubProvider{})
}

// TestHandleInbound_DebugCommand verifies that a /debug message is intercepted:
// a reply is sent to externalBus and nothing is forwarded to agentBus.
func TestHandleInbound_DebugCommand(t *testing.T) {
	agentBus := bus.NewMessageBus()
	extBus := bus.NewMessageBus()
	mgr := &DebugManager{agentLoop: newBridgeTestLoop(), externalBus: extBus}

	ctx := t.Context()

	handleInbound(ctx, bus.InboundMessage{
		Channel: "telegram",
		ChatID:  "chat99",
		Content: "/debug",
	}, mgr, agentBus, extBus)

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

// TestHandleInbound_NormalMessage verifies that a non-debug message is
// forwarded to agentBus and no reply appears on externalBus.
func TestHandleInbound_NormalMessage(t *testing.T) {
	agentBus := bus.NewMessageBus()
	extBus := bus.NewMessageBus()
	mgr := &DebugManager{agentLoop: newBridgeTestLoop(), externalBus: extBus}

	ctx := t.Context()

	handleInbound(ctx, bus.InboundMessage{
		Channel: "telegram",
		ChatID:  "chat88",
		Content: "hello world",
	}, mgr, agentBus, extBus)

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
