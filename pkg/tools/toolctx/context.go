package toolctx

import "context"

type chatIDKey struct{}
type channelKey struct{}
type senderIDKey struct{}

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
