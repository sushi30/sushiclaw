package commands_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/pkg/commands"
)

func TestLookupFound(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	def, found := reg.Lookup("help")
	assert.True(t, found)
	assert.Equal(t, "help", def.Name)
}

func TestLookupNotFound(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	_, found := reg.Lookup("nonexistent")
	assert.False(t, found)
}

func TestHasCommandPrefix(t *testing.T) {
	assert.True(t, commands.HasCommandPrefix("/help"))
	assert.True(t, commands.HasCommandPrefix("!help"))
	assert.False(t, commands.HasCommandPrefix("hello"))
	assert.False(t, commands.HasCommandPrefix(""))
}

func TestCommandName(t *testing.T) {
	name, ok := commands.CommandName("/help")
	require.True(t, ok)
	assert.Equal(t, "help", name)

	name, ok = commands.CommandName("/help@bot")
	require.True(t, ok)
	assert.Equal(t, "help", name)

	_, ok = commands.CommandName("hello world")
	assert.False(t, ok)
}

func TestExecuteHelp(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	rt := &commands.Runtime{ListDefinitions: reg.Definitions}
	exec := commands.NewExecutor(reg, rt)

	var replied string
	req := commands.Request{
		Text:  "/help",
		Reply: func(s string) error { replied = s; return nil },
	}

	result := exec.Execute(context.Background(), req)
	assert.Equal(t, commands.OutcomeHandled, result.Outcome)
	assert.Contains(t, replied, "/help")
	assert.Contains(t, replied, "/clear")
}

func TestExecuteHelpNoRuntime(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	exec := commands.NewExecutor(reg, &commands.Runtime{})

	var replied string
	result := exec.Execute(context.Background(), commands.Request{
		Text:  "/help",
		Reply: func(s string) error { replied = s; return nil },
	})
	assert.Equal(t, commands.OutcomeHandled, result.Outcome)
	assert.Contains(t, replied, "No commands available.")
}

func TestExecuteStart(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	exec := commands.NewExecutor(reg, &commands.Runtime{})

	var replied string
	result := exec.Execute(context.Background(), commands.Request{
		Text:  "/start",
		Reply: func(s string) error { replied = s; return nil },
	})
	assert.Equal(t, commands.OutcomeHandled, result.Outcome)
	assert.NotEmpty(t, replied)
}

func TestExecuteClearCallsCallback(t *testing.T) {
	cleared := false
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	rt := &commands.Runtime{
		ClearHistory: func() error { cleared = true; return nil },
	}
	exec := commands.NewExecutor(reg, rt)

	var replied string
	result := exec.Execute(context.Background(), commands.Request{
		Text:  "/clear",
		Reply: func(s string) error { replied = s; return nil },
	})
	assert.Equal(t, commands.OutcomeHandled, result.Outcome)
	assert.True(t, cleared)
	assert.Contains(t, replied, "cleared")
}

func TestExecutePassthroughNoHandler(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	exec := commands.NewExecutor(reg, &commands.Runtime{})

	// /use has no handler — should pass through to agent
	result := exec.Execute(context.Background(), commands.Request{Text: "/use"})
	assert.Equal(t, commands.OutcomePassthrough, result.Outcome)
}

func TestExecuteUnrecognizedReturnsPassthrough(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	exec := commands.NewExecutor(reg, &commands.Runtime{})

	result := exec.Execute(context.Background(), commands.Request{Text: "/unknowncmd"})
	assert.Equal(t, commands.OutcomePassthrough, result.Outcome)
}
