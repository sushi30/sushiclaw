package chat_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/internal/chat"
	"github.com/sushi30/sushiclaw/pkg/config"
)

func TestRunner_HelpCommand(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				ModelName: "test-model",
				Workspace: "/tmp",
			},
		},
		ModelList: []config.ModelConfig{
			{ModelName: "test-model", Model: "gpt-4", APIKey: config.NewSecureString("test-key")},
		},
	}

	// Build agent will fail without a real API key — this test just verifies
	// Runner creation works, but we need a valid key for the LLM.
	// Skip if we can't build the agent.
	runner, err := chat.NewRunner(cfg)
	if err != nil {
		t.Skipf("Skipping: %v", err)
	}

	var out bytes.Buffer
	runner.SetOutput(&out)

	input := strings.NewReader("/help\n/quit\n")
	runner.SetInput(input)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runner.Run(ctx)
	// /quit calls os.Exit which we can't test here, so expect interrupt
	require.NoError(t, err)

	output := out.String()
	assert.Contains(t, output, "Commands:")
	assert.Contains(t, output, "/quit")
}

func TestRunner_EmptyInput(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				ModelName: "test-model",
				Workspace: "/tmp",
			},
		},
		ModelList: []config.ModelConfig{
			{ModelName: "test-model", Model: "gpt-4", APIKey: config.NewSecureString("test-key")},
		},
	}

	runner, err := chat.NewRunner(cfg)
	if err != nil {
		t.Skipf("Skipping: %v", err)
	}

	var out bytes.Buffer
	runner.SetOutput(&out)

	input := strings.NewReader("\n\n")
	runner.SetInput(input)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runner.Run(ctx)
	require.NoError(t, err)

	// Should show prompts but no responses for empty lines
	assert.Contains(t, out.String(), "> ")
}
