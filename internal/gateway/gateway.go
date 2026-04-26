package gateway

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sushi30/sushiclaw/internal/agent"
	"github.com/sushi30/sushiclaw/internal/commandfilter"
	"github.com/sushi30/sushiclaw/internal/envresolve"
	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/channels"
	"github.com/sushi30/sushiclaw/pkg/channels/email"
	"github.com/sushi30/sushiclaw/pkg/commands"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/logger"
	"github.com/sushi30/sushiclaw/pkg/media"
	sushitools "github.com/sushi30/sushiclaw/pkg/tools"
	"github.com/sushi30/sushiclaw/pkg/tools/websearch"
)

const (
	gracefulShutdownTimeout = 15 * time.Second

	logPath   = "logs"
	panicFile = "gateway_panic.log"
	logFile   = "gateway.log"
)

// Run starts the sushiclaw gateway.
func Run(debug bool, homePath, configPath string, allowEmptyStartup bool) error {
	panicPath := filepath.Join(homePath, logPath, panicFile)
	panicFunc, err := logger.InitPanic(panicPath)
	if err != nil {
		return fmt.Errorf("error initializing panic log: %w", err)
	}
	defer panicFunc()

	if err = logger.EnableFileLogging(filepath.Join(homePath, logPath, logFile)); err != nil {
		return fmt.Errorf("error enabling file logging: %w", err)
	}
	defer logger.DisableFileLogging()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}
	envresolve.Config(cfg)

	if debug {
		logger.SetLevel(logger.DEBUG)
		fmt.Println("Debug mode enabled")
	} else {
		logger.SetLevelFromString(cfg.Gateway.LogLevel)
	}

	if cfg.Agents.Defaults.ModelName == "" && !allowEmptyStartup {
		return fmt.Errorf("no default model configured (use --allow-empty to start without a model)")
	}

	// Single bus architecture: all channels and the agent share one MessageBus.
	messageBus := bus.NewMessageBus()
	cmdFilter := commandfilter.NewCommandFilter()

	reg := commands.NewRegistry(commands.BuiltinDefinitions())
	heartbeat := time.Duration(cfg.Gateway.DebugHeartbeatSeconds) * time.Second
	dm := NewDebugManager(messageBus, heartbeat)
	rt := &commands.Runtime{
		ListDefinitions: reg.Definitions,
	}

	allowedSenders := sushitools.ParseAllowedSenders()
	tools, err := sushitools.NewGatewayTools(cfg, allowedSenders)
	if err != nil {
		logger.WarnCF("gateway", "Failed to init trusted exec tool",
			map[string]any{"error": err.Error()})
	}
	if cfg.Tools.IsToolEnabled("exec") && len(allowedSenders) > 0 && err == nil {
		logger.InfoCF("gateway", "Trusted exec registered",
			map[string]any{"senders": allowedSenders})
	}

	if cfg.Tools.IsToolEnabled("web_search") {
		wsTool, err := websearch.NewTool(cfg.Tools.WebSearch)
		if err != nil {
			logger.WarnCF("gateway", "Failed to init web search tool",
				map[string]any{"error": err.Error()})
		} else {
			tools = append(tools, wsTool)
			logger.InfoCF("gateway", "Web search tool registered",
				map[string]any{"provider": cfg.Tools.WebSearch.Provider})
		}
	}

	sessionMgr, err := agent.NewSessionManager(cfg, messageBus, tools, agent.WithProgressSink(dm))
	if err != nil {
		if allowEmptyStartup {
			logger.WarnC("gateway", fmt.Sprintf("Failed to create agent session: %v", err))
		} else {
			return fmt.Errorf("error creating agent session: %w", err)
		}
	}

	mediaStore := media.NewFileMediaStoreWithCleanup(media.MediaCleanerConfig{
		Enabled:  cfg.Tools.MediaCleanup.Enabled,
		MaxAge:   time.Duration(cfg.Tools.MediaCleanup.MaxAge) * time.Minute,
		Interval: time.Duration(cfg.Tools.MediaCleanup.Interval) * time.Minute,
	})
	mediaStore.Start()
	defer mediaStore.Stop()

	cm, err := channels.NewManager(cfg, messageBus, mediaStore)
	if err != nil {
		return fmt.Errorf("error creating channel manager: %w", err)
	}

	emailCh, err := email.InitChannel(messageBus)
	if err != nil {
		return fmt.Errorf("email channel: %w", err)
	}
	if emailCh != nil {
		cm.RegisterChannel("email", emailCh)
	}

	messageBus.SetStreamDelegate(cm)

	enabledChannels := cm.GetEnabledChannels()
	if len(enabledChannels) > 0 {
		fmt.Printf("Channels enabled: %s\n", enabledChannels)
	} else {
		fmt.Println("Warning: no channels enabled")
	}

	if err = cm.StartAll(context.Background()); err != nil {
		return fmt.Errorf("error starting channels: %w", err)
	}

	fmt.Printf("Gateway started\n")
	fmt.Println("Press Ctrl+C to stop")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt.SetDebug = dm.Set
	if sessionMgr != nil {
		rt.ClearHistory = sessionMgr.ClearHistory
		rt.GetModelInfo = sessionMgr.GetModelInfo
		rt.ListModels = sessionMgr.ListModels
		rt.ListSkills = sessionMgr.ListSkills
		rt.ActivateSkill = sessionMgr.ActivateSkill
	}
	executor := commands.NewExecutor(reg, rt)

	// Inbound processing: filter commands, execute handled ones locally,
	// forward the rest to the agent session.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-messageBus.InboundChan():
				if !ok {
					return
				}
				dec := cmdFilter.Filter(msg)
				logger.DebugCF("commandfilter", "Filtered message",
					map[string]any{
						"text":    msg.Content,
						"result":  dec.Result,
						"command": dec.Command,
					})
				if dec.Result == commandfilter.Block {
					logger.InfoCF("commandfilter", "Blocked unrecognized slash command",
						map[string]any{
							"channel": msg.Channel,
							"chat_id": msg.ChatID,
							"command": dec.Command,
						})
					_ = messageBus.PublishOutbound(ctx, bus.OutboundMessage{
						Channel: msg.Channel,
						ChatID:  msg.ChatID,
						Content: dec.ErrMsg,
					})
					continue
				}

				if commands.HasCommandPrefix(msg.Content) {
					var reply string
					result := executor.Execute(ctx, commands.Request{
						Channel:  msg.Channel,
						ChatID:   msg.ChatID,
						SenderID: msg.SenderID,
						Text:     msg.Content,
						Reply:    func(text string) error { reply = text; return nil },
					})
					logger.DebugCF("executor", "Command executed",
						map[string]any{
							"text":    msg.Content,
							"command": result.Command,
							"outcome": result.Outcome,
							"handled": result.Outcome == commands.OutcomeHandled,
						})
					if result.Outcome == commands.OutcomeHandled {
						if reply != "" {
							_ = messageBus.PublishOutbound(ctx, bus.OutboundMessage{
								Channel: msg.Channel,
								ChatID:  msg.ChatID,
								Content: reply,
							})
						}
						continue
					}
					// OutcomePassthrough: forward to agent.
				}

				// Dispatch to agent session directly (avoids bus read race).
				if sessionMgr != nil {
					go sessionMgr.Dispatch(ctx, msg)
				}
			}
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
	defer shutdownCancel()
	_ = cm.StopAll(shutdownCtx)

	messageBus.Close()

	logger.Info("Gateway stopped")
	return nil
}

// GetHome returns the sushiclaw home directory.
// Priority: $SUSHICLAW_HOME > $PICOCLAW_HOME > ~/.picoclaw
func GetHome() string {
	if h := os.Getenv("SUSHICLAW_HOME"); h != "" {
		return h
	}
	return config.GetHome()
}

// GetConfigPath returns the config file path.
func GetConfigPath() string {
	if p := os.Getenv("SUSHICLAW_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(GetHome(), "config.json")
}
