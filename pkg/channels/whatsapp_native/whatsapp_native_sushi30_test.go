//go:build whatsapp_native

package whatsapp

import (
	"context"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/media"
)

// noopMediaStore is a MediaStore that records Store calls without touching the filesystem.
type noopMediaStore struct {
	stored []string // local paths passed to Store
}

func (s *noopMediaStore) Store(localPath string, _ media.MediaMeta, _ string) (string, error) {
	s.stored = append(s.stored, localPath)
	return "media://noop-" + localPath, nil
}

func (s *noopMediaStore) Resolve(ref string) (string, error)                      { return ref, nil }
func (s *noopMediaStore) ResolveWithMeta(ref string) (string, media.MediaMeta, error) {
	return ref, media.MediaMeta{}, nil
}
func (s *noopMediaStore) ReleaseAll(_ string) error { return nil }

// makeTestChannel creates a WhatsAppNativeChannel with no real whatsmeow client
// (client stays nil, so DownloadAny is never called — suitable for text/caption tests).
func makeTestChannel(store media.MediaStore) (*WhatsAppNativeChannel, *bus.MessageBus) {
	mb := bus.NewMessageBus()
	bc := channels.NewBaseChannel("whatsapp_native", config.WhatsAppConfig{}, mb, nil)
	if store != nil {
		bc.SetMediaStore(store)
	}
	ch := &WhatsAppNativeChannel{
		BaseChannel: bc,
		runCtx:      context.Background(),
	}
	return ch, mb
}

func receiveInbound(t *testing.T, mb *bus.MessageBus) bus.InboundMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	select {
	case <-ctx.Done():
		t.Fatal("timeout: no inbound message forwarded")
	case msg := <-mb.InboundChan():
		return msg
	}
	panic("unreachable")
}

// TestHandleIncoming_ImageWithCaption_UsesCaption verifies that when a WhatsApp
// ImageMessage carries a caption and no plain-text conversation body, the caption
// becomes the message content forwarded to the agent.
func TestHandleIncoming_ImageWithCaption_UsesCaption(t *testing.T) {
	ch, mb := makeTestChannel(nil)

	caption := "look at this photo"
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Sender: types.NewJID("1001", types.DefaultUserServer),
				Chat:   types.NewJID("1001", types.DefaultUserServer),
			},
			ID:       "mid-img",
			PushName: "Bob",
		},
		Message: &waE2E.Message{
			ImageMessage: &waE2E.ImageMessage{
				Caption:  proto.String(caption),
				Mimetype: proto.String("image/jpeg"),
			},
		},
	}

	ch.handleIncoming(evt)

	msg := receiveInbound(t, mb)
	if msg.Content != caption {
		t.Fatalf("expected content=%q, got %q", caption, msg.Content)
	}
}

// TestHandleIncoming_MediaOnly_NotDropped verifies that a media-only WhatsApp
// message (image with no caption, no conversation text) is NOT silently dropped
// when a MediaStore is configured. Because the test channel has no real whatsmeow
// client the download will be skipped, but the message should still reach the bus
// if the store returns at least one ref (here we verify the opposite path:
// without a store the message IS dropped, confirming the guard logic).
func TestHandleIncoming_MediaOnly_Dropped_WithoutStoreAndNoCaption(t *testing.T) {
	ch, mb := makeTestChannel(nil) // no store, no caption → should be dropped

	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Sender: types.NewJID("1001", types.DefaultUserServer),
				Chat:   types.NewJID("1001", types.DefaultUserServer),
			},
			ID:       "mid-notext",
			PushName: "Carol",
		},
		Message: &waE2E.Message{
			ImageMessage: &waE2E.ImageMessage{
				Mimetype: proto.String("image/jpeg"),
				// no caption
			},
		},
	}

	ch.handleIncoming(evt)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	select {
	case <-ctx.Done():
		// correct: message was dropped because no content and no media refs
	case <-mb.InboundChan():
		t.Fatal("expected message to be dropped, but it was forwarded")
	}
}
