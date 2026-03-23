#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  tools/scripts/local-dev-e2e.sh --package-id <owner/repo> --repo <path> --tag <tag> [options]

Options:
  --indexer-port <port>     Local indexer port (default: 18080)
  --activation-dir <path>   Activation dir for the user CLI (default: temp dir)
  --state-dir <path>        State dir for the user CLI (default: temp dir)
  --keep                    Keep temp directories after completion
  --help                    Show this help

This script:
1. generates a local dev JWT and JWKS
2. runs fontpub-indexer in local Git dev mode
3. publishes the requested package version into a temp artifacts directory
4. reads published metadata back from the local indexer
5. runs fontpub ls-remote/show/install/ls/verify against the local indexer
EOF
}

PACKAGE_ID=""
REPO_ROOT=""
TAG_NAME=""
INDEXER_PORT="18080"
ACTIVATION_DIR=""
STATE_DIR=""
KEEP="0"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --package-id)
      PACKAGE_ID="${2:-}"
      shift 2
      ;;
    --repo)
      REPO_ROOT="${2:-}"
      shift 2
      ;;
    --tag)
      TAG_NAME="${2:-}"
      shift 2
      ;;
    --indexer-port)
      INDEXER_PORT="${2:-}"
      shift 2
      ;;
    --activation-dir)
      ACTIVATION_DIR="${2:-}"
      shift 2
      ;;
    --state-dir)
      STATE_DIR="${2:-}"
      shift 2
      ;;
    --keep)
      KEEP="1"
      shift
      ;;
    --help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -z "$PACKAGE_ID" || -z "$REPO_ROOT" || -z "$TAG_NAME" ]]; then
  usage >&2
  exit 2
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_TOP="$(cd "${SCRIPT_DIR}/../.." && pwd)"
GO_DIR="${REPO_TOP}/go"
GOCACHE_DIR="${REPO_TOP}/.gocache"

TEMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/fontpub-local-e2e.XXXXXX")"
ARTIFACTS_DIR="${TEMP_ROOT}/artifacts"
AUTH_DIR="${TEMP_ROOT}/auth"
LOG_DIR="${TEMP_ROOT}/logs"
mkdir -p "${ARTIFACTS_DIR}" "${AUTH_DIR}" "${LOG_DIR}"

if [[ -z "$STATE_DIR" ]]; then
  STATE_DIR="${TEMP_ROOT}/state"
fi
if [[ -z "$ACTIVATION_DIR" ]]; then
  ACTIVATION_DIR="${TEMP_ROOT}/fonts"
fi

INDEXER_PID=""

cleanup() {
  if [[ -n "$INDEXER_PID" ]]; then
    kill "$INDEXER_PID" >/dev/null 2>&1 || true
  fi
  if [[ "$KEEP" != "1" ]]; then
    rm -rf "$TEMP_ROOT"
  fi
}
trap cleanup EXIT

LOWER_PACKAGE_ID="$(printf '%s' "$PACKAGE_ID" | tr '[:upper:]' '[:lower:]')"
REPOSITORY_ID_SAFE="$(printf '%s' "$PACKAGE_ID" | tr '/[:upper:]' '-[:lower:]')"
SHA="$(git -C "$REPO_ROOT" rev-parse "$TAG_NAME")"
WORKFLOW_REF="${LOWER_PACKAGE_ID}/.github/workflows/fontpub.yml@refs/heads/main"

env GOCACHE="${GOCACHE_DIR}" go run "${REPO_TOP}/tools/scripts/gen-dev-token.go" \
  --output-dir "${AUTH_DIR}" \
  --repository "${PACKAGE_ID}" \
  --repository-id "fontpub-dev-${REPOSITORY_ID_SAFE}" \
  --sha "${SHA}" \
  --ref "refs/tags/${TAG_NAME}" \
  --workflow-ref "${WORKFLOW_REF}" \
  --workflow-sha "1111111111111111111111111111111111111111" \
  --event-name "push"

JWKS_JSON="$(cat "${AUTH_DIR}/jwks.json")"
(
  cd "${GO_DIR}"
  env \
    GOCACHE="${GOCACHE_DIR}" \
    FONTPUB_ARTIFACTS_DIR="${ARTIFACTS_DIR}" \
    FONTPUB_DEV_LOCAL_REPO_MAP="${LOWER_PACKAGE_ID}=${REPO_ROOT}" \
    FONTPUB_INDEXER_ADDR="127.0.0.1:${INDEXER_PORT}" \
    FONTPUB_GITHUB_JWKS_JSON="${JWKS_JSON}" \
    go run ./cmd/fontpub-indexer >"${LOG_DIR}/indexer.log" 2>&1
) &
INDEXER_PID="$!"

for _ in $(seq 1 50); do
  if curl -fsS "http://127.0.0.1:${INDEXER_PORT}/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done
curl -fsS "http://127.0.0.1:${INDEXER_PORT}/healthz" >/dev/null

TOKEN="$(cat "${AUTH_DIR}/token.txt")"
BODY="$(printf '{"repository":"%s","sha":"%s","ref":"refs/tags/%s"}' "$PACKAGE_ID" "$SHA" "$TAG_NAME")"
curl -fsS \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d "${BODY}" \
  "http://127.0.0.1:${INDEXER_PORT}/v1/update" >"${LOG_DIR}/publish.json"
curl -fsS "http://127.0.0.1:${INDEXER_PORT}/v1/index.json" >/dev/null

FONTPUB_ENV=(
  "GOCACHE=${GOCACHE_DIR}"
  "FONTPUB_BASE_URL=http://127.0.0.1:${INDEXER_PORT}"
  "FONTPUB_STATE_DIR=${STATE_DIR}"
  "FONTPUB_ACTIVATION_DIR=${ACTIVATION_DIR}"
  "FONTPUB_DEV_LOCAL_REPO_MAP=${LOWER_PACKAGE_ID}=${REPO_ROOT}"
)

echo "== publish response =="
cat "${LOG_DIR}/publish.json"
echo

echo "== fontpub ls-remote --json =="
(cd "${GO_DIR}" && env "${FONTPUB_ENV[@]}" go run ./cmd/fontpub ls-remote --json)
echo

echo "== fontpub show ${LOWER_PACKAGE_ID} --json =="
(cd "${GO_DIR}" && env "${FONTPUB_ENV[@]}" go run ./cmd/fontpub show "${LOWER_PACKAGE_ID}" --json)
echo

echo "== fontpub install ${LOWER_PACKAGE_ID} --activate --json =="
(cd "${GO_DIR}" && env "${FONTPUB_ENV[@]}" go run ./cmd/fontpub install "${LOWER_PACKAGE_ID}" --activate --json)
echo

echo "== fontpub ls ${LOWER_PACKAGE_ID} --json =="
(cd "${GO_DIR}" && env "${FONTPUB_ENV[@]}" go run ./cmd/fontpub ls "${LOWER_PACKAGE_ID}" --json)
echo

echo "== fontpub verify ${LOWER_PACKAGE_ID} --json =="
(cd "${GO_DIR}" && env "${FONTPUB_ENV[@]}" go run ./cmd/fontpub verify "${LOWER_PACKAGE_ID}" --json)
echo

echo "Artifacts: ${ARTIFACTS_DIR}"
echo "State: ${STATE_DIR}"
echo "Activation: ${ACTIVATION_DIR}"
echo "Logs: ${LOG_DIR}"
if [[ "$KEEP" == "1" ]]; then
  echo "Temporary files were kept."
fi
