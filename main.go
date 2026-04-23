// Sushiclaw - Personal AI agent, WhatsApp-first
// Based on picoclaw (github.com/sipeed/picoclaw)

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sushi30/sushiclaw/internal/chat"
	"github.com/sushi30/sushiclaw/internal/envresolve"
	"github.com/sushi30/sushiclaw/internal/gateway"
	"github.com/sushi30/sushiclaw/internal/version"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/logger"

	// Register owned channel implementations.
	_ "github.com/sushi30/sushiclaw/pkg/channels/telegram"
	_ "github.com/sushi30/sushiclaw/pkg/channels/whatsapp_native"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stdout, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sushiclaw",
		Short: "Sushiclaw personal AI agent",
	}
	cmd.AddCommand(newGatewayCommand())
	cmd.AddCommand(newVersionCommand())
	cmd.AddCommand(newChatCommand())
	return cmd
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Aliases: []string{"v"},
		Short:   "Print build version info",
		Args:    cobra.NoArgs,
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println(version.FormatVersion())
		},
	}
}

func newGatewayCommand() *cobra.Command {
	var debug bool
	var allowEmpty bool

	cmd := &cobra.Command{
		Use:     "gateway",
		Aliases: []string{"g"},
		Short:   "Start the sushiclaw gateway",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return gateway.Run(debug, gateway.GetHome(), gateway.GetConfigPath(), allowEmpty)
		},
	}

	cmd.Flags().BoolVarP(&debug, "debug", "d", false, "Enable debug logging")
	cmd.Flags().BoolVarP(&allowEmpty, "allow-empty", "E", false, "Start even without a default model configured")

	return cmd
}

func newChatCommand() *cobra.Command {
	var debug bool

	cmd := &cobra.Command{
		Use:     "chat",
		Aliases: []string{"c"},
		Short:   "Start an interactive terminal chat with the agent",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if debug {
				logger.SetLevel(logger.DEBUG)
			}

			cfg, err := config.LoadConfig(gateway.GetConfigPath())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			envresolve.Config(cfg)

			if cfg.Agents.Defaults.ModelName == "" {
				return fmt.Errorf("no default model configured (set model_name in config)")
			}

			runner, err := chat.NewRunner(cfg)
			if err != nil {
				return err
			}

			return runner.Run(cmd.Context())
		},
	}

	cmd.Flags().BoolVarP(&debug, "debug", "d", false, "Enable debug logging")

	return cmd
}
