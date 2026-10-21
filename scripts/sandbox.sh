#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
KEEP=0

usage() {
  cat <<'EOF'
Usage: scripts/sandbox.sh [--keep] [COMMAND...]

Runs passer against a throwaway GPG key, password store, and config, so manual
testing never touches your real store, keyring, or ~/.config/passer.

With no COMMAND, starts passer from the current sources (go run). Otherwise
runs COMMAND inside the sandbox environment instead, e.g.:

  scripts/sandbox.sh pass ls
  scripts/sandbox.sh "$SHELL"      # a shell with the sandbox exported

The sandbox is deleted on exit unless --keep is given.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --keep)
      KEEP=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      break
      ;;
    *)
      break
      ;;
  esac
done

for tool in gpg gpgconf pass go; do
  if ! command -v "${tool}" >/dev/null 2>&1; then
    echo "'${tool}' was not found on your PATH." >&2
    exit 1
  fi
done

# A short path is load-bearing: gpg-agent's socket lives under GNUPGHOME, and
# unix socket paths are capped near 100 bytes. macOS's per-user $TMPDIR is
# already long enough to break key generation with "No agent running".
SANDBOX="$(mktemp -d /tmp/passer-sandbox.XXXXXX)"
export GNUPGHOME="${SANDBOX}/gnupg"
export PASSWORD_STORE_DIR="${SANDBOX}/store"
export PASSER_CONFIG="${SANDBOX}/config.toml"
# An exported store key from the user's shell would otherwise leak in.
unset PASSWORD_STORE_KEY PASSWORD_STORE_GPG_OPTS

cleanup() {
  gpgconf --kill gpg-agent >/dev/null 2>&1 || true
  if [[ "${KEEP}" -eq 1 ]]; then
    echo "Sandbox kept at ${SANDBOX}"
    echo "  export GNUPGHOME='${GNUPGHOME}' PASSWORD_STORE_DIR='${PASSWORD_STORE_DIR}' PASSER_CONFIG='${PASSER_CONFIG}'"
  else
    rm -rf "${SANDBOX}"
  fi
}
trap cleanup EXIT

mkdir -m 700 "${GNUPGHOME}"

KEY_ID="passer-sandbox@example.com"
gpg --batch --quiet --pinentry-mode loopback --passphrase '' \
  --quick-gen-key "${KEY_ID}" default default never 2>/dev/null
pass init "${KEY_ID}" >/dev/null

# Seed entries that exercise the interesting paths: metadata fields, a pass-otp
# URI (must render masked), a folder to test renames against, and a bare
# password-only entry.
seed() { pass insert --multiline "$1" >/dev/null; }
printf 'hunter2\nlogin: han\nurl: https://github.com\notpauth://totp/GitHub:han?secret=JBSWY3DPEHPK3PXP&issuer=GitHub\n' | seed web/github
printf 'correct horse battery staple\nusername: han@example.com\nurl: https://gitlab.com\nrecovery codes: 1111 2222 3333\n' | seed web/gitlab
printf 's3cret\nemail: han@example.com\nThis line is a free-form note.\n' | seed email/fastmail
printf 'only-a-password\n' | seed wifi
# The two unlabelled account conventions `u` has to fall back to: an address on
# a bare line, and pass's own "site/user@host" naming.
printf 'bare-line-pw\nhan@example.com\nThis line is a free-form note.\n' | seed web/reddit
printf 'name-only-pw\n' | seed 'web/aws.amazon.com/han@example.com'

install -m 0600 "${ROOT_DIR}/config.example.toml" "${PASSER_CONFIG}"

echo "Sandbox ready at ${SANDBOX}"

if [[ $# -eq 0 ]]; then
  cd "${ROOT_DIR}"
  go run ./cmd/passer
else
  "$@"
fi
