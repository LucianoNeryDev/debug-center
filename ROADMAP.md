# Roadmap

Debug Center is in early development. The desktop agent (`dbc-agent`) and the
technician's command-line controller (`dbc`) are written for Windows, Linux and
macOS, but nothing has been released yet and neither has run against a real
support tailnet. This page lists what comes next, roughly in order.

## First release

The goal of the first release: a technician sets up a tailnet, hands a
customer one file, and services the machine with `dbc`.

- **Documentation.** A README with a quick start, and guides in `docs/` for
  setting up the coordination server (Headscale) with tags, access rules and
  pre-auth keys; installing and removing the agent; using `dbc`; the security
  model; and troubleshooting. Architecture decisions recorded in `docs/adr/`.
- **Field testing** against a real Headscale tailnet on Windows 10 and 11,
  common Linux distributions and macOS: install, remote uninstall and one-off
  sessions.
- **Tests for the security-sensitive parts of the agent:** who is allowed in,
  how action arguments are quoted, command execution and file transfer.
- **Windows code signing**, so SmartScreen and antivirus software do not block
  the agent when a customer downloads it.

## Phones

Debug Center is growing from PCs to every kind of device, starting with
phones.

- **Android agent app.** The same agent core built for Android, running as a
  foreground service behind a visible consent screen. It reports device
  details, battery, storage, network and installed apps, and transfers files
  from shared storage.
- **Android over USB.** A phone plugged into a computer running the agent is
  serviced through adb: logs, system service dumps, app management and bug
  reports, which reach deeper than an app can.
- **iPhone over USB.** iOS does not allow an agent app with this kind of
  access, so iPhones are serviced through a computer running the agent, using
  libimobiledevice: device details, battery health, the system log, crash
  reports and backups.
- **Controller support** for listing and targeting the phones attached to an
  agent.

## Agent and controller features

### Safety and trust

- **Time-limited installs.** The agent removes itself after a set date or a
  period without use, so nothing is left behind when nobody remembers to
  uninstall it.
- **Visible sessions.** A notice on the device's screen while a technician is
  connected or a command is running.
- **Roles.** Read-only access (inventory, live load, logs) for some people,
  full control for others.
- **Disruptive actions confirmed by the agent**, not only by the controller,
  so every client has to confirm a reboot or reset explicitly.
- **Audit copies** kept on the technician's side as well as on the device.

### Remote work

- **Interactive shell** with a real terminal, for tools that need one, such as
  editors, interactive installers and `diskpart`.
- **Screenshots** of the signed-in user's desktop, and remote desktop through
  an existing tool carried over the tailnet.
- **Port forwarding** to services on the device or its local network, such as
  a router's admin page or a printer.
- **Folder transfers**, with progress and resume for large files.
- **Agent updates** pushed from the controller.

### Diagnostics

- **Customer report.** `dbc report` collects inventory, health checks and
  action results into a report to share with the customer, before and after a
  repair.
- **Hardware tests.** CPU and memory stress tests, disk speed tests, and
  scheduling the Windows memory diagnostic.
- **More actions on Linux and macOS**, to match the Windows catalogue.
- **Installed software and startup items** in `dbc info`.

### Controller

- **Shell completion** for commands and agent names.
- **Several agents at once.** Run a command or action on every agent matching
  a name or tag.
- **Dashboard.** A full-screen terminal view of every agent and its live load.

## Later

- Other devices reachable through an agent, such as routers, NAS boxes and
  printers on the customer's network.
- A web dashboard for technicians.
- Shared use by a team of technicians.
