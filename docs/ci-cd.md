# Builds and CI

The pipeline runs on Forgejo Actions (`.forgejo/workflows/build.yml`) on the
owner's runner (label `docker`). Every push to any branch, and every pull
request, is vetted, tested and built for:

| OS | amd64 | arm64 |
| --- | --- | --- |
| Windows | `dbc-agent-windows-amd64.exe`, `dbc-windows-amd64.exe` | `dbc-agent-windows-arm64.exe`, `dbc-windows-arm64.exe` |
| Linux | `dbc-agent-linux-amd64`, `dbc-linux-amd64` | `dbc-agent-linux-arm64`, `dbc-linux-arm64` |
| macOS | `dbc-agent-darwin-amd64`, `dbc-darwin-amd64` | `dbc-agent-darwin-arm64`, `dbc-darwin-arm64` |

Each binary reports its version with `version`: the tag for a release,
otherwise `git describe` (for example `v0.1.0-3-gabc1234`, or the commit hash
before the first tag).

## Downloads

- **A commit:** open its run under the repository's Actions tab. The run keeps
  three artifacts, `debug-center-<version>-windows`, `-linux` and `-macos`,
  each a zip with that OS's binaries and `SHA256SUMS`. Artifacts expire after
  the server's retention period. On Linux and macOS, `chmod +x` the binary
  after unzipping.
- **A release:** pushing a tag that starts with `v` (for example `v0.1.0`) also
  publishes every binary and `SHA256SUMS` as a release on the Releases page.
  Releases do not expire.

Check a download with `sha256sum --ignore-missing -c SHA256SUMS`.

## Baked-in settings

These optional repository variables (Settings → Actions → Variables) are baked
into the agent, so a customer can install it without flags:

- `DBC_CONTROL_URL`: the coordination server (Headscale) URL.
- `DBC_ALLOW_PEERS`: comma-separated tailnet logins or `tag:` values allowed to
  control the agent.
- `DBC_AGENT_TAGS`: comma-separated tags the agent advertises.

The binaries are public, so never bake a pre-auth key into them. Pass it at
install time with `--auth-key` or `DBC_AUTH_KEY`.

## Building locally

`scripts/build.sh` writes the same binaries and `SHA256SUMS` to `dist/`. It
reads `VERSION` and the three variables above from the environment.
