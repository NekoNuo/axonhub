#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FRONTEND_DIR="${ROOT_DIR}/frontend"
LOG_FILE="${ROOT_DIR}/axonhub.log"
PID_FILE="${ROOT_DIR}/axonhub.pid"
PORT="${AXONHUB_SERVER_PORT:-}"

info() {
  printf '[INFO] %s\n' "$1"
}

warn() {
  printf '[WARN] %s\n' "$1"
}

fail() {
  printf '[ERROR] %s\n' "$1" >&2
  exit 1
}

ensure_macos_sdk() {
  if [[ "$(uname -s)" != "Darwin" ]]; then
    return
  fi

  if ! command -v xcrun >/dev/null 2>&1; then
    fail "xcrun not found. Install Xcode Command Line Tools first."
  fi

  export SDKROOT
  SDKROOT="$(xcrun --sdk macosx --show-sdk-path)"
  export CGO_CFLAGS="-isysroot ${SDKROOT}"
  export CGO_CPPFLAGS="-isysroot ${SDKROOT}"
  export CGO_LDFLAGS="-isysroot ${SDKROOT}"

  info "Using SDKROOT=${SDKROOT}"
}

ensure_frontend_deps() {
  if [[ -x "${FRONTEND_DIR}/node_modules/.bin/vite" ]]; then
    return
  fi

  info "vite not found in frontend dependencies, running pnpm install"
  (
    cd "${FRONTEND_DIR}"
    CI=true pnpm install
  )
}

check_running() {
  if [[ -f "${PID_FILE}" ]]; then
    local pid
    pid="$(cat "${PID_FILE}")"
    if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null; then
      fail "axonhub is already running with PID ${pid}. Stop it first or remove ${PID_FILE} if it is stale."
    fi
    warn "Removing stale PID file ${PID_FILE}"
    rm -f "${PID_FILE}"
  fi
}

build_project() {
  info "Building project"
  (
    cd "${ROOT_DIR}"
    make build
  )
}

start_server() {
  if [[ -n "${PORT}" ]]; then
    export AXONHUB_SERVER_PORT="${PORT}"
    info "Starting axonhub on port ${AXONHUB_SERVER_PORT}"
  else
    info "Starting axonhub"
  fi

  (
    cd "${ROOT_DIR}"
    nohup ./axonhub >> axonhub.log 2>&1 &
    echo $! > "${PID_FILE}"
  )

  sleep 2

  local pid
  pid="$(cat "${PID_FILE}")"
  if ! kill -0 "${pid}" 2>/dev/null; then
    tail -n 50 "${LOG_FILE}" 2>/dev/null || true
    fail "axonhub failed to start"
  fi

  info "axonhub started with PID ${pid}"
  info "Log file: ${LOG_FILE}"
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --port)
        [[ $# -ge 2 ]] || fail "Missing value for --port"
        [[ "$2" =~ ^[0-9]+$ ]] || fail "Port must be a number"
        PORT="$2"
        shift 2
        ;;
      --help|-h)
        cat <<'EOF'
Usage: ./scripts/build_and_start.sh [--port <port>]

Examples:
  ./scripts/build_and_start.sh
  ./scripts/build_and_start.sh --port 8091
  AXONHUB_SERVER_PORT=8092 ./scripts/build_and_start.sh
EOF
        exit 0
        ;;
      *)
        fail "Unknown argument: $1"
        ;;
    esac
  done
}

main() {
  parse_args "$@"
  ensure_macos_sdk
  ensure_frontend_deps
  check_running
  build_project
  start_server
}

main "$@"
