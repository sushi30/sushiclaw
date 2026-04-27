# sushiclaw

Personal AI agent built on top of [`agent-sdk-go`](https://github.com/Ingenimax/agent-sdk-go).

Runs on WhatsApp, Telegram, and Email. Customizable via workspace files and skills.

---

## Quick start

### 1. Clone

```bash
git clone https://github.com/sushi30/sushiclaw.git
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
  -e OPENAI_API_KEY=sk-... \
  ghcr.io/sushi30/sushiclaw:latest gateway
```

Health check hits `http://localhost:18790/health`.

---

## Configuration

Copy `config.example.json` to `~/.picoclaw/config.json`. Key sections:

```json
{
  "agents": {
    "defaults": {
      "workspace": "~/.picoclaw/workspace",
      "model_name": "gpt-4o-mini"
    }
  },
  "model_list": [{ "model_name": "gpt-4o-mini", "api_key": "env://OPENAI_API_KEY" }],
  "channels": {
    "email": { "enabled": false, "type": "email", "...": "..." }
  },
  "tools": { ... }
}
```

Override config path with `$SUSHICLAW_CONFIG`.

---

## Added features

### `env://` config resolver

API keys in `config.json` can reference environment variables:

```json
{ "api_key": "env://OPENAI_API_KEY" }
```

Resolved at load time by `pkg/config.SecureString` during JSON unmarshal.

For Telegram image handling, enable the vision tool with a vision-capable model:

```json
{
  "tools": {
    "vision": {
      "enabled": true,
      "model_name": "openrouter-glm-5v"
    }
  }
}
```

---

### MCP server support

Connect the agent to [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) servers for external tools (filesystem, GitHub, databases, etc.).

```json
{
  "mcp": {
    "mcpServers": {
      "github": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-github"],
        "env": { "GITHUB_PERSONAL_ACCESS_TOKEN": "env://GITHUB_TOKEN" }
      }
    }
  }
}
```

See [docs/MCP.md](docs/MCP.md) for full configuration options and examples.

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

### Email Channel

SMTP (outbound) + IMAP polling (inbound). Config lives under `channels.email`.
The legacy top-level `email_channel` key is no longer supported.

```json
{
  "channels": {
    "email": {
      "enabled": true,
      "type": "email",
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

- Port 465 → implicit TLS (SMTP). Port 587 → STARTTLS.
- Port 993 → implicit TLS (IMAP).
- Processed messages are marked `\Seen`.

> **Migrating from an older config?** See [RELEASE_NOTES.md](RELEASE_NOTES.md) for step-by-step instructions if your config uses the old top-level `email_channel` format.

---

## Workspace customization

The agent loads workspace markdown entrypoints from `agents.defaults.workspace` at startup:

| File | Purpose |
|------|---------|
| `AGENT.md` | Agent name, role, mission, capabilities |
| `IDENTITY.md` | Identity, profile details, stable preferences |
| `SOUL.md` | Personality and communication style |
| `USER.md` | Legacy alias for `IDENTITY.md` in older workspaces |

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
```

---

See [RELEASE.md](RELEASE.md) for versioning and release instructions.
