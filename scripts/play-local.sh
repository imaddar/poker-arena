#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENGINE_DIR="${ROOT_DIR}/services/engine"

if ! command -v go >/dev/null 2>&1; then
  echo "error: go is not installed or not in PATH" >&2
  exit 1
fi

# Usage:
#   ./scripts/play-local.sh
#   ./scripts/play-local.sh <seat>
#   ./scripts/play-local.sh <seat> <hands>
#   ./scripts/play-local.sh <seat> <hands> [extra go-run args...]
# Examples:
#   ./scripts/play-local.sh 1 3
#   ./scripts/play-local.sh 2 5 -mode play

SEAT="1"
HANDS="1"

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  cat <<'USAGE'
Usage: ./scripts/play-local.sh [seat] [hands] [extra args...]

Positional parameters:
  seat    Human-controlled seat number (default: 1)
  hands   Number of hands to run (default: 1)

Examples:
  ./scripts/play-local.sh
  ./scripts/play-local.sh 1 3
  ./scripts/play-local.sh 2 5 -mode play
USAGE
  exit 0
fi

if [[ -n "${1:-}" && "${1}" != -* ]]; then
  SEAT="${1}"
  shift
fi

if [[ -n "${1:-}" && "${1}" != -* ]]; then
  HANDS="${1}"
  shift
fi

exec go -C "${ENGINE_DIR}" run ./cmd/engine -mode play -hands "${HANDS}" -human-seat "${SEAT}" "$@"
