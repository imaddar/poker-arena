#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEB_DIR="${ROOT_DIR}/frontend/web"

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${ADMIN_TOKEN:-local-admin-token}"
CORS_ORIGIN="${CORS_ORIGIN:-http://localhost:5173}"
TABLE_NAME="${TABLE_NAME:-web-smoke-table}"

LAST_STEP=""
LAST_BODY=""
LAST_CODE=""

log() {
  printf '[web-smoke] %s\n' "$*"
}

fail() {
  local message="${1:-unknown failure}"
  printf '\n[web-smoke][FAIL] step=%s msg=%s\n' "${LAST_STEP}" "${message}" >&2
  if [[ -n "${LAST_CODE}" ]]; then
    printf '[web-smoke][FAIL] last_http_code=%s\n' "${LAST_CODE}" >&2
  fi
  if [[ -n "${LAST_BODY}" ]]; then
    printf '[web-smoke][FAIL] last_http_body=%s\n' "${LAST_BODY}" >&2
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

jq_get() {
  local expr="$1"
  printf '%s' "${LAST_BODY}" | jq -er "${expr}" 2>/dev/null || fail "jq parse failed: ${expr}"
}

main() {
  require_cmd curl
  require_cmd jq
  require_cmd npm

  LAST_STEP="reachability_check"
  curl -sS -o /dev/null "${BASE_URL}/unknown" || fail "control-plane is not reachable at ${BASE_URL}"
  log "backend reachable at ${BASE_URL}"

  LAST_STEP="cors_preflight"
  local cors_headers
  cors_headers="$(
    curl -sS -D - -o /dev/null \
      -X OPTIONS "${BASE_URL}/tables" \
      -H "Origin: ${CORS_ORIGIN}" \
      -H "Access-Control-Request-Method: GET"
  )" || fail "cors preflight request failed"
  if printf '%s' "${cors_headers}" | grep -qi "HTTP/.* 204"; then
    printf '%s' "${cors_headers}" | grep -qi "Access-Control-Allow-Origin: ${CORS_ORIGIN}" || fail "missing Access-Control-Allow-Origin for ${CORS_ORIGIN}"
    log "cors preflight passed for ${CORS_ORIGIN}"
  else
    fail "cors preflight did not return 204; ensure CONTROLPLANE_CORS_ALLOWED_ORIGINS includes ${CORS_ORIGIN}"
  fi

  LAST_STEP="tables_auth_boundary"
  api GET "/tables"
  expect_code 401
  api GET "/tables" "${ADMIN_TOKEN}"
  expect_code 200
  log "auth boundary passed for GET /tables"

  LAST_STEP="ensure_table"
  local table_id
  table_id="$(jq_get 'if length > 0 then .[0].id else empty end' || true)"
  if [[ -z "${table_id}" ]]; then
    api POST "/tables" "${ADMIN_TOKEN}" "{\"name\":\"${TABLE_NAME}\",\"max_seats\":6,\"small_blind\":50,\"big_blind\":100}"
    expect_code 200
    table_id="$(jq_get '.id')"
    log "created table ${table_id}"
  else
    log "using existing table ${table_id}"
  fi

  LAST_STEP="table_state"
  api GET "/tables/${table_id}/state" "${ADMIN_TOKEN}"
  expect_code 200
  jq_get '.table.id' >/dev/null
  log "table state endpoint passed for ${table_id}"

  LAST_STEP="history_routes"
  api GET "/tables/${table_id}/hands" "${ADMIN_TOKEN}"
  if [[ "${LAST_CODE}" == "404" ]]; then
    log "hands endpoint returned 404 (no run history yet) for ${table_id}"
  else
    expect_code 200
    local hands_len
    hands_len="$(jq_get 'length')"
    if [[ "${hands_len}" -gt 0 ]]; then
      local hand_id
      hand_id="$(jq_get '.[0].hand_id')"
      api GET "/hands/${hand_id}/actions" "${ADMIN_TOKEN}"
      expect_code 200
      log "history endpoints passed for hand ${hand_id}"
    else
      log "hands endpoint returned empty history for ${table_id}"
    fi
  fi

  LAST_STEP="latest_replay_route"
  api GET "/tables/${table_id}/replay/latest" "${ADMIN_TOKEN}"
  expect_code 200
  jq_get '.table.id' >/dev/null
  local latest_hand_id
  latest_hand_id="$(printf '%s' "${LAST_BODY}" | jq -r '.latest_hand.hand_id // empty')"
  if [[ -n "${latest_hand_id}" ]]; then
    printf '%s' "${LAST_BODY}" | jq -e '.replay.actions | type == "array"' >/dev/null || fail "expected replay.actions array when latest_hand exists"
    log "latest replay endpoint passed for table ${table_id} hand ${latest_hand_id}"
  else
    log "latest replay endpoint passed for table ${table_id} (no hand history yet)"
  fi

  LAST_STEP="frontend_tests"
  npm --prefix "${WEB_DIR}" run test:logic
  LAST_STEP="frontend_build"
  npm --prefix "${WEB_DIR}" run build

  log "PASS: web smoke checks completed"
}

main "$@"
