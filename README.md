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

### WhatsApp interactive widgets (buttons and lists)

When the agent sets MIME-style metadata on an outbound message, the WhatsApp channel renders native interactive widgets instead of plain text.

**Outbound metadata schema:**

| Key | Value |
|-----|-------|
| `Content-Type` | `application/x-wa-buttons` or `application/x-wa-list` |
| `X-WA-Body` | Body text shown above the options (falls back to `Content` if absent) |
| `X-WA-Option-0`, `X-WA-Option-1`, … | Individual option labels (0-indexed, contiguous) |

- `application/x-wa-buttons` renders a WhatsApp `ButtonsMessage` (max 3 tappable buttons). If more than 3 options are provided, the first 2 are kept and the rest are collapsed into a synthetic "Other (chat about this)" button.
- `application/x-wa-list` renders a `ListMessage` with a single-select row list (no limit on rows).
- If `Content-Type` is absent, unknown, or options are empty, the message falls back to plain text.

**Inbound widget replies:**

When the user taps a button or selects a list row, the reply is forwarded to the agent as plain `Content` (the selected label text) with `metadata["wa_reply_type"] = "button"` attached.

### Unauthorized sender reply

When a message arrives from a sender not listed in `allow_from`, sushiclaw replies with a rejection message ("You are not authorized to use this bot.") instead of silently dropping the message. Applies to the WhatsApp native channel.

### Email channel

SMTP (outbound) + IMAP polling (inbound) channel. The agent can receive and reply to emails.

Email is wired as a sidecar outside picoclaw's channel registry. Its config lives under a
top-level `email_channel` key — **not** inside `channels`:

```json
{
  "email_channel": {
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
```

- Port 465 → implicit TLS. Port 587 (default) → STARTTLS.
- Port 993 (default) → implicit TLS for IMAP.
- Polled messages are marked `\Seen` after processing.
- `allow_from` restricts which sender addresses the agent will respond to.

> **Migrating from an older config?** See [RELEASE_NOTES.md](RELEASE_NOTES.md) for
> step-by-step instructions if your config uses the old `channels.email` format.
