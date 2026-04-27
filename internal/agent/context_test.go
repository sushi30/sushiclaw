package agent_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/internal/agent"
)

func TestBuildSystemPromptUsesIdentityEntryPoint(t *testing.T) {
	workspace := t.TempDir()
	writeWorkspaceFile(t, workspace, "AGENT.md", `---
name: test-agent
---

You are the test agent.
`)
	writeWorkspaceFile(t, workspace, "IDENTITY.md", `# Identity

Preferred identity content.
`)
	writeWorkspaceFile(t, workspace, "USER.md", `Legacy identity content should not be used when IDENTITY.md exists.
`)
	writeWorkspaceFile(t, workspace, "SOUL.md", `# Soul

Calm and concise.
`)

	prompt, err := agent.NewContextBuilder(workspace).BuildSystemPromptWithCache()
	require.NoError(t, err)

	assert.Contains(t, prompt, "## Workspace entrypoints")
	assert.Contains(t, prompt, "`AGENT.md`")
	assert.Contains(t, prompt, "`IDENTITY.md`")
	assert.Contains(t, prompt, "`SOUL.md`")
	assert.Contains(t, prompt, "`USER.md`")
	assert.Contains(t, prompt, "Preferred identity content.")
	assert.NotContains(t, prompt, "Legacy identity content should not be used when IDENTITY.md exists.")
	assert.Contains(t, prompt, "When a user asks you to change how the assistant behaves")
}

func TestBuildSystemPromptFallsBackToUserIdentity(t *testing.T) {
	workspace := t.TempDir()
	writeWorkspaceFile(t, workspace, "AGENT.md", `You are the test agent.`)
	writeWorkspaceFile(t, workspace, "USER.md", `Fallback identity content.
`)
	writeWorkspaceFile(t, workspace, "SOUL.md", `# Soul

Calm and concise.
`)

	prompt, err := agent.NewContextBuilder(workspace).BuildSystemPromptWithCache()
	require.NoError(t, err)

	assert.Contains(t, prompt, "## Identity")
	assert.Contains(t, prompt, "Fallback identity content.")
}

func writeWorkspaceFile(t *testing.T, workspace, name, content string) {
	t.Helper()

	err := os.WriteFile(filepath.Join(workspace, name), []byte(content), 0o600)
	require.NoError(t, err)
}
