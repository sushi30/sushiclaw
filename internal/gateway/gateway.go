package gateway

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/sushi30/sushiclaw/internal/envresolve"
	"github.com/sushi30/sushiclaw/pkg/channels/email"
	sushitools "github.com/sushi30/sushiclaw/pkg/tools"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/audio/asr"
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
		return fmt.Errorf("error enabling file logging: %w", err)
	}
	defer logger.DisableFileLogging()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}
	envresolve.Config(cfg)

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

	provider = wrapWithRetryEmpty(provider)

	// externalBus: channels publish inbound here; channel manager reads outbound from here.
	// agentBus: agent loop reads inbound from here; publishes outbound here.
	// A bridge goroutine intercepts /debug on the inbound path and forwards everything
	// else to agentBus. A second bridge forwards agent responses back to externalBus.
	externalBus := bus.NewMessageBus()
	agentBus := bus.NewMessageBus()
	agentLoop := agent.NewAgentLoop(cfg, agentBus, provider)

	if allowedSenders := sushitools.ParseAllowedSenders(); len(allowedSenders) > 0 {
		if cfg.Tools.IsToolEnabled("exec") {
			workingDir := cfg.Agents.Defaults.Workspace
			restrict := cfg.Agents.Defaults.RestrictToWorkspace
			trustedExec, err := sushitools.NewTrustedExecTool(cfg, workingDir, restrict, allowedSenders)
			if err != nil {
				logger.WarnCF("gateway", "Failed to init trusted exec tool",
					map[string]any{"error": err.Error()})
			} else {
				agentLoop.RegisterTool(trustedExec)
				logger.InfoCF("gateway", "Trusted exec registered",
					map[string]any{"senders": allowedSenders})
			}
		}
	}

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

	cm, err := channels.NewManager(cfg, externalBus, mediaStore)
	if err != nil {
		return fmt.Errorf("error creating channel manager: %w", err)
	}

	emailCh, err := email.InitChannel(externalBus)
	if err != nil {
		return fmt.Errorf("email channel: %w", err)
	}
	if emailCh != nil {
		cm.RegisterChannel("email", emailCh)
	}

	// Also register the channel manager as stream delegate on agentBus so that
	// the agent loop's streaming queries (al.bus.GetStreamer) reach the manager.
	agentBus.SetStreamDelegate(cm)

	agentLoop.SetChannelManager(cm)
	agentLoop.SetMediaStore(mediaStore)

	if transcriber := asr.DetectTranscriber(cfg); transcriber != nil {
		agentLoop.SetTranscriber(transcriber)
		logger.InfoCF("voice", "Transcription enabled", map[string]any{"provider": transcriber.Name()})
	}

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

	debugMgr := &DebugManager{agentLoop: agentLoop, externalBus: externalBus}

	// Inbound bridge: intercept /debug before forwarding to agent loop.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-externalBus.InboundChan():
				if !ok {
					return
				}
				handleInbound(ctx, msg, debugMgr, agentBus, externalBus)
			}
		}
	}()

	// Outbound bridge: forward agent responses and media back to channels.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-agentBus.OutboundChan():
				if !ok {
					return
				}
				_ = externalBus.PublishOutbound(ctx, msg)
			}
		}
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-agentBus.OutboundMediaChan():
				if !ok {
					return
				}
				_ = externalBus.PublishOutboundMedia(ctx, msg)
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

	if cp, ok := provider.(providers.StatefulProvider); ok {
		cp.Close()
	}
	agentLoop.Stop()
	agentLoop.Close()
	agentBus.Close()
	externalBus.Close()

	logger.Info("Gateway stopped")
	return nil
}

// handleInbound processes a single inbound message from externalBus.
// /debug is intercepted and handled by debugMgr; all other messages are
// forwarded to agentBus for normal agent-loop processing.
func handleInbound(ctx context.Context, msg bus.InboundMessage, debugMgr *DebugManager, agentBus, externalBus *bus.MessageBus) {
	if strings.TrimSpace(msg.Content) == "/debug" {
		reply := debugMgr.Toggle(ctx, msg.Channel, msg.ChatID)
		_ = externalBus.PublishOutbound(ctx, bus.OutboundMessage{
			Channel: msg.Channel,
			ChatID:  msg.ChatID,
			Content: reply,
		})
		return
	}
	_ = agentBus.PublishInbound(ctx, msg)
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
