#!/usr/bin/env bash
# Cross-builds dbc-agent and dbc for every supported platform into dist/, with
# a SHA256SUMS file. Used by CI and for local builds.
#
# Optional environment:
#   VERSION           version stamped into both binaries (default: git describe)
#   DBC_CONTROL_URL   coordination server (Headscale) URL baked into the agent
#   DBC_ALLOW_PEERS   comma-separated tailnet identities allowed to control it
#   DBC_AGENT_TAGS    comma-separated tags the agent advertises
#
# Pre-auth keys are never baked in here: pass --auth-key at install time.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

targets=(linux/amd64 linux/arm64 windows/amd64 windows/arm64 darwin/amd64 darwin/arm64)
version="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
pkg=git.lucianonery.dev/LucianoNeryDev/debug-center/internal/buildinfo
ldflags="-s -w -X '$pkg.Version=$version'"
ldflags+=" -X '$pkg.ControlURL=${DBC_CONTROL_URL:-}'"
ldflags+=" -X '$pkg.AllowPeers=${DBC_ALLOW_PEERS:-}'"
ldflags+=" -X '$pkg.AgentTags=${DBC_AGENT_TAGS:-}'"

rm -rf dist
mkdir dist
for t in "${targets[@]}"; do
  os="${t%/*}" arch="${t#*/}"
  ext=""
  [[ "$os" == windows ]] && ext=".exe"
  for bin in dbc-agent dbc; do
    out="dist/$bin-$os-$arch$ext"
    echo "building $out"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
      go build -trimpath -ldflags "$ldflags" -o "$out" "./cmd/$bin"
  done
done

(cd dist && sha256sum -- * >SHA256SUMS)
echo "built $version into dist/"
