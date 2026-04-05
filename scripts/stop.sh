#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PID_FILE="${ROOT_DIR}/axonhub.pid"
BINARY_PATH="${ROOT_DIR}/axonhub"

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

read_pid() {
  if [[ ! -f "${PID_FILE}" ]]; then
    fail "PID file not found: ${PID_FILE}"
  fi

  local pid
  pid="$(cat "${PID_FILE}")"
  if [[ ! "${pid}" =~ ^[0-9]+$ ]]; then
    rm -f "${PID_FILE}"
    fail "Invalid PID file content"
  fi

  printf '%s\n' "${pid}"
}

is_target_process() {
  local pid="$1"

  if ! command -v lsof >/dev/null 2>&1; then
    return 1
  fi

  local exe_path
  exe_path="$(lsof -a -p "${pid}" -d txt -Fn 2>/dev/null | sed -n 's/^n//p' | head -n 1)"
  [[ "${exe_path}" == "${BINARY_PATH}" ]]
}

wait_for_exit() {
  local pid="$1"

  if ! kill -0 "${pid}" 2>/dev/null; then
    return 0
  fi

  for _ in $(seq 1 10); do
    if ! kill -0 "${pid}" 2>/dev/null; then
      return 0
    fi
    sleep 1
  done

  return 1
}

stop_pid() {
  local pid="$1"

  if ! kill -0 "${pid}" 2>/dev/null; then
    warn "Process ${pid} is not running"
    return 0
  fi

  info "Stopping axonhub PID ${pid}"
  kill -TERM "${pid}"

  if wait_for_exit "${pid}"; then
    info "axonhub PID ${pid} stopped"
    return 0
  fi

  return 1
}

force_stop_pid() {
  local pid="$1"

  if ! kill -0 "${pid}" 2>/dev/null; then
    warn "Process ${pid} is not running"
    return 0
  fi

  warn "Force killing axonhub PID ${pid}"
  kill -KILL "${pid}"
  sleep 1
  info "axonhub PID ${pid} force stopped"
}

stop_residual_processes() {
  local force="${1:-}"
  local found=0
  local pids

  pids="$(pgrep -x axonhub 2>/dev/null || true)"
  if [[ -z "${pids}" ]]; then
    return 0
  fi

  for pid in ${pids}; do
    if ! is_target_process "${pid}"; then
      continue
    fi

    found=1
    if [[ "${force}" == "--force" ]]; then
      force_stop_pid "${pid}"
      continue
    fi

    if ! stop_pid "${pid}"; then
      warn "PID ${pid} did not stop gracefully, forcing kill"
      force_stop_pid "${pid}"
    fi
  done

  if [[ "${found}" -eq 1 ]]; then
    info "Residual axonhub processes under ${ROOT_DIR} have been cleaned up"
  fi
}

main() {
  local force="${1:-}"
  local pid=""

  if [[ -f "${PID_FILE}" ]]; then
    pid="$(read_pid)"

    if [[ "${force}" == "--force" ]]; then
      force_stop_pid "${pid}"
    else
      if ! stop_pid "${pid}"; then
        fail "axonhub did not stop within timeout, run './scripts/stop.sh --force'"
      fi
    fi
  else
    warn "PID file not found: ${PID_FILE}"
  fi

  rm -f "${PID_FILE}"
  stop_residual_processes "${force}"
  info "axonhub stop routine completed"
}

main "${1:-}"
