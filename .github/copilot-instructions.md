# Copilot / AI coding agent instructions — cf-smtp-relay

These instructions apply to any AI coding agent (GitHub Copilot, Claude Code, or similar) working in this repository. They're meant to prevent scope drift: this project has a deliberately small MVP, and it's easy for an agent to "helpfully" add things that were explicitly deferred.

## What this project is

A small Go daemon that accepts locally-sent SMTP messages and forwards each one as a single HTTPS call to the Cloudflare Email Service REST API (`POST /accounts/{account_id}/email/sending/send`). See `README.md` for configuration and the roadmap, and `ARCHITECTURE.md` for the full architecture and design-decision rationale — **read both before making changes**, especially `ARCHITECTURE.md`'s "Key decisions log" section and `README.md`'s "Roadmap / Milestones" section.

## Non-negotiable constraints (do not "fix" these without being asked)

- **No SMTP authentication in the MVP.** The relay trusts its network. Do not add `AUTH LOGIN`/`AUTH PLAIN` support unless a milestone explicitly calls for it.
- **No internal retry queue in the MVP.** Transient Cloudflare errors (`429`/`5xx`) map to SMTP `4xx` and are returned to the caller immediately. Do not add persistent queuing, background workers, or a database unless M3 has been updated to call for it.
- **No config file.** Configuration is env vars only, bound via `viper`. Do not introduce YAML/TOML file loading unless the README's decisions log has been updated.
- **Prefer the Go standard library.** `net/mail`, `mime`, `mime/multipart`, `net/http`, `log/slog` first. Only reach for a third-party package when the stdlib genuinely can't do the job (this is why `github.com/emersion/go-smtp` is a dependency — Go has no stdlib SMTP *server*).

## Required dependencies

- `github.com/emersion/go-smtp` — SMTP server protocol state machine.
- `github.com/spf13/viper` — configuration binding (env vars only for now; see above).
- `github.com/urfave/cli/v3` — CLI scaffolding (subcommands, flags, `--version`/`version` output).

Don't introduce alternative libraries that overlap with these (e.g. another CLI framework or config loader) without discussion.

## Version / commit / build-time reporting

The binary must print its version, git commit, and build time at startup (and via a `version` command/flag). These are injected at build time via `-ldflags`, not hardcoded:

```go
var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)
```

```
-ldflags "-s -w -X main.version=$VERSION -X main.commit=$COMMIT -X main.buildTime=$BUILD_TIME"
```

Both `.github/workflows/release.yml` and any local `Makefile`/build script should set these three values consistently. If you add a new entry point (`cmd/...`), it must wire up the same three variables.

## CI/CD expectations

- `.github/workflows/build-test.yml` runs on every push/PR and **ignores markdown-only changes** (`paths-ignore: **/*.md`). If you add a new top-level directory with only docs in it, make sure it's covered by that ignore pattern, not exempted from it.
- `.github/workflows/release.yml` runs only on `v*.*.*` tags (plus manual dispatch), builds cross-platform binaries (linux/darwin/windows × amd64/arm64) with the ldflags above, attaches them to the GitHub Release, then builds and pushes a multi-arch (`linux/amd64`, `linux/arm64`) Docker image to `ghcr.io`.
- Keep both workflows green. `gofmt`, `go vet`, and `go test -race` must all pass before merging.

## Code conventions

- Structured logging via `log/slog` only — no `fmt.Println`/`log.Printf` for application logs.
- Errors are wrapped with `%w` and given context; no bare error returns without a message.
- Table-driven tests for anything parsing/mapping logic (especially the Cloudflare-error-code → SMTP-status-code mapping table in the README — keep code and docs in sync).
- Favor small, focused packages under `internal/` (e.g. `internal/smtpserver`, `internal/cfclient`, `internal/config`) over one large `main` package.

## Integration test execution

- For full integration verification, run `user-end-to-end-test/run-e2e.sh`.
- On Windows hosts, run it inside WSL (Linux shell), not in native PowerShell.
- On Linux hosts, run it directly in the local shell.
- This script is the preferred end-to-end validation path for Docker + SMTP send + optional Himalaya mailbox verification.

## When implementing a milestone

1. Check `README.md`'s roadmap for the milestone's scope — implement exactly that, not more.
2. If you find a genuine reason to deviate (e.g. a decision turns out to be wrong), update the README's decisions log / roadmap in the same change, and call it out explicitly rather than silently expanding scope.
3. Tick the relevant roadmap checkbox in `README.md` when a milestone is complete.
