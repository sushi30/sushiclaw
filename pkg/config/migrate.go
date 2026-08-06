package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// MigrateJSONConfig converts a legacy JSON config to YAML, preserving the JSON
// source as a backup alongside the new file.
func MigrateJSONConfig(sourcePath, destPath string, force bool) error {
	if !force {
		if _, err := os.Stat(destPath); err == nil {
			return fmt.Errorf("destination config %q already exists", destPath)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("check destination config %q: %w", destPath, err)
		}
	}

	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read legacy config %q: %w", sourcePath, err)
	}

	if _, err := LoadJSONConfig(sourcePath); err != nil {
		return err
	}

	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return fmt.Errorf("convert legacy config %q to YAML: %w", sourcePath, err)
	}

	out, err := yaml.Marshal(&node)
	if err != nil {
		return fmt.Errorf("render YAML config %q: %w", destPath, err)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create config directory for %q: %w", destPath, err)
	}

	backupPath := sourcePath + ".bak"
	if !force {
		if _, err := os.Stat(backupPath); err == nil {
			return fmt.Errorf("backup config %q already exists", backupPath)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("check backup config %q: %w", backupPath, err)
		}
	}

	if err := os.WriteFile(backupPath, data, 0o600); err != nil {
		return fmt.Errorf("write backup config %q: %w", backupPath, err)
	}
	if err := os.WriteFile(destPath, out, 0o600); err != nil {
		return fmt.Errorf("write YAML config %q: %w", destPath, err)
	}

	return nil
}
