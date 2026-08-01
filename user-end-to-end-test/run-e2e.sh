#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
ENV_FILE="${SCRIPT_DIR}/.env"

NO_BUILD=0
KEEP_CONTAINER=0
SKIP_HIMALAYA_CHECK=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-build)
      NO_BUILD=1
      shift
      ;;
    --keep-container)
      KEEP_CONTAINER=1
      shift
      ;;
    --skip-himalaya-check)
      SKIP_HIMALAYA_CHECK=1
      shift
      ;;
    -h|--help)
      cat <<'USAGE'
Usage: ./run-e2e.sh [--no-build] [--keep-container] [--skip-himalaya-check]

Options:
  --no-build             Skip docker image build
  --keep-container       Keep test container running after test
  --skip-himalaya-check  Skip IMAP mailbox verification with Himalaya
USAGE
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "Missing .env file at ${ENV_FILE}. Copy sample.env to .env and fill values first." >&2
  exit 1
fi

trim_spaces() {
  local s="$1"
  s="${s#${s%%[![:space:]]*}}"
  s="${s%${s##*[![:space:]]}}"
  printf '%s' "$s"
}

load_dotenv() {
  local path="$1"
  local line key value

  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%$'\r'}"
    line="$(trim_spaces "$line")"

    [[ -z "$line" || "${line:0:1}" == "#" ]] && continue
    [[ "$line" != *=* ]] && continue

    key="${line%%=*}"
    value="${line#*=}"

    key="$(trim_spaces "$key")"
    value="$(trim_spaces "$value")"

    if [[ ( "${value:0:1}" == '"' && "${value: -1}" == '"' ) || ( "${value:0:1}" == "'" && "${value: -1}" == "'" ) ]]; then
      value="${value:1:${#value}-2}"
    fi

    export "${key}=${value}"
  done < "$path"
}

load_dotenv "${ENV_FILE}"

require_env() {
  local key="$1"
  if [[ -z "${!key:-}" ]]; then
    echo "Missing required env var '${key}' in ${ENV_FILE}" >&2
    exit 1
  fi
}

require_cmd() {
  local cmd="$1"
  if ! command -v "${cmd}" >/dev/null 2>&1; then
    echo "Required command not found: ${cmd}" >&2
    exit 1
  fi
}

require_env CF_API_TOKEN
require_env CF_ACCOUNT_ID
require_env EMAIL
require_env FROM_EMAIL

SMTP_HOST="${SMTP_HOST:-127.0.0.1}"
SMTP_PORT="${SMTP_PORT:-2525}"
E2E_DOCKER_IMAGE="${E2E_DOCKER_IMAGE:-cf-smtp-relay:e2e}"
E2E_CONTAINER_NAME="${E2E_CONTAINER_NAME:-cf-smtp-relay-e2e}"
E2E_HIMALAYA_POLL_SECONDS="${E2E_HIMALAYA_POLL_SECONDS:-60}"
E2E_HIMALAYA_POLL_INTERVAL_SECONDS="${E2E_HIMALAYA_POLL_INTERVAL_SECONDS:-5}"
E2E_HIMALAYA_CONFIG_PATH="${E2E_HIMALAYA_CONFIG_PATH:-}"

require_cmd docker

if ! docker version --format '{{.Server.Version}}' >/dev/null 2>&1; then
  echo "Docker daemon is not reachable. Start Docker and retry." >&2
  exit 1
fi

if [[ "${NO_BUILD}" -eq 0 ]]; then
  echo "Building Docker image '${E2E_DOCKER_IMAGE}' from ${REPO_ROOT} ..."
  docker build -t "${E2E_DOCKER_IMAGE}" "${REPO_ROOT}"
fi

if docker ps -aq -f "name=^${E2E_CONTAINER_NAME}$" | grep -q .; then
  docker rm -f "${E2E_CONTAINER_NAME}" >/dev/null
fi

echo "Starting container '${E2E_CONTAINER_NAME}' ..."
docker run -d --name "${E2E_CONTAINER_NAME}" --env-file "${ENV_FILE}" -p "${SMTP_PORT}:2525" "${E2E_DOCKER_IMAGE}" >/dev/null

cleanup() {
  if [[ -n "${TMP_HIMALAYA_CONFIG_FILE:-}" && -f "${TMP_HIMALAYA_CONFIG_FILE}" ]]; then
    rm -f "${TMP_HIMALAYA_CONFIG_FILE}" || true
  fi

  if [[ "${KEEP_CONTAINER}" -eq 0 ]]; then
    echo "Stopping and removing test container '${E2E_CONTAINER_NAME}' ..."
    docker rm -f "${E2E_CONTAINER_NAME}" >/dev/null 2>&1 || true
  else
    echo "Leaving container '${E2E_CONTAINER_NAME}' running because --keep-container was set."
  fi
}
trap cleanup EXIT

echo "Waiting for SMTP listener on ${SMTP_HOST}:${SMTP_PORT} ..."
ready=0
for _ in $(seq 1 30); do
  if (echo > "/dev/tcp/${SMTP_HOST}/${SMTP_PORT}") >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 1
done

if [[ "${ready}" -ne 1 ]]; then
  echo "SMTP listener did not become ready in time." >&2
  exit 1
fi

subject="cf-smtp-relay e2e $(date -u +'%Y-%m-%dT%H:%M:%SZ')"
body="End-to-end test via relay container '${E2E_CONTAINER_NAME}' and Himalaya verification."

# Send through local relay using curl's SMTP support.
echo "Sending test email to '${EMAIL}' via relay ${SMTP_HOST}:${SMTP_PORT} ..."
curl --silent --show-error --fail \
  --url "smtp://${SMTP_HOST}:${SMTP_PORT}" \
  --mail-from "${FROM_EMAIL}" \
  --mail-rcpt "${EMAIL}" \
  --upload-file <(
    printf 'From: %s\r\n' "${FROM_EMAIL}"
    printf 'To: %s\r\n' "${EMAIL}"
    printf 'Subject: %s\r\n' "${subject}"
    printf 'Date: %s\r\n' "$(date -R)"
    printf 'Message-ID: <%s@local.test>\r\n' "$(date +%s)-$RANDOM"
    printf '\r\n%s\r\n' "${body}"
  ) >/dev/null

echo "Relay accepted SMTP message."

if [[ "${SKIP_HIMALAYA_CHECK}" -eq 0 ]]; then
  require_cmd himalaya
  require_cmd jq

  if [[ -n "${E2E_HIMALAYA_CONFIG_PATH}" ]]; then
    HIMALAYA_CONFIG_FILE="${E2E_HIMALAYA_CONFIG_PATH}"
    if [[ ! -f "${HIMALAYA_CONFIG_FILE}" ]]; then
      echo "E2E_HIMALAYA_CONFIG_PATH points to a missing file: ${HIMALAYA_CONFIG_FILE}" >&2
      exit 1
    fi
  else
    require_env IMAP_HOST
    require_env IMAP_PORT
    require_env IMAP_USER
    require_env IMAP_PASS

    TMP_HIMALAYA_CONFIG_FILE="$(mktemp)"
    HIMALAYA_CONFIG_FILE="${TMP_HIMALAYA_CONFIG_FILE}"
    cat > "${HIMALAYA_CONFIG_FILE}" <<EOF
[accounts.e2e]
default = true
imap.server = "imaps://${IMAP_HOST}:${IMAP_PORT}"
imap.sasl.plain.username = "${IMAP_USER}"
imap.sasl.plain.password.raw = "${IMAP_PASS}"
mailbox.alias.inbox = "${IMAP_MAILBOX_INBOX:-INBOX}"
EOF
  fi

  echo "Polling mailbox with Himalaya for subject: ${subject}"
  deadline=$(( $(date +%s) + E2E_HIMALAYA_POLL_SECONDS ))
  found=0

  while [[ $(date +%s) -lt ${deadline} ]]; do
    set +e
    out="$(himalaya -c "${HIMALAYA_CONFIG_FILE}" --json envelope list -s 25 2>/dev/null)"
    rc=$?
    set -e

    if [[ ${rc} -eq 0 ]] && [[ -n "${out}" ]]; then
      if echo "${out}" | jq -e --arg s "${subject}" '.. | objects | .subject? // empty | select(. == $s)' >/dev/null 2>&1; then
        found=1
        break
      fi
    fi

    sleep "${E2E_HIMALAYA_POLL_INTERVAL_SECONDS}"
  done

  if [[ "${found}" -ne 1 ]]; then
    echo "Himalaya could not find the email in IMAP within ${E2E_HIMALAYA_POLL_SECONDS}s." >&2
    echo "Recent relay logs:" >&2
    docker logs --tail 80 "${E2E_CONTAINER_NAME}" >&2 || true
    exit 1
  fi

  echo "Himalaya verification succeeded: message found in mailbox."
fi

echo "Recent relay logs:"
docker logs --tail 50 "${E2E_CONTAINER_NAME}" || true

echo "E2E test completed successfully."
