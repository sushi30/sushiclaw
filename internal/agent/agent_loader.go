package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"

	"github.com/sushi30/sushiclaw/pkg/config"
)

// AgentProfile describes a subagent discovered from the workspace.
type AgentProfile struct {
	Name         string
	Description  string
	ModelName    string
	SystemPrompt string
	Tools        []string // whitelist of tool names; empty means inherit all
	SourcePath   string
}

// LoadAgentProfiles discovers subagent profiles from workspace/agents/*/AGENT.md.
// Each directory under workspace/agents/ is treated as a profile. The AGENT.md
// inside may contain YAML frontmatter for metadata; the body becomes the
// system prompt.
func LoadAgentProfiles(workspace string) ([]AgentProfile, error) {
	if workspace == "" {
		return nil, nil
	}

	agentsDir := filepath.Join(workspace, "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read agents directory: %w", err)
	}

	var profiles []AgentProfile
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		agentFile := filepath.Join(agentsDir, e.Name(), "AGENT.md")
		profile, err := parseAgentFile(e.Name(), agentFile)
		if err != nil {
			if os.IsNotExist(err) {
				continue // directory without AGENT.md is ignored
			}
			return nil, fmt.Errorf("parse agent %q: %w", e.Name(), err)
		}
		profiles = append(profiles, profile)
	}

	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})

	return profiles, nil
}

// LoadSubAgentConfigs loads workspace agent profiles and converts them to
// config.SubAgentConfig values. This is a convenience wrapper for callers
// that need the standard config type (e.g. the subagent_task tool).
func LoadSubAgentConfigs(workspace string) (map[string]config.SubAgentConfig, error) {
	profiles, err := LoadAgentProfiles(workspace)
	if err != nil {
		return nil, err
	}
	result := make(map[string]config.SubAgentConfig, len(profiles))
	for _, p := range profiles {
		result[p.Name] = config.SubAgentConfig{
			ModelName:    p.ModelName,
			Description:  p.Description,
			SystemPrompt: p.SystemPrompt,
			Tools:        p.Tools,
		}
	}
	return result, nil
}

// parseAgentFile reads a single AGENT.md and extracts frontmatter + body.
func parseAgentFile(dirName, path string) (AgentProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AgentProfile{}, err
	}
	content := strings.TrimSpace(string(data))

	p := AgentProfile{
		Name:       dirName,
		SourcePath: path,
	}

	if strings.HasPrefix(content, "---") {
		p.Description = frontmatterValue(content, "description")
		p.ModelName = frontmatterValue(content, "model_name")
		if toolsStr := frontmatterValue(content, "tools"); toolsStr != "" {
			p.Tools = splitTools(toolsStr)
		}
		p.SystemPrompt = ParseMarkdownBody(content)
	} else {
		p.SystemPrompt = content
	}

	return p, nil
}

// splitTools splits a comma-separated tool list and trims whitespace.
func splitTools(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// frontmatterValue extracts a single key:value from YAML frontmatter.
func frontmatterValue(content, key string) string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return ""
	}
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		return ""
	}
	for _, line := range strings.Split(rest[:idx], "\n") {
		k, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok || strings.TrimSpace(k) != key {
			continue
		}
		return strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return ""
}

// FilterTools returns a subset of tools whose names appear in whitelist.
// If whitelist is empty, all tools are returned.
func FilterTools(tools []interfaces.Tool, whitelist []string) []interfaces.Tool {
	if len(whitelist) == 0 {
		out := make([]interfaces.Tool, len(tools))
		copy(out, tools)
		return out
	}
	allowed := make(map[string]bool, len(whitelist))
	for _, name := range whitelist {
		allowed[name] = true
	}
	out := make([]interfaces.Tool, 0, len(tools))
	for _, t := range tools {
		if allowed[t.Name()] {
			out = append(out, t)
		}
	}
	return out
}

// MergeSubAgentConfigs merges workspace profiles with config overrides.
// Workspace profiles take precedence; config profiles fill in missing names.
func MergeSubAgentConfigs(workspaceProfiles map[string]config.SubAgentConfig, cfgProfiles map[string]config.SubAgentConfig) map[string]config.SubAgentConfig {
	merged := make(map[string]config.SubAgentConfig, len(workspaceProfiles)+len(cfgProfiles))
	for name, p := range cfgProfiles {
		merged[name] = p
	}
	for name, p := range workspaceProfiles {
		merged[name] = p
	}
	return merged
}
