// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package whatsapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"

	"github.com/google/uuid"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/identity"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/media"
	"github.com/sipeed/picoclaw/pkg/utils"
)

const (
	sqliteDriver   = "sqlite"
	whatsappDBName = "store.db"

	reconnectInitial    = 5 * time.Second
	reconnectMax        = 5 * time.Minute
	reconnectMultiplier = 2.0
)

// WhatsAppNativeChannel implements the WhatsApp channel using whatsmeow (in-process, no external bridge).
type WhatsAppNativeChannel struct {
	*channels.BaseChannel
	config       *config.WhatsAppSettings
	voiceConfig  config.VoiceConfig
	storePath    string
	client       *whatsmeow.Client
	container    *sqlstore.Container
	mu           sync.Mutex
	runCtx       context.Context
	runCancel    context.CancelFunc
	reconnectMu  sync.Mutex
	reconnecting bool
	stopping     atomic.Bool    // set once Stop begins; prevents new wg.Add calls
	wg           sync.WaitGroup // tracks background goroutines (QR handler, reconnect)
	startTime    time.Time      // used to filter history-sync messages on reconnect
}

// NewWhatsAppNativeChannel creates a WhatsApp channel that uses whatsmeow for connection.
// storePath is the directory for the SQLite session store (e.g. workspace/whatsapp).
func NewWhatsAppNativeChannel(
	bc *config.Channel,
	cfg *config.WhatsAppSettings,
	voiceCfg config.VoiceConfig,
	bus *bus.MessageBus,
	storePath string,
) (channels.Channel, error) {
	base := channels.NewBaseChannel(bc.Name(), cfg, bus, bc.AllowFrom,
		channels.WithMaxMessageLength(65536),
		channels.WithReasoningChannelID(bc.ReasoningChannelID),
	)
	if storePath == "" {
		storePath = "whatsapp"
	}
	c := &WhatsAppNativeChannel{
		BaseChannel: base,
		config:      cfg,
		voiceConfig: voiceCfg,
		storePath:   storePath,
	}
	return c, nil
}

func (c *WhatsAppNativeChannel) Start(ctx context.Context) error {
	logger.InfoCF("whatsapp", "Starting WhatsApp native channel (whatsmeow)", map[string]any{"store": c.storePath})

	// Reset lifecycle state from any previous Stop() so a restarted channel
	// behaves correctly.  Use reconnectMu to be consistent with eventHandler
	// and Stop() which coordinate under the same lock.
	c.reconnectMu.Lock()
	c.stopping.Store(false)
	c.reconnecting = false
	c.reconnectMu.Unlock()

	if err := os.MkdirAll(c.storePath, 0o700); err != nil {
		return fmt.Errorf("create session store dir: %w", err)
	}

	dbPath := filepath.Join(c.storePath, whatsappDBName)
	connStr := "file:" + dbPath + "?_foreign_keys=on"

	db, err := sql.Open(sqliteDriver, connStr)
	if err != nil {
		return fmt.Errorf("open whatsapp store: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err = db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return fmt.Errorf("enable foreign keys: %w", err)
	}

	waLogger := waLog.Stdout("WhatsApp", "WARN", true)
	container := sqlstore.NewWithDB(db, sqliteDriver, waLogger)
	if err = container.Upgrade(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("open whatsapp store: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		_ = container.Close()
		return fmt.Errorf("get device store: %w", err)
	}

	client := whatsmeow.NewClient(deviceStore, waLogger)

	// Create runCtx/runCancel BEFORE registering event handler and starting
	// goroutines so that Stop() can cancel them at any time, including during
	// the QR-login flow.
	c.runCtx, c.runCancel = context.WithCancel(ctx)
	c.startTime = time.Now()

	client.AddEventHandler(c.eventHandler)

	c.mu.Lock()
	c.container = container
	c.client = client
	c.mu.Unlock()

	// cleanupOnError clears struct references and releases resources when
	// Start() fails after fields are already assigned.  This prevents
	// Stop() from operating on stale references (double-close, disconnect
	// of a partially-initialized client, or stray event handler callbacks).
	startOK := false
	defer func() {
		if startOK {
			return
		}
		c.runCancel()
		client.Disconnect()
		c.mu.Lock()
		c.client = nil
		c.container = nil
		c.mu.Unlock()
		_ = container.Close()
	}()

	if client.Store.ID == nil {
		qrChan, err := client.GetQRChannel(c.runCtx)
		if err != nil {
			return fmt.Errorf("get QR channel: %w", err)
		}
		if err := client.Connect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}
		// Handle QR events in a background goroutine so Start() returns
		// promptly.  The goroutine is tracked via c.wg and respects
		// c.runCtx for cancellation.
		// Guard wg.Add with reconnectMu + stopping check (same protocol
		// as eventHandler) so a concurrent Stop() cannot enter wg.Wait()
		// while we call wg.Add(1).
		c.reconnectMu.Lock()
		if c.stopping.Load() {
			c.reconnectMu.Unlock()
			return fmt.Errorf("channel stopped during QR setup")
		}
		c.wg.Add(1)
		c.reconnectMu.Unlock()
		go func() {
			defer c.wg.Done()
			for {
				select {
				case <-c.runCtx.Done():
					return
				case evt, ok := <-qrChan:
					if !ok {
						return
					}
					if evt.Event == "code" {
						logger.InfoCF("whatsapp", "Scan this QR code with WhatsApp (Linked Devices):", nil)
						qrterminal.GenerateWithConfig(evt.Code, qrterminal.Config{
							Level:      qrterminal.L,
							Writer:     os.Stdout,
							HalfBlocks: true,
						})
					} else {
						logger.InfoCF("whatsapp", "WhatsApp login event", map[string]any{"event": evt.Event})
					}
				}
			}
		}()
	} else {
		if err := client.Connect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}
	}

	startOK = true
	c.SetRunning(true)
	logger.InfoC("whatsapp", "WhatsApp native channel connected")
	return nil
}

func (c *WhatsAppNativeChannel) Stop(ctx context.Context) error {
	logger.InfoC("whatsapp", "Stopping WhatsApp native channel")

	// Mark as stopping under reconnectMu so the flag is visible to
	// eventHandler atomically with respect to its wg.Add(1) call.
	// This closes the TOCTOU window where eventHandler could check
	// stopping (false), then Stop sets it true + enters wg.Wait,
	// then eventHandler calls wg.Add(1) — causing a panic.
	c.reconnectMu.Lock()
	c.stopping.Store(true)
	c.reconnectMu.Unlock()

	if c.runCancel != nil {
		c.runCancel()
	}

	// Disconnect the client first so any blocking Connect()/reconnect loops
	// can be interrupted before we wait on the goroutines.
	c.mu.Lock()
	client := c.client
	container := c.container
	c.mu.Unlock()

	if client != nil {
		client.Disconnect()
	}

	// Wait for background goroutines (QR handler, reconnect) to finish in a
	// context-aware way so Stop can be bounded by ctx.
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All goroutines have finished.
	case <-ctx.Done():
		// Context canceled or timed out; log and proceed with best-effort cleanup.
		logger.WarnC("whatsapp", fmt.Sprintf("Stop context canceled before all goroutines finished: %v", ctx.Err()))
	}

	// Now it is safe to clear and close resources.
	c.mu.Lock()
	c.client = nil
	c.container = nil
	c.mu.Unlock()

	if container != nil {
		_ = container.Close()
	}
	c.SetRunning(false)
	return nil
}

func (c *WhatsAppNativeChannel) eventHandler(evt any) {
	switch evt := evt.(type) {
	case *events.Message:
		c.handleIncoming(evt)
	case *events.Disconnected:
		logger.InfoCF("whatsapp", "WhatsApp disconnected, will attempt reconnection", nil)
		c.reconnectMu.Lock()
		if c.reconnecting {
			c.reconnectMu.Unlock()
			return
		}
		// Check stopping while holding the lock so the check and wg.Add
		// are atomic with respect to Stop() setting the flag + calling
		// wg.Wait(). This prevents the TOCTOU race.
		if c.stopping.Load() {
			c.reconnectMu.Unlock()
			return
		}
		c.reconnecting = true
		c.wg.Add(1)
		c.reconnectMu.Unlock()
		go func() {
			defer c.wg.Done()
			c.reconnectWithBackoff()
		}()
	}
}

func (c *WhatsAppNativeChannel) reconnectWithBackoff() {
	defer func() {
		c.reconnectMu.Lock()
		c.reconnecting = false
		c.reconnectMu.Unlock()
	}()

	backoff := reconnectInitial
	for {
		select {
		case <-c.runCtx.Done():
			return
		default:
		}

		c.mu.Lock()
		client := c.client
		c.mu.Unlock()
		if client == nil {
			return
		}
		if client.IsConnected() {
			return
		}

		logger.InfoCF("whatsapp", "WhatsApp reconnecting", map[string]any{"backoff": backoff.String()})
		err := client.Connect()
		if err == nil {
			logger.InfoC("whatsapp", "WhatsApp reconnected")
			return
		}

		logger.WarnCF("whatsapp", "WhatsApp reconnect failed", map[string]any{"error": err.Error()})

		select {
		case <-c.runCtx.Done():
			return
		case <-time.After(backoff):
			if backoff < reconnectMax {
				next := time.Duration(float64(backoff) * reconnectMultiplier)
				if next > reconnectMax {
					next = reconnectMax
				}
				backoff = next
			}
		}
	}
}

func (c *WhatsAppNativeChannel) handleIncoming(evt *events.Message) {
	// Skip own messages (bot's outgoing replayed on reconnect).
	if evt.Info.IsFromMe {
		return
	}
	// Skip history-sync messages delivered on reconnect.
	if evt.SourceWebMsg != nil {
		return
	}
	// Defense-in-depth: skip anything older than channel start.
	if evt.Info.Timestamp.Before(c.startTime) {
		return
	}
	if evt.Message == nil {
		return
	}
	senderID := evt.Info.Sender.String()
	chatID := evt.Info.Chat.String()
	content := evt.Message.GetConversation()
	if content == "" && evt.Message.ExtendedTextMessage != nil {
		content = evt.Message.ExtendedTextMessage.GetText()
	}

	// Extract caption from media sub-messages when there is no plain-text body.
	var waMessageType string
	if content == "" {
		switch {
		case evt.Message.GetImageMessage() != nil:
			content = evt.Message.GetImageMessage().GetCaption()
		case evt.Message.GetVideoMessage() != nil:
			content = evt.Message.GetVideoMessage().GetCaption()
		case evt.Message.GetDocumentMessage() != nil:
			content = evt.Message.GetDocumentMessage().GetCaption()
		case evt.Message.GetContactMessage() != nil:
			content = formatContactContent(evt.Message.GetContactMessage())
			waMessageType = "contact"
		case evt.Message.GetContactsArrayMessage() != nil:
			content = formatContactsArrayContent(evt.Message.GetContactsArrayMessage())
			waMessageType = "contact"
		}
	}

	content = utils.SanitizeMessageContent(content)

	// Decode interactive widget replies (button tap / list selection).
	var waReplyType string
	if content == "" {
		if br := evt.Message.GetButtonsResponseMessage(); br != nil {
			content = br.GetSelectedDisplayText()
			if content == "" {
				content = br.GetSelectedButtonID()
			}
			waReplyType = "button"
		} else if lr := evt.Message.GetListResponseMessage(); lr != nil {
			if sr := lr.GetSingleSelectReply(); sr != nil {
				content = sr.GetSelectedRowID()
			}
			waReplyType = "button"
		}
		if waReplyType != "" {
			content = utils.SanitizeMessageContent(content)
		}
	}

	var mediaPaths []string

	// Download media attachment, if present and a MediaStore is configured.
	if store := c.GetMediaStore(); store != nil {
		data, err := c.client.DownloadAny(c.runCtx, evt.Message) //nolint:staticcheck // DownloadAny is deprecated upstream; per-type Download refactor is a separate task
		if err != nil && !errors.Is(err, whatsmeow.ErrNothingDownloadableFound) {
			logger.DebugCF("whatsapp", "media download failed", map[string]any{"err": err})
		}
		if err == nil && len(data) > 0 {
			filename, mimetype := mediaFilenameAndMIME(evt.Message)
			mediaDir := media.TempDir()
			if mkErr := os.MkdirAll(mediaDir, 0o700); mkErr == nil {
				localPath := filepath.Join(mediaDir, uuid.New().String()[:8]+"_"+filename)
				if writeErr := os.WriteFile(localPath, data, 0o600); writeErr == nil {
					scope := channels.BuildMediaScope("whatsapp", chatID, evt.Info.ID)
					ref, storeErr := store.Store(localPath, media.MediaMeta{
						Filename:      filename,
						ContentType:   mimetype,
						Source:        "whatsapp",
						CleanupPolicy: media.CleanupPolicyDeleteOnCleanup,
					}, scope)
					if storeErr == nil {
						mediaPaths = append(mediaPaths, ref)
					}
				}
			}
		}
	}

	if content == "" && len(mediaPaths) == 0 {
		return
	}

	metadata := make(map[string]string)
	if waReplyType != "" {
		metadata["wa_reply_type"] = waReplyType
	}
	if waMessageType != "" {
		metadata["wa_message_type"] = waMessageType
	}
	metadata["message_id"] = evt.Info.ID
	if evt.Info.PushName != "" {
		metadata["user_name"] = evt.Info.PushName
	}
	if evt.Info.Chat.Server == types.GroupServer {
		metadata["peer_kind"] = "group"
		metadata["peer_id"] = chatID
	} else {
		metadata["peer_kind"] = "direct"
		metadata["peer_id"] = senderID
	}
	if len(mediaPaths) > 0 && c.voiceConfig.EchoTranscription {
		metadata["echo_transcription"] = "true"
	}

	chatType := "direct"
	if evt.Info.Chat.Server == types.GroupServer {
		chatType = "group"
	}
	messageID := evt.Info.ID
	sender := bus.SenderInfo{
		Platform:    "whatsapp",
		PlatformID:  senderID,
		CanonicalID: identity.BuildCanonicalID("whatsapp", senderID),
		DisplayName: evt.Info.PushName,
	}

	if !c.IsAllowedSender(sender) {
		logger.DebugCF(
			"whatsapp",
			"WhatsApp message blocked (not in allow_from)",
			map[string]any{"sender_id": senderID},
		)
		_, _ = c.Send(
			c.runCtx,
			bus.OutboundMessage{Channel: c.Name(), ChatID: chatID, Content: "You are not authorized to use this bot."},
		)
		return
	}

	isGroup := evt.Info.Chat.Server == types.GroupServer

	logger.DebugCF(
		"whatsapp",
		"WhatsApp message received",
		map[string]any{"sender_id": senderID, "content_preview": utils.Truncate(content, 50), "is_group": isGroup},
	)

	if isGroup {
		// Detect bot mention via ContextInfo.MentionedJID (populated for @mentions in groups).
		isMentioned := false
		c.mu.Lock()
		botJID := c.client.Store.ID
		c.mu.Unlock()
		var ctx2 *waE2E.ContextInfo
		if ext := evt.Message.GetExtendedTextMessage(); ext != nil {
			ctx2 = ext.GetContextInfo()
		}
		if ctx2 != nil && botJID != nil {
			botUser := botJID.User
			for _, jid := range ctx2.GetMentionedJID() {
				if strings.HasPrefix(jid, botUser+"@") || jid == botUser {
					isMentioned = true
					break
				}
			}
		}
		respond, cleaned := c.ShouldRespondInGroup(isMentioned, content)
		if !respond {
			return
		}
		content = cleaned
	}

	inboundCtx := bus.InboundContext{
		ChatType:  chatType,
		ChatID:    chatID,
		SenderID:  senderID,
		MessageID: messageID,
		Raw:       metadata,
	}
	c.HandleMessageWithContext(c.runCtx, chatID, content, mediaPaths, inboundCtx, sender)
}

// mediaFilenameAndMIME returns a safe filename and MIME type for the media
// contained in msg. It inspects sub-message types in the same order that
// whatsmeow's DownloadAny does.
func mediaFilenameAndMIME(msg *waE2E.Message) (filename, mimetype string) {
	if img := msg.GetImageMessage(); img != nil {
		mime := img.GetMimetype()
		if mime == "" {
			mime = "image/jpeg"
		}
		return "image.jpg", mime
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		mime := vid.GetMimetype()
		if mime == "" {
			mime = "video/mp4"
		}
		return "video.mp4", mime
	}
	if aud := msg.GetAudioMessage(); aud != nil {
		mime := aud.GetMimetype()
		if mime == "" {
			mime = "audio/ogg"
		}
		return "audio.ogg", mime
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		mime := doc.GetMimetype()
		if mime == "" {
			mime = "application/octet-stream"
		}
		name := doc.GetFileName()
		if name == "" {
			name = "document"
		}
		return utils.SanitizeFilename(name), mime
	}
	if stk := msg.GetStickerMessage(); stk != nil {
		mime := stk.GetMimetype()
		if mime == "" {
			mime = "image/webp"
		}
		return "sticker.webp", mime
	}
	return "media", "application/octet-stream"
}

// parseVCard extracts the first FN, TEL, EMAIL, and ORG values from a vCard string.
func parseVCard(vcard string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(vcard, "\n") {
		line = strings.TrimRight(line, "\r")
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		fieldSpec := strings.ToUpper(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if value == "" {
			continue
		}
		fieldName := strings.SplitN(fieldSpec, ";", 2)[0]
		switch fieldName {
		case "FN":
			if result["fn"] == "" {
				result["fn"] = value
			}
		case "TEL":
			if result["tel"] == "" {
				result["tel"] = value
			}
		case "EMAIL":
			if result["email"] == "" {
				result["email"] = value
			}
		case "ORG":
			if result["org"] == "" {
				result["org"] = value
			}
		}
	}
	return result
}

func formatContactContent(c *waE2E.ContactMessage) string {
	name := c.GetDisplayName()
	var parts []string
	if vc := c.GetVcard(); vc != "" {
		fields := parseVCard(vc)
		if fn := fields["fn"]; fn != "" {
			name = fn
		}
		if tel := fields["tel"]; tel != "" {
			parts = append(parts, "Phone: "+tel)
		}
		if email := fields["email"]; email != "" {
			parts = append(parts, "Email: "+email)
		}
		if org := fields["org"]; org != "" {
			parts = append(parts, "Organization: "+org)
		}
	}
	header := "[Contact Card: " + name + "]"
	if len(parts) == 0 {
		return header
	}
	return header + "\n" + strings.Join(parts, "\n")
}

func formatContactsArrayContent(ca *waE2E.ContactsArrayMessage) string {
	contacts := ca.GetContacts()
	if len(contacts) == 0 {
		return "[Contact Cards]"
	}
	lines := make([]string, 0, len(contacts)+1)
	lines = append(lines, fmt.Sprintf("[Contact Cards: %d]", len(contacts)))
	for i, c := range contacts {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, formatContactContent(c)))
	}
	return strings.Join(lines, "\n")
}

func (c *WhatsAppNativeChannel) Send(ctx context.Context, msg bus.OutboundMessage) ([]string, error) {
	if !c.IsRunning() {
		return nil, channels.ErrNotRunning
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	c.mu.Lock()
	client := c.client
	c.mu.Unlock()

	if client == nil || !client.IsConnected() {
		return nil, fmt.Errorf("whatsapp connection not established: %w", channels.ErrTemporary)
	}

	// Detect unpaired state: the client is connected (to WhatsApp servers)
	// but has not completed QR-login yet, so sending would fail.
	if client.Store.ID == nil {
		return nil, fmt.Errorf("whatsapp not yet paired (QR login pending): %w", channels.ErrTemporary)
	}

	to, err := parseJID(msg.ChatID)
	if err != nil {
		return nil, fmt.Errorf("invalid chat id %q: %w", msg.ChatID, err)
	}

	msg.Content = stripMarkdown(msg.Content)
	waMsg := buildOutboundProtoMessage(msg)

	if _, err = client.SendMessage(ctx, to, waMsg); err != nil {
		return nil, fmt.Errorf("whatsapp send: %w", channels.ErrTemporary)
	}
	return nil, nil
}

// buildOutboundProtoMessage converts an OutboundMessage into a whatsmeow proto
// message. When MIME-style metadata is present it constructs an interactive
// widget; otherwise it falls back to a plain Conversation message.
//
// Metadata schema:
//
//	Content-Type: "application/x-wa-buttons" — button widget (max 3)
//	Content-Type: "application/x-wa-list"    — list widget (any number of rows)
//	X-WA-Body:    body text shown above the options
//	X-WA-Option-N: individual option labels (0-indexed)
//
// When Content-Type is "application/x-wa-buttons" and more than 3 options are
// provided, the first 2 are kept and the rest are collapsed into a synthetic
// "Other (chat about this)" button, respecting WhatsApp's hard 3-button limit.
func buildOutboundProtoMessage(msg bus.OutboundMessage) *waE2E.Message {
	ct := msg.Context.Raw["Content-Type"]
	body := msg.Context.Raw["X-WA-Body"]
	if body == "" {
		body = msg.Content
	}

	var opts []string
	for i := 0; ; i++ {
		v, ok := msg.Context.Raw[fmt.Sprintf("X-WA-Option-%d", i)]
		if !ok {
			break
		}
		opts = append(opts, v)
	}

	switch ct {
	case "application/x-wa-buttons":
		if len(opts) == 0 {
			break
		}
		display := opts
		if len(display) > 3 {
			display = append(opts[:2:2], "Other (chat about this)")
		}
		buttons := make([]*waE2E.ButtonsMessage_Button, len(display))
		for i, label := range display {
			buttons[i] = &waE2E.ButtonsMessage_Button{
				ButtonID: proto.String(fmt.Sprintf("%d", i)),
				ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
					DisplayText: proto.String(label),
				},
				Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
			}
		}
		return &waE2E.Message{
			ButtonsMessage: &waE2E.ButtonsMessage{
				ContentText: proto.String(body),
				HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
				Buttons:     buttons,
			},
		}

	case "application/x-wa-list":
		if len(opts) == 0 {
			break
		}
		rows := make([]*waE2E.ListMessage_Row, len(opts))
		for i, label := range opts {
			// RowID is set to the label text so that an incoming
			// ListResponseMessage.SingleSelectReply.SelectedRowID directly
			// carries the user's choice as the message content.
			rows[i] = &waE2E.ListMessage_Row{
				RowID: proto.String(label),
				Title: proto.String(label),
			}
		}
		return &waE2E.Message{
			ListMessage: &waE2E.ListMessage{
				Title:      proto.String(body),
				ButtonText: proto.String("Select"),
				ListType:   waE2E.ListMessage_SINGLE_SELECT.Enum(),
				Sections: []*waE2E.ListMessage_Section{
					{Rows: rows},
				},
			},
		}
	}

	return &waE2E.Message{
		Conversation: proto.String(msg.Content),
	}
}

// parseJID converts a chat ID (phone number or JID string) to types.JID.
func parseJID(s string) (types.JID, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return types.JID{}, fmt.Errorf("empty chat id")
	}
	if strings.Contains(s, "@") {
		return types.ParseJID(s)
	}
	return types.NewJID(s, types.DefaultUserServer), nil
}
