.PHONY: all build test lint fmt fmt-check e2e clean vet install uninstall coverage release

BINARY=dotf
GO=go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GOFLAGS=-ldflags="-s -w -X main.Version=$(VERSION)"
PREFIX=/usr/local
DIST_DIR=dist

all: lint build test

build:
	$(GO) build $(GOFLAGS) -o $(BINARY) ./cmd/dotf

# Tests run with a throwaway HOME so they can never touch your dotfiles.
test:
	HOME=$$(mktemp -d) $(GO) test -race -coverprofile=coverage.txt ./...

e2e: build
	./scripts/e2e.sh ./$(BINARY)

vet:
	$(GO) vet ./...

lint: fmt-check vet

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "run 'make fmt'"; exit 1; }

clean:
	rm -f $(BINARY)
	rm -rf $(DIST_DIR)
	rm -f coverage.txt

install: build
	install -d $(DESTDIR)$(PREFIX)/bin
	install -m 755 $(BINARY) $(DESTDIR)$(PREFIX)/bin/

uninstall:
	rm -f $(DESTDIR)$(PREFIX)/bin/$(BINARY)

coverage: test
	$(GO) tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report: coverage.html"

# Release targets
release: clean vet test build-all
	@echo "Release $(VERSION) ready in $(DIST_DIR)/"

build-all: build-linux-amd64 build-linux-arm64

build-linux-amd64:
	@mkdir -p $(DIST_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(DIST_DIR)/$(BINARY)-linux-amd64 ./cmd/dotf
	@echo "Built $(DIST_DIR)/$(BINARY)-linux-amd64"

build-linux-arm64:
	@mkdir -p $(DIST_DIR)
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o $(DIST_DIR)/$(BINARY)-linux-arm64 ./cmd/dotf
	@echo "Built $(DIST_DIR)/$(BINARY)-linux-arm64"
