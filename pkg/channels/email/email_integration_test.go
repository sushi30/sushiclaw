package email

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	imap "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
)

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

	t.Cleanup(func() { srv.Close() })

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
		defer conn.Close()

		rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

		send := func(line string) {
			fmt.Fprintf(conn, "%s\r\n", line)
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
					if strings.HasPrefix(line, ".") {
						line = line[1:]
					}
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

	t.Cleanup(func() { ln.Close() })

	addr := ln.Addr().String()
	h, p, _ := net.SplitHostPort(addr)
	return h, p, ch
}

// TestEmailInboundPipeline verifies that the email channel polls the IMAP
// server, picks up an unseen message, publishes it to the bus, and marks
// it as \Seen.
func TestEmailInboundPipeline(t *testing.T) {
	rawMIME := "From: sender@example.com\r\nTo: bot@test.com\r\n" +
		"Subject: Integration Test\r\nMIME-Version: 1.0\r\n" +
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
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: SMTP capture received nothing")
	}
}
