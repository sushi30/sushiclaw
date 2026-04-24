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
	"github.com/sushi30/sushiclaw/pkg/llm/openrouter"
	"github.com/sushi30/sushiclaw/pkg/logger"
	"github.com/sushi30/sushiclaw/pkg/tools/exec"
)

// SessionManager wraps an agent-sdk-go Agent and processes inbound bus messages.
type SessionManager struct {
	agent *agentsdk.Agent
	bus   *bus.MessageBus
	mem   *InMemoryMemory
	cfg   *config.Config
	tools []interfaces.Tool
}

// BuildAgent creates an agent-sdk-go Agent from config and tools.
func BuildAgent(cfg *config.Config, tools []interfaces.Tool) (*agentsdk.Agent, error) {
	return buildAgentWithMemory(cfg, tools, NewInMemoryMemory())
}

func buildAgentWithMemory(cfg *config.Config, tools []interfaces.Tool, mem *InMemoryMemory) (*agentsdk.Agent, error) {
	llmClient, err := createLLM(cfg)
	if err != nil {
		return nil, fmt.Errorf("create LLM: %w", err)
	}

	systemPrompt := "You are Sushiclaw, a helpful personal AI assistant."
	if ws := cfg.WorkspacePath(); ws != "" {
		cb := NewContextBuilder(ws)
		if p, err := cb.BuildSystemPromptWithCache(); err == nil && p != "" {
			systemPrompt = p
		}
	}

	toolNames := make([]string, len(tools))
	for i, t := range tools {
		toolNames[i] = t.Name()
	}
	if len(tools) == 0 {
		systemPrompt += "\n\nIMPORTANT: You have no tools available. You cannot execute commands, run code, or take real-world actions. If asked to do any of these, tell the user you are unable to in the current configuration — do not simulate or pretend to execute anything."
	}
	logger.DebugCF("agent", "Building agent", map[string]any{
		"workspace":     cfg.WorkspacePath(),
		"prompt_length": len(systemPrompt),
		"tools":         toolNames,
	})

	a, err := agentsdk.NewAgent(
		agentsdk.WithName("sushiclaw"),
		agentsdk.WithLLM(llmClient),
		agentsdk.WithSystemPrompt(systemPrompt),
		agentsdk.WithTools(tools...),
		agentsdk.WithMemory(mem),
		agentsdk.WithRequirePlanApproval(false),
	)
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}

	return a, nil
}

// NewSessionManager creates a session manager from config.
func NewSessionManager(cfg *config.Config, messageBus *bus.MessageBus, tools []interfaces.Tool) (*SessionManager, error) {
	mem := NewInMemoryMemory()
	a, err := buildAgentWithMemory(cfg, tools, mem)
	if err != nil {
		return nil, err
	}

	return &SessionManager{
		agent: a,
		bus:   messageBus,
		mem:   mem,
		cfg:   cfg,
		tools: tools,
	}, nil
}

// ClearHistory resets the agent's conversation memory.
func (sm *SessionManager) ClearHistory() error {
	return sm.mem.Clear(context.Background())
}

// ListModels returns all configured model names.
func (sm *SessionManager) ListModels() []string {
	names := make([]string, 0, len(sm.cfg.ModelList))
	for _, m := range sm.cfg.ModelList {
		names = append(names, m.ModelName)
	}
	return names
}

// GetModelInfo returns the configured model name and its provider.
func (sm *SessionManager) GetModelInfo() (name, provider string) {
	name = sm.cfg.Agents.Defaults.ModelName
	model := name
	for i := range sm.cfg.ModelList {
		if sm.cfg.ModelList[i].ModelName == name {
			if sm.cfg.ModelList[i].Model != "" {
				model = sm.cfg.ModelList[i].Model
			}
			break
		}
	}
	switch {
	case strings.HasPrefix(model, "openrouter/"):
		provider = "openrouter"
	default:
		provider = "openai"
	}
	return
}

// ToolNames returns the names of the tools registered with this session manager.
func (sm *SessionManager) ToolNames() []string {
	names := make([]string, len(sm.tools))
	for i, t := range sm.tools {
		names[i] = t.Name()
	}
	return names
}

// Chat runs a single turn against the agent and returns the response.
// Bypasses the bus — useful for CLI REPL.
func (sm *SessionManager) Chat(ctx context.Context, input string) (string, error) {
	actx := exec.WithChatID(ctx, "cli")
	return sm.agent.Run(actx, input)
}

// Dispatch processes a single inbound message through the agent.
// Called by the gateway after command filtering and local execution.
func (sm *SessionManager) Dispatch(ctx context.Context, msg bus.InboundMessage) {
	sm.handleInbound(ctx, msg)
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

	// Dispatch to provider based on model prefix.
	switch {
	case strings.HasPrefix(model, "openrouter/"):
		return openrouter.NewClient(apiKey, openrouter.WithModel(model)), nil
	default:
		opts := []openai.Option{openai.WithModel(model)}
		if modelCfg.APIBase != "" {
			opts = append(opts, openai.WithBaseURL(modelCfg.APIBase))
		}
		return openai.NewClient(apiKey, opts...), nil
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
