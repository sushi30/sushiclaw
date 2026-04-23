package config

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
)

// Channel type constants.
const (
	ChannelTelegram       = "telegram"
	ChannelWhatsAppNative = "whatsapp_native"
	ChannelEmail          = "email"
)

// Config is the top-level sushiclaw configuration.
type Config struct {
	Version      int            `json:"version,omitempty"`
	Agents       AgentsConfig   `json:"agents"`
	ModelList    []ModelConfig  `json:"model_list"`
	Channels     ChannelsConfig `json:"channels"`
	EmailChannel *EmailChanCfg  `json:"email_channel,omitempty"`
	Gateway      GatewayConfig  `json:"gateway"`
	Tools        ToolsConfig    `json:"tools"`
}

type AgentsConfig struct {
	Defaults AgentDefaults `json:"defaults"`
}

type AgentDefaults struct {
	ModelName           string  `json:"model_name"`
	Workspace           string  `json:"workspace"`
	RestrictToWorkspace bool    `json:"restrict_to_workspace"`
	MaxTokens           int     `json:"max_tokens"`
	Temperature         float64 `json:"temperature"`
	MaxToolIterations   int     `json:"max_tool_iterations"`
}

type ModelConfig struct {
	ModelName string          `json:"model_name"`
	Model     string          `json:"model"`
	APIKey    *SecureString   `json:"api_key,omitzero"`
	APIKeys   []*SecureString `json:"api_keys,omitzero"`
	APIBase   string          `json:"api_base,omitempty"`
}

// APIKeyString returns the first resolved API key.
func (m *ModelConfig) APIKeyString() string {
	if m.APIKey != nil && m.APIKey.String() != "" {
		return m.APIKey.String()
	}
	for _, k := range m.APIKeys {
		if k != nil && k.String() != "" {
			return k.String()
		}
	}
	return ""
}

type GatewayConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	LogLevel string `json:"log_level"`
}

type ToolsConfig struct {
	MediaCleanup MediaCleanupCfg `json:"media_cleanup"`
	Exec         ExecToolConfig  `json:"exec"`
}

func (t ToolsConfig) IsToolEnabled(name string) bool {
	if name == "exec" {
		return t.Exec.Enabled
	}
	return false
}

type ExecToolConfig struct {
	Enabled bool `json:"enabled"`
}

type MediaCleanupCfg struct {
	Enabled  bool `json:"enabled"`
	MaxAge   int  `json:"max_age"`
	Interval int  `json:"interval"`
}

// EmailChanCfg is the top-level email_channel config in config.json.
type EmailChanCfg struct {
	Enabled            bool                `json:"enabled"`
	SMTPHost           string              `json:"smtp_host"`
	SMTPPort           int                 `json:"smtp_port"`
	SMTPFrom           SecureString        `json:"smtp_from"`
	SMTPUser           SecureString        `json:"smtp_user"`
	SMTPPassword       SecureString        `json:"smtp_password"`
	DefaultSubject     string              `json:"default_subject"`
	IMAPHost           string              `json:"imap_host"`
	IMAPPort           int                 `json:"imap_port"`
	IMAPUser           SecureString        `json:"imap_user"`
	IMAPPassword       SecureString        `json:"imap_password"`
	PollIntervalSecs   int                 `json:"poll_interval_secs"`
	AllowFrom          FlexibleStringSlice `json:"allow_from"`
	ReasoningChannelID string              `json:"reasoning_channel_id"`
}

// GroupTriggerConfig controls when the bot responds in group chats.
type GroupTriggerConfig struct {
	MentionOnly bool     `json:"mention_only,omitempty"`
	Prefixes    []string `json:"prefixes,omitempty"`
}

// TypingConfig controls typing indicator behavior.
type TypingConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

// PlaceholderConfig controls placeholder message behavior.
type PlaceholderConfig struct {
	Enabled bool                `json:"enabled"`
	Text    FlexibleStringSlice `json:"text,omitempty"`
}

// GetRandomText returns a random placeholder text, or default if none set.
func (p *PlaceholderConfig) GetRandomText() string {
	if len(p.Text) == 0 {
		return "Thinking..."
	}
	if len(p.Text) == 1 {
		return p.Text[0]
	}
	return p.Text[rand.Intn(len(p.Text))]
}

// StreamingConfig controls streaming behavior.
type StreamingConfig struct {
	Enabled         bool `json:"enabled,omitempty"`
	ThrottleSeconds int  `json:"throttle_seconds,omitempty"`
	MinGrowthChars  int  `json:"min_growth_chars,omitempty"`
}

// TelegramSettings holds Telegram-specific channel settings.
type TelegramSettings struct {
	Token         SecureString    `json:"token,omitzero"`
	BaseURL       string          `json:"base_url"`
	Proxy         string          `json:"proxy"`
	Streaming     StreamingConfig `json:"streaming,omitempty"`
	UseMarkdownV2 bool            `json:"use_markdown_v2"`
}

// WhatsAppSettings holds WhatsApp-native channel settings.
type WhatsAppSettings struct {
	BridgeURL        string `json:"bridge_url"`
	UseNative        bool   `json:"use_native"`
	SessionStorePath string `json:"session_store_path"`
}

// VoiceConfig holds voice/ASR settings.
type VoiceConfig struct {
	ModelName         string `json:"model_name,omitempty"`
	TTSModelName      string `json:"tts_model_name,omitempty"`
	EchoTranscription bool   `json:"echo_transcription"`
	ElevenLabsAPIKey  string `json:"elevenlabs_api_key,omitempty"`
}

// ChannelsConfig is a map of channel name → Channel config.
// Channel-specific settings are flat (at the same JSON level as common fields).
type ChannelsConfig map[string]*Channel

// Channel holds common channel configuration plus raw bytes for channel-specific parsing.
type Channel struct {
	name               string
	raw                json.RawMessage
	Enabled            bool                `json:"enabled"`
	Type               string              `json:"type"`
	AllowFrom          FlexibleStringSlice `json:"allow_from,omitempty"`
	ReasoningChannelID string              `json:"reasoning_channel_id"`
	GroupTrigger       GroupTriggerConfig  `json:"group_trigger,omitempty"`
	Typing             TypingConfig        `json:"typing,omitempty"`
	Placeholder        PlaceholderConfig   `json:"placeholder,omitempty"`
}

// Name returns the channel name (set by ChannelsConfig loader).
func (b *Channel) Name() string { return b.name }

// SetName sets the channel name.
func (b *Channel) SetName(name string) { b.name = name }

// Decode decodes the channel's flat JSON into target (e.g. *TelegramSettings).
// Channel-specific fields and common fields share the same JSON object.
func (b *Channel) Decode(target any) error {
	if len(b.raw) == 0 {
		return nil
	}
	return json.Unmarshal(b.raw, target)
}

// UnmarshalJSON stores raw bytes and unmarshals common fields.
func (b *Channel) UnmarshalJSON(data []byte) error {
	b.raw = data
	type Alias Channel
	return json.Unmarshal(data, (*Alias)(b))
}

// LoadConfig loads sushiclaw config from path.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	for name, ch := range cfg.Channels {
		if ch == nil {
			continue
		}
		ch.SetName(name)
	}
	return &cfg, nil
}

// WorkspacePath returns the agent workspace directory with ~ expanded.
func (c *Config) WorkspacePath() string {
	p := c.Agents.Defaults.Workspace
	if p == "" {
		p = GetHome() + "/workspace"
	}
	return expandHome(p)
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return home + p[1:]
	}
	return p
}

// Voice returns a zero VoiceConfig (sushiclaw doesn't configure voice globally).
func (c *Config) Voice() VoiceConfig { return VoiceConfig{} }

// GetHome returns the sushiclaw home directory.
func GetHome() string {
	if h := os.Getenv("SUSHICLAW_HOME"); h != "" {
		return h
	}
	if h := os.Getenv("PICOCLAW_HOME"); h != "" {
		return h
	}
	home, _ := os.UserHomeDir()
	return home + "/.picoclaw"
}

// FlexibleStringSlice is a []string that also accepts JSON numbers and single strings.
type FlexibleStringSlice []string

func (f *FlexibleStringSlice) UnmarshalJSON(data []byte) error {
	var singleString string
	if err := json.Unmarshal(data, &singleString); err == nil {
		*f = FlexibleStringSlice{singleString}
		return nil
	}
	var singleNumber float64
	if err := json.Unmarshal(data, &singleNumber); err == nil {
		*f = FlexibleStringSlice{fmt.Sprintf("%.0f", singleNumber)}
		return nil
	}
	var ss []string
	if err := json.Unmarshal(data, &ss); err == nil {
		*f = ss
		return nil
	}
	var raw []any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	result := make([]string, 0, len(raw))
	for _, v := range raw {
		switch val := v.(type) {
		case string:
			result = append(result, val)
		case float64:
			result = append(result, fmt.Sprintf("%.0f", val))
		default:
			result = append(result, fmt.Sprintf("%v", val))
		}
	}
	*f = result
	return nil
}

const notHere = `"[NOT_HERE]"`

// SecureString is a string value that is not serialized to JSON output.
// Supports env://VAR_NAME resolution.
type SecureString struct {
	value string
}

// NewSecureString creates a SecureString with the given value.
func NewSecureString(value string) *SecureString {
	s := &SecureString{}
	s.resolve(value)
	return s
}

func (s *SecureString) resolve(raw string) {
	if v, ok := strings.CutPrefix(raw, "env://"); ok {
		if resolved := os.Getenv(v); resolved != "" {
			s.value = resolved
			return
		}
	}
	s.value = raw
}

// String returns the resolved string value.
func (s *SecureString) String() string {
	if s == nil {
		return ""
	}
	return s.value
}

// Set sets the value directly (bypasses env:// resolution).
func (s *SecureString) Set(value string) *SecureString {
	s.value = value
	return s
}

// IsZero returns true if the value is empty.
func (s SecureString) IsZero() bool {
	return s.value == ""
}

// IsUnresolvedEnv returns true if the value starts with "env://" (the env var was not set).
func (s SecureString) IsUnresolvedEnv() bool {
	return strings.HasPrefix(s.value, "env://")
}

// MarshalJSON never outputs the secret value.
func (s SecureString) MarshalJSON() ([]byte, error) {
	return []byte(notHere), nil
}

// UnmarshalJSON reads the value and resolves env:// references.
func (s *SecureString) UnmarshalJSON(data []byte) error {
	if string(data) == notHere {
		return nil
	}
	var v string
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	s.resolve(v)
	return nil
}
