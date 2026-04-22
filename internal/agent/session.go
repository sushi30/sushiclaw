// Package agent wraps agent-sdk-go to provide a sushiclaw-compatible agent session.
package agent

import (
	"context"
	"fmt"
	"strings"

	agentsdk "github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/logger"
	"github.com/sushi30/sushiclaw/pkg/tools/exec"
)

// SessionManager wraps an agent-sdk-go Agent and processes inbound bus messages.
type SessionManager struct {
	agent *agentsdk.Agent
	bus   *bus.MessageBus
}

// BuildAgent creates an agent-sdk-go Agent from config and tools.
func BuildAgent(cfg *config.Config, tools []interfaces.Tool) (*agentsdk.Agent, error) {
	llmClient, err := createLLM(cfg)
	if err != nil {
		return nil, fmt.Errorf("create LLM: %w", err)
	}

	a, err := agentsdk.NewAgent(
		agentsdk.WithName("sushiclaw"),
		agentsdk.WithLLM(llmClient),
		agentsdk.WithSystemPrompt("You are Sushiclaw, a helpful personal AI assistant."),
		agentsdk.WithTools(tools...),
		agentsdk.WithMemory(NewInMemoryMemory()),
	)
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}

	return a, nil
}

// NewSessionManager creates a session manager from config.
func NewSessionManager(cfg *config.Config, messageBus *bus.MessageBus) (*SessionManager, error) {
	a, err := BuildAgent(cfg, nil)
	if err != nil {
		return nil, err
	}

	return &SessionManager{
		agent: a,
		bus:   messageBus,
	}, nil
}

// RegisterTool registers a tool with the agent.
// Note: agent-sdk-go does not support dynamic tool registration after creation.
// This is a no-op; tools must be passed during construction.
func (sm *SessionManager) RegisterTool(t interfaces.Tool) {
	logger.WarnC("agent", "RegisterTool is a no-op with agent-sdk-go; tools must be passed during construction")
}

// Chat runs a single turn against the agent and returns the response.
// Bypasses the bus — useful for CLI REPL.
func (sm *SessionManager) Chat(ctx context.Context, input string) (string, error) {
	actx := exec.WithChatID(ctx, "cli")
	return sm.agent.Run(actx, input)
}

// Run listens on the inbound bus channel and processes messages.
func (sm *SessionManager) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-sm.bus.InboundChan():
			if !ok {
				return
			}
			sm.handleInbound(ctx, msg)
		}
	}
}

func (sm *SessionManager) handleInbound(ctx context.Context, msg bus.InboundMessage) {
	chatID := msg.Context.ChatID
	if chatID == "" {
		chatID = msg.ChatID
	}

	// Attach chat ID to context for tool use.
	actx := exec.WithChatID(ctx, chatID)

	input := msg.Content
	if input == "" && len(msg.Media) > 0 {
		input = fmt.Sprintf("[media attachments: %v]", msg.Media)
	}

	logger.DebugCF("agent", "Processing message", map[string]any{
		"chat_id": chatID,
		"sender":  msg.Sender.CanonicalID,
		"preview": truncate(input, 50),
	})

	response, err := sm.agent.Run(actx, input)
	if err != nil {
		logger.ErrorCF("agent", "Agent run failed", map[string]any{"error": err.Error()})
		_ = sm.bus.PublishOutbound(ctx, bus.OutboundMessage{
			Channel: msg.Channel,
			ChatID:  chatID,
			Content: fmt.Sprintf("Error: %v", err),
		})
		return
	}

	if response != "" {
		_ = sm.bus.PublishOutbound(ctx, bus.OutboundMessage{
			Channel: msg.Channel,
			ChatID:  chatID,
			Content: response,
		})
	}
}

func createLLM(cfg *config.Config) (interfaces.LLM, error) {
	modelName := cfg.Agents.Defaults.ModelName
	if modelName == "" {
		return nil, fmt.Errorf("no default model configured")
	}

	var modelCfg *config.ModelConfig
	for i := range cfg.ModelList {
		if cfg.ModelList[i].ModelName == modelName {
			modelCfg = &cfg.ModelList[i]
			break
		}
	}
	if modelCfg == nil {
		return nil, fmt.Errorf("model %q not found in model_list", modelName)
	}

	apiKey := modelCfg.APIKeyString()
	if apiKey == "" {
		return nil, fmt.Errorf("no API key for model %q", modelName)
	}
	if strings.HasPrefix(apiKey, "env://") {
		return nil, fmt.Errorf("env var %s is not set (model %q)", strings.TrimPrefix(apiKey, "env://"), modelName)
	}

	model := modelCfg.Model
	if model == "" {
		model = modelName
	}

	baseURL := modelCfg.APIBase
	if baseURL == "" {
		// Auto-detect provider from model prefix
		switch {
		case strings.HasPrefix(model, "openrouter/"):
			baseURL = "https://openrouter.ai/api/v1"
		default:
			baseURL = "https://api.openai.com/v1"
		}
	}

	// Use agent-sdk-go's OpenAI client as the default.
	client := openai.NewClient(apiKey,
		openai.WithModel(model),
		openai.WithBaseURL(baseURL),
	)
	return client, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
