# AGENTS.md

Go module `github.com/koron/sqs-notify` (go 1.26). CLI that polls an SQS queue
and executes a command per message (message body on STDIN).

## Layout

- Root package (`main.go`, `jobs.go`, `daemon.go`, `config.go`): legacy v1.
  Docs: `doc/v1.md`. Do not extend v1; new work goes to v2.
- `sqsnotify2/`: v2 library. `cmd/sqs-notify2/`: current entrypoint
  (installed via `go install github.com/koron/sqs-notify/cmd/sqs-notify2@latest`).
- `cmd/sqs-echo`, `cmd/sqs-send`: helper CLIs. `cmd/daemon-demo`: shell
  scripts only (demo for daemon/log rotation; no Go code).
- `awsutil/`: credential loading (platform-split `aws_windows.go` /
  `aws_others.go`).
- The root `.norelease` file excludes the legacy v1 binary from CI release
  builds; CI releases every other `main` package.

## Commands

- `make build` — `go build ./...`
- `make test` — `go test ./...`
- Single test: `go test ./sqsnotify2/ -run TestName`
- `make checkall` — `go vet` + `staticcheck` (requires `staticcheck` binary;
  `staticcheck.conf` enables all checks)
- CI (`.github/workflows/go.yml`, on push, multi-OS): only runs
  `go test ./...` plus release builds. No lint in CI.

## Tests

- No external services needed: `sqsnotify2` tests spin up a local GoAWS mock
  SQS server (`servertest`) on `127.0.0.1:0` with mock AWS credentials.
- Redis cache tests in `sqsnotify2/cache_test.go` skip unless `REDIS_URL` is set.
