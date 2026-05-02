package tools

import (
	"context"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/sushi30/sushiclaw/pkg/logger"
	"github.com/sushi30/sushiclaw/pkg/tools/toolctx"
)

type debugLoggingTool struct {
	tool interfaces.Tool
}

// WithDebugLogging wraps a tool with debug logging for tool invocations.
func WithDebugLogging(tool interfaces.Tool) interfaces.Tool {
	if tool == nil {
		return nil
	}
	return debugLoggingTool{tool: tool}
}

func withDebugLogging(tool interfaces.Tool) interfaces.Tool {
	return WithDebugLogging(tool)
}

func (t debugLoggingTool) Name() string { return t.tool.Name() }

func (t debugLoggingTool) Description() string { return t.tool.Description() }

func (t debugLoggingTool) Parameters() map[string]interfaces.ParameterSpec {
	return t.tool.Parameters()
}

func (t debugLoggingTool) Run(ctx context.Context, input string) (string, error) {
	return t.logAndRun(ctx, input)
}

func (t debugLoggingTool) Execute(ctx context.Context, input string) (string, error) {
	return t.logAndRun(ctx, input)
}

func (t debugLoggingTool) logAndRun(ctx context.Context, input string) (string, error) {
	start := time.Now()
	fields := map[string]any{
		"tool":      t.tool.Name(),
		"params":    input,
		"channel":   toolctx.ChannelFromContext(ctx),
		"chat_id":   toolctx.ChatIDFromContext(ctx),
		"sender_id": toolctx.SenderIDFromContext(ctx),
	}
	logger.DebugCF("tool", "Tool invoked", fields)

	output, err := t.tool.Run(ctx, input)
	fields["duration"] = time.Since(start).Round(time.Millisecond).String()
	fields["output_bytes"] = len(output)
	if err != nil {
		fields["error"] = err.Error()
		logger.DebugCF("tool", "Tool failed", fields)
		return output, err
	}
	logger.DebugCF("tool", "Tool completed", fields)
	return output, nil
}
