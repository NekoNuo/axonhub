#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CURRENT_BINARY="${ROOT_DIR}/axonhub"
PID_FILE="${ROOT_DIR}/axonhub.pid"
BUILD_AND_START_SCRIPT="${ROOT_DIR}/scripts/build_and_start.sh"
STOP_SCRIPT="${ROOT_DIR}/scripts/stop.sh"
TMP_DIR=""
CANDIDATE_BINARY=""
BACKUP_BINARY=""
STOP_ARGS=()
START_ARGS=()

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

cleanup() {
  if [[ -n "${TMP_DIR}" && -d "${TMP_DIR}" ]]; then
    rm -rf "${TMP_DIR}"
  fi
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --force)
        STOP_ARGS+=("$1")
        shift
        ;;
      *)
        START_ARGS+=("$1")
        shift
        ;;
    esac
  done
}

build_candidate() {
  TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/axonhub-restart.XXXXXX")"
  CANDIDATE_BINARY="${TMP_DIR}/axonhub"

  info "Building candidate binary"
  if [[ "${#START_ARGS[@]}" -gt 0 ]]; then
    if ! AXONHUB_BUILD_OUTPUT="${CANDIDATE_BINARY}" "${BUILD_AND_START_SCRIPT}" --build-only "${START_ARGS[@]}"; then
      fail "Build failed, keeping current process running"
    fi
    return 0
  fi

  if ! AXONHUB_BUILD_OUTPUT="${CANDIDATE_BINARY}" "${BUILD_AND_START_SCRIPT}" --build-only; then
    fail "Build failed, keeping current process running"
  fi
}

backup_current_binary() {
  if [[ ! -f "${CURRENT_BINARY}" ]]; then
    warn "Current binary not found at ${CURRENT_BINARY}, skipping backup"
    return 0
  fi

  BACKUP_BINARY="${ROOT_DIR}/axonhub.backup.$(date +%Y%m%d%H%M%S)"
  cp -p "${CURRENT_BINARY}" "${BACKUP_BINARY}"
  info "Backed up current binary to ${BACKUP_BINARY}"
}

stop_current_process() {
  if [[ -f "${PID_FILE}" ]]; then
    if [[ "${#STOP_ARGS[@]}" -gt 0 ]]; then
      "${STOP_SCRIPT}" "${STOP_ARGS[@]}"
    else
      "${STOP_SCRIPT}"
    fi
    return 0
  fi

  info "axonhub is not running, skipping stop step"
}

activate_candidate() {
  mv "${CANDIDATE_BINARY}" "${CURRENT_BINARY}"
  chmod +x "${CURRENT_BINARY}"
  info "Activated new binary"
}

start_current_binary() {
  if [[ "${#START_ARGS[@]}" -gt 0 ]]; then
    "${BUILD_AND_START_SCRIPT}" --skip-build "${START_ARGS[@]}"
    return
  fi

  "${BUILD_AND_START_SCRIPT}" --skip-build
}

rollback() {
  if [[ -z "${BACKUP_BINARY}" || ! -f "${BACKUP_BINARY}" ]]; then
    fail "New binary failed to start and no backup is available to roll back"
  fi

  warn "New binary failed to start, rolling back to ${BACKUP_BINARY}"
  cp -p "${BACKUP_BINARY}" "${CURRENT_BINARY}"

  if ! start_current_binary; then
    fail "Rollback binary failed to start; manual intervention required"
  fi

  info "Rollback completed successfully"
}

main() {
  trap cleanup EXIT
  parse_args "$@"
  build_candidate
  backup_current_binary
  stop_current_process
  activate_candidate

  if ! start_current_binary; then
    rollback
  fi
}

main "$@"
