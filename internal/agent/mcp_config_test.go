package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/pkg/config"
)

func TestToAgentSDKMCPConfig(t *testing.T) {
	t.Run("empty config returns nil", func(t *testing.T) {
		result := toAgentSDKMCPConfig(config.MCPConfig{})
		assert.Nil(t, result)
	})

	t.Run("converts stdio and http servers", func(t *testing.T) {
		cfg := config.MCPConfig{
			MCPServers: map[string]config.MCPServerConfig{
				"stdio-server": {
					Command:      "npx",
					Args:         []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
					Env:          map[string]string{"KEY": "value"},
					AllowedTools: []string{"read_file"},
				},
				"http-server": {
					URL:          "http://localhost:3000/mcp",
					Token:        config.NewSecureString("bearer-token"),
					AllowedTools: []string{"query"},
				},
			},
		}

		result := toAgentSDKMCPConfig(cfg)
		require.NotNil(t, result)
		require.Len(t, result.MCPServers, 2)

		stdio := result.MCPServers["stdio-server"]
		assert.Equal(t, "npx", stdio.Command)
		assert.Equal(t, []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"}, stdio.Args)
		assert.Equal(t, map[string]string{"KEY": "value"}, stdio.Env)
		assert.Equal(t, "", stdio.URL)
		assert.Equal(t, "", stdio.Token)
		assert.Equal(t, []string{"read_file"}, stdio.AllowedTools)

		http := result.MCPServers["http-server"]
		assert.Equal(t, "", http.Command)
		assert.Equal(t, "http://localhost:3000/mcp", http.URL)
		assert.Equal(t, "bearer-token", http.Token)
		assert.Equal(t, []string{"query"}, http.AllowedTools)
	})

	t.Run("nil token becomes empty string", func(t *testing.T) {
		cfg := config.MCPConfig{
			MCPServers: map[string]config.MCPServerConfig{
				"no-token": {
					Command: "cmd",
				},
			},
		}

		result := toAgentSDKMCPConfig(cfg)
		require.NotNil(t, result)
		assert.Equal(t, "", result.MCPServers["no-token"].Token)
	})
}
