TEST_PACKAGE ?= ./...

ROOT_PACKAGE ?= $(shell go list -f '{{.ImportPath}}' .)
MAIN_PACKAGE ?= $(shell go list -f '{{if (eq .Name "main")}}{{.ImportPath}}{{end}}' ./...)
MAIN_DIRS = $(subst $(ROOT_PACKAGE),.,$(MAIN_PACKAGE))

.PHONY: build
build:
	go build -gcflags '-e' ./...

.PHONY: test
test:
	go test $(TEST_PACKAGE)

.PHONY: test-race
test-full:
	go test -race $(TEST_PACKAGE)

.PHONY: bench
bench:
	go test -bench $(TEST_PACKAGE)

.PHONY: tags
tags:
	gotags -f tags -R .

.PHONY: cover
cover:
	mkdir -p tmp
	go test -coverprofile tmp/_cover.out $(TEST_PACKAGE)
	go tool cover -html tmp/_cover.out -o tmp/cover.html

.PHONY: checkall
checkall: vet staticcheck

.PHONY: vet
vet:
	go vet $(TEST_PACKAGE)

.PHONY: staticcheck
staticcheck:
	staticcheck $(TEST_PACKAGE)

.PHONY: clean
clean:
	go clean
	rm -f tags
	rm -f tmp/_cover.out tmp/cover.html

list-upgradable-modules:
	@go list -m -u -f '{{if .Update}}{{.Path}} {{.Version}} [{{.Update.Version}}]{{end}}' all

.PHONY: main-build
main-build:
	@for d in $(MAIN_DIRS) ; do \
	  echo "cd $$d && go build -gcflags '-e'" ; \
	  ( cd $$d && go build -gcflags '-e' ) ; \
	done

.PHONY: main-clean
main-clean:
	@for d in $(MAIN_DIRS) ; do \
	  echo "cd $$d && go clean" ; \
	  ( cd $$d && go clean ) ; \
	done

# based on: github.com/koron-go/_skeleton/Makefile
# $Hash:b63de31e279757142e514446f73e70d9c71185931db55f3c6e688955$
