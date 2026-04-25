// Package agent wraps agent-sdk-go to provide a sushiclaw-compatible agent session.
package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agentsdk "github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/commands"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/llm/openrouter"
	"github.com/sushi30/sushiclaw/pkg/logger"
	"github.com/sushi30/sushiclaw/pkg/tools/exec"
	"github.com/sushi30/sushiclaw/pkg/tools/spawn"
)

// SessionManager wraps an agent-sdk-go Agent and processes inbound bus messages.
type SessionManager struct {
	agent           *agentsdk.Agent
	bus             *bus.MessageBus
	mem             *InMemoryMemory
	cfg             *config.Config
	tools           []interfaces.Tool
	activatedSkills map[string]bool
	subAgents       []string
}

// BuildAgent creates an agent-sdk-go Agent from config and tools.
func BuildAgent(cfg *config.Config, tools []interfaces.Tool) (*agentsdk.Agent, error) {
	return buildAgentWithOptions(cfg, agentOptions{
		name:   "sushiclaw",
		tools:  tools,
		memory: NewInMemoryMemory(),
	})
}

// BuildSubagent creates a sub-agent with optional overrides.
func BuildSubagent(cfg *config.Config, name, description, modelName, systemPrompt string, tools []interfaces.Tool) (*agentsdk.Agent, error) {
	return buildAgentWithOptions(cfg, agentOptions{
		name:         name,
		description:  description,
		modelName:    modelName,
		systemPrompt: systemPrompt,
		tools:        tools,
		memory:       NewInMemoryMemory(),
	})
}

type agentOptions struct {
	name         string
	description  string
	modelName    string
	systemPrompt string
	tools        []interfaces.Tool
	memory       *InMemoryMemory
	subAgents    []*agentsdk.Agent
}

func buildAgentWithOptions(cfg *config.Config, opts agentOptions) (*agentsdk.Agent, error) {
	maxToolIterations := cfg.Agents.Defaults.MaxToolIterations
	if maxToolIterations < 0 {
		return nil, fmt.Errorf("invalid max_tool_iterations %d: must be >= 0", maxToolIterations)
	}

	llmClient, err := createLLM(cfg, opts.modelName)
	if err != nil {
		return nil, fmt.Errorf("create LLM: %w", err)
	}

	systemPrompt := opts.systemPrompt
	if systemPrompt == "" {
		systemPrompt = "You are Sushiclaw, a helpful personal AI assistant."
		if ws := cfg.WorkspacePath(); ws != "" {
			cb := NewContextBuilder(ws)
			if p, err := cb.BuildSystemPromptWithCache(); err == nil && p != "" {
				systemPrompt = p
			}
		}
	}

	toolNames := make([]string, len(opts.tools))
	for i, t := range opts.tools {
		toolNames[i] = t.Name()
	}
	if len(opts.tools) == 0 {
		systemPrompt += "\n\nIMPORTANT: You have no tools available. You cannot execute commands, run code, or take real-world actions. If asked to do any of these, tell the user you are unable to in the current configuration — do not simulate or pretend to execute anything."
	}
	logger.DebugCF("agent", "Building agent", map[string]any{
		"name":          opts.name,
		"workspace":     cfg.WorkspacePath(),
		"prompt_length": len(systemPrompt),
		"tools":         toolNames,
	})
	for _, t := range tools {
		logger.DebugCF("agent", "Registering tool", map[string]any{
			"name":        t.Name(),
			"description": t.Description(),
			"parameters":  t.Parameters(),
		})
	}

	agentOpts := []agentsdk.Option{
		agentsdk.WithName(opts.name),
		agentsdk.WithLLM(llmClient),
		agentsdk.WithSystemPrompt(systemPrompt),
		agentsdk.WithTools(opts.tools...),
		agentsdk.WithMemory(opts.memory),
		agentsdk.WithRequirePlanApproval(false),
	}
	if maxToolIterations > 0 {
		agentOpts = append(agentOpts, agentsdk.WithMaxIterations(maxToolIterations))
	}
	if mcpCfg := toAgentSDKMCPConfig(cfg.MCP); mcpCfg != nil {
		agentOpts = append(agentOpts, agentsdk.WithMCPConfig(mcpCfg))
	}
	if len(opts.subAgents) > 0 {
		agentOpts = append(agentOpts, agentsdk.WithAgents(opts.subAgents...))
	}

	a, err := agentsdk.NewAgent(agentOpts...)
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}

	return a, nil
}

// toAgentSDKMCPConfig converts sushiclaw MCPConfig to agent-sdk-go MCPConfiguration.
func toAgentSDKMCPConfig(cfg config.MCPConfig) *agentsdk.MCPConfiguration {
	if len(cfg.MCPServers) == 0 {
		return nil
	}
	servers := make(map[string]agentsdk.MCPServerConfig, len(cfg.MCPServers))
	for name, s := range cfg.MCPServers {
		var token string
		if s.Token != nil {
			token = s.Token.String()
		}
		servers[name] = agentsdk.MCPServerConfig{
			Command:      s.Command,
			Args:         s.Args,
			Env:          s.Env,
			URL:          s.URL,
			Token:        token,
			AllowedTools: s.AllowedTools,
		}
	}
	return &agentsdk.MCPConfiguration{
		MCPServers: servers,
	}
}

// NewSessionManager creates a session manager from config.
func NewSessionManager(cfg *config.Config, messageBus *bus.MessageBus, tools []interfaces.Tool) (*SessionManager, error) {
	mem := NewInMemoryMemory()

	// Build static subagents from config (without spawn tool to prevent recursion).
	var subAgents []*agentsdk.Agent
	var subAgentNames []string
	filteredTools := toolsWithoutSpawn(tools)
	for name, sac := range cfg.SubAgents {
		sa, err := BuildSubagent(cfg, name, sac.Description, sac.ModelName, sac.SystemPrompt, filteredTools)
		if err != nil {
			return nil, fmt.Errorf("build subagent %q: %w", name, err)
		}
		subAgents = append(subAgents, sa)
		subAgentNames = append(subAgentNames, name)
	}

	a, err := buildAgentWithOptions(cfg, agentOptions{
		name:      "sushiclaw",
		tools:     tools,
		memory:    mem,
		subAgents: subAgents,
	})
	if err != nil {
		return nil, err
	}

	return &SessionManager{
		agent:           a,
		bus:             messageBus,
		mem:             mem,
		cfg:             cfg,
		tools:           tools,
		activatedSkills: make(map[string]bool),
		subAgents:       subAgentNames,
	}, nil
}

// ClearHistory resets the agent's conversation memory.
func (sm *SessionManager) ClearHistory() error {
	return sm.mem.Clear(context.Background())
}

// ActivateSkill reads a skill's SKILL.md and injects it into the conversation
// memory as a system message. If the skill is already loaded, it returns
// commands.ErrSkillAlreadyLoaded.
func (sm *SessionManager) ActivateSkill(skillName string) error {
	if sm.activatedSkills[skillName] {
		return commands.ErrSkillAlreadyLoaded
	}

	ws := sm.cfg.WorkspacePath()
	if ws == "" {
		return errors.New("no workspace configured")
	}

	skillPath := filepath.Join(ws, "skills", skillName, "SKILL.md")
	content, err := os.ReadFile(skillPath)
	if err != nil {
		return fmt.Errorf("skill %q not found", skillName)
	}

	skillContent := strings.TrimSpace(string(content))
	if skillContent == "" {
		return fmt.Errorf("skill %q is empty", skillName)
	}

	msg := interfaces.Message{
		Role:    interfaces.MessageRoleSystem,
		Content: skillContent,
	}
	if err := sm.mem.AddMessage(context.Background(), msg); err != nil {
		return fmt.Errorf("inject skill into memory: %w", err)
	}

	sm.activatedSkills[skillName] = true
	return nil
}

// ListModels returns all configured model names.
func (sm *SessionManager) ListModels() []string {
	names := make([]string, 0, len(sm.cfg.ModelList))
	for _, m := range sm.cfg.ModelList {
		names = append(names, m.ModelName)
	}
	return names
}

// ListSkills returns all skills available in the configured workspace.
func (sm *SessionManager) ListSkills() []commands.SkillInfo {
	ws := sm.cfg.WorkspacePath()
	if ws == "" {
		return nil
	}
	return listSkillsInDir(filepath.Join(ws, "skills"))
}

// ListSubAgents returns the names of configured subagents.
func (sm *SessionManager) ListSubAgents() []string {
	out := make([]string, len(sm.subAgents))
	copy(out, sm.subAgents)
	return out
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

// GetMessages returns all messages in the session memory.
func (sm *SessionManager) GetMessages(ctx context.Context) ([]interfaces.Message, error) {
	return sm.mem.GetMessages(ctx)
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
	actx = spawn.WithContext(actx, msg.Channel, chatID, msg.Context.ReplyToMessageID)

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

func createLLM(cfg *config.Config, modelName string) (interfaces.LLM, error) {
	if modelName == "" {
		modelName = cfg.Agents.Defaults.ModelName
	}
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

func toolsWithoutSpawn(tools []interfaces.Tool) []interfaces.Tool {
	out := make([]interfaces.Tool, 0, len(tools))
	for _, t := range tools {
		if t.Name() == "spawn" {
			continue
		}
		out = append(out, t)
	}
	return out
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
