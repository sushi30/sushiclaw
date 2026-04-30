package conversationlock

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sushi30/sushiclaw/pkg/commands"
)

func TestControllerRegistersRuntimeHandlers(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	m := newTestManager(now, &fakeSender{})
	c := &Controller{manager: m}
	rt := &commands.Runtime{}

	c.Register(rt)
	if rt.LockConversation == nil || rt.RequestUnlock == nil || rt.VerifyUnlockCode == nil {
		t.Fatal("expected lock runtime handlers to be registered")
	}
	req := commands.Request{SessionKey: "telegram:1"}
	if got := rt.LockConversation(context.Background(), req); got != "Conversation locked." {
		t.Fatalf("lock reply = %q", got)
	}
	if got := rt.RequestUnlock(context.Background(), req); got != "Unlock code sent." {
		t.Fatalf("unlock request reply = %q", got)
	}
	if got := rt.VerifyUnlockCode(context.Background(), req, "123456"); got != "Conversation unlocked." {
		t.Fatalf("verify reply = %q", got)
	}
}

func TestReplyForSupportRequired(t *testing.T) {
	got := replyForError(ErrSupportRequired)
	want := "Too many invalid unlock attempts. Please reach out for support using the email in the bot description."
	if got != want {
		t.Fatalf("reply = %q, want %q", got, want)
	}
}

func TestIsUnlockCommand(t *testing.T) {
	if !IsUnlockCommand("/unlock") {
		t.Fatal("/unlock should be unlock command")
	}
	if !IsUnlockCommand("!unlock 123456") {
		t.Fatal("!unlock should be unlock command")
	}
	if IsUnlockCommand("/lock") {
		t.Fatal("/lock should not be unlock command")
	}
}

func TestRedactCommand(t *testing.T) {
	if got := RedactCommand("/unlock 123456"); got != "/unlock [redacted]" {
		t.Fatalf("redacted = %q", got)
	}
	if got := RedactCommand("/unlock"); got != "/unlock" {
		t.Fatalf("redacted unlock request = %q", got)
	}
	if got := RedactCommand("/help"); got != "/help" {
		t.Fatalf("redacted non-unlock = %q", got)
	}
}

func TestReplyForInvalidCodeDoesNotMaskNonThresholdFailures(t *testing.T) {
	if got := replyForError(ErrInvalidCode); got != "Invalid unlock code." {
		t.Fatalf("reply = %q", got)
	}
	if got := replyForError(errors.New("boom")); got != "Unlock failed: boom" {
		t.Fatalf("reply = %q", got)
	}
}
