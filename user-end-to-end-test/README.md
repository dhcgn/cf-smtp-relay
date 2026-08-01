# User End-to-End Test

This folder contains:

- `run-e2e.ps1` for Windows PowerShell, sending via `Send-MailMessage`
- `run-e2e.sh` for Linux Bash, sending via local SMTP relay and verifying mailbox delivery with `himalaya`

## 1) Prepare environment file

Copy `sample.env` to `.env` and fill at least:

- `CF_API_TOKEN`
- `CF_ACCOUNT_ID`
- `EMAIL` (recipient)
- `FROM_EMAIL` (sender)

For Linux + Himalaya verification, also fill:

- `IMAP_HOST`
- `IMAP_PORT`
- `IMAP_USER`
- `IMAP_PASS`

Defaults are already set for local Docker SMTP relay testing:

- `SMTP_HOST=127.0.0.1`
- `SMTP_PORT=2525`

## 2) Run the Windows PowerShell test

From this folder:

```powershell
./run-e2e.ps1
```

On Windows, make sure Docker Desktop is started first and the engine is running.
If you see an error mentioning `npipe:////./pipe/dockerDesktopLinuxEngine`, start or restart Docker Desktop and retry.

Useful options:

```powershell
# Skip rebuilding the Docker image
./run-e2e.ps1 -NoBuild

# Keep container running after test for debugging
./run-e2e.ps1 -KeepContainer
```

## 3) Run the Linux Bash test (with Himalaya verification)

Dependencies on Linux:

- `docker`
- `curl`
- `himalaya`
- `jq`
- `base64` and `sed` (used to build the test attachment; standard on virtually any Linux system)

Run:

```bash
chmod +x ./run-e2e.sh
./run-e2e.sh
```

Useful options:

```bash
# Skip rebuilding the Docker image
./run-e2e.sh --no-build

# Keep container running after test for debugging
./run-e2e.sh --keep-container

# Skip mailbox verification via himalaya
./run-e2e.sh --skip-himalaya-check
```

## What the scripts do

Common flow:

1. Load values from `.env`.
2. Build Docker image `cf-smtp-relay:e2e` (unless skipped).
3. Start container `cf-smtp-relay-e2e` with `--env-file .env` and map host SMTP port to container `2525`.
4. Wait for SMTP listener readiness.
5. Send a message with a small test attachment through the local relay.
6. Print recent container logs.
7. Stop/remove the container unless keep-container is requested.

Linux Bash script additionally:

1. Creates a temporary Himalaya account config from the env vars.
2. Polls IMAP with `himalaya --json envelope list`.
3. Verifies that the sent test subject appears in the mailbox.
4. Downloads the delivered message's attachment with `himalaya attachment download` and byte-compares it against the file that was sent, proving the attachment survived the full relay → Cloudflare → IMAP round trip.

By default, that Himalaya config is created as a temporary file and deleted during cleanup.
No persistent `.himalaya-e2e.toml` file is required.

Options:

- Use temporary generated config (default): provide `IMAP_*` env vars in `.env`.
- Use your existing Himalaya config: set `E2E_HIMALAYA_CONFIG_PATH` in `.env` to an existing config file path.
- Skip mailbox and attachment verification completely: run `./run-e2e.sh --skip-himalaya-check`.

## Notes

- `Send-MailMessage` is deprecated by Microsoft but intentionally used here, as requested.
- The relay MVP does not support SMTP auth. `SMTP_USER` and `SMTP_PASS` can stay empty.
- The Windows script attaches a small generated text file too, but only proves the relay returned SMTP `250` (which only happens after Cloudflare accepts the message) — it has no IMAP-based verification of the delivered attachment like the Linux script does.
