# Architecture

This document covers how cf-smtp-relay works internally: the request flow, why its
dependencies were chosen, the Cloudflare-to-SMTP error mapping, design decisions, and
project layout. For installation, configuration, and the roadmap, see
[README.md](README.md).

## How it works

```
┌─────────────┐   SMTP (25/587/2525)   ┌───────────────┐   HTTPS REST   ┌────────────────────────┐
│ Self-hosted │ ─────────────────────▶ │  cf-smtp-relay │ ─────────────▶ │ Cloudflare Email Service│
│ app (Immich,│                        │  (this repo)   │                │  /email/sending/send    │
│ Vaultwarden)│ ◀───────────────────── │                │ ◀───────────── │                          │
└─────────────┘   SMTP response codes  └───────────────┘  success/error  └────────────────────────┘
```

1. The relay speaks just enough SMTP to accept `EHLO`/`MAIL FROM`/`RCPT TO`/`DATA`/`QUIT` from a local client.
2. The `DATA` payload (a raw RFC 5322 message) is parsed with Go's standard `net/mail` / `mime` / `mime/multipart` packages to extract `From`, `To`, `Subject`, plain-text and HTML bodies, and attachments.
3. The parsed message is converted into a single `POST https://api.cloudflare.com/client/v4/accounts/{account_id}/email/sending/send` call.
4. Cloudflare's response (`delivered` / `queued` / `permanent_bounces` / `errors`) is mapped back to an SMTP status code and returned to the sending app.

## Core dependencies

Kept deliberately short — stdlib first, third-party only where it earns its place:

| Package | Purpose |
|---|---|
| [`github.com/emersion/go-smtp`](https://github.com/emersion/go-smtp) | SMTP server protocol state machine (stdlib only has an SMTP *client*) |
| [`github.com/spf13/viper`](https://github.com/spf13/viper) | Configuration binding — env vars only for now, but leaves the door open for a config file later without a rewrite |
| [`github.com/urfave/cli/v3`](https://github.com/urfave/cli/v3) | CLI scaffolding: `serve` / `version` subcommands, flags, and built-in `--version` handling |

Everything else — message parsing, the Cloudflare HTTP call, logging — stays in the standard library (`net/mail`, `mime`, `mime/multipart`, `net/http`, `log/slog`).

## Error mapping: Cloudflare → SMTP

Cloudflare's REST API returns numeric error codes; the relay maps these to SMTP reply codes so sending applications retry (or give up) the way they normally would against a real MTA:

| Cloudflare code | Meaning | SMTP mapping (implemented) |
|---|---|---|
| `10004` (429, throttled) | Rate limit exceeded | `450 4.7.1` — temporary, client should retry |
| `10100` (503, upstream auth) | Auth service unavailable | `451 4.4.2` — temporary |
| `10002` / `10003` (500) | Internal / not implemented | `451 4.4.2` — temporary |
| `10101` / `10102` / `10103` (401/403) | Bad or unauthorized token | `550 5.7.1` — permanent, logged loudly for the operator (not a client-fixable problem) |
| `10203` (403, sending disabled) | Domain/account not enabled for sending | `550 5.7.1` — permanent |
| `10200` (400, too big) | Message exceeds size limit | `552 5.3.4` — permanent |
| `10001` / `10201` / `10202` (400) | Malformed request/content | `550 5.6.0` — permanent |

Any other/unrecognized Cloudflare error code (including undocumented codes) falls back to a classification by HTTP status: `429` → `450 4.7.1` (temporary), `>= 500` → `451 4.4.2` (temporary), `401`/`403` → `550 5.7.1` (permanent, logged loudly), `400`/`404` → `550 5.6.0` (permanent), anything else (including network-level failures with no Cloudflare status at all) → `451 4.4.2` (temporary).

In the MVP, every one of these mapped codes is returned straight to the connecting app — there's no internal retry queue yet, so a `450`/`451` simply means "your app should try again," the same as it would against any other flaky MTA.

## Design decisions

- **Why not 100% Go standard library?** Go's standard library ships an SMTP *client* (`net/smtp`) but no SMTP *server*. Per the project's own preference for native packages first, the relay uses stdlib wherever it can — `net/mail`, `mime`, `mime/multipart` for message parsing, `net/http` for the Cloudflare call, `log/slog` for logging — but relies on [`github.com/emersion/go-smtp`](https://github.com/emersion/go-smtp) for the actual SMTP protocol state machine, since writing and hardening a spec-compliant SMTP server from scratch isn't a good use of MVP time.
- **Go 1.26**: built with Go 1.26, taking advantage of the default Green Tea garbage collector (lower GC overhead under many short-lived goroutines/connections) and the experimental goroutine-leak profiler (`GOEXPERIMENT=goroutineleakprofile` / `runtime/pprof` `goroutineleak`) in CI tests, given the connection-per-goroutine nature of an SMTP server.
- **Docker best practices applied**: multi-stage build (build stage with full Go toolchain → minimal `distroless` runtime stage), non-root user, no shell in the final image, `SIGTERM`-aware graceful shutdown, structured logs to stdout/stderr (12-factor), semantic version + `sha-<short>` image tags, multi-arch manifest (`linux/amd64`, `linux/arm64`) published via GitHub Actions to `ghcr.io`.
- **No built-in authentication**: the relay assumes it only ever sits on an internal, trusted Docker network (no published port), the same trust model already used by plenty of internal mail relays. This keeps the MVP small and avoids building credential storage/rotation before it's needed. If a future use case needs the relay reachable from somewhere less trusted, `AUTH`/STARTTLS becomes a real milestone rather than a day-one requirement.
- **No internal retry queue in the MVP**: transient Cloudflare errors are surfaced immediately as SMTP `4xx` codes, and the sending application's own delivery/retry logic is responsible for trying again — the same behavior it would already have against a temporarily unreachable real MTA. Whether to add a persistent local retry queue later is left open for M3, once it's clear from real usage whether client-side retries are good enough.
- **Env vars only, no config file**: keeps the MVP's surface area small and matches typical Docker/12-factor deployment; a config file can be reconsidered later if the env var list grows unwieldy.
- **`viper` for configuration binding**: even though the MVP only reads env vars, `viper` gives structured, validated binding (with defaults and type coercion) for free, and means adding a config file later (if ever needed) is a small change rather than a rewrite.
- **`urfave/cli/v3` for the CLI shell**: gives the binary a proper `serve` command plus a `version` command/flag with essentially no boilerplate, instead of hand-rolling flag parsing.
- **Version reporting**: `version`, `commit`, and `buildTime` are package-level `var`s in `main`, left as zero values (`"dev"`/`"none"`/`"unknown"`) for local builds and set via `-ldflags -X` in CI, then logged once at startup and exposed through the CLI's version output — the same pattern used by most Go CLI tools.
- **Attachments**: any MIME part that isn't `text/plain`/`text/html` — or that is explicitly marked `Content-Disposition: attachment`, or carries a filename (via `Content-Disposition` or a legacy `Content-Type; name=`) — is parsed out as a regular file attachment and forwarded in Cloudflare's `attachments` array as `{content (base64), filename, type, disposition: "attachment"}`. Inline/`cid:`-referenced images are out of scope: an `inline`-disposition part with a filename is still forwarded, just as a normal attachment rather than one resolvable via `cid:` in the HTML body. A message with *no* text/HTML content at all (e.g. attachment-only) is still rejected — Cloudflare's own API requires at least one of `text`/`html` on every send, so this is no longer just an MVP simplification. There's no separate attachment size/count limit: the existing `SMTP_MAX_MESSAGE_SIZE_BYTES` cap (default 5 MiB) already matches Cloudflare's total-message-size-including-attachments limit.
- **Single recipient enforcement**: `go-smtp`'s own `MaxRecipients: 1` setting rejects a second `RCPT TO` with `452 4.5.3` before the relay's own recipient-handling code is even invoked again — no need to hand-roll that check.

## Key decisions log

| Decision | Choice | Rationale |
|---|---|---|
| SMTP auth | None — trust the Docker network | Keeps MVP small; matches how many internal relays already operate |
| Cloudflare API downtime/rate-limit | Fail fast, no local retry queue (for now) | Simplest MVP behavior; sending apps already have their own retry logic |
| Configuration | Env vars only | 12-factor style, no config file to parse/validate |
| Config binding library | `viper` | Structured/validated env var binding, config file support possible later without a rewrite |
| CLI framework | `urfave/cli/v3` | Free `serve`/`version` subcommands and flag handling |
| SMTP server library | `go-smtp` | Stdlib has no SMTP server; hand-rolling DATA dot-stuffing/line-length/reply-code framing risks silently corrupting mail, which is worse than one extra dependency |
| Attachments | Parse any non-text MIME part (or any part with a filename) as a regular file attachment; forward as Cloudflare's base64 `attachments` array; no inline/`cid:` support | Completes the M1-era "silently strip" placeholder; reuses the existing SMTP message-size cap instead of a new Cloudflare-specific limit |
| CI structure | Separate build-test (path-ignores `.md`) and tag-triggered release workflows | Docs edits don't trigger builds; releases stay tied to version tags |

## Project layout

```
cmd/cf-smtp-relay/     # main package: CLI wiring (urfave/cli/v3), version vars
internal/config/       # viper-backed configuration loading
internal/smtpserver/   # go-smtp server setup, SMTP <-> internal message mapping
internal/cfclient/     # Cloudflare Email Sending REST API client
```
