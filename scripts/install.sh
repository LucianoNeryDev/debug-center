#!/bin/sh
# Downloads the newest released dbc-agent for this Linux or macOS machine,
# checks it against SHA256SUMS and starts its install, which asks for consent
# and for the administrator password. Published with every release, so a
# customer only runs:
#
#   curl -fsSL https://git.lucianonery.dev/LucianoNeryDev/debug-center/releases/download/latest/install.sh | sh
#
# Arguments after `sh -s --` go to `dbc-agent install`, for example
# `sh -s -- --name frontdesk`. DBC_RELEASE_URL points at another release.
set -eu

base="${DBC_RELEASE_URL:-https://git.lucianonery.dev/LucianoNeryDev/debug-center/releases/download/latest}"

fail() {
  echo "error: $*" >&2
  exit 1
}

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) fail "unsupported system $(uname -s); on Windows download dbc-agent-windows-amd64.exe instead" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) fail "unsupported processor $(uname -m)" ;;
esac
file="dbc-agent-$os-$arch"

if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -q -O "$2" "$1"; }
else
  fail "curl or wget is required"
fi
if command -v sha256sum >/dev/null 2>&1; then
  sha256() { sha256sum "$1" | cut -d' ' -f1; }
else
  sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $file…"
fetch "$base/$file" "$tmp/dbc-agent" || fail "download of $base/$file failed"
fetch "$base/SHA256SUMS" "$tmp/SHA256SUMS" || fail "download of $base/SHA256SUMS failed"
want="$(awk -v f="$file" '$2 == f || $2 == "*" f { print $1 }' "$tmp/SHA256SUMS")"
[ -n "$want" ] || fail "$file is missing from SHA256SUMS"
[ "$(sha256 "$tmp/dbc-agent")" = "$want" ] || fail "checksum mismatch for $file"
chmod 755 "$tmp/dbc-agent"

# The script itself arrives on stdin through the pipe, so the consent prompt
# and sudo read from the terminal instead.
( : </dev/tty ) 2>/dev/null || fail "run this from a terminal: the install asks for consent"
if [ "$(id -u)" -eq 0 ]; then
  "$tmp/dbc-agent" install "$@" </dev/tty
else
  echo "The install needs administrator rights; sudo will ask for your password."
  # shellcheck disable=SC2024 # the redirect is the user's own terminal
  sudo "$tmp/dbc-agent" install "$@" </dev/tty
fi
