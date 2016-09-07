PROJECT ?= NONAME
VERSION ?= v0
REVISION ?= $(shell git rev-parse --short --verify HEAD)

RELEASE_GOVERSION=$(shell go version)
RELEASE_OS=$(word 1,$(subst /, ,$(lastword $(RELEASE_GOVERSION))))
RELEASE_ARCH=$(word 2,$(subst /, ,$(lastword $(RELEASE_GOVERSION))))

RELEASE_NAME=$(PROJECT)-$(VERSION)-$(RELEASE_OS)-$(RELEASE_ARCH)
RELEASE_DIR=$(PROJECT)-$(RELEASE_OS)-$(RELEASE_ARCH)

release: release-build
	rm -rf tmp/$(RELEASE_DIR)
	mkdir -p tmp/$(RELEASE_DIR)
	cp $(PROJECT)$(SUFFIX_EXE) tmp/$(RELEASE_DIR)/
	tar czf tmp/$(RELEASE_NAME).tar.gz -C tmp $(RELEASE_DIR)
	go clean

release-build:
	go clean
	GOOS=$(RELEASE_OS) GOARCH=$(RELEASE_ARCH) go build -ldflags='-X main.version=$(VERSION) -X main.revision=$(REVISION)'

release-all:
	@$(MAKE) release RELEASE_OS=windows RELEASE_ARCH=amd64 SUFFIX_EXE=.exe
	@$(MAKE) release RELEASE_OS=windows RELEASE_ARCH=386   SUFFIX_EXE=.exe
	@$(MAKE) release RELEASE_OS=linux   RELEASE_ARCH=amd64
	@$(MAKE) release RELEASE_OS=linux   RELEASE_ARCH=386
	#@$(MAKE) release RELEASE_OS=darwin  RELEASE_ARCH=amd64

.PHONY: release release-all
