#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ -f "${ROOT_DIR}/axonhub.pid" ]]; then
  "${ROOT_DIR}/scripts/stop.sh" "${1:-}"
else
  printf '[INFO] axonhub is not running, skipping stop step\n'
fi

"${ROOT_DIR}/scripts/build_and_start.sh"
