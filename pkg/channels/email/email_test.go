package email

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	imap "github.com/emersion/go-imap/v2"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
)

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
				"enabled":   true,
				"smtp_host": "smtp.example.com",
				"smtp_from": "bot@example.com",
				"smtp_password": "env://EMAIL_SMTP_PASS",
				"imap_host": "imap.example.com",
				"imap_user": "bot@example.com",
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
	}
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
