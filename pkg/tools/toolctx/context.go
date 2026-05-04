package toolctx

import "context"

type chatIDKey struct{}
type channelKey struct{}
type senderIDKey struct{}
type inboundContextKey struct{}

// WithChatID returns a context with the chat ID set.
func WithChatID(ctx context.Context, chatID string) context.Context {
	return context.WithValue(ctx, chatIDKey{}, chatID)
}

// ChatIDFromContext returns the chat ID from the context, if any.
func ChatIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(chatIDKey{}).(string)
	return v
}

// WithChannel returns a context with the channel name set.
func WithChannel(ctx context.Context, channel string) context.Context {
	return context.WithValue(ctx, channelKey{}, channel)
}

// ChannelFromContext returns the channel name from the context, if any.
func ChannelFromContext(ctx context.Context) string {
	v, _ := ctx.Value(channelKey{}).(string)
	return v
}

// WithSenderID returns a context with the sender ID set.
func WithSenderID(ctx context.Context, senderID string) context.Context {
	return context.WithValue(ctx, senderIDKey{}, senderID)
}

// SenderIDFromContext returns the sender ID from the context, if any.
func SenderIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(senderIDKey{}).(string)
	return v
}

// WithInboundContext returns a context with the inbound message context set.
func WithInboundContext(ctx context.Context, inboundCtx any) context.Context {
	return context.WithValue(ctx, inboundContextKey{}, inboundCtx)
}

// InboundContextFromContext returns the inbound message context from the context, if any.
func InboundContextFromContext[T any](ctx context.Context) (T, bool) {
	v, ok := ctx.Value(inboundContextKey{}).(T)
	var zero T
	if !ok {
		return zero, false
	}
	return v, true
}
