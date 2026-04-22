package channels_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sushi30/sushiclaw/pkg/channels"
	"github.com/sushi30/sushiclaw/pkg/config"
)

func TestWithMaxMessageLength(t *testing.T) {
	opt := channels.WithMaxMessageLength(500)
	assert.NotNil(t, opt)
}

func TestWithGroupTrigger(t *testing.T) {
	gt := config.GroupTriggerConfig{MentionOnly: true}
	opt := channels.WithGroupTrigger(gt)
	assert.NotNil(t, opt)
}

func TestWithReasoningChannelID(t *testing.T) {
	opt := channels.WithReasoningChannelID("reasoning")
	assert.NotNil(t, opt)
}

func TestErrNotRunning(t *testing.T) {
	assert.NotNil(t, channels.ErrNotRunning)
}

func TestErrRateLimit(t *testing.T) {
	assert.NotNil(t, channels.ErrRateLimit)
}

func TestErrTemporary(t *testing.T) {
	assert.NotNil(t, channels.ErrTemporary)
}

func TestSplitMessage_CodeBlockLong(t *testing.T) {
	msg := "```go\n" + "fmt.Println(\"hello\")\n" + "```\n" + "some text after code block that is quite long and should be split properly"
	parts := channels.SplitMessage(msg, 30)
	assert.GreaterOrEqual(t, len(parts), 1)
}

func TestSplitMessage_VeryLong(t *testing.T) {
	var msg string
	for i := 0; i < 200; i++ {
		msg += "word "
	}
	parts := channels.SplitMessage(msg, 50)
	assert.Greater(t, len(parts), 1)
}
