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
