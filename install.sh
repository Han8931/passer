#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PREFIX="/usr/local"
BIN_NAME="passer"

usage() {
  cat <<'EOF'
Usage: ./install.sh [--prefix DIR] [--bin-name NAME]

Builds the passer binary and installs it into DIR/bin (default: /usr/local/bin).
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

if [[ -z "${PREFIX}" ]]; then
  echo "Prefix is required." >&2
  exit 1
fi

# passer is a front end for pass; without it there is nothing to install onto.
if ! command -v pass >/dev/null 2>&1; then
  echo "WARNING: 'pass' was not found on your PATH." >&2
  echo "         passer drives the pass command line tool and will refuse to start without it." >&2
  echo "         Install it (e.g. 'sudo pacman -S pass' or 'brew install pass') and run 'pass init <gpg-id>'." >&2
  echo >&2
fi

CONFIG_DIR="${XDG_CONFIG_HOME:-${HOME}/.config}/passer"
CONFIG_PATH="${CONFIG_DIR}/config.toml"

mkdir -p "${CONFIG_DIR}"
chmod 700 "${CONFIG_DIR}"

if [[ ! -f "${CONFIG_PATH}" ]]; then
  install -m 0600 "${ROOT_DIR}/config.example.toml" "${CONFIG_PATH}"
  echo "Wrote default config to ${CONFIG_PATH}"
fi

BIN_DIR="${PREFIX}/bin"
TARGET="${BIN_DIR}/${BIN_NAME}"
LOCAL_BIN="${ROOT_DIR}/bin/${BIN_NAME}"

# Clean build: drop any stale local artifact so we never re-install an old
# binary, and rebuild from the current sources.
echo "Cleaning previous build artifacts..."
rm -f "${LOCAL_BIN}"

echo "Building ${BIN_NAME}..."
mkdir -p "${ROOT_DIR}/bin"
GOFLAGS=${GOFLAGS:-}
go build ${GOFLAGS} -o "${LOCAL_BIN}" "${ROOT_DIR}/cmd/passer"

echo "Installing to ${TARGET}..."
if [[ -w "${BIN_DIR}" ]]; then
  install -m 0755 "${LOCAL_BIN}" "${TARGET}"
else
  echo "No write access to ${BIN_DIR}. Trying sudo..."
  sudo install -m 0755 "${LOCAL_BIN}" "${TARGET}"
fi

echo "Installed ${BIN_NAME} to ${TARGET}"

# Guard against the classic "old version keeps running" trap: another copy of
# the binary sitting earlier on PATH shadows the one we just installed.
RESOLVED="$(command -v "${BIN_NAME}" 2>/dev/null || true)"
if [[ -n "${RESOLVED}" && "${RESOLVED}" != "${TARGET}" ]]; then
  echo
  echo "WARNING: '${BIN_NAME}' on your PATH resolves to:" >&2
  echo "    ${RESOLVED}" >&2
  echo "but it was just installed to:" >&2
  echo "    ${TARGET}" >&2
  echo "The shadowing copy will run instead of the new build. Remove it with:" >&2
  echo "    rm -f '${RESOLVED}'" >&2
  echo "(or run ./uninstall.sh, which clears known stale copies), then re-open your shell." >&2
elif [[ -z "${RESOLVED}" ]]; then
  echo
  echo "NOTE: ${BIN_DIR} is not on your PATH. Add it, e.g.:" >&2
  echo "    export PATH=\"${BIN_DIR}:\$PATH\"" >&2
fi

# A clipboard tool is optional but nearly everything useful goes through it.
if ! command -v wl-copy >/dev/null 2>&1 \
  && ! command -v xclip >/dev/null 2>&1 \
  && ! command -v xsel >/dev/null 2>&1 \
  && ! command -v pbcopy >/dev/null 2>&1; then
  echo
  echo "NOTE: no clipboard tool found. passer runs fine without one, but copying" >&2
  echo "      will fail. Install wl-clipboard (Wayland), or xclip/xsel (X11)." >&2
fi
