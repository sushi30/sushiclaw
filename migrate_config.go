package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sushi30/sushiclaw/internal/gateway"
	"github.com/sushi30/sushiclaw/pkg/config"
)

func newMigrateConfigCommand() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "migrate-config",
		Short: "Convert a legacy JSON config to YAML",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			destPath := gateway.GetConfigPath()
			if strings.EqualFold(filepath.Ext(destPath), ".json") {
				destPath = strings.TrimSuffix(destPath, filepath.Ext(destPath)) + ".yaml"
			}

			sourcePath := legacyConfigPath(destPath)
			if _, err := os.Stat(sourcePath); err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("legacy config %q not found", sourcePath)
				}
				return fmt.Errorf("check legacy config %q: %w", sourcePath, err)
			}

			if err := config.MigrateJSONConfig(sourcePath, destPath, force); err != nil {
				return err
			}

			fmt.Printf("Migrated %s -> %s\n", sourcePath, destPath)
			fmt.Printf("Backup saved to %s\n", sourcePath+".bak")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Overwrite the YAML destination and backup if they already exist")
	return cmd
}

func legacyConfigPath(destPath string) string {
	switch strings.ToLower(filepath.Ext(destPath)) {
	case ".yaml", ".yml":
		return strings.TrimSuffix(destPath, filepath.Ext(destPath)) + ".json"
	case ".json":
		return destPath
	default:
		return destPath + ".json"
	}
}
