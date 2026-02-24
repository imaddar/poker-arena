#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${ADMIN_TOKEN:-local-admin-token}"
SEAT_TOKEN="${SEAT_TOKEN:-local-seat-1-token}"
TABLE_NAME="${TABLE_NAME:-backend-auth-smoke}"

LAST_STEP=""
LAST_BODY=""
LAST_CODE=""

log() {
  printf '[backend-auth] %s\n' "$*"
}

fail() {
  local message="${1:-unknown failure}"
  printf '\n[backend-auth][FAIL] step=%s msg=%s\n' "${LAST_STEP}" "${message}" >&2
  if [[ -n "${LAST_CODE}" ]]; then
    printf '[backend-auth][FAIL] last_http_code=%s\n' "${LAST_CODE}" >&2
  fi
  if [[ -n "${LAST_BODY}" ]]; then
    printf '[backend-auth][FAIL] last_http_body=%s\n' "${LAST_BODY}" >&2
  fi
  exit 1
}

require_cmd() {
  local c="$1"
  command -v "${c}" >/dev/null 2>&1 || fail "missing required command: ${c}"
}

api() {
  local method="$1"
  local path="$2"
  local token="${3:-}"
  local body="${4:-}"

  local curl_args=(
    -sS
    -X "${method}"
    "${BASE_URL}${path}"
    -H "Content-Type: application/json"
    -w $'\n%{http_code}'
  )

  if [[ -n "${token}" ]]; then
    curl_args+=(-H "Authorization: Bearer ${token}")
  fi
  if [[ -n "${body}" ]]; then
    curl_args+=(-d "${body}")
  fi

  local out
  out="$(curl "${curl_args[@]}")" || fail "curl failed for ${method} ${path}"
  LAST_BODY="$(printf '%s' "${out}" | sed '$d')"
  LAST_CODE="$(printf '%s' "${out}" | tail -n1)"
}

expect_code() {
  local want="$1"
  [[ "${LAST_CODE}" == "${want}" ]] || fail "expected HTTP ${want}, got ${LAST_CODE}"
}

jq_body() {
  local expr="$1"
  printf '%s' "${LAST_BODY}" | jq -er "${expr}" 2>/dev/null || fail "jq parse failed: ${expr}"
}

main() {
  require_cmd curl
  require_cmd jq

  LAST_STEP="reachability"
  curl -sS -o /dev/null "${BASE_URL}/unknown" || fail "backend unreachable at ${BASE_URL}"
  log "backend reachable at ${BASE_URL}"

  LAST_STEP="session_admin"
  api GET "/session" "${ADMIN_TOKEN}"
  expect_code 200
  jq_body '.role == "admin"' >/dev/null
  log "admin /session check passed"

  LAST_STEP="session_seat"
  api GET "/session" "${SEAT_TOKEN}"
  expect_code 200
  jq_body '.role == "seat"' >/dev/null
  log "seat /session check passed"

  LAST_STEP="tables_unauth"
  api GET "/tables"
  expect_code 401

  LAST_STEP="tables_admin"
  api GET "/tables" "${ADMIN_TOKEN}"
  expect_code 200
  log "admin tables list passed"

  LAST_STEP="tables_seat_forbidden"
  api GET "/tables" "${SEAT_TOKEN}"
  expect_code 403
  log "seat tables access correctly forbidden"

  LAST_STEP="ensure_table"
  local table_id
  table_id="$(jq_body 'if length > 0 then .[0].id else empty end' || true)"
  if [[ -z "${table_id}" ]]; then
    api POST "/tables" "${ADMIN_TOKEN}" "{\"name\":\"${TABLE_NAME}\",\"max_seats\":6,\"small_blind\":50,\"big_blind\":100}"
    expect_code 200
    table_id="$(jq_body '.id')"
    log "created table ${table_id}"
  else
    log "using table ${table_id}"
  fi

  LAST_STEP="table_state_admin"
  api GET "/tables/${table_id}/state" "${ADMIN_TOKEN}"
  expect_code 200
  jq_body '.table.id' >/dev/null

  LAST_STEP="latest_replay_admin"
  api GET "/tables/${table_id}/replay/latest" "${ADMIN_TOKEN}"
  expect_code 200
  jq_body '.table.id' >/dev/null

  LAST_STEP="latest_replay_seat"
  api GET "/tables/${table_id}/replay/latest" "${SEAT_TOKEN}"
  expect_code 200
  jq_body '.table.id' >/dev/null

  log "PASS: backend auth/replay smoke checks completed"
}

main "$@"
