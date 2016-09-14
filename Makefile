PROJECT = sqs-notify
VERSION = v1.5.2
RELEASE_TARGETS = release-windows-amd64 release-windows-386 release-linux-amd64 release-linux-386


default: test

test:
	go test ./...

test-full:
	go test -v -race ./...

lint:
	-go vet ./...
	@echo ""
	-golint ./...

cyclo:
	-gocyclo -top 10 -avg .

report:
	@echo "misspell"
	@find . -name "*.go" | xargs misspell
	@echo ""
	-errcheck ./...
	@echo ""
	-gocyclo -over 14 -avg .
	@echo ""
	-go vet ./...
	@echo ""
	-golint ./...

-include Mk/*.mk

.PHONY: test test-full lint cyclo report
