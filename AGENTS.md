# AGENTS.md

Go module `github.com/koron/sqs-notify/v2` (go 1.26). CLI that polls an SQS
queue and executes a command per message (message body on STDIN).

## Layout

- Root package (`main.go`): the `sqs-notify` v2 CLI (flag parsing, daemon,
  log/pidfile, graceful shutdown wiring).
- `internal/sqsnotify`: core library (queue polling, workers, remove
  policies, config).
- `internal/cache`: message cache (`memory://` and `redis://` backends);
  `internal/stage`: cache-entry lifecycle states.
- `cmd/sqs-echo`, `cmd/sqs-send`: helper CLIs. `cmd/daemon-demo`: shell
  scripts only (demo for daemon/log rotation; no Go code).
- `doc/v1.md`: docs for the removed legacy v1; do not resurrect v1 code.
- CI releases every `main` package (root `sqs-notify`, `sqs-echo`,
  `sqs-send`); drop a `.norelease` file in a package dir to opt it out.

## Commands

- `make build` — `go build -gcflags '-e' ./...`
- `make test` — `go test ./...` (`make race` adds `-race`)
- Single test: `go test ./internal/sqsnotify/ -run TestName`
- `make checkall` — `go vet` + `staticcheck` (requires the `staticcheck`
  binary; `staticcheck.conf` enables all checks)
- CI (`.github/workflows/go.yml`, on push, multi-OS): only runs
  `go test ./...` plus release builds. No lint in CI.

## Tests

- No external services needed: `internal/sqsnotify` tests spin up a local
  GoAWS mock SQS server (`servertest`) on `127.0.0.1:0` with mock AWS
  credentials set inside the tests; `internal/cache` redis tests use
  miniredis.
