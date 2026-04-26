package subagenttask

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	agentpkg "github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	taskexecutor "github.com/Ingenimax/agent-sdk-go/pkg/task/executor"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/logger"
)

const taskTimeout = 10 * time.Minute

// SubAgentFactory creates a fresh sub-agent instance for a configured profile.
type SubAgentFactory func(cfg *config.Config, name, description, modelName, systemPrompt string, tools []interfaces.Tool) (*agentpkg.Agent, error)

// Address carries the originating chat information needed for async callbacks.
type Address struct {
	Channel          string
	ChatID           string
	ReplyToMessageID string
}

type ctxKey struct{}

// WithContext injects callback addressing metadata into a tool context.
func WithContext(ctx context.Context, channel, chatID, replyToMessageID string) context.Context {
	return context.WithValue(ctx, ctxKey{}, Address{
		Channel:          channel,
		ChatID:           chatID,
		ReplyToMessageID: replyToMessageID,
	})
}

func addressFromContext(ctx context.Context) (Address, bool) {
	addr, ok := ctx.Value(ctxKey{}).(Address)
	return addr, ok
}

// Tool starts configured subagents as background SDK tasks and reports results
// to the originating chat through the message bus.
type Tool struct {
	cfg      *config.Config
	bus      *bus.MessageBus
	tools    []interfaces.Tool
	factory  SubAgentFactory
	executor *taskexecutor.TaskExecutor
	taskID   atomic.Int64
}

// NewTool creates the async subagent task bridge.
func NewTool(cfg *config.Config, messageBus *bus.MessageBus, tools []interfaces.Tool, factory SubAgentFactory) *Tool {
	t := &Tool{
		cfg:      cfg,
		bus:      messageBus,
		tools:    tools,
		factory:  factory,
		executor: taskexecutor.NewTaskExecutor(),
	}
	for _, name := range sortedProfileNames(cfg.SubAgents) {
		profileName := name
		t.executor.RegisterTask(profileName, t.taskFunc(profileName))
	}
	return t
}

func (t *Tool) Name() string { return "subagent_task" }

func (t *Tool) Description() string {
	return "Start a configured subagent task in the background and send the result back to this chat."
}

func (t *Tool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"agent_type": {
			Type:        "string",
			Description: "Configured subagent profile name.",
			Required:    true,
		},
		"task": {
			Type:        "string",
			Description: "Task to run in the background.",
			Required:    true,
		},
	}
}

func (t *Tool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *Tool) Execute(ctx context.Context, args string) (string, error) {
	var params struct {
		AgentType string `json:"agent_type"`
		Task      string `json:"task"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("invalid subagent_task arguments: %w", err)
	}
	params.AgentType = strings.TrimSpace(params.AgentType)
	params.Task = strings.TrimSpace(params.Task)
	if params.AgentType == "" {
		return "", fmt.Errorf("agent_type is required")
	}
	if params.Task == "" {
		return "", fmt.Errorf("task is required")
	}
	if _, ok := t.cfg.SubAgents[params.AgentType]; !ok {
		return "", fmt.Errorf("unknown subagent %q", params.AgentType)
	}

	addr, _ := addressFromContext(ctx)
	taskID := t.taskID.Add(1)
	resultCh, err := t.executor.ExecuteAsync(ctx, params.AgentType, params.Task, &interfaces.TaskOptions{
		Timeout:  durationPtr(taskTimeout),
		Metadata: map[string]interface{}{"task_id": taskID, "agent_type": params.AgentType},
	})
	if err != nil {
		return "", err
	}
	go t.publishWhenDone(addr, taskID, resultCh)
	return fmt.Sprintf("Task %d started.", taskID), nil
}

func (t *Tool) taskFunc(name string) func(context.Context, interface{}) (interface{}, error) {
	return func(ctx context.Context, params interface{}) (interface{}, error) {
		task, ok := params.(string)
		if !ok || strings.TrimSpace(task) == "" {
			return nil, fmt.Errorf("task is required")
		}
		profile := t.cfg.SubAgents[name]
		agent, err := t.factory(t.cfg, name, profile.Description, profile.ModelName, profile.SystemPrompt, t.tools)
		if err != nil {
			return nil, fmt.Errorf("create subagent %q: %w", name, err)
		}
		return agent.Run(ctx, task)
	}
}

func (t *Tool) publishWhenDone(addr Address, taskID int64, resultCh <-chan *interfaces.TaskResult) {
	result := <-resultCh
	content := fmt.Sprintf("Task %d completed:\n%v", taskID, result.Data)
	if result.Error != nil {
		content = fmt.Sprintf("Task %d failed: %v", taskID, result.Error)
	}

	if t.bus == nil {
		logger.WarnC("subagent_task", "Cannot publish async result: bus not set")
		return
	}
	if addr.Channel == "" || addr.ChatID == "" {
		logger.WarnC("subagent_task", "Cannot publish async result: missing channel or chat ID")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	outMsg := bus.OutboundMessage{
		Channel:          addr.Channel,
		ChatID:           addr.ChatID,
		Context:          bus.NewOutboundContext(addr.Channel, addr.ChatID, addr.ReplyToMessageID),
		Content:          content,
		ReplyToMessageID: addr.ReplyToMessageID,
	}
	if err := t.bus.PublishOutbound(ctx, outMsg); err != nil {
		logger.ErrorCF("subagent_task", "Failed to publish async result", map[string]any{"error": err.Error()})
	}
}

func sortedProfileNames(profiles map[string]config.SubAgentConfig) []string {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func durationPtr(d time.Duration) *time.Duration {
	return &d
}
