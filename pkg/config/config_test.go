package config_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/pkg/config"
)

func TestParseExampleConfig(t *testing.T) {
	cfg, err := config.LoadConfig("../../config.example.json")
	require.NoError(t, err)

	assert.Equal(t, "gpt-4o-mini", cfg.Agents.Defaults.ModelName)
	assert.NotEmpty(t, cfg.ModelList)
	assert.Equal(t, "gpt-4o-mini", cfg.ModelList[0].ModelName)
	assert.Equal(t, 18800, cfg.Gateway.Port)
	assert.NotNil(t, cfg.Channels["telegram"])
}

func TestChannelDecode(t *testing.T) {
	cfg, err := config.LoadConfig("../../config.example.json")
	require.NoError(t, err)

	tgCh := cfg.Channels["telegram"]
	require.NotNil(t, tgCh)
	assert.Equal(t, "telegram", tgCh.Name())

	var tgSettings config.TelegramSettings
	require.NoError(t, tgCh.Decode(&tgSettings))
	// Token value is an env:// reference in example config.
	assert.NotEmpty(t, tgSettings.Token.String())
}

func TestSecureStringEnvResolve(t *testing.T) {
	t.Setenv("TEST_KEY", "secret-value")
	s := config.NewSecureString("env://TEST_KEY")
	assert.Equal(t, "secret-value", s.String())
}

func TestFlexibleStringSlice(t *testing.T) {
	cfg, err := config.LoadConfig("../../config.example.json")
	require.NoError(t, err)

	tgCh := cfg.Channels["telegram"]
	require.NotNil(t, tgCh)
	// allow_from is [] in example config
	assert.Empty(t, tgCh.AllowFrom)
}

func TestMCPConfigParsing(t *testing.T) {
	cfg, err := config.LoadConfig("../../config.example.json")
	require.NoError(t, err)

	require.NotEmpty(t, cfg.MCP.MCPServers)
	assert.Contains(t, cfg.MCP.MCPServers, "github")
	assert.Contains(t, cfg.MCP.MCPServers, "filesystem")

	gh := cfg.MCP.MCPServers["github"]
	assert.Equal(t, "npx", gh.Command)
	assert.Equal(t, []string{"-y", "@modelcontextprotocol/server-github"}, gh.Args)
	assert.Equal(t, "env://GITHUB_TOKEN", gh.Env["GITHUB_PERSONAL_ACCESS_TOKEN"])

	fs := cfg.MCP.MCPServers["filesystem"]
	assert.Equal(t, "npx", fs.Command)
	assert.Equal(t, []string{"-y", "@modelcontextprotocol/server-filesystem", "/home/user/workspace"}, fs.Args)
}

func TestMCPConfigTokenEnvResolve(t *testing.T) {
	t.Setenv("MCP_TEST_TOKEN", "resolved-token")

	jsonData := `{
		"version": 2,
		"agents": {"defaults": {"model_name": "test"}},
		"model_list": [{"model_name": "test", "api_key": "test-key"}],
		"channels": {},
		"gateway": {"host": "0.0.0.0", "port": 18800, "log_level": "info"},
		"tools": {"media_cleanup": {"enabled": false}, "exec": {"enabled": false}},
		"mcp": {
			"mcpServers": {
				"remote": {
					"url": "http://localhost:3000/mcp",
					"token": "env://MCP_TEST_TOKEN"
				}
			}
		}
	}`

	var cfg config.Config
	err := json.Unmarshal([]byte(jsonData), &cfg)
	require.NoError(t, err)

	remote := cfg.MCP.MCPServers["remote"]
	require.NotNil(t, remote.Token)
	assert.Equal(t, "resolved-token", remote.Token.String())
}

func TestWebSearchConfigParsing(t *testing.T) {
	jsonData := `{
		"version": 2,
		"agents": {"defaults": {"model_name": "test"}},
		"model_list": [{"model_name": "test", "api_key": "test-key"}],
		"channels": {},
		"gateway": {"host": "0.0.0.0", "port": 18800, "log_level": "info"},
		"tools": {
			"media_cleanup": {"enabled": false},
			"exec": {"enabled": false},
			"web_search": {
				"enabled": true,
				"provider": "brave",
				"max_results": 7,
				"brave": {"enabled": true, "api_key": "env://BRAVE_API_KEY"},
				"duckduckgo": {"enabled": false}
			}
		}
	}`

	var cfg config.Config
	err := json.Unmarshal([]byte(jsonData), &cfg)
	require.NoError(t, err)
	assert.True(t, cfg.Tools.WebSearch.Enabled)
	assert.Equal(t, "brave", cfg.Tools.WebSearch.Provider)
	assert.Equal(t, 7, cfg.Tools.WebSearch.MaxResults)
	assert.True(t, cfg.Tools.WebSearch.Brave.Enabled)
	assert.Equal(t, "env://BRAVE_API_KEY", cfg.Tools.WebSearch.Brave.APIKey.String())
	assert.False(t, cfg.Tools.WebSearch.DuckDuckGo.Enabled)
}
