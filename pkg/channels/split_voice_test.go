package channels_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/channels"
)

func TestSplitMessage_Short(t *testing.T) {
	msg := "hello world"
	parts := channels.SplitMessage(msg, 100)
	assert.Len(t, parts, 1)
	assert.Equal(t, msg, parts[0])
}

func TestSplitMessage_Empty(t *testing.T) {
	parts := channels.SplitMessage("", 100)
	assert.Nil(t, parts)
}

func TestSplitMessage_MaxLenZero(t *testing.T) {
	msg := "hello"
	parts := channels.SplitMessage(msg, 0)
	assert.Len(t, parts, 1)
	assert.Equal(t, msg, parts[0])
}

func TestSplitMessage_Long(t *testing.T) {
	msg := "a b c d e f g h i j k l m n o p q r s t u v w x y z"
	parts := channels.SplitMessage(msg, 10)
	assert.GreaterOrEqual(t, len(parts), 2)
}

func TestSplitMessage_CodeBlock(t *testing.T) {
	msg := "```go\nfmt.Println(\"hello\")\n```\nmore text here"
	parts := channels.SplitMessage(msg, 50)
	assert.GreaterOrEqual(t, len(parts), 1)
}

func TestDetectVoiceCapabilities_NilChannel(t *testing.T) {
	caps := channels.DetectVoiceCapabilities("telegram", nil, true, true)
	assert.False(t, caps.ASR)
	assert.False(t, caps.TTS)
}

func TestDetectVoiceCapabilities_Provider(t *testing.T) {
	ch := &voiceCapChannel{asr: true, tts: true}
	caps := channels.DetectVoiceCapabilities("telegram", ch, true, true)
	assert.True(t, caps.ASR)
	assert.True(t, caps.TTS)
}

func TestDetectVoiceCapabilities_ProviderDisabled(t *testing.T) {
	ch := &voiceCapChannel{asr: true, tts: true}
	caps := channels.DetectVoiceCapabilities("telegram", ch, false, false)
	assert.False(t, caps.ASR)
	assert.False(t, caps.TTS)
}

func TestDetectVoiceCapabilities_DefaultChannels(t *testing.T) {
	ch := &dummyChannel{}
	caps := channels.DetectVoiceCapabilities("telegram", ch, true, false)
	assert.True(t, caps.ASR)
	assert.False(t, caps.TTS)
}

func TestDetectVoiceCapabilities_UnknownChannel(t *testing.T) {
	ch := &dummyChannel{}
	caps := channels.DetectVoiceCapabilities("unknown", ch, true, false)
	assert.False(t, caps.ASR)
	assert.False(t, caps.TTS)
}

type voiceCapChannel struct {
	asr bool
	tts bool
}

func (v *voiceCapChannel) Start(ctx context.Context) error { return nil }
func (v *voiceCapChannel) Stop(ctx context.Context) error  { return nil }
func (v *voiceCapChannel) Send(ctx context.Context, msg bus.OutboundMessage) ([]string, error) {
	return nil, nil
}
func (v *voiceCapChannel) Name() string                               { return "test" }
func (v *voiceCapChannel) SetName(string)                             {}
func (v *voiceCapChannel) IsRunning() bool                            { return false }
func (v *voiceCapChannel) SetRunning(bool)                            {}
func (v *voiceCapChannel) IsAllowed(string) bool                      { return true }
func (v *voiceCapChannel) IsAllowedSender(sender bus.SenderInfo) bool { return true }
func (v *voiceCapChannel) ReasoningChannelID() string                 { return "" }
func (v *voiceCapChannel) VoiceCapabilities() channels.VoiceCapabilities {
	return channels.VoiceCapabilities{ASR: v.asr, TTS: v.tts}
}

type dummyChannel struct{}

func (d *dummyChannel) Start(ctx context.Context) error { return nil }
func (d *dummyChannel) Stop(ctx context.Context) error  { return nil }
func (d *dummyChannel) Send(ctx context.Context, msg bus.OutboundMessage) ([]string, error) {
	return nil, nil
}
func (d *dummyChannel) Name() string                               { return "test" }
func (d *dummyChannel) SetName(string)                             {}
func (d *dummyChannel) IsRunning() bool                            { return false }
func (d *dummyChannel) SetRunning(bool)                            {}
func (d *dummyChannel) IsAllowed(string) bool                      { return true }
func (d *dummyChannel) IsAllowedSender(sender bus.SenderInfo) bool { return true }
func (d *dummyChannel) ReasoningChannelID() string                 { return "" }
