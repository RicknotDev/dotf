.PHONY: all build test lint clean vet install uninstall coverage release

BINARY=dotf
GO=go
VERSION=v0.6.0
GOFLAGS=-ldflags="-s -w -X main.Version=$(VERSION)"
PREFIX=/usr/local
DIST_DIR=dist

all: vet build test

build:
	$(GO) build $(GOFLAGS) -o $(BINARY) ./cmd/dotf

test:
	$(GO) test -v -race -coverprofile=coverage.txt ./...

vet:
	$(GO) vet ./...

lint:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

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
