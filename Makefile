.PHONY: all build test lint clean vet

BINARY=dotf
GO=go
GOFLAGS=-ldflags="-s -w"
PREFIX=/usr/local

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
	rm -f coverage.txt

install: build
	install -d $(DESTDIR)$(PREFIX)/bin
	install -m 755 $(BINARY) $(DESTDIR)$(PREFIX)/bin/

uninstall:
	rm -f $(DESTDIR)$(PREFIX)/bin/$(BINARY)

coverage: test
	$(GO) tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report: coverage.html"
