package gateway

import (
	"context"
	"testing"

	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/stretchr/testify/assert"
	testifymock "github.com/stretchr/testify/mock"
)

type mockProvider struct {
	testifymock.Mock
}

func (m *mockProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string, options map[string]any) (*providers.LLMResponse, error) {
	args := m.Called(ctx, messages, tools, model, options)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*providers.LLMResponse), args.Error(1)
}

func (m *mockProvider) GetDefaultModel() string {
	args := m.Called()
	return args.String(0)
}

type mockStatefulProvider struct {
	mockProvider
}

func (m *mockStatefulProvider) Close() {
	m.Called()
}

func TestRetryEmpty_NonEmptyResponsePassthrough(t *testing.T) {
	mock := new(mockProvider)
	expected := &providers.LLMResponse{Content: "hello", FinishReason: "stop"}
	mock.On("Chat", testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything).Return(expected, nil)
	p := &retryEmptyProvider{inner: mock}

	resp, err := p.Chat(context.Background(), []providers.Message{{Role: "user", Content: "hi"}}, nil, "mock", nil)

	assert.NoError(t, err)
	assert.Equal(t, "hello", resp.Content)
	mock.AssertNumberOfCalls(t, "Chat", 1)
}

func TestRetryEmpty_EmptyResponseTriggersRetry(t *testing.T) {
	empty := &providers.LLMResponse{Content: "", FinishReason: "stop"}
	filled := &providers.LLMResponse{Content: "world", FinishReason: "stop"}
	mock := new(mockProvider)
	mock.On("Chat", testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything).Return(empty, nil).Once()
	mock.On("Chat", testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything).Return(filled, nil).Once()
	p := &retryEmptyProvider{inner: mock}

	resp, err := p.Chat(context.Background(), []providers.Message{{Role: "user", Content: "hi"}}, nil, "mock", nil)

	assert.NoError(t, err)
	assert.Equal(t, "world", resp.Content)
	mock.AssertNumberOfCalls(t, "Chat", 2)

	retryMsgs := mock.Calls[1].Arguments.Get(1).([]providers.Message)
	assert.Len(t, retryMsgs, 3)
	assert.Equal(t, "assistant", retryMsgs[1].Role)
	assert.Equal(t, "", retryMsgs[1].Content)
	assert.Equal(t, "user", retryMsgs[2].Role)
	assert.Equal(t, retryEmptyPrompt, retryMsgs[2].Content)
}

func TestRetryEmpty_DoubleEmptyReturnsFirst(t *testing.T) {
	empty := &providers.LLMResponse{Content: "", FinishReason: "stop"}
	mock := new(mockProvider)
	mock.On("Chat", testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything).Return(empty, nil)
	p := &retryEmptyProvider{inner: mock}

	resp, err := p.Chat(context.Background(), []providers.Message{{Role: "user", Content: "hi"}}, nil, "mock", nil)

	assert.NoError(t, err)
	assert.Equal(t, "", resp.Content)
	assert.Equal(t, "stop", resp.FinishReason)
	mock.AssertNumberOfCalls(t, "Chat", 2)
}

func TestRetryEmpty_ErrorPassesThrough(t *testing.T) {
	mock := new(mockProvider)
	mock.On("Chat", testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything).Return(nil, context.Canceled)
	p := &retryEmptyProvider{inner: mock}

	resp, err := p.Chat(context.Background(), nil, nil, "mock", nil)

	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, resp)
	mock.AssertNumberOfCalls(t, "Chat", 1)
}

func TestRetryEmpty_GetDefaultModel(t *testing.T) {
	mock := new(mockProvider)
	mock.On("GetDefaultModel").Return("mock")
	p := &retryEmptyProvider{inner: mock}

	assert.Equal(t, "mock", p.GetDefaultModel())
}

func TestRetryEmpty_CloseDelegates(t *testing.T) {
	inner := new(mockStatefulProvider)
	inner.On("Close").Once()
	p := &retryEmptyProvider{inner: inner}
	p.Close()
	inner.AssertCalled(t, "Close")
}

func TestRetryEmpty_CloseNoStatefulProvider(t *testing.T) {
	inner := new(mockProvider)
	p := &retryEmptyProvider{inner: inner}
	p.Close()
	inner.AssertNotCalled(t, "Close")
}

func TestWrapWithRetryEmpty_SkipsStartupBlocked(t *testing.T) {
	sb := &startupBlockedProvider{reason: "test"}
	result := wrapWithRetryEmpty(sb)
	assert.IsType(t, &startupBlockedProvider{}, result)
}

func TestWrapWithRetryEmpty_WrapsNormalProvider(t *testing.T) {
	mock := new(mockProvider)
	result := wrapWithRetryEmpty(mock)
	assert.IsType(t, &retryEmptyProvider{}, result)
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
			assert.Equal(t, tt.expected, isEmptyResponse(tt.resp))
		})
	}
}
