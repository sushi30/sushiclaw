package commandfilter

import (
	"testing"

	"github.com/sushi30/sushiclaw/pkg/bus"
)

func msg(content string) bus.InboundMessage {
	return bus.InboundMessage{Channel: "telegram", ChatID: "123", Content: content}
}

func systemMsg(content string) bus.InboundMessage {
	return bus.InboundMessage{Channel: "system", ChatID: "123", Content: content}
}

func TestFilter_PlainText(t *testing.T) {
	f := NewCommandFilter()
	dec := f.Filter(msg("hello world"))
	if dec.Result != Pass {
		t.Errorf("plain text should pass, got %v", dec.Result)
	}
}

func TestFilter_SystemChannel(t *testing.T) {
	f := NewCommandFilter()
	dec := f.Filter(systemMsg("/nonexistent"))
	if dec.Result != Pass {
		t.Errorf("system messages should always pass, got %v", dec.Result)
	}
}

func TestFilter_KnownCommands(t *testing.T) {
	f := NewCommandFilter()
	cases := []string{
		"/start", "/help", "/show", "/list", "/use",
		"/switch", "/check", "/clear", "/subagents", "/reload",
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			dec := f.Filter(msg(cmd))
			if dec.Result != Pass {
				t.Errorf("known command %q should pass, got Block with err=%q", cmd, dec.ErrMsg)
			}
		})
	}
}

func TestFilter_KnownCommandsWithArgs(t *testing.T) {
	f := NewCommandFilter()
	cases := []string{
		"/show model",
		"/list skills",
		"/switch model gpt-4o",
		"/check channel telegram",
		"/use python",
		"/clear",
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			dec := f.Filter(msg(cmd))
			if dec.Result != Pass {
				t.Errorf("known command %q should pass, got Block with err=%q", cmd, dec.ErrMsg)
			}
		})
	}
}

func TestFilter_UnknownCommands(t *testing.T) {
	f := NewCommandFilter()
	cases := []struct {
		input   string
		cmdName string
	}{
		{"/nonexistent", "nonexistent"},
		{"/foo", "foo"},
		{"/123", "123"},
		{"/do_something arg1", "do_something"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			dec := f.Filter(msg(tc.input))
			if dec.Result != Block {
				t.Errorf("unknown command %q should be blocked, got %v", tc.input, dec.Result)
			}
			if dec.Command != tc.cmdName {
				t.Errorf("expected command name %q, got %q", tc.cmdName, dec.Command)
			}
			expected := "Unknown command: /" + tc.cmdName
			if dec.ErrMsg != expected {
				t.Errorf("expected error %q, got %q", expected, dec.ErrMsg)
			}
		})
	}
}

func TestFilter_BangPrefix(t *testing.T) {
	f := NewCommandFilter()
	dec := f.Filter(msg("!help"))
	if dec.Result != Pass {
		t.Errorf("!help should pass as known command, got %v", dec.Result)
	}

	dec = f.Filter(msg("!unknown"))
	if dec.Result != Block {
		t.Errorf("!unknown should be blocked, got %v", dec.Result)
	}
}

func TestFilter_CaseInsensitive(t *testing.T) {
	f := NewCommandFilter()
	dec := f.Filter(msg("/HELP"))
	if dec.Result != Pass {
		t.Errorf("/HELP should pass (case-insensitive), got %v", dec.Result)
	}

	dec = f.Filter(msg("/Help"))
	if dec.Result != Pass {
		t.Errorf("/Help should pass (case-insensitive), got %v", dec.Result)
	}
}

func TestFilter_LeadingWhitespace(t *testing.T) {
	f := NewCommandFilter()
	dec := f.Filter(msg("  /help"))
	if dec.Result != Pass {
		t.Errorf("  /help should pass (leading whitespace), got %v", dec.Result)
	}
}

func TestFilter_EmptyString(t *testing.T) {
	f := NewCommandFilter()
	dec := f.Filter(msg(""))
	if dec.Result != Pass {
		t.Errorf("empty string should pass, got %v", dec.Result)
	}
}

func TestFilter_PlainTextWithSlash(t *testing.T) {
	f := NewCommandFilter()
	dec := f.Filter(msg("hello /world"))
	if dec.Result != Pass {
		t.Errorf("slash not at start should pass as plain text, got %v", dec.Result)
	}
}
