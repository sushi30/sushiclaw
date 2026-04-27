# Voice Transcription

Sushiclaw can transcribe inbound voice and audio messages before they reach
the agent. The LLM receives the transcript wrapped in `<transcription>` tags
instead of raw audio.

Supported inbound channels:

- WhatsApp native voice/audio messages
- Telegram voice messages (`.ogg`) and audio attachments

Raw audio is stored through the media store and resolved only for
transcription. The agent prompt receives text, not audio bytes.

## Configuration

Add an ASR-capable model to `model_list`, then reference it from the
top-level `voice` block:

```json
{
  "model_list": [
    {
      "model_name": "gpt-4o-mini",
      "model": "gpt-4o-mini",
      "api_key": "env://OPENAI_API_KEY",
      "api_base": "https://api.openai.com/v1"
    },
    {
      "model_name": "whisper-1",
      "model": "whisper-1",
      "api_key": "env://OPENAI_API_KEY",
      "api_base": "https://api.openai.com/v1"
    }
  ],
  "voice": {
    "model_name": "whisper-1",
    "echo_transcription": false
  }
}
```

`voice.model_name` must match a `model_list[].model_name` entry. If it is empty,
voice transcription is disabled.

## Providers

The transcriber uses an OpenAI-compatible `POST /audio/transcriptions` endpoint.
Set `api_base` to the provider base URL and `model` to that provider's ASR
model ID.

Examples:

- OpenAI: `api_base: "https://api.openai.com/v1"`, `model: "whisper-1"`
- Groq or another compatible provider: use that provider's base URL and ASR
  model ID

API keys can use the existing `env://VAR_NAME` form:

```json
{
  "api_key": "env://OPENAI_API_KEY"
}
```

## Behavior

When a supported channel receives audio:

1. The channel downloads the audio and stores a media ref.
2. The agent session resolves the media ref through the media store.
3. Audio files are sent to the configured ASR provider.
4. `[voice]` or `[audio]` markers are replaced with transcription tags.
5. The transcribed text is sent to the LLM as the user input.

If transcription fails for all audio attachments in a message, the sender receives:

```text
Sorry, I couldn't transcribe that voice message.
```

If transcription succeeds but produces no text, the sender receives:

```text
Sorry, I couldn't understand that voice message.
```

## Notes

- Telegram voice messages are downloaded as `.ogg` and are supported.
- Supported audio extensions include `.ogg`, `.oga`, `.mp3`, `.wav`, `.m4a`, `.aac`,
  `.flac`, `.opus`, and `.webm`.
- `echo_transcription` is currently used by the WhatsApp native channel behavior.
