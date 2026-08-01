## cf-smtp-relay v0.1.0

First release of `cf-smtp-relay`: a small Go SMTP relay daemon that accepts local SMTP messages and forwards each message as a single HTTPS request to Cloudflare Email Service.

### Highlights

- Implements MVP SMTP relay flow:
  - Accepts core SMTP commands via `go-smtp` (`EHLO`, `MAIL FROM`, `RCPT TO`, `DATA`, `QUIT`)
  - Parses RFC 5322 email content using Go standard library
  - Sends to Cloudflare Email Service API (`/accounts/{account_id}/email/sending/send`)
- Adds structured configuration and validation via environment variables (`viper`)
- Adds Cloudflare error-to-SMTP status mapping for retryable vs permanent failures
- Adds structured logging (`log/slog`) and graceful shutdown handling
- Adds CLI app structure (`urfave/cli/v3`) with:
  - `serve` command
  - `version` command and startup/build metadata (`version`, `commit`, `buildTime`)
- Adds containerization and CI/CD foundation:
  - Multi-stage `Dockerfile`
  - CI workflow for format/vet/test/build checks
  - Release workflow that:
    - builds cross-platform binaries
    - attaches artifacts to GitHub Releases
    - builds/pushes multi-arch image to GHCR

### Test Coverage

- Unit tests added across:
  - Cloudflare client and error mapping
  - Configuration loading/validation
  - SMTP message parsing and session behavior
- Verified with:
  - `go test ./...`

### Notes

- This is the initial MVP-oriented release and intentionally keeps scope small.
- No SMTP authentication and no internal retry queue are included in this milestone by design.
