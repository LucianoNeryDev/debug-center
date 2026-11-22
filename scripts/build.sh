#!/usr/bin/env bash
# Cross-builds dbc-agent and dbc for every supported platform into dist/, with
# a SHA256SUMS file. Used by CI and for local builds.
#
# Optional environment:
#   VERSION           version stamped into both binaries (default: git describe)
#   DBC_CONTROL_URL   coordination server (Headscale) URL baked into the agent
#   DBC_ALLOW_PEERS   comma-separated tailnet identities allowed to control it
#   DBC_AGENT_TAGS    comma-separated tags the agent advertises
#   DBC_AUTH_KEY      pre-auth key baked into dbc-agent only. Anyone holding
#                     the binary can read it, so use a reusable key limited to
#                     the agent tag and expiring, that the tailnet ACL lets
#                     reach nothing (see docs/ci-cd.md).
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

targets=(linux/amd64 linux/arm64 windows/amd64 windows/arm64 darwin/amd64 darwin/arm64)
version="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
pkg=git.lucianonery.dev/LucianoNeryDev/debug-center/internal/buildinfo
ldflags="-s -w -X '$pkg.Version=$version'"
ldflags+=" -X '$pkg.ControlURL=${DBC_CONTROL_URL:-}'"
ldflags+=" -X '$pkg.AllowPeers=${DBC_ALLOW_PEERS:-}'"
ldflags+=" -X '$pkg.AgentTags=${DBC_AGENT_TAGS:-}'"
# The controller never needs the key, so only the agent carries it.
agent_ldflags="$ldflags -X '$pkg.AuthKey=${DBC_AUTH_KEY:-}'"

rm -rf dist
mkdir dist
for t in "${targets[@]}"; do
  os="${t%/*}" arch="${t#*/}"
  ext=""
  [[ "$os" == windows ]] && ext=".exe"
  for bin in dbc-agent dbc; do
    out="dist/$bin-$os-$arch$ext"
    flags="$ldflags"
    [[ "$bin" == dbc-agent ]] && flags="$agent_ldflags"
    echo "building $out"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
      go build -trimpath -ldflags "$flags" -o "$out" "./cmd/$bin"
  done
done
cp scripts/install.sh dist/install.sh

(cd dist && sha256sum -- * >SHA256SUMS)
echo "built $version into dist/"
