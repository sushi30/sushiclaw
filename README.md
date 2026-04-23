# sushiclaw

Personal AI agent built on top of [picoclaw](https://github.com/sipeed/picoclaw).

Runs on WhatsApp, Telegram, and Email. Customizable via workspace files and skills.

For general picoclaw concepts see the [picoclaw docs](https://github.com/sipeed/picoclaw).
This README documents what sushiclaw adds on top.

---

## Quick start

### 1. Clone

```bash
git clone --recurse-submodules https://github.com/sushi30/sushiclaw.git
cd sushiclaw
```

### 2. Configure

```bash
mkdir -p ~/.picoclaw
cp config.example.json ~/.picoclaw/config.json
# Edit config.json — set your model API key and enable at least one channel
```

### 3. Build and run

```bash
make build
./sushiclaw gateway
```

Or install to `~/.local/bin`:

```bash
make install
sushiclaw gateway
```

---

## Commands

| Command | Description |
|---------|-------------|
| `sushiclaw gateway` | Start the full gateway (all channels) |
| `sushiclaw chat` | Interactive terminal chat with the agent |
| `sushiclaw version` | Print build version info |

### `gateway` flags

| Flag | Default | Description |
|------|---------|-------------|
| `-d, --debug` | false | Enable debug logging |
| `-E, --allow-empty` | false | Start even without a default model configured |

### `chat` flags

| Flag | Default | Description |
|------|---------|-------------|
| `-d, --debug` | false | Enable debug logging |

---

## Docker

```bash
docker pull ghcr.io/sushi30/sushiclaw:latest

docker run -d \
  -v ~/.picoclaw:/home/sushiclaw/.picoclaw \
  -e ANTHROPIC_API_KEY=sk-... \
  ghcr.io/sushi30/sushiclaw:latest gateway
```

Health check hits `http://localhost:18790/health` (picoclaw health server).

---

## Configuration

Copy `config.example.json` to `~/.picoclaw/config.json`. Key sections:

```json
{
  "agents": {
    "defaults": {
      "workspace": "~/.picoclaw/workspace",
      "model_name": "claude-sonnet"
    }
  },
  "model_list": [{ "model_name": "claude-sonnet", "api_key": "env://ANTHROPIC_API_KEY" }],
  "channels": { ... },
  "email_channel": { ... },
  "tools": { ... }
}
```

Override config path with `$SUSHICLAW_CONFIG`.

---

## Added features

### `env://` config resolver

API keys in `config.json` can reference environment variables:

```json
{ "api_key": "env://ANTHROPIC_API_KEY" }
```

Resolved at startup by `internal/envresolve`. Fills a gap in upstream picoclaw (which handles `enc://` and `file://` but not `env://`).

---

### WhatsApp voice memo transcription

Incoming WhatsApp audio messages are transcribed before reaching the agent. Powered by the ASR provider in the `voice` config block. Set `echo_transcription: true` to send the transcript back to the sender.

Requires the `whatsapp_native` build tag (included in `make build`).

---

### WhatsApp interactive widgets

When the agent sets MIME-style metadata on an outbound message, the WhatsApp channel renders native interactive widgets.

**Metadata schema:**

| Key | Value |
|-----|-------|
| `Content-Type` | `application/x-wa-buttons` or `application/x-wa-list` |
| `X-WA-Body` | Body text above options (falls back to `Content`) |
| `X-WA-Option-0`, `X-WA-Option-1`, … | Option labels (0-indexed, contiguous) |

- `application/x-wa-buttons` → WhatsApp `ButtonsMessage` (max 3). More than 3 options: first 2 kept, rest collapsed into "Other (chat about this)".
- `application/x-wa-list` → `ListMessage` with single-select rows (no row limit).
- Missing/unknown `Content-Type` or no options → plain text fallback.

Tapped replies arrive as plain `Content` with `metadata["wa_reply_type"] = "button"`.

---

### Unauthorized sender reply

Messages from senders not in `allow_from` receive a rejection reply instead of being silently dropped. Applies to the WhatsApp native channel.

---

### Email channel

SMTP (outbound) + IMAP polling (inbound). Config lives under `email_channel` (not inside `channels`):

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

- Port 465 → implicit TLS (SMTP). Port 587 → STARTTLS.
- Port 993 → implicit TLS (IMAP).
- Processed messages are marked `\Seen`.

> **Migrating from an older config?** See [RELEASE_NOTES.md](RELEASE_NOTES.md) for step-by-step instructions if your config uses the old `channels.email` format.

---

## Workspace customization

The agent loads three Markdown files from `agents.defaults.workspace` at startup:

| File | Purpose |
|------|---------|
| `AGENT.md` | Agent name, role, mission, capabilities |
| `SOUL.md` | Personality and communication style |
| `USER.md` | Information about you (name, timezone, preferences) |

Edit these to shape how the agent behaves and presents itself.

### Skills

Drop skill directories into `workspace/skills/`. Each skill is a folder with a descriptor that the agent can invoke. Built-in skills: `weather`, `summarize`, `github`, `hardware`, `tmux`, `agent-browser`, `skill-creator`.

---

## Development

```bash
make build          # Build binary (whatsapp_native tag required)
make test           # Run tests
make lint           # golangci-lint
make deps           # go mod tidy
make sync-picoclaw  # Update picoclaw submodule to latest upstream
```

### picoclaw submodule

picoclaw is a git submodule at `picoclaw/`. The `go.mod` replace directive points to it:

```
replace github.com/sipeed/picoclaw => ./picoclaw
```

To update:

```bash
make sync-picoclaw
# commit: picoclaw submodule pointer + go.mod + go.sum
```

### Syncing channel fixes from upstream

```bash
git -C picoclaw diff <old-sha>..<new-sha> -- pkg/channels/whatsapp_native/
git -C picoclaw diff <old-sha>..<new-sha> -- pkg/channels/telegram/
git -C picoclaw diff <old-sha>..<new-sha> -- pkg/channels/email/
```

Apply patches manually. Watch for interface changes in:
- `pkg/channels/base.go` (`channels.Channel`)
- `pkg/bus/types.go` (`bus.OutboundMessage` / `bus.InboundMessage`)

---

See [RELEASE.md](RELEASE.md) for versioning and release instructions.
