package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/pkg/config"
)

func TestParseExampleConfig(t *testing.T) {
	cfg, err := config.LoadConfig("../../config.example.json")
	require.NoError(t, err)

	assert.Equal(t, "claude-sonnet", cfg.Agents.Defaults.ModelName)
	assert.NotEmpty(t, cfg.ModelList)
	assert.Equal(t, "claude-sonnet", cfg.ModelList[0].ModelName)
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
	// Token value is "YOUR_TELEGRAM_BOT_TOKEN" in example
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
