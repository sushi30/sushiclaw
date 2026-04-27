package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Handler is the function signature for command handlers.
type Handler func(ctx context.Context, req Request, rt *Runtime) error

// SubCommand defines a single sub-command within a parent command.
type SubCommand struct {
	Name        string
	Description string
	ArgsUsage   string
	Handler     Handler
}

// Definition is the single-source metadata and behavior for a slash command.
type Definition struct {
	Name        string
	Description string
	Usage       string
	Aliases     []string
	SubCommands []SubCommand
	Handler     Handler
}

// EffectiveUsage returns the usage string.
func (d Definition) EffectiveUsage() string {
	if len(d.SubCommands) == 0 {
		return d.Usage
	}
	names := make([]string, 0, len(d.SubCommands))
	for _, sc := range d.SubCommands {
		name := sc.Name
		if sc.ArgsUsage != "" {
			name += " " + sc.ArgsUsage
		}
		names = append(names, name)
	}
	return fmt.Sprintf("/%s [%s]", d.Name, strings.Join(names, "|"))
}

// Request is the input to a command handler.
type Request struct {
	Channel    string
	ChatID     string
	SenderID   string
	Text       string
	SessionKey string
	Reply      func(text string) error
}

// SkillInfo describes a skill available in the configured workspace.
type SkillInfo struct {
	Name        string
	Description string
}

// Runtime provides runtime dependencies to command handlers.
type Runtime struct {
	GetModelInfo    func() (name, provider string)
	ListDefinitions func() []Definition
	ListModels      func() []string
	ListSkills      func() []SkillInfo
	ListCronJobs    func() (string, error)
	ClearHistory    func(req Request) error
	SetDebug        func(ctx context.Context, channel, chatID, mode string) string
	ActivateSkill   func(req Request, skillName string) error
}

// Registry stores command definitions indexed by name and alias.
type Registry struct {
	defs  []Definition
	index map[string]int
}

// NewRegistry creates a Registry from the given definitions.
func NewRegistry(defs []Definition) *Registry {
	stored := make([]Definition, len(defs))
	copy(stored, defs)
	index := make(map[string]int, len(stored)*2)
	for i, def := range stored {
		registerName(index, def.Name, i)
		for _, alias := range def.Aliases {
			registerName(index, alias, i)
		}
	}
	return &Registry{defs: stored, index: index}
}

// Lookup returns a command definition by normalized name or alias.
func (r *Registry) Lookup(name string) (Definition, bool) {
	key := normalize(name)
	if key == "" {
		return Definition{}, false
	}
	idx, ok := r.index[key]
	if !ok {
		return Definition{}, false
	}
	return r.defs[idx], true
}

// Definitions returns all registered definitions.
func (r *Registry) Definitions() []Definition {
	out := make([]Definition, len(r.defs))
	copy(out, r.defs)
	return out
}

func registerName(index map[string]int, name string, i int) {
	key := normalize(name)
	if key == "" {
		return
	}
	if _, exists := index[key]; exists {
		return
	}
	index[key] = i
}

// Executor dispatches commands using a Registry.
type Executor struct {
	reg *Registry
	rt  *Runtime
}

// NewExecutor creates a new Executor.
func NewExecutor(reg *Registry, rt *Runtime) *Executor {
	return &Executor{reg: reg, rt: rt}
}

// Outcome describes the result of Execute.
type Outcome int

const (
	OutcomePassthrough Outcome = iota
	OutcomeHandled
)

// ExecuteResult is the result of Execute.
type ExecuteResult struct {
	Outcome Outcome
	Command string
	Err     error
}

// Execute dispatches a command or returns OutcomePassthrough if not a command.
func (e *Executor) Execute(ctx context.Context, req Request) ExecuteResult {
	cmdName, ok := CommandName(req.Text)
	if !ok {
		return ExecuteResult{Outcome: OutcomePassthrough}
	}
	if e == nil || e.reg == nil {
		return ExecuteResult{Outcome: OutcomePassthrough, Command: cmdName}
	}
	def, found := e.reg.Lookup(cmdName)
	if !found {
		return ExecuteResult{Outcome: OutcomePassthrough, Command: cmdName}
	}
	return e.executeDefinition(ctx, req, def)
}

func (e *Executor) executeDefinition(ctx context.Context, req Request, def Definition) ExecuteResult {
	if req.Reply == nil {
		req.Reply = func(string) error { return nil }
	}
	if len(def.SubCommands) == 0 {
		if def.Handler == nil {
			return ExecuteResult{Outcome: OutcomePassthrough, Command: def.Name}
		}
		err := def.Handler(ctx, req, e.rt)
		return ExecuteResult{Outcome: OutcomeHandled, Command: def.Name, Err: err}
	}
	subName := nthToken(req.Text, 1)
	if subName == "" {
		_ = req.Reply("Usage: " + def.EffectiveUsage())
		return ExecuteResult{Outcome: OutcomeHandled, Command: def.Name}
	}
	for _, sc := range def.SubCommands {
		if normalize(sc.Name) == normalize(subName) {
			if sc.Handler == nil {
				return ExecuteResult{Outcome: OutcomePassthrough, Command: def.Name}
			}
			err := sc.Handler(ctx, req, e.rt)
			return ExecuteResult{Outcome: OutcomeHandled, Command: def.Name, Err: err}
		}
	}
	_ = req.Reply(fmt.Sprintf("Unknown option: %s. Usage: %s", subName, def.EffectiveUsage()))
	return ExecuteResult{Outcome: OutcomeHandled, Command: def.Name}
}

var commandPrefixes = []string{"/", "!"}

// HasCommandPrefix returns true if input starts with / or !
func HasCommandPrefix(input string) bool {
	token := nthToken(input, 0)
	if token == "" {
		return false
	}
	_, ok := trimCommandPrefix(token)
	return ok
}

// CommandName returns the normalized command name for an input if present.
func CommandName(input string) (string, bool) {
	return parseCommandName(input)
}

func parseCommandName(input string) (string, bool) {
	token := nthToken(input, 0)
	if token == "" {
		return "", false
	}
	name, ok := trimCommandPrefix(token)
	if !ok {
		return "", false
	}
	if i := strings.Index(name, "@"); i >= 0 {
		name = name[:i]
	}
	name = normalize(name)
	if name == "" {
		return "", false
	}
	return name, true
}

func trimCommandPrefix(token string) (string, bool) {
	for _, prefix := range commandPrefixes {
		if strings.HasPrefix(token, prefix) {
			return strings.TrimPrefix(token, prefix), true
		}
	}
	return "", false
}

func nthToken(input string, n int) string {
	parts := strings.Fields(strings.TrimSpace(input))
	if n >= len(parts) {
		return ""
	}
	return parts[n]
}

func normalize(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// BuiltinDefinitions returns sushiclaw's recognized slash commands.
// These are used by the command filter to allow/block commands.
func BuiltinDefinitions() []Definition {
	return []Definition{
		{Name: "start", Description: "Start the bot", Handler: startHandler},
		{Name: "help", Description: "Show this help message", Handler: helpHandler},
		{Name: "clear", Description: "Clear conversation history", Handler: clearHandler},
		{Name: "debug", Description: "Toggle debug event forwarding", Handler: debugHandler, Usage: "/debug [on|off]"},
		{Name: "model", Description: "Show or switch model", Handler: modelHandler},
		{Name: "show", Description: "Show current configuration"},
		{Name: "list", Description: "List available options", SubCommands: []SubCommand{
			{Name: "models", Description: "List configured models", Handler: listModelsHandler},
			{Name: "skills", Description: "List available skills", Handler: listSkillsHandler},
			{Name: "cron", Description: "List scheduled cron jobs", Handler: listCronHandler},
		}},
		{Name: "use", Description: "Use a specific skill", Handler: useHandler, Usage: "/use <skill-name>"},
		{Name: "btw", Description: "Add a note to conversation context"},
		{Name: "switch", Description: "Switch model or channel"},
		{Name: "check", Description: "Check system status"},
		{Name: "subagents", Description: "Manage subagents"},
		{Name: "reload", Description: "Reload configuration"},
	}
}

func startHandler(_ context.Context, req Request, _ *Runtime) error {
	return req.Reply("👋 Sushiclaw is running. Type /help for available commands.")
}

func helpHandler(_ context.Context, req Request, rt *Runtime) error {
	var defs []Definition
	if rt != nil && rt.ListDefinitions != nil {
		defs = rt.ListDefinitions()
	}
	var available []Definition
	for _, d := range defs {
		if d.Handler != nil || len(d.SubCommands) > 0 {
			available = append(available, d)
		}
	}
	if len(available) == 0 {
		return req.Reply("No commands available.")
	}
	var sb strings.Builder
	sb.WriteString("Available commands:\n")
	for _, d := range available {
		sb.WriteString("/" + d.Name)
		if d.Description != "" {
			sb.WriteString(" — " + d.Description)
		}
		sb.WriteByte('\n')
	}
	return req.Reply(strings.TrimRight(sb.String(), "\n"))
}

func listModelsHandler(_ context.Context, req Request, rt *Runtime) error {
	if rt == nil || rt.ListModels == nil {
		return req.Reply("Model list unavailable.")
	}
	models := rt.ListModels()
	if len(models) == 0 {
		return req.Reply("No models configured.")
	}
	var sb strings.Builder
	sb.WriteString("Configured models:\n")
	for _, m := range models {
		sb.WriteString("• " + m + "\n")
	}
	return req.Reply(strings.TrimRight(sb.String(), "\n"))
}

func listSkillsHandler(_ context.Context, req Request, rt *Runtime) error {
	if rt == nil || rt.ListSkills == nil {
		return req.Reply("Skill list unavailable.")
	}
	skills := rt.ListSkills()
	if len(skills) == 0 {
		return req.Reply("No skills available.")
	}
	var sb strings.Builder
	sb.WriteString("Available skills:\n")
	for _, s := range skills {
		sb.WriteString("• " + s.Name)
		if s.Description != "" {
			sb.WriteString(" — " + s.Description)
		}
		sb.WriteByte('\n')
	}
	return req.Reply(strings.TrimRight(sb.String(), "\n"))
}

func listCronHandler(_ context.Context, req Request, rt *Runtime) error {
	if rt == nil || rt.ListCronJobs == nil {
		return req.Reply("Cron job list unavailable.")
	}
	jobs, err := rt.ListCronJobs()
	if err != nil {
		return req.Reply("Failed to list cron jobs: " + err.Error())
	}
	return req.Reply(jobs)
}

func clearHandler(_ context.Context, req Request, rt *Runtime) error {
	if rt != nil && rt.ClearHistory != nil {
		if err := rt.ClearHistory(req); err != nil {
			return req.Reply("Failed to clear history: " + err.Error())
		}
	}
	return req.Reply("History cleared.")
}

func modelHandler(_ context.Context, req Request, rt *Runtime) error {
	if rt == nil || rt.GetModelInfo == nil {
		return req.Reply("Model info unavailable.")
	}
	name, provider := rt.GetModelInfo()
	if name == "" {
		return req.Reply("No model configured.")
	}
	return req.Reply(fmt.Sprintf("Current model: %s (%s)", name, provider))
}

func debugHandler(ctx context.Context, req Request, rt *Runtime) error {
	if rt == nil || rt.SetDebug == nil {
		return req.Reply("Debug toggle unavailable.")
	}
	mode := nthToken(req.Text, 1)
	if mode == "" {
		mode = "toggle"
	}
	mode = normalize(mode)
	if mode != "toggle" && mode != "on" && mode != "off" {
		return req.Reply("Usage: /debug [on|off]")
	}
	if nthToken(req.Text, 2) != "" {
		return req.Reply("Usage: /debug [on|off]")
	}
	reply := rt.SetDebug(ctx, req.Channel, req.ChatID, mode)
	return req.Reply(reply)
}

// ErrSkillAlreadyLoaded is returned when a skill is already active in the session.
var ErrSkillAlreadyLoaded = errors.New("skill already loaded")

func useHandler(_ context.Context, req Request, rt *Runtime) error {
	skillName := nthToken(req.Text, 1)
	if skillName == "" {
		return req.Reply("Usage: /use <skill-name>")
	}
	if rt == nil || rt.ActivateSkill == nil {
		return req.Reply("Skill activation is not available.")
	}
	if err := rt.ActivateSkill(req, skillName); err != nil {
		if errors.Is(err, ErrSkillAlreadyLoaded) {
			return req.Reply(fmt.Sprintf("Skill %s is already loaded.", skillName))
		}
		return req.Reply(fmt.Sprintf("Failed to activate skill: %v", err))
	}
	return req.Reply(fmt.Sprintf("Skill %s activated.", skillName))
}
