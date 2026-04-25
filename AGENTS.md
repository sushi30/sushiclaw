# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

**sushiclaw** is a personal AI agent built on top of [picoclaw](https://github.com/sipeed/picoclaw).
It owns three channel packages (WhatsApp native, Telegram, Email) and imports everything else
from picoclaw as a versioned Go module dependency.

## Architecture

- `main.go` — Cobra CLI; blank-imports only the three owned channel packages
- `internal/gateway/` — Minimal gateway wiring (no cron/heartbeat/devices/hot-reload)
- `pkg/channels/whatsapp_native/` — Owned copy of picoclaw's WhatsApp native channel
- `pkg/channels/telegram/` — Owned copy of picoclaw's Telegram channel
- `pkg/channels/email/` — Owned copy of picoclaw's email channel
- `picoclaw/` — git submodule tracking `sipeed/picoclaw` main

All other picoclaw packages (agent loop, bus, providers, config, media, health, pid, logger)
are consumed directly via `github.com/sipeed/picoclaw` in go.mod, resolved through the submodule.

## Build Commands

```bash
make build           # Build binary (always use: go build -tags whatsapp_native)
make test            # Run tests
make install         # Install to ~/.local/bin
make lint            # golangci-lint
make deps            # go mod tidy
make sync-picoclaw   # Update picoclaw submodule to latest upstream + go mod tidy
```

## Cloning

Always clone with submodules:

```bash
git clone --recurse-submodules https://github.com/sushi30/sushiclaw.git
# or after cloning without submodules:
git submodule update --init
```

## Configuration

Copy `config.example.json` to `~/.picoclaw/config.json` (or set `$SUSHICLAW_CONFIG`).

Note: the `env://VAR_NAME` scheme for API keys requires the env:// feature to be implemented
natively (not yet done). For now, set API keys as plain strings in the config file.

## picoclaw dependency

picoclaw is a git submodule at `picoclaw/`. The `go.mod` replace directive points to it:

```
replace github.com/sipeed/picoclaw => ./picoclaw
```

To update picoclaw to the latest upstream:

```bash
make sync-picoclaw
# then commit: picoclaw (updated submodule pointer), go.mod, go.sum
```

## Syncing channel fixes from upstream picoclaw

When upstream picoclaw fixes something in a channel we own, sync selectively:

```bash
# See what changed in whatsapp_native between two upstream commits
git -C picoclaw diff <old-sha>..<new-sha> -- pkg/channels/whatsapp_native/

# Same for telegram or email
git -C picoclaw diff <old-sha>..<new-sha> -- pkg/channels/telegram/
git -C picoclaw diff <old-sha>..<new-sha> -- pkg/channels/email/
```

Apply relevant patches manually. The interface contract to watch for:
- `channels.Channel` interface (in picoclaw's `pkg/channels/base.go`)
- `bus.OutboundMessage` / `bus.InboundMessage` (in picoclaw's `pkg/bus/types.go`)

## Customizing agent behavior

sushiclaw consumes picoclaw's agent loop (`pkg/agent/loop.go`) directly via the
submodule. When we need to change agent behavior (e.g. how slash commands are
handled), follow the gateway pattern: wrap or extend picoclaw's implementation
in `internal/gateway/` or sushiclaw-specific packages rather than forking the
agent loop code. This keeps us on a clean upgrade path — we update the
picoclaw submodule and only maintain our own wrapper code.

Examples:
- **Slash command behavior**: Controlled by `pkg/commands/executor.go` in
  picoclaw. If sushiclaw needs different command routing, wrap the executor in
  `internal/gateway/` rather than copying executor code.
- **Channel-specific logic**: Owned channels (whatsapp_native, telegram, email)
  can be customized freely since we own the full package. Upstream changes to
  these channels must be synced manually (see "Syncing channel fixes" above).

## go.mod hygiene

Always run `make deps` (`go mod tidy`) after:
- Updating the picoclaw submodule
- Adding or removing imports
- Bumping any dependency

Commit `go.mod`, `go.sum`, and (if picoclaw was updated) the `picoclaw` submodule pointer together.
Never use `-mod=mod` as a workaround — fix go.sum at the source with `go mod tidy`.
CI enforces this with a `go mod tidy` check.
