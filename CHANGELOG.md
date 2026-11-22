# Changelog

Notable changes to Debug Center, by version.

## Unreleased

### Added

- `dbc-agent`, the agent for Windows, Linux and macOS. It joins the support
  tailnet under an identifiable name, installs as a service once the owner
  consents and an administrator approves, and removes itself completely on
  request, either on the machine or from the controller. `dbc-agent run` starts
  a one-off session that cleans up when it ends.
- What the agent offers: system inventory, live load, processes, remote
  commands with streamed output, a catalogue of diagnostic and repair actions
  for each OS, file transfer, and an audit log of every request. Only allowed
  tailnet identities can use it.
- `dbc`, the technician's controller. It lists the agents on the tailnet and
  inspects, controls and transfers files with them: `ls`, `info`, `stats`,
  `top`, `ps`, `audit`, `exec`, `actions`, `run`, `kill`, `uninstall`, `files`,
  `pull` and `push`.
- `scripts/build.sh`, which builds both programs for Windows, Linux and macOS
  on amd64 and arm64 with checksums, and can bake the coordination server, the
  allowed technicians and the agent tags into the agent.
- Continuous integration on Forgejo Actions: every push is vetted, tested and
  built, and the run keeps the binaries for download. A `v*` tag publishes them
  as a release.
- Tests for the agent's allowlist, command building, action argument quoting
  and audit log.
- A README and contribution guide.
- Releases a customer can install with no flags: release builds bake in a
  pre-auth key kept harmless by the tailnet ACL, `dbc-agent` with no arguments
  starts the install on every OS, and `install.sh` fetches, checks and installs
  the newest agent on macOS and Linux in one command.

### Fixed

- Exec requests the agent rejects, such as an unknown action or an
  unsupported shell, are now recorded in the audit log with `allowed: false`
  instead of `true`.
- The agent no longer acts without an audit line. When the log cannot be
  written, commands, actions, process kills, uploads and remote uninstalls are
  refused with an error; read-only requests go ahead and the failure is
  reported on stderr.
