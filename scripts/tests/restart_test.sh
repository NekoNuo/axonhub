#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

fail() {
  printf 'not ok - %s\n' "$1" >&2
  exit 1
}

pass() {
  printf 'ok - %s\n' "$1"
}

assert_file_contains() {
  local file="$1"
  local expected="$2"

  [[ -f "${file}" ]] || fail "missing file: ${file}"
  local actual
  actual="$(cat "${file}")"
  [[ "${actual}" == "${expected}" ]] || fail "expected ${file} to contain '${expected}', got '${actual}'"
}

assert_file_includes() {
  local file="$1"
  local expected="$2"

  [[ -f "${file}" ]] || fail "missing file: ${file}"
  grep -Fq "${expected}" "${file}" || fail "expected ${file} to include '${expected}'"
}

assert_file_excludes() {
  local file="$1"
  local unexpected="$2"

  [[ -f "${file}" ]] || return 0
  if grep -Fq "${unexpected}" "${file}"; then
    fail "expected ${file} to exclude '${unexpected}'"
  fi
}

assert_file_exists() {
  local file="$1"
  [[ -f "${file}" ]] || fail "expected file to exist: ${file}"
}

assert_file_missing() {
  local file="$1"
  [[ ! -e "${file}" ]] || fail "expected file to be absent: ${file}"
}

assert_glob_count() {
  local pattern="$1"
  local expected="$2"
  local count

  shopt -s nullglob
  local matches=( ${pattern} )
  shopt -u nullglob
  count="${#matches[@]}"
  [[ "${count}" == "${expected}" ]] || fail "expected ${expected} matches for ${pattern}, got ${count}"
}

make_fake_root() {
  local root
  root="$(mktemp -d)"
  mkdir -p "${root}/scripts"

  cp "${REPO_ROOT}/scripts/restart.sh" "${root}/scripts/restart.sh"
  chmod +x "${root}/scripts/restart.sh"

  cat > "${root}/scripts/stop.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
printf 'stop %s\n' "${1:-}" >> "${ROOT_DIR}/calls.log"
rm -f "${ROOT_DIR}/axonhub.pid"
EOF
  chmod +x "${root}/scripts/stop.sh"

  cat > "${root}/scripts/build_and_start.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
printf 'build_and_start %s %s\n' "${1:-}" "${AXONHUB_BUILD_OUTPUT:-}" >> "${ROOT_DIR}/calls.log"

if [[ "${1:-}" == "--build-only" ]]; then
  if [[ -f "${ROOT_DIR}/fail-build" ]]; then
    exit 1
  fi

  printf '%s' "${BUILD_ARTIFACT_CONTENT:-new}" > "${AXONHUB_BUILD_OUTPUT}"
  chmod +x "${AXONHUB_BUILD_OUTPUT}"
  exit 0
fi

if [[ "${1:-}" == "--skip-build" ]]; then
  local_binary="$(cat "${ROOT_DIR}/axonhub")"
  if [[ "${local_binary}" == "new" ]] && [[ -f "${ROOT_DIR}/fail-new-start-once" ]]; then
    rm -f "${ROOT_DIR}/fail-new-start-once"
    exit 1
  fi

  printf '4321' > "${ROOT_DIR}/axonhub.pid"
  exit 0
fi

exit 1
EOF
  chmod +x "${root}/scripts/build_and_start.sh"

  printf '%s\n' "${root}"
}

test_build_failure_keeps_current_process_running() {
  local root
  root="$(make_fake_root)"

  printf 'old' > "${root}/axonhub"
  printf '1234' > "${root}/axonhub.pid"
  touch "${root}/fail-build"

  if "${root}/scripts/restart.sh" >"${root}/stdout.log" 2>"${root}/stderr.log"; then
    fail "restart should fail when build fails"
  fi

  assert_file_contains "${root}/axonhub" "old"
  assert_file_contains "${root}/axonhub.pid" "1234"
  assert_file_includes "${root}/calls.log" "build_and_start --build-only"
  assert_file_excludes "${root}/calls.log" "stop "
  pass "build failure does not stop current process"
}

test_start_failure_restores_backup_and_restarts_old_binary() {
  local root
  root="$(make_fake_root)"

  printf 'old' > "${root}/axonhub"
  chmod +x "${root}/axonhub"
  printf '1234' > "${root}/axonhub.pid"
  touch "${root}/fail-new-start-once"

  "${root}/scripts/restart.sh" >"${root}/stdout.log" 2>"${root}/stderr.log" || fail "restart should roll back and succeed"

  assert_file_contains "${root}/axonhub" "old"
  assert_file_contains "${root}/axonhub.pid" "4321"
  assert_glob_count "${root}/axonhub.backup.*" "1"
  assert_file_includes "${root}/calls.log" "build_and_start --build-only"
  assert_file_includes "${root}/calls.log" "stop "
  assert_file_includes "${root}/calls.log" "build_and_start --skip-build"
  pass "start failure rolls back to previous binary"
}

main() {
  test_build_failure_keeps_current_process_running
  test_start_failure_restores_backup_and_restarts_old_binary
}

main "$@"
