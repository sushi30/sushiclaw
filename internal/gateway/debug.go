package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/bus"
)

// DebugManager toggles per-chat debug event forwarding.
// When active, it subscribes to the agent event bus and publishes
// formatted event messages back to the chat that issued /debug.
type DebugManager struct {
	mu          sync.Mutex
	active      bool
	cancel      context.CancelFunc
	agentLoop   *agent.AgentLoop
	externalBus *bus.MessageBus
	channel     string
	chatID      string
}

// Toggle flips debug mode and returns a status string to send back to the user.
func (d *DebugManager) Toggle(ctx context.Context, channel, chatID string) string {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.active {
		d.cancel()
		d.cancel = nil
		d.active = false
		d.channel = ""
		d.chatID = ""
		return "Debug mode disabled."
	}

	d.channel = channel
	d.chatID = chatID
	d.active = true

	fwdCtx, cancel := context.WithCancel(ctx)
	d.cancel = cancel
	go d.runEventForwarder(fwdCtx)

	return "Debug mode enabled. Agent events will be sent to this chat."
}

// runEventForwarder subscribes to agent events and forwards formatted
// messages to the chat that enabled debug mode.
func (d *DebugManager) runEventForwarder(ctx context.Context) {
	sub := d.agentLoop.SubscribeEvents(64)
	defer d.agentLoop.UnsubscribeEvents(sub.ID)

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-sub.C:
			if !ok {
				return
			}
			msg := d.formatEvent(evt)
			if msg == "" {
				continue
			}
			d.mu.Lock()
			ch := d.channel
			cid := d.chatID
			d.mu.Unlock()
			if ch == "" {
				continue
			}
			_ = d.externalBus.PublishOutbound(ctx, bus.OutboundMessage{
				Channel: ch,
				ChatID:  cid,
				Content: msg,
			})
		}
	}
}

// formatEvent converts an agent Event into a compact human-readable string.
// Returns "" for events that should be suppressed (e.g. llm_delta, session_summarize).
func (d *DebugManager) formatEvent(evt agent.Event) string {
	switch evt.Kind {
	case agent.EventKindTurnStart:
		p, ok := evt.Payload.(agent.TurnStartPayload)
		if !ok {
			break
		}
		return fmt.Sprintf("[debug] turn start: msg=%s media=%d", p.UserMessage, p.MediaCount)

	case agent.EventKindTurnEnd:
		p, ok := evt.Payload.(agent.TurnEndPayload)
		if !ok {
			break
		}
		return fmt.Sprintf("[debug] turn end: status=%s iter=%d dur=%dms",
			p.Status, p.Iterations, p.Duration.Milliseconds())

	case agent.EventKindLLMRequest:
		p, ok := evt.Payload.(agent.LLMRequestPayload)
		if !ok {
			break
		}
		return fmt.Sprintf("[debug] llm→ model=%s msgs=%d tools=%d",
			p.Model, p.MessagesCount, p.ToolsCount)

	case agent.EventKindLLMResponse:
		p, ok := evt.Payload.(agent.LLMResponsePayload)
		if !ok {
			break
		}
		return fmt.Sprintf("[debug] llm← calls=%d content=%d reasoning=%v",
			p.ToolCalls, p.ContentLen, p.HasReasoning)

	case agent.EventKindLLMRetry:
		p, ok := evt.Payload.(agent.LLMRetryPayload)
		if !ok {
			break
		}
		return fmt.Sprintf("[debug] llm retry: attempt=%d/%d reason=%s backoff=%dms",
			p.Attempt, p.MaxRetries, p.Reason, p.Backoff.Milliseconds())

	case agent.EventKindToolExecStart:
		p, ok := evt.Payload.(agent.ToolExecStartPayload)
		if !ok {
			break
		}
		args := compactJSON(p.Arguments)
		return fmt.Sprintf("[debug] tool↓ %s args=%s", p.Tool, args)

	case agent.EventKindToolExecEnd:
		p, ok := evt.Payload.(agent.ToolExecEndPayload)
		if !ok {
			break
		}
		return fmt.Sprintf("[debug] tool↑ %s dur=%dms err=%v",
			p.Tool, p.Duration.Milliseconds(), p.IsError)

	case agent.EventKindToolExecSkipped:
		p, ok := evt.Payload.(agent.ToolExecSkippedPayload)
		if !ok {
			break
		}
		return fmt.Sprintf("[debug] tool skipped: %s reason=%s", p.Tool, p.Reason)

	case agent.EventKindContextCompress:
		p, ok := evt.Payload.(agent.ContextCompressPayload)
		if !ok {
			break
		}
		return fmt.Sprintf("[debug] context compressed: dropped=%d remaining=%d reason=%s",
			p.DroppedMessages, p.RemainingMessages, p.Reason)

	case agent.EventKindError:
		p, ok := evt.Payload.(agent.ErrorPayload)
		if !ok {
			break
		}
		return fmt.Sprintf("[debug] error: stage=%s msg=%s", p.Stage, p.Message)

	// Suppressed: too noisy or not user-facing.
	case agent.EventKindLLMDelta,
		agent.EventKindSessionSummarize,
		agent.EventKindSteeringInjected,
		agent.EventKindFollowUpQueued,
		agent.EventKindInterruptReceived,
		agent.EventKindSubTurnSpawn,
		agent.EventKindSubTurnEnd,
		agent.EventKindSubTurnResultDelivered,
		agent.EventKindSubTurnOrphan:
		return ""
	}
	return ""
}

// compactJSON marshals v to a compact JSON string, falling back to fmt.Sprint on error.
func compactJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}
