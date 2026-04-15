package commandfilter

import (
	"fmt"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/commands"
)

type CommandFilter struct {
	reg *commands.Registry
}

func NewCommandFilter() *CommandFilter {
	return &CommandFilter{
		reg: commands.NewRegistry(commands.BuiltinDefinitions()),
	}
}

type FilterResult int

const (
	Pass FilterResult = iota
	Block
)

type FilterDecision struct {
	Result  FilterResult
	ErrMsg  string
	Command string
}

func (f *CommandFilter) Filter(msg bus.InboundMessage) FilterDecision {
	if msg.Channel == "system" {
		return FilterDecision{Result: Pass}
	}

	if !commands.HasCommandPrefix(msg.Content) {
		return FilterDecision{Result: Pass}
	}

	cmdName, ok := commands.CommandName(msg.Content)
	if !ok {
		return FilterDecision{Result: Pass}
	}

	if _, found := f.reg.Lookup(cmdName); found {
		return FilterDecision{Result: Pass}
	}

	return FilterDecision{
		Result:  Block,
		ErrMsg:  fmt.Sprintf("Unknown command: /%s", cmdName),
		Command: cmdName,
	}
}
