PROJECT = sqs-notify
VERSION = v1.5.1

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
