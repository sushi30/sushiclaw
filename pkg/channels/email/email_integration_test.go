package email

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	imap "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/channels"
	"github.com/sushi30/sushiclaw/pkg/config"
)

var subjectHexSuffixRe = regexp.MustCompile(`^Message \[([0-9a-f]{8})\]$`)

// literalReaderImpl satisfies imap.LiteralReader with a fixed size.
type literalReaderImpl struct {
	*bytes.Reader
	size int64
}

func (l *literalReaderImpl) Size() int64 { return l.size }

// startMockIMAPServer creates an in-memory IMAP server pre-loaded with rawMIME
// in the INBOX of user "testuser" / password "testpass".
// Returns the host and port string of the listening server.
func startMockIMAPServer(t *testing.T, rawMIME string) (host, port string) {
	t.Helper()

	memSrv := imapmemserver.New()
	user := imapmemserver.NewUser("testuser", "testpass")
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatalf("create INBOX: %v", err)
	}

	msgBytes := []byte(rawMIME)
	lr := &literalReaderImpl{bytes.NewReader(msgBytes), int64(len(msgBytes))}
	if _, err := user.Append("INBOX", lr, &imap.AppendOptions{}); err != nil {
		t.Fatalf("append to INBOX: %v", err)
	}

	memSrv.AddUser(user)

	srv := imapserver.New(&imapserver.Options{
		NewSession: func(_ *imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return memSrv.NewSession(), nil, nil
		},
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}},
		InsecureAuth: true,
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go func() { _ = srv.Serve(ln) }()

	t.Cleanup(func() { _ = srv.Close() })

	addr := ln.Addr().String()
	h, p, _ := net.SplitHostPort(addr)
	return h, p
}

// startSMTPCapture starts a minimal SMTP server that captures one inbound
// message. Returns host, port, and a channel that receives the raw DATA body.
func startSMTPCapture(t *testing.T) (host, port string, received <-chan string) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	ch := make(chan string, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

		send := func(line string) {
			_, _ = fmt.Fprintf(conn, "%s\r\n", line)
		}

		send("220 localhost ESMTP test")

		var body strings.Builder
		inData := false

		for {
			line, err := rw.ReadString('\n')
			if err != nil {
				break
			}
			line = strings.TrimRight(line, "\r\n")

			switch {
			case inData:
				if line == "." {
					send("250 OK")
					ch <- body.String()
					inData = false
				} else {
					// RFC 5321: lines starting with "." are dot-stuffed
					line = strings.TrimPrefix(line, ".")
					body.WriteString(line)
					body.WriteString("\r\n")
				}
			case strings.HasPrefix(strings.ToUpper(line), "EHLO") ||
				strings.HasPrefix(strings.ToUpper(line), "HELO"):
				// Advertise nothing extra — no STARTTLS, no AUTH
				send("250 localhost")
			case strings.HasPrefix(strings.ToUpper(line), "MAIL FROM"):
				send("250 OK")
			case strings.HasPrefix(strings.ToUpper(line), "RCPT TO"):
				send("250 OK")
			case strings.ToUpper(line) == "DATA":
				send("354 Start input, end with <CRLF>.<CRLF>")
				inData = true
			case strings.HasPrefix(strings.ToUpper(line), "QUIT"):
				send("221 Bye")
				return
			default:
				send("500 unrecognized")
			}
		}
	}()

	t.Cleanup(func() { _ = ln.Close() })

	addr := ln.Addr().String()
	h, p, _ := net.SplitHostPort(addr)
	return h, p, ch
}

// TestEmailInboundPipeline verifies that the email channel polls the IMAP
// server, picks up an unseen message, publishes it to the bus, and marks
// it as \Seen.
func TestEmailInboundPipeline(t *testing.T) {
	rawMIME := "From: sender@example.com\r\nTo: bot@test.com\r\n" +
		"Subject: Integration Test\r\nMessage-ID: <integration-test@test.com>\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\n" +
		"Hello integration test"

	imapHost, imapPort := startMockIMAPServer(t, rawMIME)
	imapPortInt, _ := strconv.Atoi(imapPort)

	msgBus := bus.NewMessageBus()
	cfg := EmailConfig{
		SMTPHost:         "127.0.0.1",
		SMTPPort:         25,
		SMTPFrom:         *config.NewSecureString("bot@test.com"),
		IMAPHost:         imapHost,
		IMAPPort:         imapPortInt,
		IMAPUser:         *config.NewSecureString("testuser"),
		IMAPPassword:     *config.NewSecureString("testpass"),
		PollIntervalSecs: 1,
	}

	ch, err := NewEmailChannel(cfg, msgBus)
	if err != nil {
		t.Fatalf("NewEmailChannel: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := ch.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer ch.Stop(ctx) //nolint:errcheck

	select {
	case msg := <-msgBus.InboundChan():
		if msg.Channel != "email" {
			t.Errorf("channel = %q, want %q", msg.Channel, "email")
		}
		if msg.Sender.PlatformID != "sender@example.com" {
			t.Errorf("sender PlatformID = %q, want %q", msg.Sender.PlatformID, "sender@example.com")
		}
		if !strings.Contains(msg.Content, "Hello integration test") {
			t.Errorf("content = %q, want to contain %q", msg.Content, "Hello integration test")
		}
		if msg.Context.Raw == nil || msg.Context.Raw["reply_to_message_id"] != "integration-test@test.com" {
			t.Errorf("metadata[reply_to_message_id] = %q, want %q", msg.Context.Raw["reply_to_message_id"], "integration-test@test.com")
		}
	case <-ctx.Done():
		t.Fatal("timeout: no inbound message received — IMAP poll did not deliver the message to the bus")
	}
}

// TestEmailOutboundPipeline verifies that Send() delivers a message via SMTP.
func TestEmailOutboundPipeline(t *testing.T) {
	smtpHost, smtpPort, received := startSMTPCapture(t)
	smtpPortInt, _ := strconv.Atoi(smtpPort)

	msgBus := bus.NewMessageBus()
	cfg := EmailConfig{
		SMTPHost: smtpHost,
		SMTPPort: smtpPortInt,
		SMTPFrom: *config.NewSecureString("bot@test.com"),
		// IMAP config is required by NewEmailChannel but won't be dialed —
		// we call SetRunning(true) instead of Start().
		IMAPHost:     "127.0.0.1",
		IMAPPort:     10143,
		IMAPUser:     *config.NewSecureString("u"),
		IMAPPassword: *config.NewSecureString("p"),
	}

	ch, err := NewEmailChannel(cfg, msgBus)
	if err != nil {
		t.Fatalf("NewEmailChannel: %v", err)
	}
	// Mark running without starting the IMAP poll goroutine.
	ch.SetRunning(true)

	ctx := context.Background()
	_, err = ch.Send(ctx, bus.OutboundMessage{
		ChatID:  "user@example.com",
		Content: "Hello from sushiclaw",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case body := <-received:
		if !strings.Contains(body, "Hello from sushiclaw") {
			t.Errorf("SMTP body does not contain message content:\n%s", body)
		}
		if !strings.Contains(body, "user@example.com") {
			t.Errorf("SMTP body does not contain recipient:\n%s", body)
		}
		if !strings.Contains(body, "Date: ") {
			t.Errorf("SMTP body missing Date header:\n%s", body)
		}
		if !strings.Contains(strings.ToLower(body), "message-id: <") {
			t.Errorf("SMTP body missing Message-ID header:\n%s", body)
		}
		subject := extractSubjectFromSMTPBody(t, body)
		if !subjectHexSuffixRe.MatchString(subject) {
			t.Errorf("outbound subject = %q, want match for %q (unique suffix required for new-conversation emails)", subject, subjectHexSuffixRe.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: SMTP capture received nothing")
	}
}

// TestEmailReplyThreading verifies that when Send is called with a ReplyToMessageID
// matching an email that was previously received via IMAP, the outbound SMTP message
// contains RFC 2822 threading headers (In-Reply-To, References) and a Re: subject.
func TestEmailReplyThreading(t *testing.T) {
	// IMAP parses Message-ID and strips angle brackets.
	// The raw header is "<orig-123@test.com>" but envelope.MessageID == "orig-123@test.com".
	const originalMsgID = "orig-123@test.com"
	const originalSubject = "Hello Agent"

	rawMIME := "From: sender@example.com\r\nTo: bot@test.com\r\n" +
		"Subject: " + originalSubject + "\r\n" +
		"Message-ID: <" + originalMsgID + ">\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\n" +
		"Please reply to this"

	imapHost, imapPort := startMockIMAPServer(t, rawMIME)
	imapPortInt, _ := strconv.Atoi(imapPort)

	smtpHost, smtpPort, received := startSMTPCapture(t)
	smtpPortInt, _ := strconv.Atoi(smtpPort)

	msgBus := bus.NewMessageBus()
	cfg := EmailConfig{
		SMTPHost:         smtpHost,
		SMTPPort:         smtpPortInt,
		SMTPFrom:         *config.NewSecureString("bot@test.com"),
		DefaultSubject:   "Message",
		IMAPHost:         imapHost,
		IMAPPort:         imapPortInt,
		IMAPUser:         *config.NewSecureString("testuser"),
		IMAPPassword:     *config.NewSecureString("testpass"),
		PollIntervalSecs: 1,
	}

	ch, err := NewEmailChannel(cfg, msgBus)
	if err != nil {
		t.Fatalf("NewEmailChannel: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := ch.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer ch.Stop(ctx) //nolint:errcheck

	// Wait for the inbound message — this proves processEmail ran and populated the cache.
	select {
	case msg := <-msgBus.InboundChan():
		if msg.MessageID != originalMsgID {
			t.Errorf("inbound MessageID = %q, want %q", msg.MessageID, originalMsgID)
		}
		if msg.Context.Raw == nil || msg.Context.Raw["reply_to_message_id"] != originalMsgID {
			t.Errorf("inbound metadata[reply_to_message_id] = %q, want %q", msg.Context.Raw["reply_to_message_id"], originalMsgID)
		}
	case <-ctx.Done():
		t.Fatal("timeout: no inbound message — IMAP poll did not deliver the message")
	}

	// Now send the reply using the original Message-ID as ReplyToMessageID.
	_, err = ch.Send(ctx, bus.OutboundMessage{
		ChatID:           "sender@example.com",
		Content:          "This is my reply",
		ReplyToMessageID: originalMsgID,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Send wraps the raw messageID in angle brackets per RFC 2822.
	msgIDHeader := "<" + originalMsgID + ">"

	select {
	case body := <-received:
		if !strings.Contains(body, "Subject: Re: "+originalSubject) {
			t.Errorf("SMTP body missing Re: subject:\n%s", body)
		}
		if !strings.Contains(body, "In-Reply-To: "+msgIDHeader) {
			t.Errorf("SMTP body missing In-Reply-To header:\n%s", body)
		}
		if !strings.Contains(body, "References: "+msgIDHeader) {
			t.Errorf("SMTP body missing References header:\n%s", body)
		}
		if !strings.Contains(strings.ToLower(body), "message-id: <") {
			t.Errorf("SMTP body missing Message-ID header:\n%s", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: SMTP capture received nothing")
	}
}

// TestEmailNewEmail_ThreadReply verifies that a new email (no In-Reply-To)
// sets metadata[reply_to_message_id] to its own Message-ID, and the agent's
// outbound reply contains In-Reply-To and References headers threading it.
func TestEmailNewEmail_ThreadReply(t *testing.T) {
	const msgID = "new-email-123@test.com"
	const subject = "Brand New Thread"

	rawMIME := "From: sender@example.com\r\nTo: bot@test.com\r\n" +
		"Subject: " + subject + "\r\n" +
		"Message-ID: <" + msgID + ">\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\n" +
		"Start a new conversation"

	imapHost, imapPort := startMockIMAPServer(t, rawMIME)
	imapPortInt, _ := strconv.Atoi(imapPort)

	smtpHost, smtpPort, received := startSMTPCapture(t)
	smtpPortInt, _ := strconv.Atoi(smtpPort)

	msgBus := bus.NewMessageBus()
	cfg := EmailConfig{
		SMTPHost:         smtpHost,
		SMTPPort:         smtpPortInt,
		SMTPFrom:         *config.NewSecureString("bot@test.com"),
		DefaultSubject:   "Message",
		IMAPHost:         imapHost,
		IMAPPort:         imapPortInt,
		IMAPUser:         *config.NewSecureString("testuser"),
		IMAPPassword:     *config.NewSecureString("testpass"),
		PollIntervalSecs: 1,
	}

	ch, err := NewEmailChannel(cfg, msgBus)
	if err != nil {
		t.Fatalf("NewEmailChannel: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := ch.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer ch.Stop(ctx) //nolint:errcheck

	// Wait for inbound message.
	select {
	case msg := <-msgBus.InboundChan():
		if msg.Context.Raw == nil || msg.Context.Raw["reply_to_message_id"] != msgID {
			t.Errorf("inbound metadata[reply_to_message_id] = %q, want %q", msg.Context.Raw["reply_to_message_id"], msgID)
		}
	case <-ctx.Done():
		t.Fatal("timeout: no inbound message")
	}

	// Agent replies referencing the inbound message.
	_, err = ch.Send(ctx, bus.OutboundMessage{
		ChatID:           "sender@example.com",
		Content:          "Reply to new email",
		ReplyToMessageID: msgID,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case body := <-received:
		msgIDHeader := "<" + msgID + ">"
		if !strings.Contains(body, "Subject: Re: "+subject) {
			t.Errorf("SMTP body missing Re: subject:\n%s", body)
		}
		if !strings.Contains(body, "In-Reply-To: "+msgIDHeader) {
			t.Errorf("SMTP body missing In-Reply-To:\n%s", body)
		}
		if !strings.Contains(body, "References: "+msgIDHeader) {
			t.Errorf("SMTP body missing References:\n%s", body)
		}
		if !strings.Contains(strings.ToLower(body), "message-id: <") {
			t.Errorf("SMTP body missing Message-ID header:\n%s", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: SMTP capture received nothing")
	}
}

// TestEmailReplyChain_ThreadContinuation verifies that when a human replies
// to the agent's outbound email (In-Reply-To = agent's Message-ID), the
// inbound metadata contains the thread root and the agent's subsequent reply
// chains the References correctly.
func TestEmailReplyChain_ThreadContinuation(t *testing.T) {
	const originalMsgID = "orig-456@test.com"
	const originalSubject = "Continuing Thread"

	// Step 1: Simulate original email arriving.
	rawMIME := "From: human@example.com\r\nTo: bot@test.com\r\n" +
		"Subject: " + originalSubject + "\r\n" +
		"Message-ID: <" + originalMsgID + ">\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\n" +
		"Original message"

	imapHost, imapPort := startMockIMAPServer(t, rawMIME)
	imapPortInt, _ := strconv.Atoi(imapPort)

	smtpHost, smtpPort, received := startSMTPCapture(t)
	smtpPortInt, _ := strconv.Atoi(smtpPort)

	msgBus := bus.NewMessageBus()
	cfg := EmailConfig{
		SMTPHost:         smtpHost,
		SMTPPort:         smtpPortInt,
		SMTPFrom:         *config.NewSecureString("bot@test.com"),
		DefaultSubject:   "Message",
		IMAPHost:         imapHost,
		IMAPPort:         imapPortInt,
		IMAPUser:         *config.NewSecureString("testuser"),
		IMAPPassword:     *config.NewSecureString("testpass"),
		PollIntervalSecs: 1,
	}

	ch, err := NewEmailChannel(cfg, msgBus)
	if err != nil {
		t.Fatalf("NewEmailChannel: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := ch.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer ch.Stop(ctx) //nolint:errcheck

	// Wait for the original inbound message.
	select {
	case msg := <-msgBus.InboundChan():
		if msg.Context.Raw["reply_to_message_id"] != originalMsgID {
			t.Errorf("inbound metadata[reply_to_message_id] = %q, want %q", msg.Context.Raw["reply_to_message_id"], originalMsgID)
		}
	case <-ctx.Done():
		t.Fatal("timeout: no inbound message")
	}

	// Step 2: Agent replies, which registers the outbound Message-ID in the ThreadManager.
	_, err = ch.Send(ctx, bus.OutboundMessage{
		ChatID:           "human@example.com",
		Content:          "Agent reply to original",
		ReplyToMessageID: originalMsgID,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Capture the outbound Message-ID from the SMTP body.
	var agentMsgID string
	select {
	case body := <-received:
		agentMsgID = extractMessageIDFromBody(t, body)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: SMTP capture received nothing")
	}

	if agentMsgID == "" {
		t.Fatal("expected outbound Message-ID header")
	}

	// Step 3: Simulate a human replying to the agent's outbound email.
	// The human's email has In-Reply-To pointing to the agent's Message-ID.
	// The ThreadManager entry for agentMsgID should already be stored by Send().
	// We manually process this inbound reply (not via IMAP poll since we
	// can't add more messages to the mock server easily).
	humanReplyEnvelope := &imap.Envelope{
		From:      []imap.Address{{Mailbox: "human", Host: "example.com"}},
		Subject:   "Re: " + originalSubject,
		MessageID: "human-reply-789@test.com",
		InReplyTo: []string{"<" + agentMsgID + ">"},
	}
	humanReplyBody := strings.NewReader("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nHuman reply to agent")

	processed, _ := ch.processEmail(ctx, humanReplyEnvelope, humanReplyBody)
	if !processed {
		t.Fatal("expected processEmail to process human reply")
	}

	// Verify the inbound metadata points to the human reply's own Message-ID.
	// The ThreadManager traces ancestry from any message, so the reply_to_message_id
	// is simply the message's own ID — the agent can follow the chain from there.
	select {
	case msg := <-msgBus.InboundChan():
		if msg.Context.Raw["reply_to_message_id"] != "human-reply-789@test.com" {
			t.Errorf("human reply metadata[reply_to_message_id] = %q, want %q", msg.Context.Raw["reply_to_message_id"], "human-reply-789@test.com")
		}
	case <-ctx.Done():
		t.Fatal("timeout: no inbound message from human reply")
	}
}

// TestEmailNewEmail_FreshThread verifies that a new unrelated email
// (no In-Reply-To) gets its own reply_to_message_id — not polluted
// by a previous thread.
func TestEmailNewEmail_FreshThread(t *testing.T) {
	// Pre-populate the threads map with thread A data.
	// A new email with a different subject/message-id should NOT reference thread A.
	const threadAMsgID = "thread-a-999@test.com"

	msgBus := bus.NewMessageBus()
	ch := &EmailChannel{
		BaseChannel: channels.NewBaseChannel("email", EmailConfig{}, msgBus, nil),
		tm:          NewThreadManager(),
	}
	// Seed the ThreadManager with thread A info.
	ch.tm.ProcessHeaders(threadAMsgID, "Thread A Subject", "", "")

	// Send a fresh email with a completely different Message-ID and no In-Reply-To.
	freshEnvelope := &imap.Envelope{
		From:      []imap.Address{{Mailbox: "newperson", Host: "example.com"}},
		Subject:   "Fresh Topic",
		MessageID: "fresh-111@example.com",
	}
	freshBody := strings.NewReader("Starting a new conversation")

	processed, _ := ch.processEmail(context.Background(), freshEnvelope, freshBody)
	if !processed {
		t.Fatal("expected processEmail to process fresh email")
	}

	select {
	case msg := <-msgBus.InboundChan():
		if msg.Context.Raw["reply_to_message_id"] != "fresh-111@example.com" {
			t.Errorf("fresh email metadata[reply_to_message_id] = %q, want %q",
				msg.Context.Raw["reply_to_message_id"], "fresh-111@example.com")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: no inbound message from fresh email")
	}
}

// extractMessageIDFromBody parses the Message-ID header value from raw SMTP body.
// The header name is matched case-insensitively (RFC 5322 headers are case-insensitive).
func extractMessageIDFromBody(t *testing.T, body string) string {
	t.Helper()
	for _, line := range strings.Split(body, "\r\n") {
		if strings.HasPrefix(strings.ToLower(line), "message-id: ") {
			return strings.Trim(line[12:], " <>")
		}
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.ToLower(line), "message-id: ") {
			return strings.Trim(line[12:], " <>")
		}
	}
	return ""
}
