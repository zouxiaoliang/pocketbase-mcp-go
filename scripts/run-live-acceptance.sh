#!/usr/bin/env bash
# Run live acceptance tests against a real PocketBase instance.
# Downloads PocketBase if needed, starts a temporary instance, runs integration tests, then cleans up.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LOCAL_DIR="$ROOT/.local"
PB_VERSION="${PB_VERSION:-0.39.3}"
PB_BIN="$LOCAL_DIR/pocketbase"
PB_DATA="$LOCAL_DIR/pb_data"
PB_ADDR="127.0.0.1:18090"
PB_URL="http://${PB_ADDR}"
ADMIN_EMAIL="${POCKETBASE_ADMIN_EMAIL:-admin@example.com}"
ADMIN_PASSWORD="${POCKETBASE_ADMIN_PASSWORD:-testpassword123}"
MCP_ADDR="127.0.0.1:18080"
PB_PID=""
MCP_PID=""

cleanup() {
  if [[ -n "${MCP_PID}" ]] && kill -0 "${MCP_PID}" 2>/dev/null; then
    kill "${MCP_PID}" 2>/dev/null || true
    wait "${MCP_PID}" 2>/dev/null || true
  fi
  if [[ -n "${PB_PID}" ]] && kill -0 "${PB_PID}" 2>/dev/null; then
    kill "${PB_PID}" 2>/dev/null || true
    wait "${PB_PID}" 2>/dev/null || true
  fi
}
trap cleanup EXIT

log() { printf '==> %s\n' "$*"; }

detect_platform() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"
  case "${arch}" in
    x86_64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) echo "unsupported arch: ${arch}" >&2; exit 1 ;;
  esac
  case "${os}" in
    darwin|linux) ;;
    *) echo "unsupported os: ${os}" >&2; exit 1 ;;
  esac
  echo "${os}_${arch}"
}

download_pocketbase() {
  if [[ -x "${PB_BIN}" ]]; then
    log "PocketBase binary already present at ${PB_BIN}"
    return
  fi
  mkdir -p "${LOCAL_DIR}"
  local platform archive url tmp
  platform="$(detect_platform)"
  archive="pocketbase_${PB_VERSION}_${platform}.zip"
  url="https://github.com/pocketbase/pocketbase/releases/download/v${PB_VERSION}/${archive}"
  tmp="$(mktemp -d)"
  log "Downloading PocketBase v${PB_VERSION} (${platform})"
  curl -fsSL "${url}" -o "${tmp}/${archive}"
  unzip -q "${tmp}/${archive}" -d "${tmp}"
  mv "${tmp}/pocketbase" "${PB_BIN}"
  chmod +x "${PB_BIN}"
  rm -rf "${tmp}"
}

wait_for_pb() {
  local i
  for i in $(seq 1 60); do
    if curl -fsS "${PB_URL}/api/health" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  echo "PocketBase failed to start at ${PB_URL}" >&2
  return 1
}

start_pocketbase() {
  if curl -fsS "${PB_URL}/api/health" >/dev/null 2>&1; then
    log "Using existing PocketBase at ${PB_URL}"
    PB_URL="${POCKETBASE_URL:-${PB_URL}}"
    return
  fi

  rm -rf "${PB_DATA}"
  mkdir -p "${PB_DATA}"
  log "Starting PocketBase on ${PB_URL}"
  "${PB_BIN}" serve --http="${PB_ADDR}" --dir="${PB_DATA}" >/dev/null 2>&1 &
  PB_PID=$!
  wait_for_pb

  log "Creating superuser ${ADMIN_EMAIL}"
  "${PB_BIN}" superuser upsert "${ADMIN_EMAIL}" "${ADMIN_PASSWORD}" --dir="${PB_DATA}" >/dev/null
}

verify_http_mode() {
  log "Verifying Gin HTTP /health endpoint"
  cd "${ROOT}"
  export POCKETBASE_URL="${PB_URL}"
  export POCKETBASE_ADMIN_EMAIL="${ADMIN_EMAIL}"
  export POCKETBASE_ADMIN_PASSWORD="${ADMIN_PASSWORD}"
  export MCP_HTTP_ADDR="${MCP_ADDR}"
  go build -o "${LOCAL_DIR}/pocketbase-mcp-go" .
  "${LOCAL_DIR}/pocketbase-mcp-go" -transport=http -http-addr="${MCP_ADDR}" >/dev/null 2>&1 &
  MCP_PID=$!
  for i in $(seq 1 40); do
    if resp="$(curl -fsS "http://${MCP_ADDR}/health" 2>/dev/null)" && printf '%s' "${resp}" | grep -q '"status":"ok"'; then
      log "HTTP health check passed (http://${MCP_ADDR}/health)"
      return 0
    fi
    sleep 0.3
  done
  echo "MCP HTTP health check failed" >&2
  return 1
}

run_integration_tests() {
  log "Running integration tests"
  (
    cd "${ROOT}"
    export POCKETBASE_URL="${PB_URL}"
    export POCKETBASE_ADMIN_EMAIL="${ADMIN_EMAIL}"
    export POCKETBASE_ADMIN_PASSWORD="${ADMIN_PASSWORD}"
    go test -tags=integration -count=1 -v -run 'TestLive' .
  )
}

main() {
  case "${1:-}" in
    --download-only)
      download_pocketbase
      exit 0
      ;;
  esac

  download_pocketbase
  start_pocketbase
  run_integration_tests
  verify_http_mode
  log "Live acceptance completed successfully"
}

main "$@"
