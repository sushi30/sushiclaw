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

All other picoclaw packages (agent loop, bus, providers, config, media, health, pid, logger)
are consumed directly via `github.com/sipeed/picoclaw` in go.mod.

## Build Commands

```bash
make build       # Build binary
make test        # Run tests
make install     # Install to ~/.local/bin
make lint        # golangci-lint
make deps        # go mod tidy
```

## Configuration

Copy `config.example.json` to `~/.picoclaw/config.json` (or set `$SUSHICLAW_CONFIG`).
API keys support the `env://VAR_NAME` scheme: `"api_key": "env://ANTHROPIC_API_KEY"`.

## Syncing channel fixes from upstream picoclaw

When upstream picoclaw fixes something in a channel we own, sync selectively:

```bash
# See what changed in whatsapp_native between two upstream versions
git -C ../picoclaw diff v1.2.3..v1.3.0 -- pkg/channels/whatsapp_native/

# Same for telegram or email
git -C ../picoclaw diff v1.2.3..v1.3.0 -- pkg/channels/telegram/
git -C ../picoclaw diff v1.2.3..v1.3.0 -- pkg/channels/email/
```

Apply relevant patches manually. The interface contract to watch for:
- `channels.Channel` interface (in picoclaw's `pkg/channels/base.go`)
- `bus.OutboundMessage` / `bus.InboundMessage` (in picoclaw's `pkg/bus/types.go`)

## env:// scheme status

The `env://` key resolution is implemented in picoclaw's `pkg/credential/` package on the
`sushi30` branch. The `go.mod` `replace` directive currently points to the local picoclaw fork:

```
replace github.com/sipeed/picoclaw => ../picoclaw
```

Once env:// is merged upstream to `sipeed/picoclaw`, remove the replace directive and pin
to a released version: `go get github.com/sipeed/picoclaw@vX.Y.Z`.

## Bumping picoclaw version

```bash
# With replace directive (local fork):
# Just ensure ../picoclaw is on the right branch.

# Without replace directive (upstream release):
go get github.com/sipeed/picoclaw@latest
make deps
make build
```

## Fork-specific test files

New tests for sushiclaw-specific behaviour go in `*_sushi30_test.go` files within the
owned packages. This convention makes bespoke additions easy to identify.
