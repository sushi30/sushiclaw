package bus

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// ErrBusClosed is returned when publishing to a closed MessageBus.
var ErrBusClosed = errors.New("message bus closed")

var (
	ErrMissingInboundContext       = errors.New("inbound message context is required")
	ErrMissingOutboundContext      = errors.New("outbound message context is required")
	ErrMissingOutboundMediaContext = errors.New("outbound media context is required")
)

const defaultBusBufferSize = 64

// StreamDelegate is implemented by the channel Manager to provide streaming
// capabilities without tight coupling.
type StreamDelegate interface {
	GetStreamer(ctx context.Context, channel, chatID string) (Streamer, bool)
}

// Streamer pushes incremental content to a streaming-capable channel.
type Streamer interface {
	Update(ctx context.Context, content string) error
	Finalize(ctx context.Context, content string) error
	Cancel(ctx context.Context)
}

type MessageBus struct {
	inbound       chan InboundMessage
	outbound      chan OutboundMessage
	outboundMedia chan OutboundMediaMessage
	audioChunks   chan AudioChunk
	voiceControls chan VoiceControl

	closeOnce      sync.Once
	done           chan struct{}
	closed         atomic.Bool
	wg             sync.WaitGroup
	streamDelegate atomic.Value
}

func NewMessageBus() *MessageBus {
	return &MessageBus{
		inbound:       make(chan InboundMessage, defaultBusBufferSize),
		outbound:      make(chan OutboundMessage, defaultBusBufferSize),
		outboundMedia: make(chan OutboundMediaMessage, defaultBusBufferSize),
		audioChunks:   make(chan AudioChunk, defaultBusBufferSize*4),
		voiceControls: make(chan VoiceControl, defaultBusBufferSize),
		done:          make(chan struct{}),
	}
}

func publish[T any](ctx context.Context, mb *MessageBus, ch chan T, msg T) error {
	if mb.closed.Load() {
		return ErrBusClosed
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-mb.done:
		return ErrBusClosed
	default:
	}

	mb.wg.Add(1)
	defer mb.wg.Done()

	select {
	case ch <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-mb.done:
		return ErrBusClosed
	}
}

func (mb *MessageBus) PublishInbound(ctx context.Context, msg InboundMessage) error {
	msg = NormalizeInboundMessage(msg)
	if msg.Context.isZero() {
		return ErrMissingInboundContext
	}
	return publish(ctx, mb, mb.inbound, msg)
}

func (mb *MessageBus) InboundChan() <-chan InboundMessage {
	return mb.inbound
}

func (mb *MessageBus) PublishOutbound(ctx context.Context, msg OutboundMessage) error {
	msg = NormalizeOutboundMessage(msg)
	if msg.Context.isZero() {
		return ErrMissingOutboundContext
	}
	return publish(ctx, mb, mb.outbound, msg)
}

func (mb *MessageBus) OutboundChan() <-chan OutboundMessage {
	return mb.outbound
}

func (mb *MessageBus) PublishOutboundMedia(ctx context.Context, msg OutboundMediaMessage) error {
	msg = NormalizeOutboundMediaMessage(msg)
	if msg.Context.isZero() {
		return ErrMissingOutboundMediaContext
	}
	return publish(ctx, mb, mb.outboundMedia, msg)
}

func (mb *MessageBus) OutboundMediaChan() <-chan OutboundMediaMessage {
	return mb.outboundMedia
}

func (mb *MessageBus) PublishAudioChunk(ctx context.Context, chunk AudioChunk) error {
	return publish(ctx, mb, mb.audioChunks, chunk)
}

func (mb *MessageBus) AudioChunksChan() <-chan AudioChunk {
	return mb.audioChunks
}

func (mb *MessageBus) PublishVoiceControl(ctx context.Context, ctrl VoiceControl) error {
	return publish(ctx, mb, mb.voiceControls, ctrl)
}

func (mb *MessageBus) VoiceControlsChan() <-chan VoiceControl {
	return mb.voiceControls
}

// SetStreamDelegate registers a StreamDelegate (typically the channel Manager).
func (mb *MessageBus) SetStreamDelegate(d StreamDelegate) {
	mb.streamDelegate.Store(d)
}

// GetStreamer returns a Streamer for the given channel+chatID via the delegate.
func (mb *MessageBus) GetStreamer(ctx context.Context, channel, chatID string) (Streamer, bool) {
	if d, ok := mb.streamDelegate.Load().(StreamDelegate); ok && d != nil {
		return d.GetStreamer(ctx, channel, chatID)
	}
	return nil, false
}

func (mb *MessageBus) Close() {
	mb.closeOnce.Do(func() {
		close(mb.done)
		mb.closed.Store(true)
		mb.wg.Wait()

		close(mb.inbound)
		close(mb.outbound)
		close(mb.outboundMedia)
		close(mb.audioChunks)
		close(mb.voiceControls)

		for range mb.inbound {
		}
		for range mb.outbound {
		}
		for range mb.outboundMedia {
		}
		for range mb.audioChunks {
		}
		for range mb.voiceControls {
		}
	})
}
