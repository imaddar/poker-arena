#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${ADMIN_TOKEN:-local-admin-token}"
SEAT_TOKEN="${SEAT_TOKEN:-local-seat-1-token}"

log() {
  printf '[test-all] %s\n' "$*"
}

main() {
  log "running backend auth checks"
  BASE_URL="${BASE_URL}" ADMIN_TOKEN="${ADMIN_TOKEN}" SEAT_TOKEN="${SEAT_TOKEN}" \
    "${ROOT_DIR}/scripts/test-backend-auth-local.sh"

  log "running frontend checks"
  "${ROOT_DIR}/scripts/test-frontend-local.sh"

  log "running full web smoke"
  BASE_URL="${BASE_URL}" ADMIN_TOKEN="${ADMIN_TOKEN}" SEAT_TOKEN="${SEAT_TOKEN}" \
    "${ROOT_DIR}/scripts/web-smoke-local.sh"

  log "PASS: all local checks completed"
}

main "$@"
