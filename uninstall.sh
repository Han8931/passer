#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PREFIX="/usr/local"
BIN_NAME="passer"
PURGE=0

usage() {
  cat <<'EOF'
Usage: ./uninstall.sh [--prefix DIR] [--bin-name NAME] [--purge]

Removes the installed passer binary and the local build artifact.
--purge also deletes the config directory (~/.config/passer).

Your password store is NEVER touched: it belongs to pass, not to passer.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --prefix)
      PREFIX="${2:-}"
      shift 2
      ;;
    --bin-name)
      BIN_NAME="${2:-}"
      shift 2
      ;;
    --purge)
      PURGE=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

TARGET="${PREFIX}/bin/${BIN_NAME}"

remove() {
  local path="$1"
  [[ -e "${path}" ]] || return 0
  if [[ -w "$(dirname "${path}")" ]]; then
    rm -f "${path}"
  else
    sudo rm -f "${path}"
  fi
  echo "Removed ${path}"
}

remove "${TARGET}"
remove "${ROOT_DIR}/bin/${BIN_NAME}"

# Clear stale copies elsewhere on PATH, the usual cause of "it's still here".
while true; do
  RESOLVED="$(command -v "${BIN_NAME}" 2>/dev/null || true)"
  [[ -n "${RESOLVED}" ]] || break
  remove "${RESOLVED}"
  hash -r 2>/dev/null || true
done

if [[ "${PURGE}" -eq 1 ]]; then
  CONFIG_DIR="${XDG_CONFIG_HOME:-${HOME}/.config}/passer"
  if [[ -d "${CONFIG_DIR}" ]]; then
    rm -rf "${CONFIG_DIR}"
    echo "Removed ${CONFIG_DIR}"
  fi
fi

echo
echo "Done. Your password store was not touched — it is managed by pass."
