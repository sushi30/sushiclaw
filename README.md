# sushiclaw

Personal AI agent built on top of [picoclaw](https://github.com/sipeed/picoclaw).

For general setup, configuration, and picoclaw concepts see the [picoclaw docs](https://github.com/sipeed/picoclaw).
This README documents only what sushiclaw adds on top.

---

## Added features

### WhatsApp voice memo transcription

Incoming WhatsApp audio messages (voice notes) are transcribed to text before being forwarded to the agent. Transcription is powered by the ASR provider configured in `voice` config block. When `echo_transcription` is enabled in `voice` config, the transcript is echoed back to the sender alongside the agent reply.

Build requirement: the `whatsapp_native` build tag must be set.

```bash
go build -tags whatsapp_native -o sushiclaw .
# or just:
make build
```

### `env://` config resolver

API keys and secrets in `config.json` can reference environment variables instead of being stored as plaintext:

```json
{
  "model_list": [
    {
      "api_keys": ["env://OPENAI_API_KEY"]
    }
  ]
}
```

The `env://VAR_NAME` scheme is resolved at startup by `internal/envresolve`. This fills a gap in upstream picoclaw which handles `enc://` and `file://` but not `env://`.

### Unauthorized sender reply

When a message arrives from a sender not listed in `allow_from`, sushiclaw replies with a rejection message ("You are not authorized to use this bot.") instead of silently dropping the message. Applies to the WhatsApp native channel.

### Email channel

SMTP (outbound) + IMAP polling (inbound) channel. The agent can receive and reply to emails.

Config block (`channels.email` in `config.json`):

```json
{
  "channels": {
    "email": {
      "enabled": true,
      "smtp_host": "smtp.example.com",
      "smtp_port": 587,
      "smtp_from": "bot@example.com",
      "smtp_user": "bot@example.com",
      "smtp_password": "env://SMTP_PASSWORD",
      "imap_host": "imap.example.com",
      "imap_port": 993,
      "imap_user": "bot@example.com",
      "imap_password": "env://IMAP_PASSWORD",
      "poll_interval_secs": 30,
      "allow_from": ["trusted@example.com"],
      "default_subject": "Re: your message"
    }
  }
}
```

- Port 465 → implicit TLS. Port 587 (default) → STARTTLS.
- Port 993 (default) → implicit TLS for IMAP.
- Polled messages are marked `\Seen` after processing.
- `allow_from` restricts which sender addresses the agent will respond to.
