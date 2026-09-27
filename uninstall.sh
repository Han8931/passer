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

# Other copies elsewhere on PATH are the usual cause of "it's still here", but
# a file that merely shares the name isn't necessarily ours, so each one is
# confirmed rather than deleted outright.
declare -a SEEN=()
while true; do
  hash -r 2>/dev/null || true
  RESOLVED="$(type -P "${BIN_NAME}" 2>/dev/null || true)"
  [[ -n "${RESOLVED}" && -f "${RESOLVED}" ]] || break
  # Stop if a declined or failed removal leaves the same file resolving.
  for seen in "${SEEN[@]+"${SEEN[@]}"}"; do
    [[ "${seen}" == "${RESOLVED}" ]] && break 2
  done
  SEEN+=("${RESOLVED}")

  if [[ -t 0 ]]; then
    read -r -p "Another '${BIN_NAME}' is on your PATH at ${RESOLVED}. Remove it? [y/N] " reply
    [[ "${reply}" =~ ^[Yy]$ ]] && remove "${RESOLVED}"
  else
    echo "NOTE: another '${BIN_NAME}' remains at ${RESOLVED}; remove it by hand if it is a stale passer." >&2
  fi
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
