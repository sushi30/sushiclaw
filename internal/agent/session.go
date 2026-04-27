// Package agent wraps agent-sdk-go to provide a sushiclaw-compatible agent session.
package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	agentsdk "github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/commands"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/llm/openrouter"
	"github.com/sushi30/sushiclaw/pkg/logger"
	"github.com/sushi30/sushiclaw/pkg/media"
	"github.com/sushi30/sushiclaw/pkg/tools/exec"
	"github.com/sushi30/sushiclaw/pkg/tools/toolctx"
)

// SessionManager wraps an agent-sdk-go Agent and processes inbound bus messages.
type SessionManager struct {
	agent           agentRunner
	bus             *bus.MessageBus
	mem             *InMemoryMemory
	cfg             *config.Config
	tools           []interfaces.Tool
	activatedSkills map[string]bool
	progress        ProgressSink
	mediaStore      media.MediaStore
}

type agentRunner interface {
	Run(ctx context.Context, input string) (string, error)
	RunDetailed(ctx context.Context, input string) (*interfaces.AgentResponse, error)
	RunStream(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error)
}

type SessionOption func(*SessionManager)

func WithProgressSink(sink ProgressSink) SessionOption {
	return func(sm *SessionManager) {
		sm.progress = sink
	}
}

// BuildAgent creates an agent-sdk-go Agent from config and tools.
func BuildAgent(cfg *config.Config, tools []interfaces.Tool) (*agentsdk.Agent, error) {
	return buildAgentWithMemory(cfg, tools, NewInMemoryMemory())
}

func buildAgentWithMemory(cfg *config.Config, tools []interfaces.Tool, mem *InMemoryMemory) (*agentsdk.Agent, error) {
	maxToolIterations := cfg.Agents.Defaults.MaxToolIterations
	if maxToolIterations < 0 {
		return nil, fmt.Errorf("invalid max_tool_iterations %d: must be >= 0", maxToolIterations)
	}

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
	for _, t := range tools {
		logger.DebugCF("agent", "Registering tool", map[string]any{
			"name":        t.Name(),
			"description": t.Description(),
			"parameters":  t.Parameters(),
		})
	}

	opts := []agentsdk.Option{
		agentsdk.WithName("sushiclaw"),
		agentsdk.WithLLM(llmClient),
		agentsdk.WithSystemPrompt(systemPrompt),
		agentsdk.WithTools(tools...),
		agentsdk.WithMemory(mem),
		agentsdk.WithRequirePlanApproval(false),
	}
	if maxToolIterations > 0 {
		opts = append(opts, agentsdk.WithMaxIterations(maxToolIterations))
	}
	if mcpCfg := toAgentSDKMCPConfig(cfg.MCP); mcpCfg != nil {
		opts = append(opts, agentsdk.WithMCPConfig(mcpCfg))
	}

	a, err := agentsdk.NewAgent(opts...)
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
func NewSessionManager(cfg *config.Config, messageBus *bus.MessageBus, tools []interfaces.Tool, store media.MediaStore, opts ...SessionOption) (*SessionManager, error) {
	mem := NewInMemoryMemory()
	a, err := buildAgentWithMemory(cfg, tools, mem)
	if err != nil {
		return nil, err
	}

	sm := &SessionManager{
		agent:           a,
		bus:             messageBus,
		mem:             mem,
		cfg:             cfg,
		tools:           tools,
		activatedSkills: make(map[string]bool),
		mediaStore:      store,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(sm)
		}
	}
	return sm, nil
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
	actx = toolctx.WithChannel(actx, msg.Channel)
	actx = toolctx.WithSenderID(actx, msg.SenderID)

	input := msg.Content
	if len(msg.Media) > 0 {
		paths := make([]string, 0, len(msg.Media))
		for _, ref := range msg.Media {
			if sm.mediaStore != nil && strings.HasPrefix(ref, "media://") {
				if p, err := sm.mediaStore.Resolve(ref); err == nil {
					paths = append(paths, p)
					continue
				}
			}
			paths = append(paths, ref)
		}
		mediaDesc := fmt.Sprintf("[attached files: %v]", paths)
		if input == "" {
			input = mediaDesc
		} else {
			input = input + "\n\n" + mediaDesc
		}
	}

	logger.DebugCF("agent", "Processing message", map[string]any{
		"chat_id": chatID,
		"sender":  msg.Sender.CanonicalID,
		"preview": truncate(input, 50),
	})

	start := time.Now()
	sm.emitProgress(ctx, ProgressEvent{Channel: msg.Channel, ChatID: chatID, Kind: ProgressTurnStarted})

	response, usage, toolCalls, err := sm.runStreamingTurn(actx, ctx, msg.Channel, chatID, input, start)
	if err != nil {
		logger.ErrorCF("agent", "Agent run failed", map[string]any{"error": err.Error()})
		_ = sm.bus.PublishOutbound(ctx, bus.OutboundMessage{
			Channel: msg.Channel,
			ChatID:  chatID,
			Content: fmt.Sprintf("Error: %v", err),
		})
		sm.emitProgress(ctx, ProgressEvent{Channel: msg.Channel, ChatID: chatID, Kind: ProgressFailed, Error: err, Elapsed: time.Since(start)})
		sm.emitSummary(ctx, ProgressSummary{
			Channel:   msg.Channel,
			ChatID:    chatID,
			Success:   false,
			ToolCalls: toolCalls,
			Usage:     usage,
			Duration:  time.Since(start),
			Error:     err,
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
	sm.emitProgress(ctx, ProgressEvent{Channel: msg.Channel, ChatID: chatID, Kind: ProgressCompleted, Elapsed: time.Since(start)})
	sm.emitSummary(ctx, ProgressSummary{
		Channel:   msg.Channel,
		ChatID:    chatID,
		Success:   true,
		ToolCalls: toolCalls,
		Usage:     usage,
		Duration:  time.Since(start),
	})
}

func (sm *SessionManager) runStreamingTurn(
	actx context.Context,
	outCtx context.Context,
	channel string,
	chatID string,
	input string,
	start time.Time,
) (string, *interfaces.TokenUsage, int, error) {
	events, err := sm.agent.RunStream(actx, input)
	if err != nil {
		sm.emitProgress(outCtx, ProgressEvent{Channel: channel, ChatID: chatID, Kind: ProgressFallback, Error: err, Elapsed: time.Since(start)})
		return sm.runDetailedTurn(actx, input)
	}
	if events == nil {
		err := errors.New("agent stream returned nil event channel")
		sm.emitProgress(outCtx, ProgressEvent{Channel: channel, ChatID: chatID, Kind: ProgressFallback, Error: err, Elapsed: time.Since(start)})
		return sm.runDetailedTurn(actx, input)
	}

	var streamer bus.Streamer
	var hasStreamer bool
	if sm.bus != nil {
		streamer, hasStreamer = sm.bus.GetStreamer(outCtx, channel, chatID)
	}

	var sb strings.Builder
	var usage *interfaces.TokenUsage
	var toolCalls int
	var streamErr error
	firstActivity := false
	lastActivity := time.Now()
	heartbeatInterval := sm.heartbeatInterval()
	heartbeat := time.NewTimer(heartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case event, ok := <-events:
			if !ok {
				if streamErr != nil && hasStreamer {
					streamer.Cancel(outCtx)
				}
				return sb.String(), usage, toolCalls, streamErr
			}
			lastActivity = time.Now()
			resetTimer(heartbeat, heartbeatInterval)
			if u, ok := tokenUsageFromMetadata(event.Metadata); ok {
				usage = u
			}
			if !firstActivity {
				firstActivity = true
				sm.emitProgress(outCtx, ProgressEvent{Channel: channel, ChatID: chatID, Kind: ProgressFirstActivity, Elapsed: time.Since(start)})
			}
			switch event.Type {
			case interfaces.AgentEventContent:
				if event.Content != "" {
					sb.WriteString(event.Content)
					if hasStreamer {
						_ = streamer.Update(outCtx, sb.String())
					}
				}
			case interfaces.AgentEventToolCall:
				toolCalls++
				sm.emitProgress(outCtx, ProgressEvent{
					Channel:  channel,
					ChatID:   chatID,
					Kind:     ProgressToolCallStarted,
					ToolName: safeToolName(event.ToolCall),
					Elapsed:  time.Since(start),
				})
			case interfaces.AgentEventToolResult:
				sm.emitProgress(outCtx, ProgressEvent{
					Channel:  channel,
					ChatID:   chatID,
					Kind:     ProgressToolCallFinished,
					ToolName: safeToolName(event.ToolCall),
					Elapsed:  time.Since(start),
				})
			case interfaces.AgentEventError:
				if event.Error != nil {
					streamErr = event.Error
				} else {
					streamErr = errors.New("agent stream error")
				}
			case interfaces.AgentEventComplete:
				// Completion is finalized when the event channel closes; SDKs may
				// still send a trailing error after a complete event.
			}
			if streamErr != nil {
				if hasStreamer {
					streamer.Cancel(outCtx)
				}
				return sb.String(), usage, toolCalls, streamErr
			}
		case <-heartbeat.C:
			sm.emitProgress(outCtx, ProgressEvent{
				Channel: channel,
				ChatID:  chatID,
				Kind:    ProgressHeartbeat,
				Elapsed: time.Since(lastActivity),
			})
			resetTimer(heartbeat, heartbeatInterval)
		case <-outCtx.Done():
			if hasStreamer {
				streamer.Cancel(outCtx)
			}
			return sb.String(), usage, toolCalls, outCtx.Err()
		}
	}
}

func (sm *SessionManager) runDetailedTurn(ctx context.Context, input string) (string, *interfaces.TokenUsage, int, error) {
	response, err := sm.agent.RunDetailed(ctx, input)
	if err != nil {
		return "", nil, 0, err
	}
	toolCalls := response.ExecutionSummary.ToolCalls
	return response.Content, meaningfulUsage(response.Usage, response.ExecutionSummary.LLMCalls), toolCalls, nil
}

func (sm *SessionManager) heartbeatInterval() time.Duration {
	if sm.progress == nil || sm.progress.HeartbeatInterval() <= 0 {
		return DefaultDebugHeartbeatInterval
	}
	return sm.progress.HeartbeatInterval()
}

func (sm *SessionManager) emitProgress(ctx context.Context, event ProgressEvent) {
	if sm.progress != nil {
		sm.progress.Progress(ctx, event)
	}
}

func (sm *SessionManager) emitSummary(ctx context.Context, summary ProgressSummary) {
	if sm.progress != nil {
		sm.progress.Summary(ctx, summary)
	}
}

func resetTimer(timer *time.Timer, d time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(d)
}

func safeToolName(toolCall *interfaces.ToolCallEvent) string {
	if toolCall == nil || strings.TrimSpace(toolCall.Name) == "" {
		return "unknown"
	}
	return toolCall.Name
}

func meaningfulUsage(usage *interfaces.TokenUsage, llmCalls int) *interfaces.TokenUsage {
	if usage == nil {
		return nil
	}
	if llmCalls == 0 &&
		usage.InputTokens == 0 &&
		usage.OutputTokens == 0 &&
		usage.TotalTokens == 0 &&
		usage.ReasoningTokens == 0 &&
		usage.CacheCreationInputTokens == 0 &&
		usage.CacheReadInputTokens == 0 {
		return nil
	}
	return usage
}

func tokenUsageFromMetadata(metadata map[string]interface{}) (*interfaces.TokenUsage, bool) {
	if len(metadata) == 0 {
		return nil, false
	}
	source := metadata
	if nested, ok := metadata["usage"]; ok {
		if nestedMap, ok := nested.(map[string]interface{}); ok {
			source = nestedMap
		} else if usage, ok := nested.(*interfaces.TokenUsage); ok && usage != nil {
			return usage, true
		} else {
			return nil, false
		}
	}

	input, hasInput := firstInt(source, "input_tokens", "prompt_tokens")
	output, hasOutput := firstInt(source, "output_tokens", "completion_tokens")
	total, hasTotal := firstInt(source, "total_tokens")
	if !hasInput && !hasOutput && !hasTotal {
		return nil, false
	}
	if !hasTotal {
		total = input + output
	}
	return &interfaces.TokenUsage{
		InputTokens:  input,
		OutputTokens: output,
		TotalTokens:  total,
	}, true
}

func firstInt(m map[string]interface{}, keys ...string) (int, bool) {
	for _, key := range keys {
		v, ok := m[key]
		if !ok {
			continue
		}
		switch n := v.(type) {
		case int:
			return n, true
		case int32:
			return int(n), true
		case int64:
			return int(n), true
		case float32:
			return int(n), true
		case float64:
			return int(n), true
		}
	}
	return 0, false
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
