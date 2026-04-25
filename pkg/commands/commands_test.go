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

func TestExecuteModel(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	rt := &commands.Runtime{
		GetModelInfo: func() (name, provider string) { return "gpt-4", "openai" },
	}
	exec := commands.NewExecutor(reg, rt)

	var replied string
	result := exec.Execute(context.Background(), commands.Request{
		Text:  "/model",
		Reply: func(s string) error { replied = s; return nil },
	})
	assert.Equal(t, commands.OutcomeHandled, result.Outcome)
	assert.Contains(t, replied, "gpt-4")
	assert.Contains(t, replied, "openai")
}

func TestExecuteModelNoRuntime(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	exec := commands.NewExecutor(reg, &commands.Runtime{})

	var replied string
	result := exec.Execute(context.Background(), commands.Request{
		Text:  "/model",
		Reply: func(s string) error { replied = s; return nil },
	})
	assert.Equal(t, commands.OutcomeHandled, result.Outcome)
	assert.Contains(t, replied, "unavailable")
}

func TestExecuteDebug(t *testing.T) {
	toggled := false
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	rt := &commands.Runtime{
		ToggleDebug: func(_ context.Context, _, _ string) string { toggled = true; return "Debug toggled." },
	}
	exec := commands.NewExecutor(reg, rt)

	var replied string
	result := exec.Execute(context.Background(), commands.Request{
		Text:  "/debug",
		Reply: func(s string) error { replied = s; return nil },
	})
	assert.Equal(t, commands.OutcomeHandled, result.Outcome)
	assert.True(t, toggled)
	assert.Equal(t, "Debug toggled.", replied)
}

func TestExecuteDebugNoRuntime(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	exec := commands.NewExecutor(reg, &commands.Runtime{})

	var replied string
	result := exec.Execute(context.Background(), commands.Request{
		Text:  "/debug",
		Reply: func(s string) error { replied = s; return nil },
	})
	assert.Equal(t, commands.OutcomeHandled, result.Outcome)
	assert.Contains(t, replied, "unavailable")
}

func TestExecutePassthroughNoHandler(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	exec := commands.NewExecutor(reg, &commands.Runtime{})

	// /debug has no handler — should pass through to agent
	result := exec.Execute(context.Background(), commands.Request{Text: "/debug"})
	assert.Equal(t, commands.OutcomePassthrough, result.Outcome)
}

func TestExecuteUseSuccess(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	rt := &commands.Runtime{
		ActivateSkill: func(name string) error { return nil },
	}
	exec := commands.NewExecutor(reg, rt)

	var replied string
	result := exec.Execute(context.Background(), commands.Request{
		Text:  "/use python",
		Reply: func(s string) error { replied = s; return nil },
	})
	assert.Equal(t, commands.OutcomeHandled, result.Outcome)
	assert.Equal(t, "Skill python activated.", replied)
}

func TestExecuteUseMissingArg(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	exec := commands.NewExecutor(reg, &commands.Runtime{})

	var replied string
	result := exec.Execute(context.Background(), commands.Request{
		Text:  "/use",
		Reply: func(s string) error { replied = s; return nil },
	})
	assert.Equal(t, commands.OutcomeHandled, result.Outcome)
	assert.Contains(t, replied, "Usage: /use <skill-name>")
}

func TestExecuteUseAlreadyLoaded(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	rt := &commands.Runtime{
		ActivateSkill: func(name string) error { return commands.ErrSkillAlreadyLoaded },
	}
	exec := commands.NewExecutor(reg, rt)

	var replied string
	result := exec.Execute(context.Background(), commands.Request{
		Text:  "/use python",
		Reply: func(s string) error { replied = s; return nil },
	})
	assert.Equal(t, commands.OutcomeHandled, result.Outcome)
	assert.Equal(t, "Skill python is already loaded.", replied)
}

func TestExecuteUseCallbackError(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	rt := &commands.Runtime{
		ActivateSkill: func(name string) error { return assert.AnError },
	}
	exec := commands.NewExecutor(reg, rt)

	var replied string
	result := exec.Execute(context.Background(), commands.Request{
		Text:  "/use python",
		Reply: func(s string) error { replied = s; return nil },
	})
	assert.Equal(t, commands.OutcomeHandled, result.Outcome)
	assert.Contains(t, replied, "Failed to activate skill")
}

func TestExecuteUnrecognizedReturnsPassthrough(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	exec := commands.NewExecutor(reg, &commands.Runtime{})

	result := exec.Execute(context.Background(), commands.Request{Text: "/unknowncmd"})
	assert.Equal(t, commands.OutcomePassthrough, result.Outcome)
}
