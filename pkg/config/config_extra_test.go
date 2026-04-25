package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/pkg/config"
)

func TestSecureString_MarshalJSON(t *testing.T) {
	s := config.NewSecureString("secret")
	b, err := json.Marshal(s)
	require.NoError(t, err)
	assert.Equal(t, `"[NOT_HERE]"`, string(b))
}

func TestSecureString_IsZero(t *testing.T) {
	empty := config.NewSecureString("")
	assert.True(t, empty.IsZero())

	nonEmpty := config.NewSecureString("value")
	assert.False(t, nonEmpty.IsZero())
}

func TestSecureString_IsUnresolvedEnv(t *testing.T) {
	_ = os.Unsetenv("MISSING_VAR")
	s := config.NewSecureString("env://MISSING_VAR")
	assert.True(t, s.IsUnresolvedEnv())

	s2 := config.NewSecureString("plain-value")
	assert.False(t, s2.IsUnresolvedEnv())
}

func TestSecureString_Set(t *testing.T) {
	s := config.NewSecureString("old")
	s.Set("new")
	assert.Equal(t, "new", s.String())
}

func TestFlexibleStringSlice_String(t *testing.T) {
	var fs config.FlexibleStringSlice
	data := []byte(`"single-value"`)
	require.NoError(t, json.Unmarshal(data, &fs))
	assert.Equal(t, []string{"single-value"}, []string(fs))
}

func TestFlexibleStringSlice_Number(t *testing.T) {
	var fs config.FlexibleStringSlice
	data := []byte(`42`)
	require.NoError(t, json.Unmarshal(data, &fs))
	assert.Equal(t, []string{"42"}, []string(fs))
}

func TestFlexibleStringSlice_Array(t *testing.T) {
	var fs config.FlexibleStringSlice
	data := []byte(`["a", "b", "c"]`)
	require.NoError(t, json.Unmarshal(data, &fs))
	assert.Equal(t, []string{"a", "b", "c"}, []string(fs))
}

func TestChannel_Name(t *testing.T) {
	cfg, err := config.LoadConfig("../../config.example.json")
	require.NoError(t, err)

	tgCh := cfg.Channels["telegram"]
	require.NotNil(t, tgCh)
	assert.Equal(t, "telegram", tgCh.Name())
}

func TestConfig_WorkspacePath(t *testing.T) {
	cfg := &config.Config{}
	cfg.Agents.Defaults.Workspace = "/tmp/workspace"
	assert.Equal(t, "/tmp/workspace", cfg.WorkspacePath())
}

func TestConfig_Voice(t *testing.T) {
	cfg := &config.Config{}
	voice := cfg.Voice()
	assert.Empty(t, voice.ModelName)
}

func TestGetHome(t *testing.T) {
	// Test with SUSHICLAW_HOME
	t.Setenv("SUSHICLAW_HOME", "/tmp/sushiclaw")
	assert.Equal(t, "/tmp/sushiclaw", config.GetHome())
}

func TestModelConfig_APIKeyString(t *testing.T) {
	m := &config.ModelConfig{
		APIKey: config.NewSecureString("key1"),
		APIKeys: []*config.SecureString{
			config.NewSecureString("key2"),
		},
	}
	assert.Equal(t, "key1", m.APIKeyString())

	m2 := &config.ModelConfig{
		APIKeys: []*config.SecureString{
			config.NewSecureString("key2"),
		},
	}
	assert.Equal(t, "key2", m2.APIKeyString())

	m3 := &config.ModelConfig{}
	assert.Equal(t, "", m3.APIKeyString())
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := config.LoadConfig("/nonexistent/path/config.json")
	assert.Error(t, err)
}

func TestLoadConfig_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	data := []byte(`{
		"version": 2,
		"agents": {"defaults": {"model_name": "test-model", "workspace": "/tmp", "restrict_to_workspace": false, "max_tokens": 1000, "temperature": 0.5, "max_tool_iterations": 5}},
		"model_list": [{"model_name": "test-model", "model": "gpt-4", "api_key": "test-key"}],
		"channels": {"telegram": {"enabled": true, "type": "telegram", "token": "bot-token"}},
		"gateway": {"host": "0.0.0.0", "port": 8080, "log_level": "info"},
		"tools": {"media_cleanup": {"enabled": true, "max_age": 60, "interval": 10}, "exec": {"enabled": false}}
	}`)
	require.NoError(t, os.WriteFile(cfgPath, data, 0o600))

	cfg, err := config.LoadConfig(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, 2, cfg.Version)
	assert.Equal(t, "test-model", cfg.Agents.Defaults.ModelName)
	assert.Equal(t, 5, cfg.Agents.Defaults.MaxToolIterations)
	assert.Equal(t, 8080, cfg.Gateway.Port)
	assert.NotNil(t, cfg.Channels["telegram"])
	assert.Equal(t, "telegram", cfg.Channels["telegram"].Name())
}

func TestToolsConfig_IsToolEnabled(t *testing.T) {
	cfg := config.ToolsConfig{
		Exec:      config.ExecToolConfig{Enabled: true},
		ReadFile:  config.ToolConfig{Enabled: true},
		WriteFile: config.ToolConfig{Enabled: true},
		ListDir:   config.ToolConfig{Enabled: true},
		WebSearch: config.WebSearchToolConfig{Enabled: true},
	}
	assert.True(t, cfg.IsToolEnabled("exec"))
	assert.True(t, cfg.IsToolEnabled("read_file"))
	assert.True(t, cfg.IsToolEnabled("write_file"))
	assert.True(t, cfg.IsToolEnabled("list_dir"))
	assert.True(t, cfg.IsToolEnabled("web_search"))
	assert.False(t, cfg.IsToolEnabled("other"))
}

func TestChannel_Decode_Missing(t *testing.T) {
	var ch config.Channel
	data := []byte(`{"enabled": true, "type": "telegram"}`)
	require.NoError(t, json.Unmarshal(data, &ch))

	var settings config.TelegramSettings
	err := ch.Decode(&settings)
	require.NoError(t, err)
	assert.Empty(t, settings.Token.String())
}
