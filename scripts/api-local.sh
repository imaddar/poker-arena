#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENGINE_DIR="${ROOT_DIR}/services/engine"
BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
TABLE_ID="${TABLE_ID:-local-table-1}"
ADMIN_API_TOKEN="${ADMIN_API_TOKEN:-}"
SEAT_API_TOKEN="${SEAT_API_TOKEN:-}"

if ! command -v go >/dev/null 2>&1; then
  echo "error: go is not installed or not in PATH" >&2
  exit 1
fi

usage() {
  cat <<'USAGE'
Usage: ./scripts/api-local.sh <command> [args]

Commands:
  serve [addr]                  Run control-plane server (default addr: :8080)
  create-user <name> <token>    Create a user record
  create-agent <user_id> <name> Create an agent for a user
  create-version <agent_id> <endpoint_url>
                                Create agent version with callback endpoint
  create-table <name> [max] [sb] [bb]
                                Create a table record
  join <seat_no> <agent_id> <agent_version_id> [stack] [status]
                                Seat an agent version at TABLE_ID
  table-state                   Get table metadata + seats + run summary
  start [hands]                 Start a table run from persisted table seats (default: 5)
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
USAGE
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

  create-user)
    name="${1:-}"
    token="${2:-}"
    if [[ -z "${name}" || -z "${token}" ]]; then
      echo "error: create-user requires <name> <token>" >&2
      exit 1
    fi
    curl -sS \
      -X POST "${BASE_URL}/users" \
      -H "Content-Type: application/json" \
      -H "$(admin_auth_header)" \
      -d "{\"name\":\"${name}\",\"token\":\"${token}\"}"
    echo
    ;;

  create-agent)
    user_id="${1:-}"
    name="${2:-}"
    if [[ -z "${user_id}" || -z "${name}" ]]; then
      echo "error: create-agent requires <user_id> <name>" >&2
      exit 1
    fi
    curl -sS \
      -X POST "${BASE_URL}/agents" \
      -H "Content-Type: application/json" \
      -H "$(admin_auth_header)" \
      -d "{\"user_id\":\"${user_id}\",\"name\":\"${name}\"}"
    echo
    ;;

  create-version)
    agent_id="${1:-}"
    endpoint_url="${2:-}"
    if [[ -z "${agent_id}" || -z "${endpoint_url}" ]]; then
      echo "error: create-version requires <agent_id> <endpoint_url>" >&2
      exit 1
    fi
    curl -sS \
      -X POST "${BASE_URL}/agents/${agent_id}/versions" \
      -H "Content-Type: application/json" \
      -H "$(admin_auth_header)" \
      -d "{\"endpoint_url\":\"${endpoint_url}\"}"
    echo
    ;;

  create-table)
    name="${1:-}"
    max="${2:-6}"
    sb="${3:-50}"
    bb="${4:-100}"
    if [[ -z "${name}" ]]; then
      echo "error: create-table requires <name> [max] [sb] [bb]" >&2
      exit 1
    fi
    curl -sS \
      -X POST "${BASE_URL}/tables" \
      -H "Content-Type: application/json" \
      -H "$(admin_auth_header)" \
      -d "{\"name\":\"${name}\",\"max_seats\":${max},\"small_blind\":${sb},\"big_blind\":${bb}}"
    echo
    ;;

  join)
    seat_no="${1:-}"
    agent_id="${2:-}"
    agent_version_id="${3:-}"
    stack="${4:-10000}"
    status="${5:-active}"
    if [[ -z "${seat_no}" || -z "${agent_id}" || -z "${agent_version_id}" ]]; then
      echo "error: join requires <seat_no> <agent_id> <agent_version_id> [stack] [status]" >&2
      exit 1
    fi
    curl -sS \
      -X POST "${BASE_URL}/tables/${TABLE_ID}/join" \
      -H "Content-Type: application/json" \
      -H "$(admin_auth_header)" \
      -d "{\"seat_no\":${seat_no},\"agent_id\":\"${agent_id}\",\"agent_version_id\":\"${agent_version_id}\",\"stack\":${stack},\"status\":\"${status}\"}"
    echo
    ;;

  table-state)
    curl -sS "${BASE_URL}/tables/${TABLE_ID}/state" -H "$(admin_auth_header)"
    echo
    ;;

  start)
    hands="${1:-5}"
    if ! [[ "${hands}" =~ ^[0-9]+$ ]] || (( hands <= 0 )); then
      echo "error: hands must be a positive integer (got '${hands}')" >&2
      exit 1
    fi
    curl -sS \
      -X POST "${BASE_URL}/tables/${TABLE_ID}/start" \
      -H "Content-Type: application/json" \
      -H "$(admin_auth_header)" \
      -d "{\"hands_to_run\":${hands}}"
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
