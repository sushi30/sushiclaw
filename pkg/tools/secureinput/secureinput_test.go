package secureinput

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/sushi30/sushiclaw/pkg/tools/exec"
)

type recordingTool struct {
	args string
	out  string
	err  error
}

func (t *recordingTool) Name() string { return "record" }

func (t *recordingTool) Description() string { return "" }

func (t *recordingTool) Parameters() map[string]interfaces.ParameterSpec { return nil }

func (t *recordingTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *recordingTool) Execute(_ context.Context, args string) (string, error) {
	t.args = args
	return t.out, t.err
}

func TestToolCapturesHandleWithoutSecret(t *testing.T) {
	store := NewStore()
	tool := NewTool(store, func(string) (string, error) { return "super-secret", nil })
	ctx := exec.WithChatID(context.Background(), "chat-1")

	out, err := tool.Execute(ctx, `{"prompt":"API key","name":"api_key"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(out, "super-secret") {
		t.Fatalf("result leaked secret: %s", out)
	}
	if !strings.Contains(out, `"status":"captured"`) || !strings.Contains(out, `"handle":"secure-input://chat-1/`) {
		t.Fatalf("result missing captured handle: %s", out)
	}
	if !strings.Contains(out, `"name":"api_key"`) {
		t.Fatalf("result missing name: %s", out)
	}
}

func TestToolUnavailableReturnsGenericError(t *testing.T) {
	tool := NewTool(NewStore(), func(string) (string, error) {
		return "should-not-leak", errors.New("contains should-not-leak")
	})
	_, err := tool.Execute(exec.WithChatID(context.Background(), "chat-1"), `{}`)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != "secure input unavailable" {
		t.Fatalf("error = %q", got)
	}
}

func TestHandlesAreSessionScoped(t *testing.T) {
	store := NewStore()
	ctx1 := exec.WithChatID(context.Background(), "chat-1")
	ctx2 := exec.WithChatID(context.Background(), "chat-2")

	handle, err := store.Store(ctx1, "secret")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if _, err := store.Resolve(ctx2, handle); err == nil {
		t.Fatal("expected foreign handle to fail")
	}
	value, err := store.Resolve(ctx1, handle)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if value != "secret" {
		t.Fatalf("value = %q", value)
	}
}

func TestWrapperResolvesJSONArgumentsAndRedactsOutput(t *testing.T) {
	store := NewStore()
	ctx := exec.WithChatID(context.Background(), "chat-1")
	handle, err := store.Store(ctx, "super-secret")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	inner := &recordingTool{out: "tool saw super-secret"}
	wrapped := Wrap(inner, store)
	out, err := wrapped.Execute(ctx, `{"token":"`+handle+`","nested":["ok","`+handle+`"]}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(inner.args, "super-secret") {
		t.Fatalf("inner args were not resolved: %s", inner.args)
	}
	if strings.Contains(out, "super-secret") || out != "tool saw [REDACTED]" {
		t.Fatalf("output not redacted: %s", out)
	}
}

func TestWrapperRedactsErrors(t *testing.T) {
	store := NewStore()
	ctx := exec.WithChatID(context.Background(), "chat-1")
	handle, err := store.Store(ctx, "super-secret")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	inner := &recordingTool{err: errors.New("bad super-secret")}
	wrapped := Wrap(inner, store)
	_, err = wrapped.Execute(ctx, `{"token":"`+handle+`"}`)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != "bad [REDACTED]" {
		t.Fatalf("error = %q", got)
	}
}

func TestWrapperInvalidForeignHandleFailsGenerically(t *testing.T) {
	store := NewStore()
	ctx1 := exec.WithChatID(context.Background(), "chat-1")
	ctx2 := exec.WithChatID(context.Background(), "chat-2")
	handle, err := store.Store(ctx1, "super-secret")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	inner := &recordingTool{}
	wrapped := Wrap(inner, store)
	_, err = wrapped.Execute(ctx2, `{"token":"`+handle+`"}`)
	if err == nil {
		t.Fatal("expected error")
	}
	if inner.args != "" {
		t.Fatalf("inner tool should not run, args = %s", inner.args)
	}
	if got := err.Error(); got != "secure input unavailable" {
		t.Fatalf("error = %q", got)
	}
}
