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

These repository variables (Settings → Actions → Variables) are baked into the
agent, so a customer can install it without flags:

- `DBC_CONTROL_URL`: the coordination server (Headscale) URL.
- `DBC_ALLOW_PEERS`: comma-separated tailnet logins or `tag:` values allowed to
  control the agent.
- `DBC_AGENT_TAGS`: comma-separated tags the agent advertises.

Release builds (`v*` tags) also bake in the `DBC_AUTH_KEY` Actions secret, so
the released agent joins the tailnet with no flags at all. Branch and pull
request builds never receive it and stay keyless.

The release binaries are public, so that key is public too: anyone can read it
out of the binary and join a node of their own. Make that harmless before
setting the secret:

- Create the key **reusable**, **tagged** with the agent tag (for example
  `tag:dbc-agent`) and **expiring** (90 days). Replace it before each release
  and expire the previous one. Machines that already joined stay online, since
  the key is only used for the first login; only new installs of older
  releases stop working.
- In the tailnet ACL, let only the technicians' devices reach the agent tag on
  TCP 7420, and grant nothing **from** the agent tag. A stranger's node then
  reaches nothing, and the agent still refuses every caller missing from its
  allowlist.
- What remains possible: a stranger can add nodes that show up in `dbc ls`
  looking like customers. Check that an agent is the expected machine
  (`dbc info`) before trusting what it reports.

## Customer downloads

Each release carries stable links to its newest files, of the form
`<repository>/releases/download/latest/<file>`:

- **Windows:** `dbc-agent-windows-amd64.exe` (or `-arm64.exe`). Double-clicking
  it starts the install: UAC asks for administrator rights and the owner types
  `yes` to consent. The binary is unsigned, so SmartScreen may ask to confirm
  first (More info → Run anyway).
- **macOS and Linux:** `install.sh` picks the binary for the machine, checks it
  against `SHA256SUMS` and starts the same install through `sudo`:

  ```sh
  curl -fsSL https://git.lucianonery.dev/LucianoNeryDev/debug-center/releases/download/latest/install.sh | sh
  ```

  Arguments after `sh -s --` go to `dbc-agent install`, for example
  `sh -s -- --name frontdesk`. Downloading with `curl` also avoids macOS
  quarantine, which blocks unsigned binaries downloaded with a browser.

## Building locally

`scripts/build.sh` writes the same binaries and `SHA256SUMS` to `dist/`. It
reads `VERSION` and the three variables above from the environment.
