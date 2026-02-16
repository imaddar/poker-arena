#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENGINE_DIR="${ROOT_DIR}/services/engine"
BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
TABLE_ID="${TABLE_ID:-local-table-1}"
ADMIN_API_TOKEN="${ADMIN_API_TOKEN:-}"
SEAT_API_TOKEN="${SEAT_API_TOKEN:-}"
AGENT_ENDPOINTS="${AGENT_ENDPOINTS:-}"

if ! command -v go >/dev/null 2>&1; then
  echo "error: go is not installed or not in PATH" >&2
  exit 1
fi

usage() {
  cat <<'USAGE'
Usage: ./scripts/api-local.sh <command> [args]

Commands:
  serve [addr]                  Run control-plane server (default addr: :8080)
  start [hands] [players]       Start a table run (defaults: hands=5 players=2)
  status                         Get table status
  hands                          List persisted hands for table
  actions <hand_id>              List persisted actions for a hand
  replay <hand_id>               Get replay payload for a hand
  stop                           Stop the current table run

Environment:
  BASE_URL   API base URL (default: http://127.0.0.1:8080)
  TABLE_ID   Table ID to target (default: local-table-1)
  ADMIN_API_TOKEN  Bearer token for control routes (required for start/status/stop)
  SEAT_API_TOKEN   Bearer token for history routes; falls back to ADMIN_API_TOKEN
  AGENT_ENDPOINTS Comma-separated endpoints by seat order for start (required for start)
USAGE
}

build_seats_json() {
  local players="$1"
  local endpoints_csv="$2"
  IFS=',' read -r -a endpoints <<<"${endpoints_csv}"
  if (( ${#endpoints[@]} < players )); then
    echo "error: AGENT_ENDPOINTS must include at least ${players} endpoints (got ${#endpoints[@]})" >&2
    exit 1
  fi
  local i
  local out=""
  for ((i=1; i<=players; i++)); do
    local endpoint
    endpoint="$(echo "${endpoints[$((i-1))]}" | xargs)"
    if [[ -z "${endpoint}" ]]; then
      echo "error: AGENT_ENDPOINTS entry for seat ${i} is empty" >&2
      exit 1
    fi
    if [[ -n "${out}" ]]; then
      out+=","
    fi
    out+="{\"seat_no\":${i},\"stack\":10000,\"status\":\"active\",\"agent_endpoint\":\"${endpoint}\"}"
  done
  printf '%s' "${out}"
}

admin_auth_header() {
  if [[ -z "${ADMIN_API_TOKEN}" ]]; then
    echo "error: ADMIN_API_TOKEN is required for control-plane admin commands" >&2
    exit 1
  fi
  printf 'Authorization: Bearer %s' "${ADMIN_API_TOKEN}"
}

history_auth_header() {
  local token="${SEAT_API_TOKEN:-${ADMIN_API_TOKEN}}"
  if [[ -z "${token}" ]]; then
    echo "error: set SEAT_API_TOKEN or ADMIN_API_TOKEN for history commands" >&2
    exit 1
  fi
  printf 'Authorization: Bearer %s' "${token}"
}

cmd="${1:-}"
if [[ -z "${cmd}" || "${cmd}" == "-h" || "${cmd}" == "--help" ]]; then
  usage
  exit 0
fi
shift || true

case "${cmd}" in
  serve)
    addr="${1:-:8080}"
    exec go -C "${ENGINE_DIR}" run ./cmd/controlplane -addr "${addr}"
    ;;

  start)
    hands="${1:-5}"
    players="${2:-2}"
    if ! [[ "${hands}" =~ ^[0-9]+$ ]] || (( hands <= 0 )); then
      echo "error: hands must be a positive integer (got '${hands}')" >&2
      exit 1
    fi
    if ! [[ "${players}" =~ ^[0-9]+$ ]] || (( players < 2 || players > 6 )); then
      echo "error: players must be an integer in range 2..6 (got '${players}')" >&2
      exit 1
    fi
    if [[ -z "${AGENT_ENDPOINTS}" ]]; then
      echo "error: AGENT_ENDPOINTS is required for start (comma-separated endpoint URLs)" >&2
      exit 1
    fi
    seats_json="$(build_seats_json "${players}" "${AGENT_ENDPOINTS}")"
    curl -sS \
      -X POST "${BASE_URL}/tables/${TABLE_ID}/start" \
      -H "Content-Type: application/json" \
      -H "$(admin_auth_header)" \
      -d "{\"hands_to_run\":${hands},\"seats\":[${seats_json}]}"
    echo
    ;;

  status)
    curl -sS "${BASE_URL}/tables/${TABLE_ID}/status" -H "$(admin_auth_header)"
    echo
    ;;

  hands)
    curl -sS "${BASE_URL}/tables/${TABLE_ID}/hands" -H "$(history_auth_header)"
    echo
    ;;

  actions)
    hand_id="${1:-}"
    if [[ -z "${hand_id}" ]]; then
      echo "error: actions requires <hand_id>" >&2
      exit 1
    fi
    curl -sS "${BASE_URL}/hands/${hand_id}/actions" -H "$(history_auth_header)"
    echo
    ;;

  replay)
    hand_id="${1:-}"
    if [[ -z "${hand_id}" ]]; then
      echo "error: replay requires <hand_id>" >&2
      exit 1
    fi
    curl -sS "${BASE_URL}/hands/${hand_id}/replay" -H "$(history_auth_header)"
    echo
    ;;

  stop)
    curl -sS -X POST "${BASE_URL}/tables/${TABLE_ID}/stop" -H "$(admin_auth_header)"
    echo
    ;;

  *)
    echo "error: unknown command '${cmd}'" >&2
    usage
    exit 1
    ;;
esac
