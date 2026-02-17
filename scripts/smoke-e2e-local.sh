#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENGINE_DIR="${ROOT_DIR}/services/engine"

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
CONTROL_ADDR="${CONTROL_ADDR:-:8080}"
ADMIN_TOKEN="${ADMIN_TOKEN:-local-admin-token}"
SEAT1_TOKEN="${SEAT1_TOKEN:-local-seat-1-token}"
SEAT2_TOKEN="${SEAT2_TOKEN:-local-seat-2-token}"
DB_CONTAINER="${DB_CONTAINER:-poker-arena-postgres}"
DATABASE_URL="${DATABASE_URL:-postgres://postgres:postgres@127.0.0.1:5432/poker_arena?sslmode=disable}"
HANDS_TO_RUN="${HANDS_TO_RUN:-3}"
POLL_TIMEOUT_SEC="${POLL_TIMEOUT_SEC:-25}"
KEEP_PROCESSES="${KEEP_PROCESSES:-0}"

TMP_DIR="$(mktemp -d)"
CONTROL_LOG="${TMP_DIR}/controlplane.log"
AGENT_A_LOG="${TMP_DIR}/agent-a.log"
AGENT_B_LOG="${TMP_DIR}/agent-b.log"

PID_AGENT_A=""
PID_AGENT_B=""
PID_CONTROL=""
LAST_STEP=""
LAST_BODY=""
LAST_CODE=""
FAILED=0

log() {
  printf '[smoke] %s\n' "$*"
}

fail() {
  FAILED=1
  local message="${1:-unknown failure}"
  printf '\n[smoke][FAIL] step=%s msg=%s\n' "${LAST_STEP}" "${message}" >&2
  if [[ -n "${LAST_CODE}" ]]; then
    printf '[smoke][FAIL] last_http_code=%s\n' "${LAST_CODE}" >&2
  fi
  if [[ -n "${LAST_BODY}" ]]; then
    printf '[smoke][FAIL] last_http_body=%s\n' "${LAST_BODY}" >&2
  fi
  printf '[smoke][FAIL] logs:\n' >&2
  printf '  control-plane: %s\n' "${CONTROL_LOG}" >&2
  printf '  agent-a:       %s\n' "${AGENT_A_LOG}" >&2
  printf '  agent-b:       %s\n' "${AGENT_B_LOG}" >&2
  printf '[smoke][FAIL] control-plane tail:\n' >&2
  tail -n 40 "${CONTROL_LOG}" 2>/dev/null >&2 || true
  printf '[smoke][FAIL] agent-a tail:\n' >&2
  tail -n 20 "${AGENT_A_LOG}" 2>/dev/null >&2 || true
  printf '[smoke][FAIL] agent-b tail:\n' >&2
  tail -n 20 "${AGENT_B_LOG}" 2>/dev/null >&2 || true
  exit 1
}

cleanup() {
  local exit_code=$?
  if [[ "${KEEP_PROCESSES}" != "1" ]]; then
    [[ -n "${PID_CONTROL}" ]] && kill "${PID_CONTROL}" >/dev/null 2>&1 || true
    [[ -n "${PID_AGENT_A}" ]] && kill "${PID_AGENT_A}" >/dev/null 2>&1 || true
    [[ -n "${PID_AGENT_B}" ]] && kill "${PID_AGENT_B}" >/dev/null 2>&1 || true
  fi
  if [[ ${exit_code} -ne 0 && ${FAILED} -eq 0 ]]; then
    fail "script terminated unexpectedly"
  fi
}
trap cleanup EXIT

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

ensure_postgres() {
  LAST_STEP="ensure_postgres"
  if docker ps -a --format '{{.Names}}' | grep -qx "${DB_CONTAINER}"; then
    docker start "${DB_CONTAINER}" >/dev/null || fail "failed to start postgres container ${DB_CONTAINER}"
  else
    docker run --name "${DB_CONTAINER}" \
      -e POSTGRES_PASSWORD=postgres \
      -e POSTGRES_USER=postgres \
      -e POSTGRES_DB=poker_arena \
      -p 5432:5432 \
      -d postgres:16 >/dev/null || fail "failed to create postgres container ${DB_CONTAINER}"
  fi
}

write_mock_agents() {
  cat >"${TMP_DIR}/mock-agent-a.go" <<'EOF'
package main

import (
	"encoding/json"
	"log"
	"net/http"
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
	log.Println("mock agent A listening on :9001")
	log.Fatal(http.ListenAndServe(":9001", nil))
}
EOF

  cat >"${TMP_DIR}/mock-agent-b.go" <<'EOF'
package main

import (
	"encoding/json"
	"log"
	"net/http"
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
	log.Println("mock agent B listening on :9002")
	log.Fatal(http.ListenAndServe(":9002", nil))
}
EOF
}

wait_for_http() {
  LAST_STEP="wait_for_http"
  local attempts=60
  for _ in $(seq 1 "${attempts}"); do
    if curl -sS -o /dev/null "${BASE_URL}/unknown" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.25
  done
  fail "control plane did not become reachable at ${BASE_URL}"
}

main() {
  require_cmd go
  require_cmd jq
  require_cmd curl
  require_cmd docker

  log "tmp dir: ${TMP_DIR}"
  ensure_postgres

  LAST_STEP="start_mock_agents"
  write_mock_agents
  go run "${TMP_DIR}/mock-agent-a.go" >"${AGENT_A_LOG}" 2>&1 &
  PID_AGENT_A=$!
  go run "${TMP_DIR}/mock-agent-b.go" >"${AGENT_B_LOG}" 2>&1 &
  PID_AGENT_B=$!

  LAST_STEP="start_control_plane"
  (
    export CONTROLPLANE_ADMIN_TOKENS="${ADMIN_TOKEN}"
    export CONTROLPLANE_SEAT_TOKENS="1:${SEAT1_TOKEN},2:${SEAT2_TOKEN}"
    export AGENT_ENDPOINT_ALLOWLIST="127.0.0.1:9001,127.0.0.1:9002"
    export AGENT_HTTP_TIMEOUT_MS="2000"
    export DATABASE_URL="${DATABASE_URL}"
    go -C "${ENGINE_DIR}" run ./cmd/controlplane -addr "${CONTROL_ADDR}"
  ) >"${CONTROL_LOG}" 2>&1 &
  PID_CONTROL=$!

  wait_for_http

  LAST_STEP="create_user"
  api POST "/users" "${ADMIN_TOKEN}" '{"name":"smoke-user","token":"smoke-user-token"}'
  expect_code 200
  local user_id
  user_id="$(jq_get '.ID')"
  log "created user: ${user_id}"

  LAST_STEP="create_agents"
  api POST "/agents" "${ADMIN_TOKEN}" "{\"user_id\":\"${user_id}\",\"name\":\"smoke-agent-a\"}"
  expect_code 200
  local agent_a_id
  agent_a_id="$(jq_get '.ID')"
  api POST "/agents" "${ADMIN_TOKEN}" "{\"user_id\":\"${user_id}\",\"name\":\"smoke-agent-b\"}"
  expect_code 200
  local agent_b_id
  agent_b_id="$(jq_get '.ID')"
  log "created agents: ${agent_a_id}, ${agent_b_id}"

  LAST_STEP="create_versions"
  api POST "/agents/${agent_a_id}/versions" "${ADMIN_TOKEN}" '{"endpoint_url":"http://127.0.0.1:9001/callback"}'
  expect_code 200
  local version_a_id
  version_a_id="$(jq_get '.ID')"
  api POST "/agents/${agent_b_id}/versions" "${ADMIN_TOKEN}" '{"endpoint_url":"http://127.0.0.1:9002/callback"}'
  expect_code 200
  local version_b_id
  version_b_id="$(jq_get '.ID')"
  log "created versions: ${version_a_id}, ${version_b_id}"

  LAST_STEP="create_table"
  api POST "/tables" "${ADMIN_TOKEN}" '{"name":"smoke-table","max_seats":6,"small_blind":50,"big_blind":100}'
  expect_code 200
  local table_id
  table_id="$(jq_get '.ID')"
  log "created table: ${table_id}"

  LAST_STEP="join_seats"
  api POST "/tables/${table_id}/join" "${ADMIN_TOKEN}" "{\"seat_no\":1,\"agent_id\":\"${agent_a_id}\",\"agent_version_id\":\"${version_a_id}\",\"stack\":10000,\"status\":\"active\"}"
  expect_code 200
  api POST "/tables/${table_id}/join" "${ADMIN_TOKEN}" "{\"seat_no\":2,\"agent_id\":\"${agent_b_id}\",\"agent_version_id\":\"${version_b_id}\",\"stack\":10000,\"status\":\"active\"}"
  expect_code 200

  LAST_STEP="verify_table_state"
  api GET "/tables/${table_id}/state" "${ADMIN_TOKEN}"
  expect_code 200
  local seats_count
  seats_count="$(jq_get '.seats | length')"
  [[ "${seats_count}" -ge 2 ]] || fail "expected at least 2 seats, got ${seats_count}"

  LAST_STEP="start_run"
  api POST "/tables/${table_id}/start" "${ADMIN_TOKEN}" "{\"hands_to_run\":${HANDS_TO_RUN}}"
  expect_code 200

  LAST_STEP="poll_status"
  local deadline
  deadline=$(( $(date +%s) + POLL_TIMEOUT_SEC ))
  local status=""
  while [[ "$(date +%s)" -lt "${deadline}" ]]; do
    api GET "/tables/${table_id}/status" "${ADMIN_TOKEN}"
    expect_code 200
    status="$(jq_get '.status // .Status')"
    if [[ "${status}" == "completed" || "${status}" == "failed" || "${status}" == "stopped" ]]; then
      break
    fi
    sleep 0.5
  done
  [[ "${status}" == "completed" ]] || fail "run did not complete successfully; status=${status}"
  local hands_completed
  hands_completed="$(jq_get '.hands_completed // .HandsCompleted')"
  [[ "${hands_completed}" -eq "${HANDS_TO_RUN}" ]] || fail "expected hands_completed=${HANDS_TO_RUN}, got ${hands_completed}"

  LAST_STEP="fetch_hands"
  api GET "/tables/${table_id}/hands" "${SEAT1_TOKEN}"
  expect_code 200
  local hands_len
  hands_len="$(jq_get 'length')"
  [[ "${hands_len}" -gt 0 ]] || fail "expected at least one hand"
  local hand_id
  hand_id="$(jq_get '.[0].hand_id')"

  LAST_STEP="fetch_actions"
  api GET "/hands/${hand_id}/actions" "${SEAT1_TOKEN}"
  expect_code 200
  local actions_len
  actions_len="$(jq_get 'length')"
  [[ "${actions_len}" -gt 0 ]] || fail "expected actions for hand ${hand_id}"

  LAST_STEP="fetch_replay"
  api GET "/hands/${hand_id}/replay" "${SEAT1_TOKEN}"
  expect_code 200
  local replay_hand
  replay_hand="$(jq_get '.final_state.hand_id')"
  [[ "${replay_hand}" == "${hand_id}" ]] || fail "replay hand mismatch: expected ${hand_id}, got ${replay_hand}"

  LAST_STEP="auth_boundary_check"
  api GET "/tables/${table_id}/status" "${SEAT1_TOKEN}"
  expect_code 403

  log "PASS: full smoke flow completed"
  log "table_id=${table_id} hand_id=${hand_id}"
  log "logs: ${CONTROL_LOG}, ${AGENT_A_LOG}, ${AGENT_B_LOG}"
}

main "$@"
