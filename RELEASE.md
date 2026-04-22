# Releases

sushiclaw uses calendar versioning: **YYYY.MM.iteration** (e.g. `2026.04.0`, `2026.04.1`).

- `YYYY` — year of release
- `MM` — month of release
- `iteration` — zero-indexed counter within that month (resets each month)

There is no semver or breaking-change tracking — bump the iteration for any release.

## Creating a release

1. Ensure main is up to date and all checks pass.
2. Tag and push:

   ```bash
   git tag 2026.04.0 -m "Release 2026.04.0"
   git push origin 2026.04.0
   ```

3. The `release` workflow builds binaries and Docker images via GoReleaser, then creates a GitHub Release.

## Docker tags

| Event                | Tags                          | Source                     |
|----------------------|-------------------------------|----------------------------|
| Push to `main`       | `dev-<8-char-commit>`        | `docker.yml` workflow      |
| Push tag `YYYY.MM.N` | `YYYY.MM.N`, `latest`        | `release.yml` (GoReleaser) |

## Checking the version

```bash
./sushiclaw version
# 2026.04.0 (git: abc12345, built: 2026-04-22T15:30:00Z, go: go1.25.0)
```

Unbuilt binaries show `dev (git: ..., built: ..., go: ...)`.

## Building locally

```bash
make build                      # VERSION=dev (auto-detected from git tags)
VERSION=2026.04.0 make build    # Override for specific version
```