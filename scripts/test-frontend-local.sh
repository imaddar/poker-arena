#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEB_DIR="${ROOT_DIR}/frontend/web"

log() {
  printf '[frontend-test] %s\n' "$*"
}

require_cmd() {
  local c="$1"
  command -v "${c}" >/dev/null 2>&1 || {
    printf '[frontend-test][FAIL] missing command: %s\n' "${c}" >&2
    exit 1
  }
}

main() {
  require_cmd npm

  log "running logic tests"
  npm --prefix "${WEB_DIR}" run test:logic

  log "running production build"
  npm --prefix "${WEB_DIR}" run build

  log "PASS: frontend checks completed"
}

main "$@"
