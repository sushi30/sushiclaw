package agent

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sushi30/sushiclaw/pkg/audio/asr"
	"github.com/sushi30/sushiclaw/pkg/bus"
	"github.com/sushi30/sushiclaw/pkg/media"
)

type fakeTranscriber struct {
	text string
	err  error
}

func (f fakeTranscriber) Name() string { return "fake" }

func (f fakeTranscriber) Transcribe(context.Context, string) (*asr.TranscriptionResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &asr.TranscriptionResponse{Text: f.text}, nil
}

func TestTranscribeAudioInMessage_ReplacesTelegramVoiceMarker(t *testing.T) {
	store := media.NewFileMediaStore()
	audioPath := makeTempMediaFile(t, "voice-*.ogg")
	ref, err := store.Store(audioPath, media.MediaMeta{
		Filename: "voice.ogg",
		Source:   "telegram",
	}, "telegram:123:456")
	require.NoError(t, err)

	sm := &SessionManager{
		mediaStore:  store,
		transcriber: fakeTranscriber{text: "please summarize my day"},
	}

	msg, hadAudio, err := sm.transcribeAudioInMessage(context.Background(), bus.InboundMessage{
		Channel: "telegram",
		ChatID:  "123",
		Content: "listen to this\n[voice]",
		Media:   []string{ref},
	})

	require.NoError(t, err)
	assert.True(t, hadAudio)
	assert.Equal(t, "listen to this\n<transcription>please summarize my day</transcription>", msg.Content)
	assert.Empty(t, msg.Media)
}

func TestTranscribeAudioInMessage_ReturnsErrorWhenTelegramAudioFails(t *testing.T) {
	store := media.NewFileMediaStore()
	audioPath := makeTempMediaFile(t, "audio-*.mp3")
	ref, err := store.Store(audioPath, media.MediaMeta{
		Filename: "audio.mp3",
		Source:   "telegram",
	}, "telegram:123:456")
	require.NoError(t, err)

	sm := &SessionManager{
		mediaStore:  store,
		transcriber: fakeTranscriber{err: errors.New("asr unavailable")},
	}

	_, hadAudio, err := sm.transcribeAudioInMessage(context.Background(), bus.InboundMessage{
		Channel: "telegram",
		ChatID:  "123",
		Content: "[audio]",
		Media:   []string{ref},
	})

	require.Error(t, err)
	assert.True(t, hadAudio)
}

func makeTempMediaFile(t *testing.T, pattern string) string {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), pattern)
	require.NoError(t, err)
	_, err = f.WriteString("test audio bytes")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}
