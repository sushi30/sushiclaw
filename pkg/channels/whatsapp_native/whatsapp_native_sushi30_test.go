package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/channels"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/media"
)

// makeTestChannel creates a WhatsAppNativeChannel with no real whatsmeow client
// (client stays nil, so DownloadAny is never called — suitable for text/caption tests).
func makeTestChannel(store media.MediaStore) (*WhatsAppNativeChannel, *bus.MessageBus) {
	mb := bus.NewMessageBus()
	bc := channels.NewBaseChannel("whatsapp_native", config.WhatsAppSettings{}, mb, nil)
	if store != nil {
		bc.SetMediaStore(store)
	}
	ch := &WhatsAppNativeChannel{
		BaseChannel: bc,
		runCtx:      context.Background(),
		startTime:   time.Now(),
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
			ID:        "mid-img",
			PushName:  "Bob",
			Timestamp: time.Now().Add(1 * time.Second),
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
			ID:        "mid-notext",
			PushName:  "Carol",
			Timestamp: time.Now().Add(1 * time.Second),
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

// TestHandleIncoming_IsFromMe_Skipped verifies that messages sent by the bot
// itself (IsFromMe=true) are skipped to prevent echo loops on reconnect.
func TestHandleIncoming_IsFromMe_Skipped(t *testing.T) {
	ch, mb := makeTestChannel(nil)

	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				IsFromMe: true,
				Sender:   types.NewJID("1001", types.DefaultUserServer),
				Chat:     types.NewJID("1001", types.DefaultUserServer),
			},
			ID: "mid-self",
		},
		Message: &waE2E.Message{
			Conversation: proto.String("self message"),
		},
	}

	ch.handleIncoming(evt)

	assertNoMessage(t, mb, "expected IsFromMe message to be skipped")
}

// TestHandleIncoming_SourceWebMsg_Skipped verifies that history-sync messages
// (SourceWebMsg != nil) are skipped to prevent reprocessing on reconnect.
func TestHandleIncoming_SourceWebMsg_Skipped(t *testing.T) {
	ch, mb := makeTestChannel(nil)

	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Sender: types.NewJID("1001", types.DefaultUserServer),
				Chat:   types.NewJID("1001", types.DefaultUserServer),
			},
			ID: "mid-history",
		},
		SourceWebMsg: &waWeb.WebMessageInfo{},
		Message: &waE2E.Message{
			Conversation: proto.String("history sync message"),
		},
	}

	ch.handleIncoming(evt)

	assertNoMessage(t, mb, "expected history-sync message to be skipped")
}

// TestHandleIncoming_StaleTimestamp_Skipped verifies that messages with
// timestamps older than the channel start time are skipped.
func TestHandleIncoming_StaleTimestamp_Skipped(t *testing.T) {
	ch, mb := makeTestChannel(nil)

	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Sender: types.NewJID("1001", types.DefaultUserServer),
				Chat:   types.NewJID("1001", types.DefaultUserServer),
			},
			ID:        "mid-stale",
			Timestamp: time.Now().Add(-1 * time.Hour),
		},
		Message: &waE2E.Message{
			Conversation: proto.String("stale message"),
		},
	}

	ch.handleIncoming(evt)

	assertNoMessage(t, mb, "expected stale message to be skipped")
}

// TestHandleIncoming_RecentMessage_Processed verifies that a valid recent
// message passes all guards and is processed normally.
func TestHandleIncoming_RecentMessage_Processed(t *testing.T) {
	ch, mb := makeTestChannel(nil)

	content := "hello world"
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Sender: types.NewJID("1001", types.DefaultUserServer),
				Chat:   types.NewJID("1001", types.DefaultUserServer),
			},
			ID:        "mid-recent",
			Timestamp: time.Now().Add(1 * time.Second),
		},
		Message: &waE2E.Message{
			Conversation: proto.String(content),
		},
	}

	ch.handleIncoming(evt)

	msg := receiveInbound(t, mb)
	if msg.Content != content {
		t.Fatalf("expected content=%q, got %q", content, msg.Content)
	}
}

// --- buildOutboundProtoMessage tests ---

func TestBuildOutboundProtoMessage_Buttons(t *testing.T) {
	msg := bus.OutboundMessage{
		Content: "fallback",
		Context: bus.InboundContext{Raw: map[string]string{
			"Content-Type":  "application/x-wa-buttons",
			"X-WA-Body":     "Pick one:",
			"X-WA-Option-0": "Alpha",
			"X-WA-Option-1": "Beta",
			"X-WA-Option-2": "Gamma",
		}},
	}
	waMsg := buildOutboundProtoMessage(msg)

	bm := waMsg.GetButtonsMessage()
	if bm == nil {
		t.Fatal("expected ButtonsMessage, got nil")
	}
	if bm.GetContentText() != "Pick one:" {
		t.Errorf("body: got %q, want %q", bm.GetContentText(), "Pick one:")
	}
	if len(bm.Buttons) != 3 {
		t.Fatalf("expected 3 buttons, got %d", len(bm.Buttons))
	}
	labels := []string{"Alpha", "Beta", "Gamma"}
	for i, btn := range bm.Buttons {
		if btn.GetButtonText().GetDisplayText() != labels[i] {
			t.Errorf("button[%d]: got %q, want %q", i, btn.GetButtonText().GetDisplayText(), labels[i])
		}
		if btn.GetButtonID() != fmt.Sprintf("%d", i) {
			t.Errorf("button[%d] ID: got %q, want %q", i, btn.GetButtonID(), fmt.Sprintf("%d", i))
		}
	}
}

func TestBuildOutboundProtoMessage_ButtonsOverflow(t *testing.T) {
	msg := bus.OutboundMessage{
		Content: "fallback",
		Context: bus.InboundContext{Raw: map[string]string{
			"Content-Type":  "application/x-wa-buttons",
			"X-WA-Body":     "Choose:",
			"X-WA-Option-0": "One",
			"X-WA-Option-1": "Two",
			"X-WA-Option-2": "Three",
			"X-WA-Option-3": "Four",
			"X-WA-Option-4": "Five",
		}},
	}
	waMsg := buildOutboundProtoMessage(msg)

	bm := waMsg.GetButtonsMessage()
	if bm == nil {
		t.Fatal("expected ButtonsMessage, got nil")
	}
	if len(bm.Buttons) != 3 {
		t.Fatalf("expected 3 buttons (overflow collapsed), got %d", len(bm.Buttons))
	}
	if bm.Buttons[0].GetButtonText().GetDisplayText() != "One" {
		t.Errorf("button[0]: got %q, want \"One\"", bm.Buttons[0].GetButtonText().GetDisplayText())
	}
	if bm.Buttons[1].GetButtonText().GetDisplayText() != "Two" {
		t.Errorf("button[1]: got %q, want \"Two\"", bm.Buttons[1].GetButtonText().GetDisplayText())
	}
	if bm.Buttons[2].GetButtonText().GetDisplayText() != "Other (chat about this)" {
		t.Errorf("button[2]: got %q, want \"Other (chat about this)\"", bm.Buttons[2].GetButtonText().GetDisplayText())
	}
}

func TestBuildOutboundProtoMessage_List(t *testing.T) {
	msg := bus.OutboundMessage{
		Content: "fallback",
		Context: bus.InboundContext{Raw: map[string]string{
			"Content-Type":  "application/x-wa-list",
			"X-WA-Body":     "Select a city:",
			"X-WA-Option-0": "New York",
			"X-WA-Option-1": "London",
			"X-WA-Option-2": "Tokyo",
			"X-WA-Option-3": "Sydney",
		}},
	}
	waMsg := buildOutboundProtoMessage(msg)

	lm := waMsg.GetListMessage()
	if lm == nil {
		t.Fatal("expected ListMessage, got nil")
	}
	if lm.GetTitle() != "Select a city:" {
		t.Errorf("title: got %q, want %q", lm.GetTitle(), "Select a city:")
	}
	if len(lm.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(lm.Sections))
	}
	rows := lm.Sections[0].Rows
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}
	labels := []string{"New York", "London", "Tokyo", "Sydney"}
	for i, row := range rows {
		if row.GetTitle() != labels[i] {
			t.Errorf("row[%d] title: got %q, want %q", i, row.GetTitle(), labels[i])
		}
		// RowID == label so incoming reply carries the choice directly.
		if row.GetRowID() != labels[i] {
			t.Errorf("row[%d] ID: got %q, want %q", i, row.GetRowID(), labels[i])
		}
	}
}

func TestBuildOutboundProtoMessage_FallbackPlainText(t *testing.T) {
	msg := bus.OutboundMessage{Content: "hello world"}
	waMsg := buildOutboundProtoMessage(msg)

	if waMsg.GetConversation() != "hello world" {
		t.Errorf("got %q, want %q", waMsg.GetConversation(), "hello world")
	}
	if waMsg.GetButtonsMessage() != nil {
		t.Error("expected no ButtonsMessage on plain fallback")
	}
	if waMsg.GetListMessage() != nil {
		t.Error("expected no ListMessage on plain fallback")
	}
}

func TestBuildOutboundProtoMessage_EmptyOptions(t *testing.T) {
	// Content-Type set but no X-WA-Option-N → fall back to plain text.
	msg := bus.OutboundMessage{
		Content: "plain",
		Context: bus.InboundContext{Raw: map[string]string{
			"Content-Type": "application/x-wa-buttons",
			"X-WA-Body":    "Choose:",
		}},
	}
	waMsg := buildOutboundProtoMessage(msg)

	if waMsg.GetConversation() != "plain" {
		t.Errorf("got %q, want %q", waMsg.GetConversation(), "plain")
	}
}

// --- handleIncoming widget reply tests ---

func TestHandleIncoming_ButtonsResponse_Forwarded(t *testing.T) {
	ch, mb := makeTestChannel(nil)

	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Sender: types.NewJID("1001", types.DefaultUserServer),
				Chat:   types.NewJID("1001", types.DefaultUserServer),
			},
			ID:        "mid-btn",
			PushName:  "Alice",
			Timestamp: time.Now().Add(1 * time.Second),
		},
		Message: &waE2E.Message{
			ButtonsResponseMessage: &waE2E.ButtonsResponseMessage{
				Response: &waE2E.ButtonsResponseMessage_SelectedDisplayText{
					SelectedDisplayText: "Book a flight",
				},
				SelectedButtonID: proto.String("0"),
				Type:             waE2E.ButtonsResponseMessage_DISPLAY_TEXT.Enum(),
			},
		},
	}

	ch.handleIncoming(evt)

	msg := receiveInbound(t, mb)
	if msg.Content != "Book a flight" {
		t.Errorf("content: got %q, want %q", msg.Content, "Book a flight")
	}
	if msg.Context.Raw["wa_reply_type"] != "button" {
		t.Errorf("wa_reply_type: got %q, want %q", msg.Context.Raw["wa_reply_type"], "button")
	}
}

func TestHandleIncoming_ListResponse_Forwarded(t *testing.T) {
	ch, mb := makeTestChannel(nil)

	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Sender: types.NewJID("1001", types.DefaultUserServer),
				Chat:   types.NewJID("1001", types.DefaultUserServer),
			},
			ID:        "mid-list",
			PushName:  "Bob",
			Timestamp: time.Now().Add(1 * time.Second),
		},
		Message: &waE2E.Message{
			ListResponseMessage: &waE2E.ListResponseMessage{
				SingleSelectReply: &waE2E.ListResponseMessage_SingleSelectReply{
					SelectedRowID: proto.String("London"),
				},
				ListType: waE2E.ListResponseMessage_SINGLE_SELECT.Enum(),
			},
		},
	}

	ch.handleIncoming(evt)

	msg := receiveInbound(t, mb)
	if msg.Content != "London" {
		t.Errorf("content: got %q, want %q", msg.Content, "London")
	}
	if msg.Context.Raw["wa_reply_type"] != "button" {
		t.Errorf("wa_reply_type: got %q, want %q", msg.Context.Raw["wa_reply_type"], "button")
	}
}

// --- Contact card tests ---

func TestHandleIncoming_ContactMessage_Forwarded(t *testing.T) {
	ch, mb := makeTestChannel(nil)

	vcard := "BEGIN:VCARD\nVERSION:3.0\nFN:John Doe\nTEL;type=CELL;waid=972501234567:+972 50-123-4567\nEMAIL:john@example.com\nORG:ACME Corp\nEND:VCARD"
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Sender: types.NewJID("1001", types.DefaultUserServer),
				Chat:   types.NewJID("1001", types.DefaultUserServer),
			},
			ID:        "mid-contact",
			PushName:  "John",
			Timestamp: time.Now().Add(1 * time.Second),
		},
		Message: &waE2E.Message{
			ContactMessage: &waE2E.ContactMessage{
				DisplayName: proto.String("John Doe"),
				Vcard:       proto.String(vcard),
			},
		},
	}

	ch.handleIncoming(evt)

	msg := receiveInbound(t, mb)
	if !strings.Contains(msg.Content, "[Contact Card: John Doe]") {
		t.Errorf("expected [Contact Card: John Doe] in content, got: %q", msg.Content)
	}
	if !strings.Contains(msg.Content, "Phone:") {
		t.Errorf("expected Phone: in content, got: %q", msg.Content)
	}
	if !strings.Contains(msg.Content, "Email:") {
		t.Errorf("expected Email: in content, got: %q", msg.Content)
	}
	if msg.Context.Raw["wa_message_type"] != "contact" {
		t.Errorf("wa_message_type: got %q, want %q", msg.Context.Raw["wa_message_type"], "contact")
	}
}

func TestHandleIncoming_ContactMessage_NoVCard_Forwarded(t *testing.T) {
	ch, mb := makeTestChannel(nil)

	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Sender: types.NewJID("1001", types.DefaultUserServer),
				Chat:   types.NewJID("1001", types.DefaultUserServer),
			},
			ID:        "mid-contact-novcard",
			Timestamp: time.Now().Add(1 * time.Second),
		},
		Message: &waE2E.Message{
			ContactMessage: &waE2E.ContactMessage{
				DisplayName: proto.String("Jane"),
			},
		},
	}

	ch.handleIncoming(evt)

	msg := receiveInbound(t, mb)
	if msg.Content != "[Contact Card: Jane]" {
		t.Errorf("expected content %q, got %q", "[Contact Card: Jane]", msg.Content)
	}
	if msg.Context.Raw["wa_message_type"] != "contact" {
		t.Errorf("wa_message_type: got %q, want %q", msg.Context.Raw["wa_message_type"], "contact")
	}
}

func TestHandleIncoming_ContactsArrayMessage_Forwarded(t *testing.T) {
	ch, mb := makeTestChannel(nil)

	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Sender: types.NewJID("1001", types.DefaultUserServer),
				Chat:   types.NewJID("1001", types.DefaultUserServer),
			},
			ID:        "mid-contacts-array",
			Timestamp: time.Now().Add(1 * time.Second),
		},
		Message: &waE2E.Message{
			ContactsArrayMessage: &waE2E.ContactsArrayMessage{
				DisplayName: proto.String("My Contacts"),
				Contacts: []*waE2E.ContactMessage{
					{DisplayName: proto.String("Alice")},
					{DisplayName: proto.String("Bob")},
				},
			},
		},
	}

	ch.handleIncoming(evt)

	msg := receiveInbound(t, mb)
	if !strings.Contains(msg.Content, "[Contact Cards: 2]") {
		t.Errorf("expected [Contact Cards: 2] in content, got: %q", msg.Content)
	}
	if !strings.Contains(msg.Content, "Alice") {
		t.Errorf("expected Alice in content, got: %q", msg.Content)
	}
	if !strings.Contains(msg.Content, "Bob") {
		t.Errorf("expected Bob in content, got: %q", msg.Content)
	}
	if msg.Context.Raw["wa_message_type"] != "contact" {
		t.Errorf("wa_message_type: got %q, want %q", msg.Context.Raw["wa_message_type"], "contact")
	}
}

// --- Widget auto-detection integration tests ---

func TestSend_OptionsAutoDetect_ButtonWidget(t *testing.T) {
	msg := bus.OutboundMessage{
		Content: "Which list?\n1. Backlog\n2. In Progress",
	}
	result := injectWidgetMetadata(msg)
	waMsg := buildOutboundProtoMessage(result)

	bm := waMsg.GetButtonsMessage()
	if bm == nil {
		t.Fatal("expected ButtonsMessage for 2-option list, got nil")
	}
	if len(bm.Buttons) != 2 {
		t.Fatalf("expected 2 buttons, got %d", len(bm.Buttons))
	}
	if bm.Buttons[0].GetButtonText().GetDisplayText() != "Backlog" {
		t.Errorf("button[0]: got %q, want %q", bm.Buttons[0].GetButtonText().GetDisplayText(), "Backlog")
	}
	if bm.Buttons[1].GetButtonText().GetDisplayText() != "In Progress" {
		t.Errorf("button[1]: got %q, want %q", bm.Buttons[1].GetButtonText().GetDisplayText(), "In Progress")
	}
	if bm.GetContentText() != "Which list?" {
		t.Errorf("body: got %q, want %q", bm.GetContentText(), "Which list?")
	}
}

func TestSend_OptionsAutoDetect_ListWidget(t *testing.T) {
	msg := bus.OutboundMessage{
		Content: "Select a list?\n1. Backlog\n2. In Progress\n3. Done\n4. Archive",
	}
	result := injectWidgetMetadata(msg)
	waMsg := buildOutboundProtoMessage(result)

	lm := waMsg.GetListMessage()
	if lm == nil {
		t.Fatal("expected ListMessage for 4-option list, got nil")
	}
	if len(lm.Sections) != 1 || len(lm.Sections[0].Rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(lm.Sections[0].Rows))
	}
	labels := []string{"Backlog", "In Progress", "Done", "Archive"}
	for i, row := range lm.Sections[0].Rows {
		if row.GetTitle() != labels[i] {
			t.Errorf("row[%d]: got %q, want %q", i, row.GetTitle(), labels[i])
		}
	}
}

func TestSend_ExistingContentType_NotOverridden(t *testing.T) {
	msg := bus.OutboundMessage{
		Content: "Which list?\n1. Backlog\n2. In Progress",
		Context: bus.InboundContext{Raw: map[string]string{
			"Content-Type":  "application/x-wa-list",
			"X-WA-Body":     "manual body",
			"X-WA-Option-0": "Custom A",
			"X-WA-Option-1": "Custom B",
		}},
	}
	result := injectWidgetMetadata(msg)
	if result.Context.Raw["X-WA-Body"] != "manual body" {
		t.Errorf("existing Content-Type should not be overridden; X-WA-Body: got %q", result.Context.Raw["X-WA-Body"])
	}
}

func TestSend_NoBodySuffix_NotDetected(t *testing.T) {
	msg := bus.OutboundMessage{
		Content: "Here are some items\n1. Alpha\n2. Beta",
	}
	result := injectWidgetMetadata(msg)
	if result.Context.Raw["Content-Type"] != "" {
		t.Errorf("expected no Content-Type for body without ?/: suffix, got %q", result.Context.Raw["Content-Type"])
	}
	waMsg := buildOutboundProtoMessage(result)
	if waMsg.GetConversation() == "" {
		t.Error("expected plain Conversation fallback")
	}
}

func TestRetrySend_SucceedsOnFirstTry(t *testing.T) {
	calls := 0
	err := retrySend(context.Background(), 3, time.Microsecond, time.Microsecond, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestRetrySend_SucceedsOnNthAttempt(t *testing.T) {
	calls := 0
	err := retrySend(context.Background(), 5, time.Microsecond, time.Microsecond, func() error {
		calls++
		if calls < 4 {
			return fmt.Errorf("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if calls != 4 {
		t.Fatalf("expected 4 calls, got %d", calls)
	}
}

func TestRetrySend_ExhaustsRetries(t *testing.T) {
	calls := 0
	err := retrySend(context.Background(), 3, time.Microsecond, time.Microsecond, func() error {
		calls++
		return fmt.Errorf("always fails")
	})
	if !errors.Is(err, channels.ErrSendFailed) {
		t.Fatalf("expected ErrSendFailed, got %v", err)
	}
	if calls != 4 { // 0..maxRetries inclusive
		t.Fatalf("expected 4 calls, got %d", calls)
	}
}

func TestRetrySend_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := retrySend(ctx, 10, time.Millisecond*50, time.Millisecond*50, func() error {
		calls++
		if calls == 1 {
			cancel()
		}
		return fmt.Errorf("fail")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func assertNoMessage(t *testing.T, mb *bus.MessageBus, msg string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	select {
	case <-ctx.Done():
		// correct: no message was forwarded
	case <-mb.InboundChan():
		t.Fatal(msg)
	}
}
