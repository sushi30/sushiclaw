package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/pkg/config"
)

func TestLoadAgentProfiles_MissingDir(t *testing.T) {
	profiles, err := LoadAgentProfiles(filepath.Join(t.TempDir(), "nonexistent"))
	require.NoError(t, err)
	assert.Empty(t, profiles)
}

func TestLoadAgentProfiles_EmptyWorkspace(t *testing.T) {
	profiles, err := LoadAgentProfiles("")
	require.NoError(t, err)
	assert.Empty(t, profiles)
}

func TestLoadAgentProfiles_SingleAgent(t *testing.T) {
	ws := t.TempDir()
	agentDir := filepath.Join(ws, "agents", "coder")
	require.NoError(t, os.MkdirAll(agentDir, 0755))
	content := `---
description: A coding assistant
model_name: claude-sonnet
tools: read_file, write_file, exec
---

You are a coding assistant.`
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "AGENT.md"), []byte(content), 0644))

	profiles, err := LoadAgentProfiles(ws)
	require.NoError(t, err)
	require.Len(t, profiles, 1)

	p := profiles[0]
	assert.Equal(t, "coder", p.Name)
	assert.Equal(t, "A coding assistant", p.Description)
	assert.Equal(t, "claude-sonnet", p.ModelName)
	assert.Equal(t, []string{"read_file", "write_file", "exec"}, p.Tools)
	assert.Equal(t, "You are a coding assistant.", p.SystemPrompt)
}

func TestLoadAgentProfiles_NoFrontmatter(t *testing.T) {
	ws := t.TempDir()
	agentDir := filepath.Join(ws, "agents", "simple")
	require.NoError(t, os.MkdirAll(agentDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "AGENT.md"), []byte("Just a simple prompt."), 0644))

	profiles, err := LoadAgentProfiles(ws)
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, "simple", profiles[0].Name)
	assert.Equal(t, "Just a simple prompt.", profiles[0].SystemPrompt)
	assert.Empty(t, profiles[0].Tools)
}

func TestLoadAgentProfiles_DirWithoutAgentFileIgnored(t *testing.T) {
	ws := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, "agents", "emptydir"), 0755))

	profiles, err := LoadAgentProfiles(ws)
	require.NoError(t, err)
	assert.Empty(t, profiles)
}

func TestLoadSubAgentConfigs(t *testing.T) {
	ws := t.TempDir()
	agentDir := filepath.Join(ws, "agents", "researcher")
	require.NoError(t, os.MkdirAll(agentDir, 0755))
	content := `---
description: Research assistant
model_name: gpt-4o
---

You research things.`
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "AGENT.md"), []byte(content), 0644))

	configs, err := LoadSubAgentConfigs(ws)
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.Equal(t, "Research assistant", configs["researcher"].Description)
	assert.Equal(t, "gpt-4o", configs["researcher"].ModelName)
}

func TestMergeSubAgentConfigs_WinsWorkspace(t *testing.T) {
	workspace := map[string]config.SubAgentConfig{
		"coder": {Description: "from workspace"},
	}
	cfg := map[string]config.SubAgentConfig{
		"coder": {Description: "from config"},
		"researcher": {Description: "from config"},
	}

	merged := MergeSubAgentConfigs(workspace, cfg)
	assert.Equal(t, "from workspace", merged["coder"].Description)
	assert.Equal(t, "from config", merged["researcher"].Description)
}

func TestFilterTools(t *testing.T) {
	tools := []interfaces.Tool{
		mockTool{name: "read_file"},
		mockTool{name: "write_file"},
		mockTool{name: "exec"},
	}

	filtered := FilterTools(tools, []string{"read_file", "exec"})
	require.Len(t, filtered, 2)
	assert.Equal(t, "read_file", filtered[0].Name())
	assert.Equal(t, "exec", filtered[1].Name())

	all := FilterTools(tools, nil)
	assert.Len(t, all, 3)
}

type mockTool struct {
	name string
}

func (m mockTool) Name() string                                    { return m.name }
func (m mockTool) Description() string                             { return "" }
func (m mockTool) Parameters() map[string]interfaces.ParameterSpec { return nil }
func (m mockTool) Run(_ context.Context, _ string) (string, error) { return "", nil }
func (m mockTool) Execute(_ context.Context, _ string) (string, error) {
	return "", nil
}
