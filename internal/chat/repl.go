// Package chat provides a terminal REPL for interacting with the sushiclaw agent.
package chat

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	agentsdk "github.com/Ingenimax/agent-sdk-go/pkg/agent"

	"github.com/sushi30/sushiclaw/internal/agent"
	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/config"
	sushitools "github.com/sushi30/sushiclaw/pkg/tools"
	"github.com/sushi30/sushiclaw/pkg/tools/exec"
)

// ErrQuit signals the REPL should exit cleanly.
var ErrQuit = errors.New("user quit")

// Runner holds the REPL state.
type Runner struct {
	agent   *agentsdk.Agent
	scanner *bufio.Scanner
	out     io.Writer
}

// NewRunner creates a chat runner from config.
func NewRunner(cfg *config.Config) (*Runner, error) {
	tools := sushitools.NewChatTools(cfg)

	// Create a local message bus so async subagent tasks can publish results
	// back to the terminal.
	messageBus := bus.NewMessageBus()
	workspaceProfiles, _ := agent.LoadSubAgentConfigs(cfg.WorkspacePath())
	profiles := agent.MergeSubAgentConfigs(workspaceProfiles, cfg.SubAgents)
	tools = sushitools.MaybeAppendSubagentTaskTool(tools, cfg, messageBus, agent.BuildSubagent, profiles)

	agentsdkAgent, err := agent.BuildAgent(cfg, tools)
	if err != nil {
		return nil, fmt.Errorf("build agent: %w", err)
	}

	r := &Runner{
		agent:   agentsdkAgent,
		scanner: bufio.NewScanner(os.Stdin),
		out:     os.Stdout,
	}

	// Start a background goroutine to print async subagent results.
	go r.consumeOutbound(messageBus)

	return r, nil
}

// consumeOutbound prints messages published to the bus by async subagent tasks.
func (r *Runner) consumeOutbound(messageBus *bus.MessageBus) {
	for msg := range messageBus.OutboundChan() {
		_, _ = fmt.Fprintln(r.out)
		_, _ = fmt.Fprintln(r.out, "[async result]")
		_, _ = fmt.Fprintln(r.out, msg.Content)
		_, _ = fmt.Fprint(r.out, "> ")
	}
}

// Run starts the REPL loop.
// SetInput replaces the scanner input (for testing).
func (r *Runner) SetInput(rd io.Reader) {
	r.scanner = bufio.NewScanner(rd)
}

// SetOutput replaces the output writer (for testing).
func (r *Runner) SetOutput(w io.Writer) {
	r.out = w
}

func (r *Runner) Run(ctx context.Context) error {
	_, _ = fmt.Fprintln(r.out, "Sushiclaw Chat")
	_, _ = fmt.Fprintln(r.out, "Type /quit to exit, /help for commands")
	_, _ = fmt.Fprintln(r.out)

	for {
		_, _ = fmt.Fprint(r.out, "> ")
		if !r.scanner.Scan() {
			break
		}

		line := r.scanner.Text()
		if line == "" {
			continue
		}

		// Handle REPL commands
		if handled, err := r.handleCommand(ctx, line); err != nil {
			if errors.Is(err, ErrQuit) {
				return nil
			}
			return err
		} else if handled {
			continue
		}

		// Send to agent
		actx := exec.WithChatID(ctx, "cli")
		response, err := r.agent.Run(actx, line)
		if err != nil {
			_, _ = fmt.Fprintf(r.out, "Error: %v\n", err)
			continue
		}

		_, _ = fmt.Fprintln(r.out, response)
	}

	return r.scanner.Err()
}

func (r *Runner) handleCommand(ctx context.Context, line string) (bool, error) {
	_ = ctx
	switch line {
	case "/quit", "/q", "/exit":
		_, _ = fmt.Fprintln(r.out, "Goodbye!")
		return true, ErrQuit
	case "/clear":
		// In-memory memory is per-agent-instance, so "clear" just means
		// we can't easily clear it without access to the memory interface.
		// For now, tell the user.
		_, _ = fmt.Fprintln(r.out, "Note: history is in-memory per session. Restart to clear.")
		return true, nil
	case "/help", "/h":
		_, _ = fmt.Fprintln(r.out, "Commands:")
		_, _ = fmt.Fprintln(r.out, "  /quit    Exit the REPL")
		_, _ = fmt.Fprintln(r.out, "  /clear   Note about history")
		_, _ = fmt.Fprintln(r.out, "  /help    Show this help")
		return true, nil
	}
	return false, nil
}
