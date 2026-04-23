package bus

import "strings"

func (ctx InboundContext) isZero() bool {
	return ctx.Channel == "" &&
		ctx.Account == "" &&
		ctx.ChatID == "" &&
		ctx.ChatType == "" &&
		ctx.TopicID == "" &&
		ctx.SpaceID == "" &&
		ctx.SpaceType == "" &&
		ctx.SenderID == "" &&
		ctx.MessageID == "" &&
		!ctx.Mentioned &&
		ctx.ReplyToMessageID == "" &&
		ctx.ReplyToSenderID == "" &&
		len(ctx.ReplyHandles) == 0 &&
		len(ctx.Raw) == 0
}

// NewOutboundContext builds the minimal normalized addressing context.
func NewOutboundContext(channel, chatID, replyToMessageID string) InboundContext {
	return normalizeInboundContext(InboundContext{
		Channel:          strings.TrimSpace(channel),
		ChatID:           strings.TrimSpace(chatID),
		ReplyToMessageID: strings.TrimSpace(replyToMessageID),
	})
}

// NormalizeInboundMessage ensures convenience mirrors stay in sync.
func NormalizeInboundMessage(msg InboundMessage) InboundMessage {
	if msg.Context.Channel == "" {
		msg.Context.Channel = msg.Channel
	}
	if msg.Context.ChatID == "" {
		msg.Context.ChatID = msg.ChatID
	}
	if msg.Context.SenderID == "" {
		msg.Context.SenderID = msg.SenderID
	}
	if msg.Context.MessageID == "" {
		msg.Context.MessageID = msg.MessageID
	}
	msg.Context = normalizeInboundContext(msg.Context)
	msg.Channel = msg.Context.Channel
	msg.SenderID = msg.Context.SenderID
	msg.ChatID = msg.Context.ChatID
	if msg.MessageID == "" {
		msg.MessageID = msg.Context.MessageID
	}
	if msg.Context.MessageID == "" {
		msg.Context.MessageID = msg.MessageID
	}
	return msg
}

// NormalizeOutboundMessage ensures Context is normalized and mirrors are in sync.
func NormalizeOutboundMessage(msg OutboundMessage) OutboundMessage {
	msg.Channel = strings.TrimSpace(msg.Channel)
	msg.ChatID = strings.TrimSpace(msg.ChatID)
	msg.ReplyToMessageID = strings.TrimSpace(msg.ReplyToMessageID)
	if msg.Context.Channel == "" {
		msg.Context.Channel = msg.Channel
	}
	if msg.Context.ChatID == "" {
		msg.Context.ChatID = msg.ChatID
	}
	if msg.Context.ReplyToMessageID == "" {
		msg.Context.ReplyToMessageID = msg.ReplyToMessageID
	}
	msg.Context = normalizeInboundContext(msg.Context)
	if msg.Channel == "" {
		msg.Channel = msg.Context.Channel
	}
	if msg.ChatID == "" {
		msg.ChatID = msg.Context.ChatID
	}
	if msg.ReplyToMessageID == "" {
		msg.ReplyToMessageID = msg.Context.ReplyToMessageID
	}
	if msg.Context.ReplyToMessageID == "" {
		msg.Context.ReplyToMessageID = msg.ReplyToMessageID
	}
	msg.Scope = cloneOutboundScope(msg.Scope)
	return msg
}

// NormalizeOutboundMediaMessage normalizes media outbound messages.
func NormalizeOutboundMediaMessage(msg OutboundMediaMessage) OutboundMediaMessage {
	msg.Channel = strings.TrimSpace(msg.Channel)
	msg.ChatID = strings.TrimSpace(msg.ChatID)
	if msg.Context.Channel == "" {
		msg.Context.Channel = msg.Channel
	}
	if msg.Context.ChatID == "" {
		msg.Context.ChatID = msg.ChatID
	}
	msg.Context = normalizeInboundContext(msg.Context)
	if msg.Channel == "" {
		msg.Channel = msg.Context.Channel
	}
	if msg.ChatID == "" {
		msg.ChatID = msg.Context.ChatID
	}
	msg.Scope = cloneOutboundScope(msg.Scope)
	return msg
}

func normalizeInboundContext(ctx InboundContext) InboundContext {
	ctx.Channel = strings.TrimSpace(ctx.Channel)
	ctx.Account = strings.TrimSpace(ctx.Account)
	ctx.ChatID = strings.TrimSpace(ctx.ChatID)
	ctx.ChatType = normalizeKind(ctx.ChatType)
	ctx.TopicID = strings.TrimSpace(ctx.TopicID)
	ctx.SpaceID = strings.TrimSpace(ctx.SpaceID)
	ctx.SpaceType = normalizeKind(ctx.SpaceType)
	ctx.SenderID = strings.TrimSpace(ctx.SenderID)
	ctx.MessageID = strings.TrimSpace(ctx.MessageID)
	ctx.ReplyToMessageID = strings.TrimSpace(ctx.ReplyToMessageID)
	ctx.ReplyToSenderID = strings.TrimSpace(ctx.ReplyToSenderID)
	ctx.ReplyHandles = cloneStringMap(ctx.ReplyHandles)
	ctx.Raw = cloneStringMap(ctx.Raw)
	return ctx
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func normalizeKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}

func cloneOutboundScope(scope *OutboundScope) *OutboundScope {
	if scope == nil {
		return nil
	}
	cloned := *scope
	if len(scope.Dimensions) > 0 {
		cloned.Dimensions = append([]string(nil), scope.Dimensions...)
	}
	if len(scope.Values) > 0 {
		cloned.Values = make(map[string]string, len(scope.Values))
		for k, v := range scope.Values {
			cloned.Values[k] = v
		}
	}
	return &cloned
}
