package email

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	imap "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	gomail "github.com/emersion/go-message/mail"
	"golang.org/x/net/html"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/channels"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/logger"
	"github.com/sushi30/sushiclaw/pkg/media"
)

// EmailConfig holds configuration for the email channel.
// Loaded from the "channels.email" section of the picoclaw config JSON.
type EmailConfig struct {
	Enabled            bool                       `json:"enabled"`
	SMTPHost           string                     `json:"smtp_host"`
	SMTPPort           int                        `json:"smtp_port"`
	SMTPFrom           config.SecureString        `json:"smtp_from"`
	SMTPUser           config.SecureString        `json:"smtp_user"`
	SMTPPassword       config.SecureString        `json:"smtp_password"`
	DefaultSubject     string                     `json:"default_subject"`
	IMAPHost           string                     `json:"imap_host"`
	IMAPPort           int                        `json:"imap_port"`
	IMAPUser           config.SecureString        `json:"imap_user"`
	IMAPPassword       config.SecureString        `json:"imap_password"`
	PollIntervalSecs   int                        `json:"poll_interval_secs"`
	AllowFrom          config.FlexibleStringSlice `json:"allow_from"`
	ReasoningChannelID string                     `json:"reasoning_channel_id"`
}

// EmailChannel implements the Channel interface using SMTP (outbound) and IMAP polling (inbound).
type EmailChannel struct {
	*channels.BaseChannel
	config          EmailConfig
	ctx             context.Context
	cancel          context.CancelFunc
	tm              *ThreadManager
	tmMu            sync.RWMutex
	lastMsgByChatID sync.Map // chatID/fromAddr (string) → most recent inbound messageID (string)
}

// NewEmailChannel creates a new email channel.
func NewEmailChannel(cfg EmailConfig, messageBus *bus.MessageBus) (*EmailChannel, error) {
	if cfg.SMTPHost == "" {
		return nil, fmt.Errorf("email smtp_host is required")
	}
	if cfg.SMTPFrom.String() == "" {
		return nil, fmt.Errorf("email smtp_from is required")
	}
	if cfg.IMAPHost == "" {
		return nil, fmt.Errorf("email imap_host is required")
	}
	if cfg.IMAPUser.String() == "" {
		return nil, fmt.Errorf("email imap_user is required")
	}

	base := channels.NewBaseChannel("email", cfg, messageBus, cfg.AllowFrom,
		channels.WithReasoningChannelID(cfg.ReasoningChannelID),
	)

	return &EmailChannel{
		BaseChannel: base,
		config:      cfg,
		tm:          NewThreadManager(),
	}, nil
}

// Start begins IMAP polling.
func (c *EmailChannel) Start(ctx context.Context) error {
	logger.InfoC("email", "Starting email channel")
	c.ctx, c.cancel = context.WithCancel(ctx)

	interval := c.config.PollIntervalSecs
	if interval <= 0 {
		interval = 30
	}

	go c.pollLoop(time.Duration(interval) * time.Second)

	c.SetRunning(true)
	logger.InfoCF("email", "Email channel started", map[string]any{
		"smtp_host": c.config.SMTPHost,
		"imap_host": c.config.IMAPHost,
		"interval":  interval,
	})
	return nil
}

// Stop cancels the polling loop.
func (c *EmailChannel) Stop(ctx context.Context) error {
	logger.InfoC("email", "Stopping email channel")
	c.SetRunning(false)
	if c.cancel != nil {
		c.cancel()
	}
	logger.InfoC("email", "Email channel stopped")
	return nil
}

// Send delivers an outbound message via SMTP.
// msg.ChatID is the recipient email address.
func (c *EmailChannel) Send(ctx context.Context, msg bus.OutboundMessage) ([]string, error) {
	if !c.IsRunning() {
		return nil, channels.ErrNotRunning
	}

	to := msg.ChatID
	if to == "" {
		return nil, fmt.Errorf("chat ID (recipient address) is empty: %w", channels.ErrSendFailed)
	}
	if strings.TrimSpace(msg.Content) == "" {
		return nil, nil
	}

	subject := c.config.DefaultSubject
	if subject == "" {
		subject = "Message"
	}

	outboundMsgID := generateMessageID(extractDomain(c.config.SMTPFrom.String()))
	outboundMsgIDRaw := strings.Trim(outboundMsgID, "<>")

	// Resolve reply target: use explicit ReplyToMessageID or fall back to the last
	// inbound from this chat. The picoclaw framework's default response path
	// (PublishResponseIfNeeded) never sets ReplyToMessageID on outbound messages,
	// so the fallback is the primary way threading works in practice.
	replyToID := msg.ReplyToMessageID
	if replyToID == "" {
		if v, ok := c.lastMsgByChatID.Load(to); ok {
			replyToID = v.(string)
		}
	}

	var ancestorRefs []string
	if replyToID != "" {
		c.tmMu.RLock()
		node, hasNode := c.tm.AllMessages[replyToID]
		if hasNode {
			ancestorRefs = c.tm.ReferencesChain(replyToID)
			if !node.IsGhost && node.Subject != "" {
				// Subject in ThreadManager is already stripped of Re:/Fwd: prefixes.
				subject = "Re: " + node.Subject
			}
		}
		c.tmMu.RUnlock()
	}

	if replyToID == "" && len(outboundMsgIDRaw) >= 8 {
		subject = fmt.Sprintf("%s [%s]", subject, outboundMsgIDRaw[:8])
	}

	var h gomail.Header
	h.SetAddressList("From", []*gomail.Address{{Address: c.config.SMTPFrom.String()}})
	h.SetAddressList("To", []*gomail.Address{{Address: to}})
	h.SetSubject(subject)
	h.SetDate(time.Now())
	h.SetMessageID(outboundMsgIDRaw)

	if replyToID != "" {
		h.SetMsgIDList("In-Reply-To", []string{replyToID})
		allRefs := append(ancestorRefs, replyToID)
		h.SetMsgIDList("References", allRefs)
	}

	h.SetContentType("text/html", map[string]string{"charset": "utf-8"})
	htmlBody := markdownToHTML(msg.Content)

	var bodyBuf bytes.Buffer
	w, err := gomail.CreateSingleInlineWriter(&bodyBuf, h)
	if err != nil {
		return nil, fmt.Errorf("create mime writer: %w: %w", err, channels.ErrSendFailed)
	}
	if _, err := w.Write([]byte(htmlBody)); err != nil {
		return nil, fmt.Errorf("write mime body: %w: %w", err, channels.ErrSendFailed)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("close mime body: %w: %w", err, channels.ErrSendFailed)
	}

	smtpPort := c.config.SMTPPort
	if smtpPort == 0 {
		smtpPort = 587
	}

	addr := fmt.Sprintf("%s:%d", c.config.SMTPHost, smtpPort)
	smtpUser := c.config.SMTPUser.String()
	if smtpUser == "" {
		smtpUser = c.config.SMTPFrom.String()
	}

	var auth smtp.Auth
	if c.config.SMTPPassword.String() != "" {
		auth = smtp.PlainAuth("", smtpUser, c.config.SMTPPassword.String(), c.config.SMTPHost)
	}

	// Port 465 uses implicit TLS; port 587 and others use STARTTLS.
	if smtpPort == 465 {
		tlsCfg := &tls.Config{ServerName: c.config.SMTPHost}
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return nil, fmt.Errorf("smtp tls dial: %w", channels.ErrTemporary)
		}
		client, err := smtp.NewClient(conn, c.config.SMTPHost)
		if err != nil {
			return nil, fmt.Errorf("smtp new client: %w", channels.ErrTemporary)
		}
		defer func() { _ = client.Close() }()
		if err := sendViaSMTPClient(client, auth, c.config.SMTPFrom.String(), to, bodyBuf.Bytes()); err != nil {
			return nil, err
		}
	} else {
		if err := smtp.SendMail(addr, auth, c.config.SMTPFrom.String(), []string{to}, bodyBuf.Bytes()); err != nil {
			return nil, fmt.Errorf("smtp send: %w: %w", err, channels.ErrTemporary)
		}
	}

	// Register outbound message so future inbound replies can trace the full chain.
	var outboundRefsStr string
	if replyToID != "" {
		parts := make([]string, 0, len(ancestorRefs)+1)
		for _, r := range ancestorRefs {
			parts = append(parts, "<"+r+">")
		}
		parts = append(parts, "<"+replyToID+">")
		outboundRefsStr = strings.Join(parts, " ")
	}
	c.tmMu.Lock()
	c.tm.ProcessHeaders(outboundMsgIDRaw, subject, replyToID, outboundRefsStr)
	c.tmMu.Unlock()

	logger.DebugCF("email", "Message sent", map[string]any{"to": to})
	return nil, nil
}

// SendMedia implements the channels.MediaSender interface for sending attachments.
func (c *EmailChannel) SendMedia(ctx context.Context, msg bus.OutboundMediaMessage) ([]string, error) {
	if !c.IsRunning() {
		return nil, channels.ErrNotRunning
	}

	to := msg.ChatID
	if to == "" {
		return nil, fmt.Errorf("chat ID (recipient address) is empty: %w", channels.ErrSendFailed)
	}
	if len(msg.Parts) == 0 {
		return nil, nil
	}

	store := c.GetMediaStore()
	if store == nil {
		return nil, fmt.Errorf("no media store available: %w", channels.ErrSendFailed)
	}

	var attachments []attachmentInfo
	for _, part := range msg.Parts {
		localPath, err := store.Resolve(part.Ref)
		if err != nil {
			return nil, fmt.Errorf("resolve media ref %q: %w: %w", part.Ref, err, channels.ErrSendFailed)
		}
		data, err := os.ReadFile(localPath)
		if err != nil {
			return nil, fmt.Errorf("read attachment %q: %w: %w", localPath, err, channels.ErrSendFailed)
		}
		ct := part.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		filename := part.Filename
		if filename == "" {
			filename = filepath.Base(localPath)
		}
		attachments = append(attachments, attachmentInfo{
			filename:    filename,
			contentType: ct,
			data:        data,
		})
	}

	// Use the first non-empty caption as the email body.
	var body string
	for _, part := range msg.Parts {
		if part.Caption != "" {
			body = part.Caption
			break
		}
	}
	return nil, c.sendMultipartEmail(ctx, to, body, msg.Context.ReplyToMessageID, attachments)
}

// sendMultipartEmail builds and sends a multipart MIME message with HTML body and attachments.
func (c *EmailChannel) sendMultipartEmail(ctx context.Context, to, content, replyToID string, attachments []attachmentInfo) error {
	subject := c.config.DefaultSubject
	if subject == "" {
		subject = "Message"
	}

	outboundMsgID := generateMessageID(extractDomain(c.config.SMTPFrom.String()))
	outboundMsgIDRaw := strings.Trim(outboundMsgID, "<>")

	if replyToID == "" {
		if v, ok := c.lastMsgByChatID.Load(to); ok {
			replyToID = v.(string)
		}
	}

	var ancestorRefs []string
	if replyToID != "" {
		c.tmMu.RLock()
		node, hasNode := c.tm.AllMessages[replyToID]
		if hasNode {
			ancestorRefs = c.tm.ReferencesChain(replyToID)
			if !node.IsGhost && node.Subject != "" {
				subject = "Re: " + node.Subject
			}
		}
		c.tmMu.RUnlock()
	}

	if replyToID == "" && len(outboundMsgIDRaw) >= 8 {
		subject = fmt.Sprintf("%s [%s]", subject, outboundMsgIDRaw[:8])
	}

	var h gomail.Header
	h.SetAddressList("From", []*gomail.Address{{Address: c.config.SMTPFrom.String()}})
	h.SetAddressList("To", []*gomail.Address{{Address: to}})
	h.SetSubject(subject)
	h.SetDate(time.Now())
	h.SetMessageID(outboundMsgIDRaw)

	if replyToID != "" {
		h.SetMsgIDList("In-Reply-To", []string{replyToID})
		allRefs := append(ancestorRefs, replyToID)
		h.SetMsgIDList("References", allRefs)
	}

	var bodyBuf bytes.Buffer
	w, err := gomail.CreateWriter(&bodyBuf, h)
	if err != nil {
		return fmt.Errorf("create mime writer: %w: %w", err, channels.ErrSendFailed)
	}

	// Write HTML body part
	if strings.TrimSpace(content) != "" {
		var inlineH gomail.InlineHeader
		inlineH.SetContentType("text/html", map[string]string{"charset": "utf-8"})
		inlineW, err := w.CreateSingleInline(inlineH)
		if err != nil {
			_ = w.Close()
			return fmt.Errorf("create inline writer: %w: %w", err, channels.ErrSendFailed)
		}
		if _, err := inlineW.Write([]byte(markdownToHTML(content))); err != nil {
			_ = inlineW.Close()
			_ = w.Close()
			return fmt.Errorf("write html body: %w: %w", err, channels.ErrSendFailed)
		}
		if err := inlineW.Close(); err != nil {
			_ = w.Close()
			return fmt.Errorf("close inline writer: %w: %w", err, channels.ErrSendFailed)
		}
	}

	// Write attachment parts
	for _, att := range attachments {
		var attH gomail.AttachmentHeader
		attH.SetFilename(att.filename)
		attH.SetContentType(att.contentType, map[string]string{})
		attW, err := w.CreateAttachment(attH)
		if err != nil {
			_ = w.Close()
			return fmt.Errorf("create attachment writer: %w: %w", err, channels.ErrSendFailed)
		}
		if _, err := attW.Write(att.data); err != nil {
			_ = attW.Close()
			_ = w.Close()
			return fmt.Errorf("write attachment: %w: %w", err, channels.ErrSendFailed)
		}
		if err := attW.Close(); err != nil {
			_ = w.Close()
			return fmt.Errorf("close attachment writer: %w: %w", err, channels.ErrSendFailed)
		}
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("close mime writer: %w: %w", err, channels.ErrSendFailed)
	}

	smtpPort := c.config.SMTPPort
	if smtpPort == 0 {
		smtpPort = 587
	}

	addr := fmt.Sprintf("%s:%d", c.config.SMTPHost, smtpPort)
	smtpUser := c.config.SMTPUser.String()
	if smtpUser == "" {
		smtpUser = c.config.SMTPFrom.String()
	}

	var auth smtp.Auth
	if c.config.SMTPPassword.String() != "" {
		auth = smtp.PlainAuth("", smtpUser, c.config.SMTPPassword.String(), c.config.SMTPHost)
	}

	if smtpPort == 465 {
		tlsCfg := &tls.Config{ServerName: c.config.SMTPHost}
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("smtp tls dial: %w", channels.ErrTemporary)
		}
		client, err := smtp.NewClient(conn, c.config.SMTPHost)
		if err != nil {
			return fmt.Errorf("smtp new client: %w", channels.ErrTemporary)
		}
		defer func() { _ = client.Close() }()
		if err := sendViaSMTPClient(client, auth, c.config.SMTPFrom.String(), to, bodyBuf.Bytes()); err != nil {
			return err
		}
	} else {
		if err := smtp.SendMail(addr, auth, c.config.SMTPFrom.String(), []string{to}, bodyBuf.Bytes()); err != nil {
			return fmt.Errorf("smtp send: %w: %w", err, channels.ErrTemporary)
		}
	}

	var outboundRefsStr string
	if replyToID != "" {
		parts := make([]string, 0, len(ancestorRefs)+1)
		for _, r := range ancestorRefs {
			parts = append(parts, "<"+r+">")
		}
		parts = append(parts, "<"+replyToID+">")
		outboundRefsStr = strings.Join(parts, " ")
	}
	c.tmMu.Lock()
	c.tm.ProcessHeaders(outboundMsgIDRaw, subject, replyToID, outboundRefsStr)
	c.tmMu.Unlock()

	logger.DebugCF("email", "Media message sent", map[string]any{"to": to, "attachments": len(attachments)})
	return nil
}

func sendViaSMTPClient(client *smtp.Client, auth smtp.Auth, from, to string, body []byte) error {
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w: %w", err, channels.ErrSendFailed)
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w: %w", err, channels.ErrSendFailed)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp RCPT TO: %w: %w", err, channels.ErrSendFailed)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w: %w", err, channels.ErrSendFailed)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("smtp write body: %w: %w", err, channels.ErrSendFailed)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close data: %w: %w", err, channels.ErrSendFailed)
	}
	return client.Quit()
}

func (c *EmailChannel) pollLoop(interval time.Duration) {
	// Poll once immediately on start, then on ticker.
	c.pollIMAP()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.pollIMAP()
		}
	}
}

func (c *EmailChannel) pollIMAP() {
	imapPort := c.config.IMAPPort
	if imapPort == 0 {
		imapPort = 993
	}
	addr := fmt.Sprintf("%s:%d", c.config.IMAPHost, imapPort)

	var (
		client *imapclient.Client
		err    error
	)

	if imapPort == 993 {
		tlsCfg := &tls.Config{ServerName: c.config.IMAPHost}
		client, err = imapclient.DialTLS(addr, &imapclient.Options{TLSConfig: tlsCfg})
	} else {
		client, err = imapclient.DialInsecure(addr, nil)
	}
	if err != nil {
		logger.WarnCF("email", "IMAP dial failed", map[string]any{"err": err})
		return
	}
	defer func() { _ = client.Close() }()

	if err = client.Login(c.config.IMAPUser.String(), c.config.IMAPPassword.String()).Wait(); err != nil {
		logger.WarnCF("email", "IMAP login failed", map[string]any{"err": err})
		return
	}

	if _, err = client.Select("INBOX", nil).Wait(); err != nil {
		logger.WarnCF("email", "IMAP SELECT INBOX failed", map[string]any{"err": err})
		return
	}

	searchData, err := client.Search(&imap.SearchCriteria{
		NotFlag: []imap.Flag{imap.FlagSeen},
	}, nil).Wait()
	if err != nil {
		logger.WarnCF("email", "IMAP SEARCH failed", map[string]any{"err": err})
		return
	}

	unseenCount := len(searchData.AllSeqNums())
	logger.DebugCF("email", "IMAP poll complete", map[string]any{"unseen": unseenCount})
	if unseenCount == 0 {
		return
	}

	seqSet := imap.SeqSetNum(searchData.AllSeqNums()...)
	bodySection := &imap.FetchItemBodySection{}
	fetchOptions := &imap.FetchOptions{
		Envelope:    true,
		BodySection: []*imap.FetchItemBodySection{bodySection},
	}

	fetchCmd := client.Fetch(seqSet, fetchOptions)

	for {
		msg := fetchCmd.Next()
		if msg == nil {
			break
		}

		var (
			envelope  *imap.Envelope
			bodyBytes []byte
			seqNum    uint32
		)

		seqNum = msg.SeqNum

		for {
			item := msg.Next()
			if item == nil {
				break
			}
			switch v := item.(type) {
			case imapclient.FetchItemDataEnvelope:
				envelope = v.Envelope
			case imapclient.FetchItemDataBodySection:
				if v.Literal != nil {
					bodyBytes, _ = io.ReadAll(v.Literal)
				}
			}
		}

		if envelope == nil || bodyBytes == nil {
			logger.DebugCF("email", "Skipping IMAP message: missing envelope or body", map[string]any{"seq": seqNum, "has_envelope": envelope != nil, "has_body": len(bodyBytes) > 0})
			continue
		}
		logger.DebugCF("email", "Processing IMAP message", map[string]any{"seq": seqNum, "from": extractFrom(envelope), "subject": envelope.Subject, "size": len(bodyBytes)})
		processed, _ := c.processEmail(c.ctx, envelope, bytes.NewReader(bodyBytes))
		if !processed {
			logger.DebugCF("email", "IMAP message skipped by processEmail", map[string]any{"seq": seqNum, "from": extractFrom(envelope), "subject": envelope.Subject})
			continue
		}

		// Mark message as \Seen
		storeSeq := imap.SeqSetNum(seqNum)
		storeFlags := imap.StoreFlags{
			Op:     imap.StoreFlagsAdd,
			Flags:  []imap.Flag{imap.FlagSeen},
			Silent: true,
		}
		if err := client.Store(storeSeq, &storeFlags, nil).Close(); err != nil {
			logger.WarnCF("email", "IMAP STORE \\Seen failed", map[string]any{"err": err, "seq": seqNum})
		}
	}

	if err := fetchCmd.Close(); err != nil {
		logger.WarnCF("email", "IMAP FETCH close error", map[string]any{"err": err})
	}
}

func (c *EmailChannel) processEmail(ctx context.Context, envelope *imap.Envelope, bodyLiteral io.Reader) (bool, string) {
	fromAddr := extractFrom(envelope)
	if fromAddr == "" {
		logger.DebugCF("email", "processEmail skipped: empty sender", map[string]any{"subject": envelope.Subject})
		return false, ""
	}

	raw, err := io.ReadAll(bodyLiteral)
	if err != nil {
		logger.DebugCF("email", "processEmail skipped: read error", map[string]any{"err": err.Error()})
		return false, ""
	}

	if isCalendarRSVP(envelope, raw) {
		logger.InfoCF("email", "Skipping calendar RSVP", map[string]any{"subject": envelope.Subject})
		return true, ""
	}

	plainText, references := extractBodyParts(bytes.NewReader(raw))
	attachments := extractAttachments(bytes.NewReader(raw))

	// Allow emails with only attachments (no text body) to be processed.
	if strings.TrimSpace(plainText) == "" && len(attachments) == 0 {
		logger.DebugCF("email", "processEmail skipped: empty body and no attachments", map[string]any{"from": fromAddr, "subject": envelope.Subject})
		return false, ""
	}

	logger.InfoCF("email", "Email received", map[string]any{
		"from":        fromAddr,
		"subject":     envelope.Subject,
		"attachments": len(attachments),
	})

	metadata := map[string]string{}
	if envelope.MessageID != "" {
		rawID := strings.Trim(envelope.MessageID, "<>")

		inReplyTo := ""
		if len(envelope.InReplyTo) > 0 {
			inReplyTo = strings.Trim(envelope.InReplyTo[0], "<> ")
		}

		c.tmMu.Lock()
		c.tm.ProcessHeaders(rawID, envelope.Subject, inReplyTo, references)
		c.tmMu.Unlock()

		c.lastMsgByChatID.Store(fromAddr, rawID)
		metadata["reply_to_message_id"] = rawID
	}

	var mediaPaths []string
	if store := c.GetMediaStore(); store != nil && len(attachments) > 0 {
		if err := os.MkdirAll(media.TempDir(), 0o700); err != nil {
			logger.WarnCF("email", "Failed to create media temp dir", map[string]any{
				"err": err.Error(),
				"dir": media.TempDir(),
			})
		} else {
			scope := channels.BuildMediaScope("email", fromAddr, envelope.MessageID)
			for _, att := range attachments {
				localPath := filepath.Join(media.TempDir(), uuid.New().String()[:8]+"_"+att.filename)
				if err := os.WriteFile(localPath, att.data, 0o600); err != nil {
					logger.WarnCF("email", "Failed to write attachment", map[string]any{
						"err":      err.Error(),
						"filename": att.filename,
					})
					continue
				}
				ref, storeErr := store.Store(localPath, media.MediaMeta{
					Filename:      att.filename,
					ContentType:   att.contentType,
					Source:        "email",
					CleanupPolicy: media.CleanupPolicyDeleteOnCleanup,
				}, scope)
				if storeErr != nil {
					logger.WarnCF("email", "Failed to store attachment", map[string]any{
						"err":      storeErr.Error(),
						"filename": att.filename,
					})
					continue
				}
			mediaPaths = append(mediaPaths, ref)
			}
		}
	}

	sender := bus.SenderInfo{
		Platform:    "email",
		PlatformID:  fromAddr,
		CanonicalID: "email:" + fromAddr,
		DisplayName: displayName(envelope),
	}

	c.HandleMessageWithContext(ctx,
		fromAddr,
		plainText,
		mediaPaths,
		bus.InboundContext{
			ChatType:  "direct",
			ChatID:    fromAddr,
			SenderID:  fromAddr,
			MessageID: envelope.MessageID,
			Raw:       metadata,
		},
		sender,
	)

	return true, plainText
}

// generateMessageID creates a unique RFC 5322 Message-ID in the form <hex@domain>.
func generateMessageID(domain string) string {
	var buf [16]byte
	_, _ = rand.Read(buf[:])
	return "<" + hex.EncodeToString(buf[:]) + "@" + domain + ">"
}

// extractDomain returns the domain part of an email address (after @).
func extractDomain(addr string) string {
	if idx := strings.LastIndex(addr, "@"); idx >= 0 {
		return addr[idx+1:]
	}
	return addr
}

func extractFrom(env *imap.Envelope) string {
	if len(env.From) == 0 {
		return ""
	}
	addr := env.From[0]
	if addr.Host == "" {
		return addr.Mailbox
	}
	return addr.Mailbox + "@" + addr.Host
}

func displayName(env *imap.Envelope) string {
	if len(env.From) == 0 {
		return ""
	}
	if env.From[0].Name != "" {
		return env.From[0].Name
	}
	return extractFrom(env)
}

// extractBodyParts reads the MIME message and returns the plain-text body and
// the value of the References header (space-separated Message-IDs).
// If only HTML is available it falls back to stripping the HTML for the text.
func extractBodyParts(r io.Reader) (text, references string) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return "", ""
	}

	mr, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return strings.TrimSpace(string(raw)), ""
	}

	// Extract References from the top-level message header.
	references, _ = mr.Header.Text("References")

	var htmlFallback string

	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		inlineHeader, ok := p.Header.(*gomail.InlineHeader)
		if !ok {
			continue
		}
		ct, _, _ := inlineHeader.ContentType()
		if ct == "text/plain" || ct == "" {
			b, _ := io.ReadAll(p.Body)
			t := strings.TrimSpace(string(b))
			if t != "" {
				return t, references
			}
			continue
		}
		if ct == "text/html" && htmlFallback == "" {
			b, _ := io.ReadAll(p.Body)
			htmlFallback = stripHTMLText(string(b))
		}
	}

	if htmlFallback != "" {
		return htmlFallback, references
	}

	return "", references
}

// attachmentInfo holds a parsed email attachment before storage.
type attachmentInfo struct {
	filename    string
	contentType string
	data        []byte
}

// extractAttachments walks the MIME message and returns all attachment parts.
func extractAttachments(r io.Reader) []attachmentInfo {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil
	}

	mr, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return nil
	}

	var attachments []attachmentInfo

	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		attachHeader, ok := p.Header.(*gomail.AttachmentHeader)
		if !ok {
			continue
		}
		filename, _ := attachHeader.Filename()
		if filename == "" {
			ct, _, _ := attachHeader.ContentType()
			filename = fallbackFilename(ct)
		}
		data, _ := io.ReadAll(p.Body)
		if len(data) == 0 {
			continue
		}
		ct, _, _ := attachHeader.ContentType()
		if ct == "" {
			ct = "application/octet-stream"
		}
		attachments = append(attachments, attachmentInfo{
			filename:    filename,
			contentType: ct,
			data:        data,
		})
	}

	return attachments
}

// fallbackFilename generates a filename from a MIME type when none is provided.
func fallbackFilename(ct string) string {
	ext := "bin"
	switch ct {
	case "text/plain":
		ext = "txt"
	case "text/html":
		ext = "html"
	case "image/jpeg", "image/jpg":
		ext = "jpg"
	case "image/png":
		ext = "png"
	case "image/gif":
		ext = "gif"
	case "image/webp":
		ext = "webp"
	case "application/pdf":
		ext = "pdf"
	case "application/zip":
		ext = "zip"
	case "application/json":
		ext = "json"
	}
	return fmt.Sprintf("attachment.%s", ext)
}

func isCalendarRSVP(envelope *imap.Envelope, raw []byte) bool {
	rsvpPrefixes := []string{"Accepted: ", "Declined: ", "Tentative: ", "Invitation: "}
	for _, prefix := range rsvpPrefixes {
		if strings.HasPrefix(envelope.Subject, prefix) {
			return true
		}
	}
	mr, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return false
	}
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		var ct string
		if h, ok := p.Header.(*gomail.InlineHeader); ok {
			ct, _, _ = h.ContentType()
		} else if h, ok := p.Header.(*gomail.AttachmentHeader); ok {
			ct, _, _ = h.ContentType()
		}
		if ct == "text/calendar" || ct == "application/ics" {
			return true
		}
	}
	return false
}

func stripHTMLText(src string) string {
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return strings.TrimSpace(src)
	}

	var b strings.Builder
	var walk func(*html.Node, bool)
	walk = func(n *html.Node, hidden bool) {
		if n == nil {
			return
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "head":
				hidden = true
			case "br", "p", "div", "li", "tr", "td", "th":
				b.WriteByte(' ')
			}
		}
		if n.Type == html.TextNode && !hidden {
			b.WriteString(n.Data)
			b.WriteByte(' ')
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child, hidden)
		}
	}
	walk(doc, false)

	return strings.Join(strings.Fields(b.String()), " ")
}
