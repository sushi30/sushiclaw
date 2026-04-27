// Sushiclaw - Personal AI agent, WhatsApp-first
// Based on picoclaw (github.com/sipeed/picoclaw)

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/sushi30/sushiclaw/internal/chat"
	"github.com/sushi30/sushiclaw/internal/envresolve"
	"github.com/sushi30/sushiclaw/internal/gateway"
	"github.com/sushi30/sushiclaw/internal/version"
	"github.com/sushi30/sushiclaw/pkg/config"
	"github.com/sushi30/sushiclaw/pkg/cron"
	"github.com/sushi30/sushiclaw/pkg/logger"

	// Register owned channel implementations.
	_ "github.com/sushi30/sushiclaw/pkg/channels/email"
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
	cmd.AddCommand(newCronCommand())
	return cmd
}

func newCronCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cron",
		Short: "Manage cron jobs",
	}
	cmd.AddCommand(newCronAddCommand())
	cmd.AddCommand(newCronListCommand())
	cmd.AddCommand(newCronRemoveCommand())
	return cmd
}

func newCronAddCommand() *cobra.Command {
	var name, message, cronExpr, channel, chatID, command string
	var every, at int
	var deliver bool

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a cron job",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig(gateway.GetConfigPath())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			envresolve.Config(cfg)

			if message == "" {
				return fmt.Errorf("--message is required")
			}
			if name == "" {
				name = fmt.Sprintf("job-%d", time.Now().Unix())
			}

			scheduleCount := 0
			if cronExpr != "" {
				scheduleCount++
			}
			if every > 0 {
				scheduleCount++
			}
			if at > 0 {
				scheduleCount++
			}
			if scheduleCount != 1 {
				return fmt.Errorf("exactly one of --cron, --every, or --at is required")
			}
			if at > 0 {
				return fmt.Errorf("--at is only available via the agent tool")
			}

			if channel == "" || chatID == "" {
				return fmt.Errorf("--channel and --chat-id are required")
			}

			store := cron.NewStore(cfg.WorkspacePath() + "/cron/jobs.json")
			jobs, err := store.Load()
			if err != nil {
				return err
			}
			for _, j := range jobs {
				if j.Name == name {
					return fmt.Errorf("job %q already exists", name)
				}
			}

			job := cron.Job{
				Name:      name,
				Message:   message,
				Channel:   channel,
				ChatID:    chatID,
				CronExpr:  cronExpr,
				Deliver:   deliver,
				Command:   command,
				Enabled:   true,
				CreatedAt: time.Now(),
			}
			if every > 0 {
				job.EverySeconds = &every
			}
			if at > 0 {
				job.AtSeconds = &at
			}

			jobs = append(jobs, job)
			if err := store.Save(jobs); err != nil {
				return err
			}

			fmt.Printf("Job %q added. Restart the gateway if it is running to activate it.\n", name)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Job name")
	cmd.Flags().StringVar(&message, "message", "", "Message content")
	cmd.Flags().StringVar(&cronExpr, "cron", "", "Cron expression")
	cmd.Flags().IntVar(&every, "every", 0, "Interval in seconds")
	cmd.Flags().IntVar(&at, "at", 0, "Delay in seconds (not available in CLI)")
	cmd.Flags().BoolVar(&deliver, "deliver", false, "Deliver directly without agent processing")
	cmd.Flags().StringVar(&channel, "channel", "", "Target channel")
	cmd.Flags().StringVar(&chatID, "chat-id", "", "Target chat ID")
	cmd.Flags().StringVar(&command, "command", "", "Shell command to execute")

	return cmd
}

func newCronListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List cron jobs",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig(gateway.GetConfigPath())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			store := cron.NewStore(cfg.WorkspacePath() + "/cron/jobs.json")
			jobs, err := store.Load()
			if err != nil {
				return err
			}
			if len(jobs) == 0 {
				fmt.Println("No cron jobs.")
				return nil
			}
			for _, j := range jobs {
				fmt.Printf("%s: %s", j.Name, j.Message)
				if j.Command != "" {
					fmt.Printf(" (cmd: %s)", j.Command)
				}
				if !j.Enabled {
					fmt.Print(" [disabled]")
				}
				fmt.Println()
			}
			return nil
		},
	}
}

func newCronRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove [name]",
		Short: "Remove a cron job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig(gateway.GetConfigPath())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			store := cron.NewStore(cfg.WorkspacePath() + "/cron/jobs.json")
			jobs, err := store.Load()
			if err != nil {
				return err
			}
			name := args[0]
			found := false
			for i, j := range jobs {
				if j.Name == name {
					jobs = append(jobs[:i], jobs[i+1:]...)
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("job %q not found", name)
			}
			if err := store.Save(jobs); err != nil {
				return err
			}
			fmt.Printf("Job %q removed.\n", name)
			return nil
		},
	}
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
