package gateway

import (
	"context"

	"github.com/sipeed/picoclaw/pkg/providers"
)

const retryEmptyPrompt = "Please provide your response to my previous message."

type retryEmptyProvider struct {
	inner providers.LLMProvider
}

func isEmptyResponse(resp *providers.LLMResponse) bool {
	return resp.Content == "" && len(resp.ToolCalls) == 0 && resp.ReasoningContent == ""
}

func (p *retryEmptyProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	resp, err := p.inner.Chat(ctx, messages, tools, model, options)
	if err != nil {
		return resp, err
	}

	if !isEmptyResponse(resp) {
		return resp, nil
	}

	extended := make([]providers.Message, len(messages), len(messages)+2)
	copy(extended, messages)
	extended = append(extended, providers.Message{
		Role:    "assistant",
		Content: "",
	})
	extended = append(extended, providers.Message{
		Role:    "user",
		Content: retryEmptyPrompt,
	})

	retryResp, retryErr := p.inner.Chat(ctx, extended, tools, model, options)
	if retryErr != nil {
		return resp, nil
	}
	if isEmptyResponse(retryResp) {
		return resp, nil
	}
	return retryResp, nil
}

func (p *retryEmptyProvider) GetDefaultModel() string {
	return p.inner.GetDefaultModel()
}

func (p *retryEmptyProvider) Close() {
	if cp, ok := p.inner.(providers.StatefulProvider); ok {
		cp.Close()
	}
}

func wrapWithRetryEmpty(provider providers.LLMProvider) providers.LLMProvider {
	if _, ok := provider.(*startupBlockedProvider); ok {
		return provider
	}
	return &retryEmptyProvider{inner: provider}
}
