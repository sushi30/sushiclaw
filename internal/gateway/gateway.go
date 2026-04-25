package gateway

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
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
	dm := NewDebugManager(messageBus)
	rt := &commands.Runtime{
		ListDefinitions: reg.Definitions,
	}

	var tools []interfaces.Tool
	if allowedSenders := sushitools.ParseAllowedSenders(); len(allowedSenders) > 0 {
		if cfg.Tools.IsToolEnabled("exec") {
			workingDir := cfg.WorkspacePath()
			restrict := cfg.Agents.Defaults.RestrictToWorkspace
			trustedExec, err := sushitools.NewTrustedExecTool(cfg, workingDir, restrict, allowedSenders)
			if err != nil {
				logger.WarnCF("gateway", "Failed to init trusted exec tool",
					map[string]any{"error": err.Error()})
			} else {
				tools = append(tools, trustedExec)
				logger.InfoCF("gateway", "Trusted exec registered",
					map[string]any{"senders": allowedSenders})
			}
		}
	}

	sessionMgr, err := agent.NewSessionManager(cfg, messageBus, tools)
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

	rt.ToggleDebug = dm.Toggle
	if sessionMgr != nil {
		rt.ClearHistory = sessionMgr.ClearHistory
		rt.GetModelInfo = sessionMgr.GetModelInfo
		rt.ListModels = sessionMgr.ListModels
	}
	executor := commands.NewExecutor(reg, rt)
	router := &inboundRouter{
		messageBus:     messageBus,
		cmdFilter:      cmdFilter,
		executor:       executor,
		autoOnboarding: newOnboardingState(cfg.Onboarding),
	}
	if sessionMgr != nil {
		router.sessionDispatcher = func(ctx context.Context, msg bus.InboundMessage) {
			go sessionMgr.Dispatch(ctx, msg)
		}
	}

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
				router.handleMessage(ctx, msg)
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
