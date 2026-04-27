# AGENTS.md

Guidance for AI agents and developers working in this repository.

## What this is

**sushiclaw** is a personal AI agent built on top of [`github.com/Ingenimax/agent-sdk-go`](https://github.com/Ingenimax/agent-sdk-go).
It provides its own implementations of channels (WhatsApp native, Telegram, Email), message bus,
configuration, command system, and media management. The agent loop, LLM interfaces, and tool
contracts come from `agent-sdk-go`.

> **Note:** Historical references to [picoclaw](https://github.com/sipeed/picoclaw) in docs and
> comments are outdated. sushiclaw no longer vendors or depends on picoclaw. The `picoclaw/`
> submodule and `replace` directive have been removed.

## Repository layout

### `main.go`
Cobra CLI entrypoint. Registers three subcommands (`gateway`, `chat`, `version`) and blank-imports
the owned channel packages (whatsapp_native, telegram) to trigger their `init()` registration.

### `internal/`

| Package | Purpose |
|---------|---------|
| `internal/agent/` | Wraps `agent-sdk-go` to build the agent, manage sessions, and handle inbound bus messages. Includes `InMemoryMemory` (no persistence), `ContextBuilder` (assembles system prompt from workspace files), and `SessionManager` (dispatches messages to the agent). |
| `internal/chat/` | Terminal REPL for `sushiclaw chat`. Creates an agent directly (bypassing the bus) and runs an interactive prompt loop. |
| `internal/commandfilter/` | Intercepts inbound messages before they reach the agent. Blocks unrecognized slash commands with a helpful error reply. |
| `internal/envresolve/` | Shim that resolves `env://VAR_NAME` references in config fields. Most resolution happens in `pkg/config.SecureString` during JSON unmarshal; this covers programmatic edge cases. |
| `internal/gateway/` | Gateway wiring: loads config, initializes the bus, channel manager, agent session, media store, and command executor. Runs the main event loop. |
| `internal/version/` | Build-time version variables injected via `-ldflags`. |

### `pkg/`

| Package | Purpose |
|---------|---------|
| `pkg/bus/` | `MessageBus` with typed inbound/outbound/media channels. All channels and the agent share one bus instance. |
| `pkg/channels/` | Channel infrastructure: `BaseChannel` (shared functionality, allow-list, group triggers, typing/reaction/placeholder orchestration), `Manager` (lifecycle and outbound dispatch), registry, and capability interfaces (`StreamingCapable`, `TypingCapable`, etc.). |
| `pkg/channels/whatsapp_native/` | Owned WhatsApp native channel (requires `whatsapp_native` build tag). |
| `pkg/channels/telegram/` | Owned Telegram channel. |
| `pkg/channels/email/` | Owned Email channel (SMTP outbound + IMAP polling inbound). Registered through the standard channel registry. |
| `pkg/commands/` | Slash command registry, executor, and built-in command definitions (`/help`, `/clear`, `/list models`, etc.). Supports `!` prefix in addition to `/`. |
| `pkg/config/` | Config loading, `SecureString` (with `env://` resolution), `FlexibleStringSlice`, and path helpers. |
| `pkg/identity/` | Sender identity matching for allow-lists. |
| `pkg/llm/openrouter/` | OpenRouter LLM client wrapper around `agent-sdk-go`'s OpenAI client. Strips `openrouter/` prefix from model names. |
| `pkg/logger/` | Structured logging with zerolog. Supports component tagging, file logging, and panic log initialization. |
| `pkg/media/` | `FileMediaStore` for media file lifecycle management. Supports TTL-based background cleanup. |
| `pkg/steering/` | Prompt steering utilities. |
| `pkg/tools/` | `TrustedExecTool` — wraps `exec.ExecTool` to allow specific chat IDs to bypass remote-channel exec restrictions. |
| `pkg/tools/exec/` | `ExecTool` — shell command execution tool for the agent. Respects `restrict_to_workspace` and `allowRemote` settings. |
| `pkg/utils/` | General utilities. |

## Architecture

```
┌─────────────────┐     inbound      ┌──────────────────┐
│  Channels       │ ───────────────► │  CommandFilter   │
│  (WhatsApp,     │                  │  (blocks unknown │
│   Telegram,     │                  │   slash cmds)    │
│   Email)        │                  └──────────────────┘
└─────────────────┘                         │
                                            ▼
                                    ┌──────────────────┐
                                    │  HasCommandPrefix│
                                    │  (/, !)          │
                                    └──────────────────┘
                                           │
                        ┌──────────────────┼──────────────────┐
                        ▼                  ▼                  ▼
                 ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
                 │  Known cmd  │    │ Unknown cmd │    │  Not a cmd  │
                 │  → Executor │    │  → Blocked  │    │  → Pass     │
                 └─────────────┘    └─────────────┘    └─────────────┘
                        │                                    │
                        ▼                                    ▼
                 ┌─────────────┐                      ┌─────────────┐
                 │  Outcome    │                      │  Agent      │
                 │  Handled    │                      │  Session    │
                 │  → Reply    │                      │  (bus)      │
                 └─────────────┘                      └─────────────┘
                                                             │
                                                             ▼
                                                      ┌─────────────┐
                                                      │  Outbound   │
                                                      │  (bus)      │
                                                      └─────────────┘
                                                             │
                                                             ▼
                                                      ┌─────────────┐
                                                      │  Channel    │
                                                      │  Manager    │
                                                      │  dispatch   │
                                                      └─────────────┘
                                                             │
                                                             ▼
                                                      ┌─────────────┐
                                                      │  Reply sent │
                                                      │  to user    │
                                                      └─────────────┘
```

All channels and the agent share a single `MessageBus`. The gateway runs one goroutine for inbound
processing (filter → executor/agent) and the channel manager runs one goroutine for outbound
dispatch (placeholder editing, typing stops, reaction undos, normal sends).

## Build commands

```bash
make build           # Build binary (whatsapp_native tag, CGO_ENABLED=0)
make test            # Run tests
make test-cover      # Run tests with coverage summary
make coverage-html   # Generate coverage.html
make coverage        # Alias for coverage-html
make install         # Build + copy to ~/.local/bin
make lint            # golangci-lint
make fmt             # gofmt -w .
make vet             # go vet ./...
make deps            # go mod tidy
make test-integration # Run email integration tests
make release-check   # Verify VERSION is set (not "dev")
make air             # Start a dev server in a new tmux pane using `air`
```

Always build with the `whatsapp_native` tag:

```bash
CGO_ENABLED=0 go build -tags whatsapp_native -o sushiclaw .
```

## Environment variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `SUSHICLAW_HOME` | Home directory for configs, workspace, logs | `$PICOCLAW_HOME` → `~/.picoclaw` |
| `SUSHICLAW_CONFIG` | Path to `config.json` | `$SUSHICLAW_HOME/config.json` |
| `SUSHICLAW_EXEC_ALLOWED_SENDERS` | Comma-separated chat IDs that can use the `exec` tool remotely (bypasses `allowRemote=false`) | *(unset = no trusted senders)* |

## CLI commands

| Command | Description |
|---------|-------------|
| `sushiclaw gateway` | Start the full gateway (all channels + agent) |
| `sushiclaw chat` | Interactive terminal chat with the agent (bypasses bus) |
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

## Configuration

Config is JSON. Load order:

1. `$SUSHICLAW_CONFIG` if set
2. `$SUSHICLAW_HOME/config.json`
3. `~/.picoclaw/config.json`

Copy `config.example.json` to get started.

### `env://` resolution

Any config field that uses `SecureString` supports `env://VAR_NAME` references:

```json
{ "api_key": "env://OPENAI_API_KEY" }
```

Resolution happens during `json.Unmarshal` in `pkg/config.SecureString.UnmarshalJSON`. If the
environment variable is not set, the raw `env://...` string is preserved; `envresolve.SecureStringRequired`
can report this as an error.

### Key config sections

| Section | Description |
|---------|-------------|
| `agents.defaults` | Default agent settings: `model_name`, `workspace`, `restrict_to_workspace`, `max_tokens`, `temperature`, `max_tool_iterations` |
| `model_list` | Array of model configs. Each needs `model_name`, `model` (provider ID), `api_key` (supports `env://`), optional `api_base` |
| `channels` | Map of channel configs (WhatsApp native, Telegram, Email). Email config lives under `channels.email`; top-level `email_channel` is no longer supported. |
| `gateway` | `host`, `port`, `log_level` |
| `tools` | `exec.enabled`, `media_cleanup.enabled`, `media_cleanup.max_age`, `media_cleanup.interval` |

### Workspace files

The agent loads markdown files from `agents.defaults.workspace` at startup to build the system prompt:

| File | Purpose |
|------|---------|
| `AGENT.md` | Agent name, role, mission, capabilities |
| `IDENTITY.md` | Identity, profile details, stable preferences |
| `MEMORY.md` | Durable notes, long-lived facts, session-independent context |
| `SOUL.md` | Personality and communication style |
| `USER.md` | Legacy alias for `IDENTITY.md` in older workspaces |

`internal/agent/context.go` (`ContextBuilder`) assembles these into a single system prompt with
mtime-based caching. Skills in `workspace/skills/*/SKILL.md` are also enumerated and included.

## Agent behavior

- **Memory**: `InMemoryMemory` only. No persistence. Restarting the gateway clears all conversation
  history. `/clear` resets memory for the running session.
- **System prompt**: Built from workspace files (see above). Falls back to a default prompt if no
  workspace files exist.
- **Tool availability**: If no tools are registered, the system prompt explicitly tells the agent it
  cannot execute commands or take real-world actions.
- **Dispatch**: Inbound messages are dispatched to the agent session in a new goroutine per message.
- **Streaming**: Supported on Telegram via `StreamingCapable`/`Streamer`. The channel manager
  coordinates placeholder messages, typing indicators, and reaction emojis around stream lifecycle.

## Slash commands

The command filter (`internal/commandfilter/`) blocks messages that start with `/` or `!` but match
no known command. This prevents typos from being sent to the LLM.

Built-in commands (defined in `pkg/commands/commands.go`):

| Command | Description |
|---------|-------------|
| `/start` | Greeting |
| `/help` | List available commands |
| `/clear` | Clear conversation history |
| `/debug` | Toggle debug event forwarding |
| `/model` | Show or switch model |
| `/show` | Show current configuration |
| `/list models` | List configured models |
| `/use` | Use a specific configuration |
| `/btw` | Add a note to conversation context |
| `/switch` | Switch model or channel |
| `/check` | Check system status |
| `/subagents` | Manage subagents |
| `/reload` | Reload configuration |

Command execution flow:
1. `CommandFilter.Filter()` — allows known commands, blocks unknown ones
2. `Executor.Execute()` — runs handler if found, returns `OutcomeHandled` or `OutcomePassthrough`
3. `OutcomeHandled` → reply sent directly to user
4. `OutcomePassthrough` → message forwarded to agent session

## Security

### `allow_from`

Every channel has an `allow_from` array. Empty means **allow everyone** (a warning is logged at
startup). Use `["*"]` to explicitly allow everyone without the warning. Supports canonical IDs,
usernames (with or without `@`), and `|` concatenated forms.

### `restrict_to_workspace`

When `true`, the `exec` tool restricts commands to the workspace directory. See
`pkg/tools/exec/exec_tool.go` for the restriction logic.

### Trusted exec

`SUSHICLAW_EXEC_ALLOWED_SENDERS` allows specific chat IDs to run the `exec` tool even from remote
channels. Without this env var, remote channels get `allowRemote=false` and shell execution is
blocked for non-local chats. See `pkg/tools/trusted_exec.go`.

## LLM providers

The agent supports two provider paths (determined in `internal/agent/session.go`):

| Prefix | Provider | Notes |
|--------|----------|-------|
| `openrouter/` | OpenRouter | Uses `pkg/llm/openrouter/`. Base URL defaults to `https://openrouter.ai/api/v1`. Prefix is stripped before API call. |
| *(none)* | OpenAI-compatible | Uses `agent-sdk-go`'s OpenAI client. Supports custom `api_base`. |

## Channels

### WhatsApp native (`whatsapp_native`)

- Requires build tag: `-tags whatsapp_native`
- Uses `go.mau.fi/whatsmeow`
- Supports voice memo transcription (ASR provider configured in `voice` block)
- Supports interactive widgets via MIME-style metadata (`application/x-wa-buttons`, `application/x-wa-list`)
- Unauthorized senders receive a rejection reply

### Telegram (`telegram`)

- Uses `mymmrac/telego`
- Supports streaming responses (`streaming.enabled`)
- Supports markdown rendering (MarkdownV2 or HTML)
- Group trigger: `mention_only`, prefix-based, or always respond

### Email (`email`)

- Registered through the standard channel registry
- Config lives under `channels.email`; top-level `email_channel` is no longer supported
- SMTP (outbound) + IMAP polling (inbound)
- Port 465 → implicit TLS. Port 587 → STARTTLS. Port 993 → implicit TLS (IMAP).
- Processed messages are marked `\Seen`

See `RELEASE_NOTES.md` for the email config migration guide if upgrading from an older config format.

## Docker

```bash
docker build -t sushiclaw .
docker run -d \
  -v ~/.picoclaw:/home/sushiclaw/.picoclaw \
  -e OPENAI_API_KEY=sk-... \
  sushiclaw gateway
```

- Multi-stage build: `golang:1.25-alpine` → `alpine:3.21`
- Health check: `wget -q --spider http://localhost:18790/health` (picoclaw heritage health server)
- Runs as non-root user `sushiclaw` (uid 1000)

## CI/CD

GitHub Actions workflows (`.github/workflows/`):

| Workflow | Trigger | Jobs |
|----------|---------|------|
| `pr.yml` | PR + push to `main` | go mod tidy check, golangci-lint, tests, Docker build (no push) |
| `release.yml` | Tag `YYYY.MM.*` | GoReleaser builds binaries + Docker images, publishes GitHub Release |
| `docker.yml` | Push to `main` | Build and push `dev-<commit>` image to GHCR |

## Releases

See `RELEASE.md` for the full release process. sushiclaw uses calendar versioning (`YYYY.MM.N`).

## go.mod hygiene

Always run `make deps` (`go mod tidy`) after:
- Adding or removing imports
- Bumping any dependency

Commit `go.mod` and `go.sum` together. Never use `-mod=mod` as a workaround.
CI enforces this with a `go mod tidy` check.
