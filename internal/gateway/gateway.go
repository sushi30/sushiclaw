package gateway

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/health"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/media"
	"github.com/sipeed/picoclaw/pkg/pid"
	"github.com/sipeed/picoclaw/pkg/providers"
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
		logger.Fatal(fmt.Sprintf("error enabling file logging: %v", err))
	}
	defer logger.DisableFileLogging()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		logger.Fatalf("error loading config: %v", err)
	}

	if cfg.Gateway.Port <= 0 || cfg.Gateway.Port > 65535 {
		return fmt.Errorf("invalid gateway port: %d", cfg.Gateway.Port)
	}

	if debug {
		logger.SetLevel(logger.DEBUG)
		fmt.Println("Debug mode enabled")
	} else {
		logger.SetLevelFromString(cfg.Gateway.LogLevel)
	}

	pidData, err := pid.WritePidFile(homePath, cfg.Gateway.Host, cfg.Gateway.Port)
	if err != nil {
		return fmt.Errorf("singleton check failed: %w", err)
	}
	defer pid.RemovePidFile(homePath)

	provider, modelID, err := createProvider(cfg, allowEmptyStartup)
	if err != nil {
		return fmt.Errorf("error creating provider: %w", err)
	}
	if modelID != "" {
		cfg.Agents.Defaults.ModelName = modelID
	}

	msgBus := bus.NewMessageBus()
	agentLoop := agent.NewAgentLoop(cfg, msgBus, provider)

	startupInfo := agentLoop.GetStartupInfo()
	toolsInfo := startupInfo["tools"].(map[string]any)
	skillsInfo := startupInfo["skills"].(map[string]any)
	fmt.Printf("\nAgent Status:\n  Tools: %d loaded\n  Skills: %d/%d available\n",
		toolsInfo["count"], skillsInfo["available"], skillsInfo["total"])

	mediaStore := media.NewFileMediaStoreWithCleanup(media.MediaCleanerConfig{
		Enabled:  cfg.Tools.MediaCleanup.Enabled,
		MaxAge:   time.Duration(cfg.Tools.MediaCleanup.MaxAge) * time.Minute,
		Interval: time.Duration(cfg.Tools.MediaCleanup.Interval) * time.Minute,
	})
	mediaStore.Start()
	defer mediaStore.Stop()

	cm, err := channels.NewManager(cfg, msgBus, mediaStore)
	if err != nil {
		return fmt.Errorf("error creating channel manager: %w", err)
	}

	agentLoop.SetChannelManager(cm)
	agentLoop.SetMediaStore(mediaStore)

	enabledChannels := cm.GetEnabledChannels()
	if len(enabledChannels) > 0 {
		fmt.Printf("Channels enabled: %s\n", enabledChannels)
	} else {
		fmt.Println("Warning: no channels enabled")
	}

	addr := fmt.Sprintf("%s:%d", cfg.Gateway.Host, cfg.Gateway.Port)
	healthServer := health.NewServer(cfg.Gateway.Host, cfg.Gateway.Port, pidData.Token)
	cm.SetupHTTPServer(addr, healthServer)

	if err = cm.StartAll(context.Background()); err != nil {
		return fmt.Errorf("error starting channels: %w", err)
	}

	fmt.Printf("Gateway started on %s:%d\n", cfg.Gateway.Host, cfg.Gateway.Port)
	fmt.Println("Press Ctrl+C to stop")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go agentLoop.Run(ctx) //nolint:errcheck

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
	defer shutdownCancel()
	cm.StopAll(shutdownCtx)

	if cp, ok := provider.(providers.StatefulProvider); ok {
		cp.Close()
	}
	agentLoop.Stop()
	agentLoop.Close()

	logger.Info("Gateway stopped")
	return nil
}

type startupBlockedProvider struct{ reason string }

func (p *startupBlockedProvider) Chat(
	_ context.Context,
	_ []providers.Message,
	_ []providers.ToolDefinition,
	_ string,
	_ map[string]any,
) (*providers.LLMResponse, error) {
	return nil, fmt.Errorf("%s", p.reason)
}

func (p *startupBlockedProvider) GetDefaultModel() string { return "" }

func createProvider(cfg *config.Config, allowEmptyStartup bool) (providers.LLMProvider, string, error) {
	if cfg.Agents.Defaults.GetModelName() == "" && allowEmptyStartup {
		reason := "no default model configured; gateway started in limited mode"
		fmt.Printf("Warning: %s\n", reason)
		return &startupBlockedProvider{reason: reason}, "", nil
	}
	return providers.CreateProvider(cfg)
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
	if p := os.Getenv(config.EnvConfig); p != "" {
		return p
	}
	return filepath.Join(GetHome(), "config.json")
}
