#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ ! -x "${ROOT_DIR}/bin/passer" ]]; then
  echo 'Build the binary first: go build -o bin/passer ./cmd/passer' >&2
  exit 1
fi

# Create the fixtures in the same process environment that launches the binary.
# Never depend on a previous shell's exports or a surviving temporary directory.
exec "${ROOT_DIR}/scripts/sandbox.sh" "${ROOT_DIR}/bin/passer" "$@"
