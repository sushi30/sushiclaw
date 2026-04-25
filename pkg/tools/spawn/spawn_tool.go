package spawn

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	agentsdk "github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/logger"
)

// SpawnTool lets the main agent delegate work to sub-agents.
type SpawnTool struct {
	cfg     *config.Config
	tools   []interfaces.Tool
	factory SubAgentFactory
	bus     *bus.MessageBus
	busMu   sync.RWMutex
	taskID  atomic.Int64
}

// SubAgentFactory creates a new sub-agent instance.
type SubAgentFactory func(cfg *config.Config, name, description, modelName, systemPrompt string, tools []interfaces.Tool) (*agentsdk.Agent, error)

// spawnContext carries addressing info from the session into the tool.
type spawnContext struct {
	channel          string
	chatID           string
	replyToMessageID string
}

type ctxKey struct{}

// WithContext injects spawn addressing metadata into a context.
func WithContext(ctx context.Context, channel, chatID, replyToMessageID string) context.Context {
	return context.WithValue(ctx, ctxKey{}, spawnContext{
		channel:          channel,
		chatID:           chatID,
		replyToMessageID: replyToMessageID,
	})
}

func contextFromCtx(ctx context.Context) (spawnContext, bool) {
	v, ok := ctx.Value(ctxKey{}).(spawnContext)
	return v, ok
}

// NewSpawnTool creates a new spawn tool.
func NewSpawnTool(cfg *config.Config, tools []interfaces.Tool, factory SubAgentFactory) *SpawnTool {
	return &SpawnTool{
		cfg:     cfg,
		tools:   tools,
		factory: factory,
	}
}

// SetBus injects the message bus after initialization.
func (s *SpawnTool) SetBus(b *bus.MessageBus) {
	s.busMu.Lock()
	defer s.busMu.Unlock()
	s.bus = b
}

func (s *SpawnTool) Name() string { return "spawn" }
func (s *SpawnTool) Description() string {
	return "Spawn a sub-agent to handle a task. Use mode='sync' to wait for completion, or mode='async' to run in background."
}

func (s *SpawnTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"task": {
			Type:        "string",
			Description: "The task to delegate to the sub-agent",
			Required:    true,
		},
		"mode": {
			Type:        "string",
			Description: "Execution mode: 'sync' (wait for result) or 'async' (run in background). Default is 'sync'.",
			Required:    false,
		},
		"agent_type": {
			Type:        "string",
			Description: "Optional subagent profile name from config (e.g. 'coder', 'researcher')",
			Required:    false,
		},
	}
}

// Run executes the tool with the given input string.
func (s *SpawnTool) Run(ctx context.Context, input string) (string, error) {
	return s.Execute(ctx, input)
}

// Execute executes the tool with the given arguments JSON string.
func (s *SpawnTool) Execute(ctx context.Context, args string) (string, error) {
	var params struct {
		Task      string `json:"task"`
		Mode      string `json:"mode"`
		AgentType string `json:"agent_type"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		// Fallback: treat the entire input as the task.
		params.Task = strings.TrimSpace(args)
	}
	if params.Task == "" {
		return "", fmt.Errorf("task is required")
	}
	if params.Mode == "" {
		params.Mode = "sync"
	}
	params.Mode = strings.ToLower(params.Mode)

	// Resolve subagent profile.
	name := "subagent"
	description := "A sub-agent for delegated tasks"
	modelName := ""
	systemPrompt := ""

	if params.AgentType != "" {
		if sac, ok := s.cfg.SubAgents[params.AgentType]; ok {
			name = params.AgentType
			if sac.Description != "" {
				description = sac.Description
			}
			if sac.ModelName != "" {
				modelName = sac.ModelName
			}
			if sac.SystemPrompt != "" {
				systemPrompt = sac.SystemPrompt
			}
		}
	}

	agent, err := s.factory(s.cfg, name, description, modelName, systemPrompt, s.tools)
	if err != nil {
		return "", fmt.Errorf("failed to create subagent: %w", err)
	}

	sc, _ := contextFromCtx(ctx)

	if params.Mode == "async" {
		s.busMu.RLock()
		hasBus := s.bus != nil
		s.busMu.RUnlock()
		if !hasBus {
			return "", fmt.Errorf("async spawn is not available in this environment")
		}
		taskID := s.taskID.Add(1)
		go s.runAsync(sc, agent, params.Task, taskID)
		return fmt.Sprintf("Task %d started in background.", taskID), nil
	}

	return agent.Run(ctx, params.Task)
}

func (s *SpawnTool) runAsync(sc spawnContext, agent *agentsdk.Agent, task string, taskID int64) {
	runCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	result, err := agent.Run(runCtx, task)
	if err != nil {
		result = fmt.Sprintf("Task %d failed: %v", taskID, err)
	} else {
		result = fmt.Sprintf("Task %d completed:\n%s", taskID, result)
	}

	s.busMu.RLock()
	b := s.bus
	s.busMu.RUnlock()
	if b == nil {
		logger.WarnC("spawn", "Cannot publish async result: bus not set")
		return
	}
	if sc.channel == "" || sc.chatID == "" {
		logger.WarnC("spawn", "Cannot publish async result: missing channel or chat ID")
		return
	}

	outMsg := bus.OutboundMessage{
		Channel:          sc.channel,
		ChatID:           sc.chatID,
		Context:          bus.NewOutboundContext(sc.channel, sc.chatID, sc.replyToMessageID),
		Content:          result,
		ReplyToMessageID: sc.replyToMessageID,
	}
	if err := b.PublishOutbound(runCtx, outMsg); err != nil {
		logger.ErrorCF("spawn", "Failed to publish async result", map[string]any{"error": err.Error()})
	}
}
