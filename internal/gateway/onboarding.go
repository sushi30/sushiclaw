package gateway

import (
	"context"
	"strings"

	"github.com/sushi30/sushiclaw/internal/commandfilter"
	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/commands"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/logger"
)

type onboardingState struct {
	enabled bool
	shown   bool
}

func newOnboardingState(cfg config.OnboardingConfig) *onboardingState {
	return &onboardingState{enabled: cfg.Auto.Enabled}
}

func (o *onboardingState) consume(msg bus.InboundMessage) (string, bool) {
	if !o.shouldAutoReply(msg) {
		return "", false
	}
	o.shown = true
	return commands.OnboardingMessage(), true
}

func (o *onboardingState) shouldAutoReply(msg bus.InboundMessage) bool {
	if o == nil || !o.enabled || o.shown {
		return false
	}
	if commands.HasCommandPrefix(msg.Content) {
		return false
	}
	channel := inboundChannel(msg)
	if channel == "system" {
		return false
	}
	if channel != config.ChannelTelegram && channel != config.ChannelWhatsAppNative {
		return false
	}
	return inboundChatType(msg) == "direct"
}

type inboundRouter struct {
	messageBus        *bus.MessageBus
	cmdFilter         *commandfilter.CommandFilter
	executor          *commands.Executor
	sessionDispatcher func(context.Context, bus.InboundMessage)
	autoOnboarding    *onboardingState
}

func (r *inboundRouter) handleMessage(ctx context.Context, msg bus.InboundMessage) {
	dec := r.cmdFilter.Filter(msg)
	logger.DebugCF("commandfilter", "Filtered message",
		map[string]any{
			"text":    msg.Content,
			"result":  dec.Result,
			"command": dec.Command,
		})
	if dec.Result == commandfilter.Block {
		logger.InfoCF("commandfilter", "Blocked unrecognized slash command",
			map[string]any{
				"channel": inboundChannel(msg),
				"chat_id": inboundChatID(msg),
				"command": dec.Command,
			})
		r.publishReply(ctx, msg, dec.ErrMsg)
		return
	}

	if commands.HasCommandPrefix(msg.Content) {
		var reply string
		result := r.executor.Execute(ctx, commands.Request{
			Channel:  inboundChannel(msg),
			ChatID:   inboundChatID(msg),
			SenderID: inboundSenderID(msg),
			Text:     msg.Content,
			Reply:    func(text string) error { reply = text; return nil },
		})
		logger.DebugCF("executor", "Command executed",
			map[string]any{
				"text":    msg.Content,
				"command": result.Command,
				"outcome": result.Outcome,
				"handled": result.Outcome == commands.OutcomeHandled,
			})
		if result.Outcome == commands.OutcomeHandled {
			if reply != "" {
				r.publishReply(ctx, msg, reply)
			}
			return
		}
	}

	if reply, ok := r.autoOnboarding.consume(msg); ok {
		r.publishReply(ctx, msg, reply)
		return
	}

	if r.sessionDispatcher != nil {
		r.sessionDispatcher(ctx, msg)
	}
}

func (r *inboundRouter) publishReply(ctx context.Context, msg bus.InboundMessage, content string) {
	_ = r.messageBus.PublishOutbound(ctx, bus.OutboundMessage{
		Channel: inboundChannel(msg),
		ChatID:  inboundChatID(msg),
		Content: content,
	})
}

func inboundChannel(msg bus.InboundMessage) string {
	if msg.Context.Channel != "" {
		return msg.Context.Channel
	}
	return strings.TrimSpace(msg.Channel)
}

func inboundChatID(msg bus.InboundMessage) string {
	if msg.Context.ChatID != "" {
		return msg.Context.ChatID
	}
	return strings.TrimSpace(msg.ChatID)
}

func inboundSenderID(msg bus.InboundMessage) string {
	if msg.Context.SenderID != "" {
		return msg.Context.SenderID
	}
	return strings.TrimSpace(msg.SenderID)
}

func inboundChatType(msg bus.InboundMessage) string {
	return strings.ToLower(strings.TrimSpace(msg.Context.ChatType))
}
