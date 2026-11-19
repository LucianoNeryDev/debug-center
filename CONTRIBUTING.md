# Contributing

Issues and pull requests are welcome. Debug Center is maintained for its
author's own support work, so larger changes are easier to land after a short
issue that describes the problem first.

Security problems go privately to the address in [SECURITY.txt](SECURITY.txt),
not into an issue.

## Building

You need Go at the version in `go.mod`. Nothing else: the binaries build with
`CGO_ENABLED=0`.

```sh
go build ./cmd/dbc-agent ./cmd/dbc   # this platform
scripts/build.sh                     # every platform, into dist/, with SHA256SUMS
```

[Builds and CI](docs/ci-cd.md) describes the build variables and the CI
artifacts.

## Before opening a pull request

Run the checks CI runs, plus `gofmt`:

```sh
gofmt -l .               # prints nothing when everything is formatted
go vet ./...
go test ./...
scripts/build.sh         # the code must build for Windows, Linux and macOS
```

`scripts/build.sh` is the quickest way to catch code that only compiles on
your own OS. For one platform, `GOOS=windows go build ./...` is enough.

## Branches and pull requests

- `dev` is the default branch and the integration branch.
- Work on a branch off `dev` named for the change: `feat/<topic>` for features,
  `fix/<topic>` for fixes.
- Open the pull request into `dev`. CI vets, tests and builds every pull
  request, and it must pass before merging.
- Add a line under **Unreleased** in [CHANGELOG.md](CHANGELOG.md) for anything a
  user or a device owner would notice.

Commit messages follow the existing history: a short subject in the imperative
("Add the dbc controller CLI", "Build every platform in CI and keep the
binaries"), no trailing period, then a body wrapped at about 76 columns that
says what changed and why.

## Conventions

- **Layout.** Programs live in `cmd/` (`dbc-agent`, `dbc`); everything else is
  in `internal/`. The wire format between them is in `internal/proto`; bump
  `proto.APIVersion` when a change breaks older controllers or agents.
- **Platform code.** OS differences go in files with build constraints
  (`exec_windows.go`, `exec_unix.go`, `actions_linux.go`, `actions_other.go`),
  each exposing the same functions, not in runtime `GOOS` checks.
- **Every endpoint is authorized.** Handlers sit behind `authorize`, which
  checks the caller's tailnet identity. Nothing is served outside it.
- **Audit before acting.** A handler records its request with `record` before
  it does anything. Handlers that run commands, kill processes, write files or
  remove the agent must refuse the request (`refuse`) when the audit line
  cannot be written; read-only handlers carry on. Requests the agent turns
  down are recorded with `allowed` set to false.
- **Actions.** Catalogue entries are declared with `def` in
  `actions_<os>.go`. Parameters are written as `{{1}}`, `{{2}}`… and are
  quoted for the action's shell; never build a command line by concatenating
  technician input. Mark entries that interrupt the user with `dangerous` and
  long-running ones with `slow`.
- **Errors.** Return errors with context (`fmt.Errorf("join tailnet: %w",
  err)`); HTTP handlers answer with `writeErr`, which sends a JSON
  `proto.Error`.
- **Comments.** Exported identifiers have doc comments, and comments explain
  why a choice was made rather than restating the code.
- **Dependencies.** Prefer the standard library. A new module needs a reason
  in the pull request.
- **Tests.** Standard `testing`, table-driven where there are several cases.
  Tests use temporary directories and loopback servers only: no tailnet, no
  internet, no changes outside the test's own files.

## License

By contributing you agree that your contribution is released under the
[MIT license](LICENSE) of the project.
