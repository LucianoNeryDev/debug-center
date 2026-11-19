<img src="assets/icon.svg" width="64" height="64" alt="">

# Debug Center

Remote support for Windows, Linux and macOS computers over a private tailnet. A
small agent, `dbc-agent`, runs on the computer being serviced; the technician
drives it with a command-line controller, `dbc`. Between them they cover the
usual support session: hardware and software inventory, live load, processes,
remote commands with streamed output, a catalogue of diagnostic and repair
actions for each OS, file transfer, and remote removal. Every request lands in
an audit log on the device.

Nothing listens on the internet or the local network. The agent joins a
[Tailscale](https://tailscale.com) or [Headscale](https://headscale.net)
tailnet as its own node and answers only callers whose tailnet identity is on
its allowlist.

```sh
dbc ls                                   # agents on the tailnet
dbc info frontdesk                       # inventory
dbc top frontdesk                        # live CPU, memory, disk, network
dbc run frontdesk net-diag               # a named diagnostic action
dbc exec frontdesk 'Get-Service | Where-Object Status -eq Stopped'
dbc pull frontdesk 'C:\Windows\Logs\CBS\CBS.log'
dbc audit frontdesk                      # who asked for what
```

## How it works

**Joining the tailnet.** `dbc-agent` embeds Tailscale's `tsnet` library, so the
device needs no Tailscale client. On first start it logs in with a pre-auth
key, registers as `dbc-<name>-<id>` (the name identifies the customer, ticket
or machine; the id is random), and listens on tailnet port 7420. Once the node
key is stored, an installed agent deletes the pre-auth key from its
configuration.

**Identity.** Every request is checked twice. The tailnet's access rules decide
who can reach the agent at all. The agent then asks its own tailnet client who
is behind the source address (`WhoIs`) and compares the answer with its
allowlist:

- an entry starting with `tag:` matches nodes carrying that tag;
- any other entry matches the login name of an untagged node, ignoring case;
- tagged nodes never match on login, since their login is a placeholder;
- if the lookup fails, the request is refused.

Refused requests are answered with 403 and recorded in the audit log with
`allowed: false`.

**Named actions and raw commands.** The agent ships a catalogue of actions for
its OS: event and journal errors, disk health, SFC and DISM scans, driver and
device problems, network diagnostics, DNS flush, pending updates, reboot and
others (`dbc actions <agent>` lists them). An action is a fixed command line;
its parameters are quoted for the target shell before substitution, so an
argument cannot turn into extra commands. Actions marked disruptive, such as
`reboot`, ask the technician for confirmation. Raw commands are the escape
hatch: `dbc exec` runs a line through PowerShell, `cmd`, `sh` or `bash`, or
runs a program directly with `--no-shell`. Output streams back over a
WebSocket, stdin can be forwarded, and the remote exit status becomes `dbc`'s.
Ctrl+C, a timeout or a dropped connection kills the whole process tree.

**Audit log.** The agent appends one JSON line per request to `audit.log` in
its directory: time, tailnet identity, node, address, operation, detail and
whether it was allowed. Requests the agent refuses, at the door or because
they do not resolve to a valid command, are recorded with `allowed: false`.
The log is written before anything happens: if it cannot be written, requests
that run commands, kill processes, upload files or uninstall the agent are
refused with an error, and read-only requests go ahead with the failure
reported on stderr. `dbc audit` shows the log from the technician's side, and
`dbc-agent uninstall --keep-audit <path>` keeps a copy when the agent is
removed.

## Install and build

Builds for every commit are kept as artifacts on the repository's Actions runs,
and `v*` tags publish them as releases: `dbc-agent` and `dbc` for Windows,
Linux and macOS on amd64 and arm64, with a `SHA256SUMS` file. See
[Builds and CI](docs/ci-cd.md).

To build from source you need Go at the version in `go.mod`:

```sh
go build ./cmd/dbc-agent ./cmd/dbc   # this platform
scripts/build.sh                     # every platform, into dist/, with SHA256SUMS
```

`scripts/build.sh` reads optional settings from the environment and bakes them
into the agent, so a customer can install it without flags:

| Variable | Baked-in setting |
| --- | --- |
| `VERSION` | version string (default: `git describe`) |
| `DBC_CONTROL_URL` | coordination server (Headscale) URL |
| `DBC_ALLOW_PEERS` | comma-separated logins or `tag:` values allowed to control the agent |
| `DBC_AGENT_TAGS` | comma-separated tags the agent advertises |

Pre-auth keys are never baked in, because the binaries are public. Pass the key
at install time with `--auth-key` or `DBC_AUTH_KEY`.

## Usage

### On the device

Install the agent as a service. It asks for administrator rights (UAC on
Windows, `sudo` elsewhere) and shows the owner what the technician will be able
to do before asking for consent. On Windows, double-clicking the `.exe` starts
the same install.

```sh
dbc-agent install --name "Front desk" --auth-key tskey-... \
  --control-url https://hs.example.com --allow tech@example.com
dbc-agent status
dbc-agent uninstall               # stops the service, leaves the tailnet, deletes every file
```

For a single session without installing anything, `run` joins as an ephemeral
node and logs out and deletes its state when it ends with Ctrl+C:

```sh
dbc-agent run --allow tag:support --auth-key tskey-...
```

Install directories: `C:\ProgramData\DebugCenter` on Windows,
`/opt/debug-center` on Linux and `/Library/Application Support/DebugCenter` on
macOS.

### From the technician's machine

`dbc` finds agents through the local `tailscaled`, so the technician's machine
must be on the same tailnet. An agent can be named by tailnet IP, hostname
with or without the `dbc-` prefix, the first characters of its id, or any
unique part of its hostname.

```sh
dbc ls --all                                  # include offline agents
dbc info pc1                                  # hardware and software inventory
dbc stats pc1                                 # one sample of load and temperatures
dbc top -i 1s pc1                             # live view
dbc ps -s mem -n 10 pc1 chrome                # processes, filtered and sorted
dbc kill pc1 4242

dbc actions pc1
dbc run pc1 restart-service nginx
dbc run --yes pc1 reboot

dbc exec pc1 'Get-Service | Where-Object Status -eq Stopped'
dbc exec --shell cmd pc1 ipconfig /all
dbc exec --no-shell --timeout 5m pc1 uname -a
dbc exec -i pc1 sh -s < diagnose.sh           # forward local stdin

dbc files pc1 /var/log
dbc pull pc1 /var/log/syslog
dbc push pc1 nginx.conf /etc/nginx/nginx.conf

dbc audit -n 20 pc1
dbc uninstall pc1                             # remove the agent remotely
```

Most listing commands take `--json`. `dbc --help` and `dbc <command> --help`
describe every flag.

## Security model and limits

- **Allowed means administrator.** An allowed technician runs commands with the
  agent's rights, which are SYSTEM or root for an installed agent. There are
  no roles yet: the allowlist is all or nothing.
- **The coordination server is trusted.** Identities and tags come from the
  tailnet's control server. Whoever controls it, or can mint pre-auth keys
  with an allowed tag, can reach the agent. Keep tag ownership and access
  rules tight.
- **Transport.** The API is plain HTTP inside the tailnet; confidentiality and
  peer authentication come from WireGuard. It is never exposed outside the
  tailnet.
- **The audit log is local.** It sits on the device, so an administrator of
  that machine, or a technician running commands as one, can edit or delete
  it. It records what was requested, not everything a command went on to do.
  Copies on the technician's side are planned.
- **Confirmation is client-side.** `dbc` asks before disruptive actions; the
  agent itself does not, so another client could skip the prompt.
- **Consent.** Installing needs administrator approval and an explicit "yes"
  from the person at the machine (`--yes` skips the prompt and is meant for
  owners who already agreed). The agent stays until it is uninstalled, locally
  or remotely; one-off sessions leave nothing behind.

Report vulnerabilities privately as described in [SECURITY.txt](SECURITY.txt).

## Status

Early development. Both programs build for every supported platform and the
security-sensitive parts of the agent are covered by tests, but there is no
release yet and the agent has not been field-tested against a production
tailnet. The [roadmap](ROADMAP.md) lists what comes next, and the
[changelog](CHANGELOG.md) what has changed.

Debug Center is maintained for its author's own support work. Issues and pull
requests are welcome; see [CONTRIBUTING.md](CONTRIBUTING.md). There is no
support commitment.

## License and credit

[MIT](LICENSE) © Luciano Nery. You can use, change and redistribute it,
commercially or not. The only ask is credit: keep the copyright notice, and a
link back to this repository is appreciated.
