#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENGINE_DIR="${ROOT_DIR}/services/engine"

BASE_URL="${BASE_URL:-http://127.0.0.1:8081}"
CONTROL_ADDR="${CONTROL_ADDR:-:8081}"
FRONTEND_URL="${FRONTEND_URL:-http://localhost:5173}"

ADMIN_TOKEN="${ADMIN_TOKEN:-local-admin-token}"
SEAT1_TOKEN="${SEAT1_TOKEN:-local-seat-1-token}"
SEAT2_TOKEN="${SEAT2_TOKEN:-local-seat-2-token}"

CONTROLPLANE_CORS_ALLOWED_ORIGINS="${CONTROLPLANE_CORS_ALLOWED_ORIGINS:-http://localhost:5173,http://localhost:5174}"
DATABASE_URL="${DATABASE_URL:-postgres://postgres:postgres@127.0.0.1:5432/poker_arena?sslmode=disable}"

HANDS_TO_RUN="${HANDS_TO_RUN:-500}"
ADMIN_SEAT_NO="${ADMIN_SEAT_NO:-3}"
MAX_SEATS="${MAX_SEATS:-6}"
SMALL_BLIND="${SMALL_BLIND:-50}"
BIG_BLIND="${BIG_BLIND:-100}"
STACK="${STACK:-10000}"

START_MOCK_AGENTS="${START_MOCK_AGENTS:-1}"
START_CONTROL_PLANE="${START_CONTROL_PLANE:-auto}"
KEEP_ALIVE="${KEEP_ALIVE:-1}"
POLL_TIMEOUT_SEC="${POLL_TIMEOUT_SEC:-20}"

TMP_DIR="$(mktemp -d)"
CONTROL_LOG="${TMP_DIR}/controlplane.log"
AGENT_A_LOG="${TMP_DIR}/agent-a.log"
AGENT_B_LOG="${TMP_DIR}/agent-b.log"

PID_CONTROL=""
PID_AGENT_A=""
PID_AGENT_B=""
STARTED_CONTROL=0
STARTED_AGENTS=0
LAST_STEP=""
LAST_BODY=""
LAST_CODE=""
TABLE_ID=""

log() {
  printf '[play-local] %s\n' "$*"
}

fail() {
  local message="${1:-unknown failure}"
  printf '\n[play-local][FAIL] step=%s msg=%s\n' "${LAST_STEP}" "${message}" >&2
  if [[ -n "${LAST_CODE}" ]]; then
    printf '[play-local][FAIL] last_http_code=%s\n' "${LAST_CODE}" >&2
  fi
  if [[ -n "${LAST_BODY}" ]]; then
    printf '[play-local][FAIL] last_http_body=%s\n' "${LAST_BODY}" >&2
  fi
  printf '[play-local][FAIL] logs: %s\n' "${TMP_DIR}" >&2
  tail -n 40 "${CONTROL_LOG}" 2>/dev/null >&2 || true
  tail -n 20 "${AGENT_A_LOG}" 2>/dev/null >&2 || true
  tail -n 20 "${AGENT_B_LOG}" 2>/dev/null >&2 || true
  exit 1
}

cleanup() {
  local exit_code=$?
  [[ -n "${PID_CONTROL}" ]] && kill "${PID_CONTROL}" >/dev/null 2>&1 || true
  [[ -n "${PID_AGENT_A}" ]] && kill "${PID_AGENT_A}" >/dev/null 2>&1 || true
  [[ -n "${PID_AGENT_B}" ]] && kill "${PID_AGENT_B}" >/dev/null 2>&1 || true
  if [[ ${exit_code} -ne 0 ]]; then
    printf '[play-local] cleanup complete after failure. logs: %s\n' "${TMP_DIR}" >&2
  fi
}
trap cleanup EXIT

usage() {
  cat <<'USAGE'
Usage: ./scripts/play-vs-agents-local.sh

Creates a local table with:
- seat 1: mock agent A
- seat 2: mock agent B
- seat 3 (default): admin human seat

Then starts a long run so you can play from the frontend.

Environment overrides:
  BASE_URL (default: http://127.0.0.1:8081)
  FRONTEND_URL (default: http://localhost:5173)
  ADMIN_TOKEN (default: local-admin-token)
  START_MOCK_AGENTS=0|1 (default: 1)
  START_CONTROL_PLANE=auto|0|1 (default: auto)
  KEEP_ALIVE=0|1 (default: 1)
  HANDS_TO_RUN (default: 500)
  ADMIN_SEAT_NO (default: 3)
USAGE
}

require_cmd() {
  local c="$1"
  command -v "${c}" >/dev/null 2>&1 || fail "missing required command: ${c}"
}

api() {
  local method="$1"
  local path="$2"
  local token="$3"
  local body="${4:-}"

  local curl_args=(
    -sS
    -X "${method}"
    "${BASE_URL}${path}"
    -H "Authorization: Bearer ${token}"
    -H "Content-Type: application/json"
    -w $'\n%{http_code}'
  )
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

jq_get_id() {
  printf '%s' "${LAST_BODY}" | jq -er '.ID // .id' 2>/dev/null || fail "jq parse failed: .ID // .id"
}

backend_reachable() {
  curl -sS -o /dev/null --max-time 1 "${BASE_URL}/unknown" >/dev/null 2>&1
}

wait_for_backend() {
  LAST_STEP="wait_for_backend"
  local deadline=$(( $(date +%s) + POLL_TIMEOUT_SEC ))
  while [[ "$(date +%s)" -lt "${deadline}" ]]; do
    if backend_reachable; then
      return 0
    fi
    sleep 0.25
  done
  fail "control plane did not become reachable at ${BASE_URL}"
}

write_mock_agent() {
  cat >"${TMP_DIR}/mock-agent.go" <<'GOEOF'
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

type req struct {
	ToCall       uint32   `json:"to_call"`
	LegalActions []string `json:"legal_actions"`
}

func has(legal []string, action string) bool {
	for _, a := range legal {
		if a == action {
			return true
		}
	}
	return false
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9001"
	}
	name := os.Getenv("AGENT_NAME")
	if name == "" {
		name = "mock-agent"
	}

	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		var body req
		_ = json.NewDecoder(r.Body).Decode(&body)
		action := "fold"
		if body.ToCall > 0 && has(body.LegalActions, "call") {
			action = "call"
		} else if body.ToCall == 0 && has(body.LegalActions, "check") {
			action = "check"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"action": action})
	})

	log.Printf("%s listening on :%s", name, port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
GOEOF
}

start_mock_agents() {
  LAST_STEP="start_mock_agents"
  write_mock_agent

  PORT=9001 AGENT_NAME=agent-a go run "${TMP_DIR}/mock-agent.go" >"${AGENT_A_LOG}" 2>&1 &
  PID_AGENT_A=$!
  PORT=9002 AGENT_NAME=agent-b go run "${TMP_DIR}/mock-agent.go" >"${AGENT_B_LOG}" 2>&1 &
  PID_AGENT_B=$!

  STARTED_AGENTS=1
}

start_control_plane() {
  LAST_STEP="start_control_plane"
  (
    export CONTROLPLANE_ADMIN_TOKENS="${ADMIN_TOKEN}"
    export CONTROLPLANE_SEAT_TOKENS="1:${SEAT1_TOKEN},2:${SEAT2_TOKEN}"
    export AGENT_ENDPOINT_ALLOWLIST="127.0.0.1:9001,127.0.0.1:9002"
    export CONTROLPLANE_CORS_ALLOWED_ORIGINS="${CONTROLPLANE_CORS_ALLOWED_ORIGINS}"
    export DATABASE_URL="${DATABASE_URL}"

    go -C "${ENGINE_DIR}" run ./cmd/controlplane -addr "${CONTROL_ADDR}"
  ) >"${CONTROL_LOG}" 2>&1 &
  PID_CONTROL=$!
  STARTED_CONTROL=1
  wait_for_backend
}

main() {
  if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
    usage
    exit 0
  fi

  require_cmd go
  require_cmd curl
  require_cmd jq

  log "tmp dir: ${TMP_DIR}"

  LAST_STEP="prepare_services"
  if [[ "${START_MOCK_AGENTS}" == "1" ]]; then
    start_mock_agents
    log "started mock agents on ports 9001 and 9002"
  fi

  if backend_reachable; then
    log "reusing existing control-plane at ${BASE_URL}"
  else
    if [[ "${START_CONTROL_PLANE}" == "0" ]]; then
      fail "backend is not reachable at ${BASE_URL}; set START_CONTROL_PLANE=1 or start it manually"
    fi
    start_control_plane
    log "started control-plane at ${BASE_URL}"
  fi

  local suffix
  suffix="$(date +%s%N)"

  LAST_STEP="create_user"
  api POST "/users" "${ADMIN_TOKEN}" "{\"name\":\"play-user-${suffix}\",\"token\":\"play-user-token-${suffix}\"}"
  expect_code 200
  local user_id
  user_id="$(jq_get_id)"

  LAST_STEP="create_agents"
  api POST "/agents" "${ADMIN_TOKEN}" "{\"user_id\":\"${user_id}\",\"name\":\"play-agent-a-${suffix}\"}"
  expect_code 200
  local agent_a_id
  agent_a_id="$(jq_get_id)"

  api POST "/agents" "${ADMIN_TOKEN}" "{\"user_id\":\"${user_id}\",\"name\":\"play-agent-b-${suffix}\"}"
  expect_code 200
  local agent_b_id
  agent_b_id="$(jq_get_id)"

  LAST_STEP="create_versions"
  api POST "/agents/${agent_a_id}/versions" "${ADMIN_TOKEN}" '{"endpoint_url":"http://127.0.0.1:9001/callback"}'
  expect_code 200
  local version_a_id
  version_a_id="$(jq_get_id)"

  api POST "/agents/${agent_b_id}/versions" "${ADMIN_TOKEN}" '{"endpoint_url":"http://127.0.0.1:9002/callback"}'
  expect_code 200
  local version_b_id
  version_b_id="$(jq_get_id)"

  LAST_STEP="create_table"
  api POST "/tables" "${ADMIN_TOKEN}" "{\"name\":\"play-vs-agents-${suffix}\",\"max_seats\":${MAX_SEATS},\"small_blind\":${SMALL_BLIND},\"big_blind\":${BIG_BLIND}}"
  expect_code 200
  TABLE_ID="$(jq_get_id)"

  LAST_STEP="seat_agents"
  api POST "/tables/${TABLE_ID}/join" "${ADMIN_TOKEN}" "{\"seat_no\":1,\"agent_id\":\"${agent_a_id}\",\"agent_version_id\":\"${version_a_id}\",\"stack\":${STACK},\"status\":\"active\"}"
  expect_code 200

  api POST "/tables/${TABLE_ID}/join" "${ADMIN_TOKEN}" "{\"seat_no\":2,\"agent_id\":\"${agent_b_id}\",\"agent_version_id\":\"${version_b_id}\",\"stack\":${STACK},\"status\":\"active\"}"
  expect_code 200

  LAST_STEP="seat_admin"
  api POST "/tables/${TABLE_ID}/join-admin" "${ADMIN_TOKEN}" "{\"seat_no\":${ADMIN_SEAT_NO},\"stack\":${STACK},\"status\":\"active\"}"
  expect_code 200

  LAST_STEP="start_run"
  api POST "/tables/${TABLE_ID}/start" "${ADMIN_TOKEN}" "{\"hands_to_run\":${HANDS_TO_RUN}}"
  expect_code 200

  LAST_STEP="verify_status"
  api GET "/tables/${TABLE_ID}/status" "${ADMIN_TOKEN}"
  expect_code 200

  log "PASS: table created and running"
  log "table_id=${TABLE_ID}"
  log "frontend_game_url=${FRONTEND_URL}/game/${TABLE_ID}"
  log "admin_login_token=${ADMIN_TOKEN}"
  log "admin_seat=${ADMIN_SEAT_NO}"
  log "logs=${TMP_DIR}"
  printf '\n'
  printf 'Next steps:\n'
  printf '1) Open %s/game/%s\n' "${FRONTEND_URL}" "${TABLE_ID}"
  printf '2) Login with token: %s\n' "${ADMIN_TOKEN}"
  printf '3) In Admin Seat Controls, use seat %s and submit actions as they appear.\n' "${ADMIN_SEAT_NO}"

  if [[ "${KEEP_ALIVE}" == "1" && ( ${STARTED_CONTROL} -eq 1 || ${STARTED_AGENTS} -eq 1 ) ]]; then
    printf '\n'
    log "running local processes; press Ctrl+C to stop"
    while true; do
      sleep 2
    done
  fi
}

main "$@"
