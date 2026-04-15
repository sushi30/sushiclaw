package email

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	imap "github.com/emersion/go-imap/v2"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
)

func TestThreadRoot(t *testing.T) {
	tests := []struct {
		name      string
		inReplyTo []string
		want      string
	}{
		{
			name:      "empty inReplyTo",
			inReplyTo: nil,
			want:      "",
		},
		{
			name:      "single message ID with angle brackets",
			inReplyTo: []string{"<msg-1@test.com>"},
			want:      "msg-1@test.com",
		},
		{
			name:      "single message ID without angle brackets",
			inReplyTo: []string{"msg-1@test.com"},
			want:      "msg-1@test.com",
		},
		{
			name:      "multiple entries returns first",
			inReplyTo: []string{"<first@test.com>", "<second@test.com>"},
			want:      "first@test.com",
		},
		{
			name:      "empty string entry",
			inReplyTo: []string{""},
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := threadRoot(tt.inReplyTo)
			if got != tt.want {
				t.Errorf("threadRoot() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateMessageID(t *testing.T) {
	got := generateMessageID("example.com")
	if got == "" {
		t.Fatal("generateMessageID() returned empty string")
	}
	if !strings.HasPrefix(got, "<") || !strings.HasSuffix(got, ">") {
		t.Errorf("generateMessageID() = %q, want angle-bracketed format", got)
	}
	if !strings.Contains(got, "@example.com>") {
		t.Errorf("generateMessageID() = %q, want to contain @example.com", got)
	}
	got2 := generateMessageID("example.com")
	if got == got2 {
		t.Errorf("generateMessageID() returned same ID twice: %q", got)
	}
}

func TestNormalizeMsgIDs(t *testing.T) {
	tests := []struct {
		name string
		ids  []string
		want []string
	}{
		{
			name: "nil input",
			ids:  nil,
			want: nil,
		},
		{
			name: "already bracketed",
			ids:  []string{"<msg@test.com>"},
			want: []string{"<msg@test.com>"},
		},
		{
			name: "bare id gets bracketed",
			ids:  []string{"msg@test.com"},
			want: []string{"<msg@test.com>"},
		},
		{
			name: "mixed bracketing",
			ids:  []string{"<first@test.com>", "second@test.com"},
			want: []string{"<first@test.com>", "<second@test.com>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeMsgIDs(tt.ids)
			if len(got) != len(tt.want) {
				t.Fatalf("normalizeMsgIDs() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("normalizeMsgIDs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestNewEmailChannel(t *testing.T) {
	msgBus := bus.NewMessageBus()

	validCfg := EmailConfig{
		SMTPHost: "smtp.example.com",
		SMTPFrom: *config.NewSecureString("bot@example.com"),
		IMAPHost: "imap.example.com",
		IMAPUser: *config.NewSecureString("bot@example.com"),
	}

	t.Run("missing smtp_host", func(t *testing.T) {
		cfg := validCfg
		cfg.SMTPHost = ""
		_, err := NewEmailChannel(cfg, msgBus)
		if err == nil {
			t.Error("expected error for missing smtp_host, got nil")
		}
	})

	t.Run("missing smtp_from", func(t *testing.T) {
		cfg := validCfg
		cfg.SMTPFrom = config.SecureString{}
		_, err := NewEmailChannel(cfg, msgBus)
		if err == nil {
			t.Error("expected error for missing smtp_from, got nil")
		}
	})

	t.Run("missing imap_host", func(t *testing.T) {
		cfg := validCfg
		cfg.IMAPHost = ""
		_, err := NewEmailChannel(cfg, msgBus)
		if err == nil {
			t.Error("expected error for missing imap_host, got nil")
		}
	})

	t.Run("missing imap_user", func(t *testing.T) {
		cfg := validCfg
		cfg.IMAPUser = config.SecureString{}
		_, err := NewEmailChannel(cfg, msgBus)
		if err == nil {
			t.Error("expected error for missing imap_user, got nil")
		}
	})

	t.Run("valid config", func(t *testing.T) {
		ch, err := NewEmailChannel(validCfg, msgBus)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ch.Name() != "email" {
			t.Errorf("Name() = %q, want %q", ch.Name(), "email")
		}
		if ch.IsRunning() {
			t.Error("new channel should not be running")
		}
	})
}

func TestExtractFrom(t *testing.T) {
	tests := []struct {
		name string
		from []imap.Address
		want string
	}{
		{
			name: "full address",
			from: []imap.Address{{Mailbox: "user", Host: "example.com"}},
			want: "user@example.com",
		},
		{
			name: "no host",
			from: []imap.Address{{Mailbox: "local"}},
			want: "local",
		},
		{
			name: "empty from list",
			from: []imap.Address{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := &imap.Envelope{From: tt.from}
			got := extractFrom(env)
			if got != tt.want {
				t.Errorf("extractFrom() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDisplayName(t *testing.T) {
	tests := []struct {
		name string
		from []imap.Address
		want string
	}{
		{
			name: "address with display name",
			from: []imap.Address{{Name: "Alice", Mailbox: "alice", Host: "example.com"}},
			want: "Alice",
		},
		{
			name: "address without display name",
			from: []imap.Address{{Mailbox: "alice", Host: "example.com"}},
			want: "alice@example.com",
		},
		{
			name: "empty from list",
			from: []imap.Address{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := &imap.Envelope{From: tt.from}
			got := displayName(env)
			if got != tt.want {
				t.Errorf("displayName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadEmailConfig_ResolvesEnvSecureStrings(t *testing.T) {
	t.Setenv("EMAIL_SMTP_PASS", "secret-smtp-pass")
	t.Setenv("EMAIL_IMAP_PASS", "secret-imap-pass")

	raw := map[string]any{
		"channels": map[string]any{
			"email": map[string]any{
				"enabled":       true,
				"smtp_host":     "smtp.example.com",
				"smtp_from":     "bot@example.com",
				"smtp_password": "env://EMAIL_SMTP_PASS",
				"imap_host":     "imap.example.com",
				"imap_user":     "bot@example.com",
				"imap_password": "env://EMAIL_IMAP_PASS",
			},
		},
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.CreateTemp(t.TempDir(), "config*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SUSHICLAW_CONFIG", f.Name())

	cfg, err := loadEmailConfig()
	if err != nil {
		t.Fatalf("loadEmailConfig() error: %v", err)
	}

	if got := cfg.SMTPPassword.String(); got != "secret-smtp-pass" {
		t.Errorf("SMTPPassword = %q, want %q", got, "secret-smtp-pass")
	}
	if got := cfg.IMAPPassword.String(); got != "secret-imap-pass" {
		t.Errorf("IMAPPassword = %q, want %q", got, "secret-imap-pass")
	}
}

func writeConfigFile(t *testing.T, raw map[string]any) string {
	t.Helper()
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.CreateTemp(t.TempDir(), "config*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func TestInitChannel_Disabled_ReturnsNil(t *testing.T) {
	raw := map[string]any{
		"channels": map[string]any{
			"email": map[string]any{"enabled": false},
		},
	}
	t.Setenv("SUSHICLAW_CONFIG", writeConfigFile(t, raw))

	ch, err := InitChannel(bus.NewMessageBus())
	if err != nil {
		t.Fatalf("InitChannel() error = %v, want nil", err)
	}
	if ch != nil {
		t.Errorf("InitChannel() = %v, want nil for disabled channel", ch)
	}
}

func TestInitChannel_MissingRequiredEnvVar_ReturnsError(t *testing.T) {
	_ = os.Unsetenv("MISSING_IMAP_USER")
	raw := map[string]any{
		"channels": map[string]any{
			"email": map[string]any{
				"enabled":       true,
				"smtp_host":     "smtp.example.com",
				"smtp_from":     "bot@example.com",
				"imap_host":     "imap.example.com",
				"imap_user":     "env://MISSING_IMAP_USER",
				"imap_password": "env://MISSING_IMAP_USER",
			},
		},
	}
	t.Setenv("SUSHICLAW_CONFIG", writeConfigFile(t, raw))

	_, err := InitChannel(bus.NewMessageBus())
	if err == nil {
		t.Fatal("InitChannel() = nil error, want error for missing env var")
	}
	if !strings.Contains(err.Error(), "MISSING_IMAP_USER") {
		t.Errorf("error %q does not mention var name", err.Error())
	}
}

func TestExtractPlainText(t *testing.T) {
	plainMIME := "MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nHello, world!"
	htmlMIME := "MIME-Version: 1.0\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<html><body>Hi</body></html>"
	multipartMIME := strings.Join([]string{
		"MIME-Version: 1.0",
		`Content-Type: multipart/alternative; boundary="boundary"`,
		"",
		"--boundary",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Plain text part",
		"--boundary",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<html><body>HTML part</body></html>",
		"--boundary--",
	}, "\r\n")

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "text/plain MIME",
			input: plainMIME,
			want:  "Hello, world!",
		},
		{
			name:  "html-only MIME",
			input: htmlMIME,
			want:  "Hi",
		},
		{
			name:  "multipart with text/plain",
			input: multipartMIME,
			want:  "Plain text part",
		},
	}

	multipartHTMLOnlyMIME := strings.Join([]string{
		"MIME-Version: 1.0",
		`Content-Type: multipart/alternative; boundary="boundary"`,
		"",
		"--boundary",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<html><head><style>.x{}</style></head><body><p>Hello <b>there</b></p></body></html>",
		"--boundary--",
	}, "\r\n")
	multipartPlainWinsMIME := strings.Join([]string{
		"MIME-Version: 1.0",
		`Content-Type: multipart/alternative; boundary="boundary"`,
		"",
		"--boundary",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Preferred plain text",
		"--boundary",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<html><body>Ignored HTML</body></html>",
		"--boundary--",
	}, "\r\n")

	tests = append(tests,
		struct {
			name  string
			input string
			want  string
		}{"multipart/alternative html-only", multipartHTMLOnlyMIME, "Hello there"},
		struct {
			name  string
			input string
			want  string
		}{"multipart/alternative plain wins over html", multipartPlainWinsMIME, "Preferred plain text"},
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPlainText(strings.NewReader(tt.input))
			if got != tt.want {
				t.Errorf("extractPlainText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProcessEmail_TextInteraction(t *testing.T) {
	messageBus := bus.NewMessageBus()
	ch := &EmailChannel{
		BaseChannel: channels.NewBaseChannel("email", EmailConfig{}, messageBus, nil),
	}

	envelope := &imap.Envelope{
		From:      []imap.Address{{Mailbox: "test", Host: "example.com", Name: "Test Sender"}},
		Subject:   "HTML only",
		MessageID: "mid-1",
	}
	body := strings.NewReader("MIME-Version: 1.0\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<html><body><p>Hello from email</p></body></html>")

	processed, got := ch.processEmail(context.Background(), envelope, body)
	if !processed {
		t.Fatal("processEmail() reported skipped message")
	}
	if got != "Hello from email" {
		t.Fatalf("processEmail() = %q, want %q", got, "Hello from email")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	select {
	case <-ctx.Done():
		t.Fatal("timeout waiting for inbound message")
	case inbound, ok := <-messageBus.InboundChan():
		if !ok {
			t.Fatal("expected inbound message")
		}
		if inbound.Channel != "email" {
			t.Fatalf("channel=%q", inbound.Channel)
		}
		if strings.TrimSpace(inbound.Content) == "" {
			t.Fatal("expected non-empty content")
		}
		if inbound.Content != "Hello from email" {
			t.Fatalf("content=%q", inbound.Content)
		}
		if inbound.Metadata == nil {
			t.Fatal("expected non-nil metadata")
		}
		if inbound.Metadata["reply_to_message_id"] != "mid-1" {
			t.Errorf("metadata[reply_to_message_id] = %q, want %q", inbound.Metadata["reply_to_message_id"], "mid-1")
		}
	}
}

func TestSend_ReplyThreadingHeaders(t *testing.T) {
	// Raw message IDs have no angle brackets (as returned by imap.Envelope.MessageID).
	// Send wraps them in <> when writing RFC 2822 headers.
	tests := []struct {
		name             string
		cachedSubject    string
		cachedReferences []string
		replyToMessageID string // raw, no angle brackets
		wantSubjectLine  string
		wantInReplyTo    string
		wantReferences   string
		wantNoThreading  bool
	}{
		{
			name:             "reply with known subject",
			cachedSubject:    "Hello Agent",
			cachedReferences: nil,
			replyToMessageID: "orig@test.com",
			wantSubjectLine:  "Subject: Re: Hello Agent",
			wantInReplyTo:    "In-Reply-To: <orig@test.com>",
			wantReferences:   "References: <orig@test.com>",
		},
		{
			name:             "subject already has Re: prefix",
			cachedSubject:    "Re: Hello Agent",
			cachedReferences: nil,
			replyToMessageID: "orig@test.com",
			wantSubjectLine:  "Subject: Re: Hello Agent",
			wantInReplyTo:    "In-Reply-To: <orig@test.com>",
			wantReferences:   "References: <orig@test.com>",
		},
		{
			name:             "reply with prior References chain",
			cachedSubject:    "Hello Agent",
			cachedReferences: []string{"<first@test.com>"},
			replyToMessageID: "orig@test.com",
			wantSubjectLine:  "Subject: Re: Hello Agent",
			wantInReplyTo:    "In-Reply-To: <orig@test.com>",
			wantReferences:   "References: <first@test.com> <orig@test.com>",
		},
		{
			name:             "no ReplyToMessageID — no threading headers, subject has unique suffix",
			cachedSubject:    "Hello Agent",
			replyToMessageID: "",
			wantNoThreading:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			smtpHost, smtpPort, received := startSMTPCapture(t)
			smtpPortInt, _ := strconv.Atoi(smtpPort)

			msgBus := bus.NewMessageBus()
			cfg := EmailConfig{
				SMTPHost:       smtpHost,
				SMTPPort:       smtpPortInt,
				SMTPFrom:       *config.NewSecureString("bot@test.com"),
				DefaultSubject: "Message",
				IMAPHost:       "127.0.0.1",
				IMAPPort:       10143,
				IMAPUser:       *config.NewSecureString("u"),
				IMAPPassword:   *config.NewSecureString("p"),
			}

			ch, err := NewEmailChannel(cfg, msgBus)
			if err != nil {
				t.Fatalf("NewEmailChannel: %v", err)
			}
			ch.SetRunning(true)

			if tt.replyToMessageID != "" {
				ch.threads.Store(tt.replyToMessageID, threadInfo{
					subject:    tt.cachedSubject,
					references: tt.cachedReferences,
					threadRoot: tt.replyToMessageID,
				})
			}

			ctx := context.Background()
			_, err = ch.Send(ctx, bus.OutboundMessage{
				ChatID:           "user@example.com",
				Content:          "Agent reply",
				ReplyToMessageID: tt.replyToMessageID,
			})
			if err != nil {
				t.Fatalf("Send: %v", err)
			}

			select {
			case body := <-received:
				if !strings.Contains(strings.ToLower(body), "message-id: <") {
					t.Errorf("expected Message-ID header in outbound:\n%s", body)
				}
				if !strings.Contains(body, "Date: ") {
					t.Errorf("expected Date header in outbound:\n%s", body)
				}
				if tt.wantNoThreading {
					if strings.Contains(body, "In-Reply-To:") {
						t.Errorf("expected no In-Reply-To header, got:\n%s", body)
					}
					if strings.Contains(body, "References:") {
						t.Errorf("expected no References header, got:\n%s", body)
					}
					subject := extractSubjectFromSMTPBody(t, body)
					m := subjectHexSuffixRe.FindStringSubmatch(subject)
					if m == nil {
						t.Errorf("new-conversation subject = %q, want match for %q", subject, subjectHexSuffixRe.String())
					}
					return
				}
				for _, want := range []string{tt.wantSubjectLine, tt.wantInReplyTo, tt.wantReferences} {
					if !strings.Contains(body, want) {
						t.Errorf("SMTP body missing %q:\n%s", want, body)
					}
				}
			case <-time.After(5 * time.Second):
				t.Fatal("timeout: SMTP capture received nothing")
			}
		})
	}
}

func TestSend_NewConversation_UniqueSubject(t *testing.T) {
	smtp1Host, smtp1Port, received1 := startSMTPCapture(t)
	smtp1PortInt, _ := strconv.Atoi(smtp1Port)

	smtp2Host, smtp2Port, received2 := startSMTPCapture(t)
	smtp2PortInt, _ := strconv.Atoi(smtp2Port)

	msgBus := bus.NewMessageBus()

	cfg1 := EmailConfig{
		SMTPHost:       smtp1Host,
		SMTPPort:       smtp1PortInt,
		SMTPFrom:       *config.NewSecureString("bot@test.com"),
		DefaultSubject: "Message",
		IMAPHost:       "127.0.0.1",
		IMAPPort:       10143,
		IMAPUser:       *config.NewSecureString("u"),
		IMAPPassword:   *config.NewSecureString("p"),
	}

	cfg2 := EmailConfig{
		SMTPHost:       smtp2Host,
		SMTPPort:       smtp2PortInt,
		SMTPFrom:       *config.NewSecureString("bot@test.com"),
		DefaultSubject: "Message",
		IMAPHost:       "127.0.0.1",
		IMAPPort:       10144,
		IMAPUser:       *config.NewSecureString("u"),
		IMAPPassword:   *config.NewSecureString("p"),
	}

	ch1, err := NewEmailChannel(cfg1, msgBus)
	if err != nil {
		t.Fatalf("NewEmailChannel: %v", err)
	}
	ch1.SetRunning(true)

	ch2, err := NewEmailChannel(cfg2, msgBus)
	if err != nil {
		t.Fatalf("NewEmailChannel: %v", err)
	}
	ch2.SetRunning(true)

	// First Send with empty ReplyToMessageID
	_, err = ch1.Send(context.Background(), bus.OutboundMessage{
		ChatID:  "user@example.com",
		Content: "First new conversation",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Second Send with empty ReplyToMessageID
	_, err = ch2.Send(context.Background(), bus.OutboundMessage{
		ChatID:  "user@example.com",
		Content: "Second new conversation",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	var body1, body2 string
	select {
	case body1 = <-received1:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: SMTP capture received nothing for first message")
	}

	select {
	case body2 = <-received2:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: SMTP capture received nothing for second message")
	}

	// Extract subject lines
	subject1 := extractSubjectFromSMTPBody(t, body1)
	subject2 := extractSubjectFromSMTPBody(t, body2)

	// Both subjects should match "Message [xxxxxxxx]" pattern
	m1 := subjectHexSuffixRe.FindStringSubmatch(subject1)
	if m1 == nil {
		t.Errorf("subject1 = %q, want match for %q", subject1, subjectHexSuffixRe.String())
	}
	m2 := subjectHexSuffixRe.FindStringSubmatch(subject2)
	if m2 == nil {
		t.Errorf("subject2 = %q, want match for %q", subject2, subjectHexSuffixRe.String())
	}

	// Subjects should be different
	if subject1 == subject2 {
		t.Errorf("two new-conversation emails should have different subjects, both = %q", subject1)
	}

	// The suffix should match the first 8 chars of the Message-ID hex portion
	mid1 := extractMessageIDFromBody(t, body1)
	if m1 != nil && len(mid1) >= 8 {
		expectedSuffix := mid1[:8]
		if m1[1] != expectedSuffix {
			t.Errorf("subject suffix = %q, want first 8 chars of Message-ID %q = %q", m1[1], mid1, expectedSuffix)
		}
	}
	mid2 := extractMessageIDFromBody(t, body2)
	if m2 != nil && len(mid2) >= 8 {
		expectedSuffix := mid2[:8]
		if m2[1] != expectedSuffix {
			t.Errorf("subject suffix = %q, want first 8 chars of Message-ID %q = %q", m2[1], mid2, expectedSuffix)
		}
	}

	// No In-Reply-To or References headers on new-conversation emails
	for _, b := range []string{body1, body2} {
		if strings.Contains(b, "In-Reply-To:") {
			t.Errorf("new-conversation email should not have In-Reply-To:\n%s", b)
		}
		if strings.Contains(b, "References:") {
			t.Errorf("new-conversation email should not have References:\n%s", b)
		}
	}
}

func TestSend_ReplyDoesNotAddSuffix(t *testing.T) {
	smtpHost, smtpPort, received := startSMTPCapture(t)
	smtpPortInt, _ := strconv.Atoi(smtpPort)

	msgBus := bus.NewMessageBus()
	cfg := EmailConfig{
		SMTPHost:       smtpHost,
		SMTPPort:       smtpPortInt,
		SMTPFrom:       *config.NewSecureString("bot@test.com"),
		DefaultSubject: "Message",
		IMAPHost:       "127.0.0.1",
		IMAPPort:       10143,
		IMAPUser:       *config.NewSecureString("u"),
		IMAPPassword:   *config.NewSecureString("p"),
	}

	ch, err := NewEmailChannel(cfg, msgBus)
	if err != nil {
		t.Fatalf("NewEmailChannel: %v", err)
	}
	ch.SetRunning(true)

	const origMsgID = "orig-suffix-test@test.com"
	ch.threads.Store(origMsgID, threadInfo{
		subject:    "Important Thread",
		references: nil,
		threadRoot: origMsgID,
	})

	_, err = ch.Send(context.Background(), bus.OutboundMessage{
		ChatID:           "user@example.com",
		Content:          "Replying to thread",
		ReplyToMessageID: origMsgID,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case body := <-received:
		subject := extractSubjectFromSMTPBody(t, body)
		if subject != "Re: Important Thread" {
			t.Errorf("reply subject = %q, want %q (no suffix on replies)", subject, "Re: Important Thread")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: SMTP capture received nothing")
	}
}

func extractSubjectFromSMTPBody(t *testing.T, body string) string {
	t.Helper()
	for _, line := range strings.Split(body, "\r\n") {
		if strings.HasPrefix(line, "Subject: ") {
			return strings.TrimPrefix(line, "Subject: ")
		}
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "Subject: ") {
			return strings.TrimPrefix(line, "Subject: ")
		}
	}
	t.Fatalf("no Subject header found in SMTP body:\n%s", body)
	return ""
}

func TestProcessEmail_SkipsEmptyOrSenderlessMessages(t *testing.T) {
	messageBus := bus.NewMessageBus()
	ch := &EmailChannel{
		BaseChannel: channels.NewBaseChannel("email", EmailConfig{}, messageBus, nil),
	}

	tests := []struct {
		name     string
		envelope *imap.Envelope
		body     string
	}{
		{
			name:     "missing sender",
			envelope: &imap.Envelope{Subject: "No sender", MessageID: "mid-2"},
			body:     "MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nHello",
		},
		{
			name: "empty extracted text",
			envelope: &imap.Envelope{
				From:      []imap.Address{{Mailbox: "test", Host: "example.com"}},
				Subject:   "Whitespace only",
				MessageID: "mid-3",
			},
			body: "MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n   \r\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processed, got := ch.processEmail(context.Background(), tt.envelope, strings.NewReader(tt.body))
			if processed {
				t.Fatal("expected processEmail() to skip message")
			}
			if got != "" {
				t.Fatalf("processEmail() text = %q, want empty", got)
			}

			select {
			case inbound := <-messageBus.InboundChan():
				t.Fatalf("unexpected inbound message: %+v", inbound)
			default:
			}
		})
	}
}
