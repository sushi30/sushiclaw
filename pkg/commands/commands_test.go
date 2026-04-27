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
		ClearHistory: func(req commands.Request) error { cleared = true; return nil },
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
	mode := ""
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	rt := &commands.Runtime{
		SetDebug: func(_ context.Context, _, _, m string) string { mode = m; return "Debug toggled." },
	}
	exec := commands.NewExecutor(reg, rt)

	var replied string
	result := exec.Execute(context.Background(), commands.Request{
		Text:  "/debug",
		Reply: func(s string) error { replied = s; return nil },
	})
	assert.Equal(t, commands.OutcomeHandled, result.Outcome)
	assert.Equal(t, "toggle", mode)
	assert.Equal(t, "Debug toggled.", replied)
}

func TestExecuteDebugOnOff(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	var modes []string
	rt := &commands.Runtime{
		SetDebug: func(_ context.Context, _, _, mode string) string {
			modes = append(modes, mode)
			return "ok"
		},
	}
	exec := commands.NewExecutor(reg, rt)

	for _, text := range []string{"/debug on", "/debug off"} {
		result := exec.Execute(context.Background(), commands.Request{
			Text:  text,
			Reply: func(string) error { return nil },
		})
		assert.Equal(t, commands.OutcomeHandled, result.Outcome)
	}

	assert.Equal(t, []string{"on", "off"}, modes)
}

func TestExecuteDebugInvalidArgs(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	exec := commands.NewExecutor(reg, &commands.Runtime{
		SetDebug: func(context.Context, string, string, string) string {
			t.Fatal("SetDebug should not be called for invalid args")
			return ""
		},
	})

	for _, text := range []string{"/debug maybe", "/debug on extra"} {
		var replied string
		result := exec.Execute(context.Background(), commands.Request{
			Text:  text,
			Reply: func(s string) error { replied = s; return nil },
		})
		assert.Equal(t, commands.OutcomeHandled, result.Outcome)
		assert.Equal(t, "Usage: /debug [on|off]", replied)
	}
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

func TestExecuteListCronCallsCallback(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	rt := &commands.Runtime{
		ListCronJobs: func() (string, error) {
			return "Scheduled jobs:\n- daily-check cron: 0 9 * * *", nil
		},
	}
	exec := commands.NewExecutor(reg, rt)

	var replied string
	result := exec.Execute(context.Background(), commands.Request{
		Text:  "/list cron",
		Reply: func(s string) error { replied = s; return nil },
	})
	assert.Equal(t, commands.OutcomeHandled, result.Outcome)
	assert.Contains(t, replied, "daily-check")
}

func TestExecuteListCronUnavailable(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	exec := commands.NewExecutor(reg, &commands.Runtime{})

	var replied string
	result := exec.Execute(context.Background(), commands.Request{
		Text:  "/list cron",
		Reply: func(s string) error { replied = s; return nil },
	})
	assert.Equal(t, commands.OutcomeHandled, result.Outcome)
	assert.Contains(t, replied, "Cron job list unavailable.")
}

func TestExecutePassthroughNoHandler(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	exec := commands.NewExecutor(reg, &commands.Runtime{})

	// /show has no handler — should pass through to agent
	result := exec.Execute(context.Background(), commands.Request{Text: "/show"})
	assert.Equal(t, commands.OutcomePassthrough, result.Outcome)
}

func TestExecuteListSkills(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	rt := &commands.Runtime{
		ListSkills: func() []commands.SkillInfo {
			return []commands.SkillInfo{
				{Name: "python", Description: "Python coding help"},
				{Name: "review"},
			}
		},
	}
	exec := commands.NewExecutor(reg, rt)

	var replied string
	result := exec.Execute(context.Background(), commands.Request{
		Text:  "/list skills",
		Reply: func(s string) error { replied = s; return nil },
	})
	assert.Equal(t, commands.OutcomeHandled, result.Outcome)
	assert.Contains(t, replied, "Available skills:")
	assert.Contains(t, replied, "• python — Python coding help")
	assert.Contains(t, replied, "• review")
}

func TestExecuteListSkillsNoRuntime(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	exec := commands.NewExecutor(reg, &commands.Runtime{})

	var replied string
	result := exec.Execute(context.Background(), commands.Request{
		Text:  "/list skills",
		Reply: func(s string) error { replied = s; return nil },
	})
	assert.Equal(t, commands.OutcomeHandled, result.Outcome)
	assert.Equal(t, "Skill list unavailable.", replied)
}

func TestExecuteListSkillsEmpty(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	rt := &commands.Runtime{
		ListSkills: func() []commands.SkillInfo { return nil },
	}
	exec := commands.NewExecutor(reg, rt)

	var replied string
	result := exec.Execute(context.Background(), commands.Request{
		Text:  "/list skills",
		Reply: func(s string) error { replied = s; return nil },
	})
	assert.Equal(t, commands.OutcomeHandled, result.Outcome)
	assert.Equal(t, "No skills available.", replied)
}

func TestExecuteListUsageIncludesSkills(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	exec := commands.NewExecutor(reg, &commands.Runtime{})

	var replied string
	result := exec.Execute(context.Background(), commands.Request{
		Text:  "/list",
		Reply: func(s string) error { replied = s; return nil },
	})
	assert.Equal(t, commands.OutcomeHandled, result.Outcome)
	assert.Contains(t, replied, "/list [models|skills|cron]")
}

func TestExecuteListUnknownOptionIncludesSkills(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	exec := commands.NewExecutor(reg, &commands.Runtime{})

	var replied string
	result := exec.Execute(context.Background(), commands.Request{
		Text:  "/list widgets",
		Reply: func(s string) error { replied = s; return nil },
	})
	assert.Equal(t, commands.OutcomeHandled, result.Outcome)
	assert.Contains(t, replied, "Unknown option: widgets")
	assert.Contains(t, replied, "/list [models|skills|cron]")
}

func TestExecuteUseSuccess(t *testing.T) {
	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	rt := &commands.Runtime{
		ActivateSkill: func(req commands.Request, name string) error { return nil },
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
		ActivateSkill: func(req commands.Request, name string) error { return commands.ErrSkillAlreadyLoaded },
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
		ActivateSkill: func(req commands.Request, name string) error { return assert.AnError },
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
