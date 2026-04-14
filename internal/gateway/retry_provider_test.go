package gateway

import (
	"context"
	"testing"

	"github.com/sipeed/picoclaw/pkg/providers"
)

type mockProvider struct {
	responses []*providers.LLMResponse
	errors    []error
	callCount int
	lastMsgs  []providers.Message
}

func (m *mockProvider) Chat(_ context.Context, messages []providers.Message, _ []providers.ToolDefinition, _ string, _ map[string]any) (*providers.LLMResponse, error) {
	idx := m.callCount
	m.callCount++
	m.lastMsgs = messages
	if idx < len(m.responses) {
		if idx < len(m.errors) && m.errors[idx] != nil {
			return m.responses[idx], m.errors[idx]
		}
		return m.responses[idx], nil
	}
	return &providers.LLMResponse{}, nil
}

func (m *mockProvider) GetDefaultModel() string { return "mock" }

func TestRetryEmpty_NonEmptyResponsePassthrough(t *testing.T) {
	expected := &providers.LLMResponse{
		Content:      "hello",
		FinishReason: "stop",
	}
	mock := &mockProvider{responses: []*providers.LLMResponse{expected}}
	p := &retryEmptyProvider{inner: mock}

	resp, err := p.Chat(context.Background(), []providers.Message{{Role: "user", Content: "hi"}}, nil, "mock", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "hello" {
		t.Fatalf("expected content 'hello', got %q", resp.Content)
	}
	if mock.callCount != 1 {
		t.Fatalf("expected 1 call, got %d", mock.callCount)
	}
}

func TestRetryEmpty_EmptyResponseTriggersRetry(t *testing.T) {
	empty := &providers.LLMResponse{Content: "", FinishReason: "stop"}
	filled := &providers.LLMResponse{Content: "world", FinishReason: "stop"}
	mock := &mockProvider{responses: []*providers.LLMResponse{empty, filled}}
	p := &retryEmptyProvider{inner: mock}

	resp, err := p.Chat(context.Background(), []providers.Message{{Role: "user", Content: "hi"}}, nil, "mock", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "world" {
		t.Fatalf("expected content 'world', got %q", resp.Content)
	}
	if mock.callCount != 2 {
		t.Fatalf("expected 2 calls, got %d", mock.callCount)
	}
	msgs := mock.lastMsgs
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages on retry, got %d", len(msgs))
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "" {
		t.Fatalf("expected empty assistant message at index 1, got role=%s content=%q", msgs[1].Role, msgs[1].Content)
	}
	if msgs[2].Role != "user" || msgs[2].Content != retryEmptyPrompt {
		t.Fatalf("expected retry prompt at index 2, got role=%s content=%q", msgs[2].Role, msgs[2].Content)
	}
}

func TestRetryEmpty_DoubleEmptyReturnsFirst(t *testing.T) {
	empty1 := &providers.LLMResponse{Content: "", FinishReason: "stop"}
	empty2 := &providers.LLMResponse{Content: "", FinishReason: "stop"}
	mock := &mockProvider{responses: []*providers.LLMResponse{empty1, empty2}}
	p := &retryEmptyProvider{inner: mock}

	resp, err := p.Chat(context.Background(), []providers.Message{{Role: "user", Content: "hi"}}, nil, "mock", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "" {
		t.Fatalf("expected empty content, got %q", resp.Content)
	}
	if resp.FinishReason != "stop" {
		t.Fatalf("expected finish_reason 'stop', got %q", resp.FinishReason)
	}
	if mock.callCount != 2 {
		t.Fatalf("expected 2 calls (original + retry), got %d", mock.callCount)
	}
}

func TestRetryEmpty_ErrorPassesThrough(t *testing.T) {
	mock := &mockProvider{
		responses: []*providers.LLMResponse{nil},
		errors:    []error{context.Canceled},
	}
	p := &retryEmptyProvider{inner: mock}

	resp, err := p.Chat(context.Background(), nil, nil, "mock", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if resp != nil {
		t.Fatalf("expected nil response on error, got %+v", resp)
	}
	if mock.callCount != 1 {
		t.Fatalf("expected 1 call (no retry on error), got %d", mock.callCount)
	}
}

func TestRetryEmpty_GetDefaultModel(t *testing.T) {
	p := &retryEmptyProvider{inner: &mockProvider{}}
	if m := p.GetDefaultModel(); m != "mock" {
		t.Fatalf("expected 'mock', got %q", m)
	}
}

type mockStatefulProvider struct {
	mockProvider
	closed bool
}

func (m *mockStatefulProvider) Close() { m.closed = true }

func TestRetryEmpty_CloseDelegates(t *testing.T) {
	inner := &mockStatefulProvider{}
	p := &retryEmptyProvider{inner: inner}
	p.Close()
	if !inner.closed {
		t.Fatal("expected Close to be delegated to inner StatefulProvider")
	}
}

func TestRetryEmpty_CloseNoStatefulProvider(t *testing.T) {
	inner := &mockProvider{}
	p := &retryEmptyProvider{inner: inner}
	p.Close()
}

func TestWrapWithRetryEmpty_SkipsStartupBlocked(t *testing.T) {
	sb := &startupBlockedProvider{reason: "test"}
	result := wrapWithRetryEmpty(sb)
	if _, ok := result.(*startupBlockedProvider); !ok {
		t.Fatal("expected startupBlockedProvider to pass through unwrapped")
	}
}

func TestWrapWithRetryEmpty_WrapsNormalProvider(t *testing.T) {
	mock := &mockProvider{}
	result := wrapWithRetryEmpty(mock)
	if _, ok := result.(*retryEmptyProvider); !ok {
		t.Fatal("expected retryEmptyProvider wrapper")
	}
}

func TestIsEmptyResponse(t *testing.T) {
	tests := []struct {
		name     string
		resp     *providers.LLMResponse
		expected bool
	}{
		{"empty content no tools", &providers.LLMResponse{Content: ""}, true},
		{"non-empty content", &providers.LLMResponse{Content: "hi"}, false},
		{"tool calls only", &providers.LLMResponse{ToolCalls: []providers.ToolCall{{ID: "1"}}}, false},
		{"reasoning content only", &providers.LLMResponse{Content: "", ReasoningContent: "thinking..."}, false},
		{"content and tools", &providers.LLMResponse{Content: "hi", ToolCalls: []providers.ToolCall{{ID: "1"}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEmptyResponse(tt.resp); got != tt.expected {
				t.Errorf("isEmptyResponse(%+v) = %v, want %v", tt.resp, got, tt.expected)
			}
		})
	}
}
