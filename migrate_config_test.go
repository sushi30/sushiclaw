package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/pkg/config"
)

func TestMigrateConfigCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SUSHICLAW_HOME", home)
	t.Setenv("SUSHICLAW_CONFIG", "")

	sourcePath := filepath.Join(home, "config.json")
	data, err := os.ReadFile("config.example.json")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(sourcePath, data, 0o600))

	cmd := newMigrateConfigCommand()
	cmd.SetArgs(nil)
	require.NoError(t, cmd.Execute())

	destPath := filepath.Join(home, "config.yaml")
	_, err = os.Stat(destPath)
	require.NoError(t, err)

	backupPath := sourcePath + ".bak"
	_, err = os.Stat(backupPath)
	require.NoError(t, err)

	cfg, err := config.LoadConfig(destPath)
	require.NoError(t, err)
	require.Equal(t, "gpt-4o-mini", cfg.Agents.Defaults.ModelName)
}
