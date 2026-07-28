BINARY  := claude-sessions
PKG     := ./cmd/claude-sessions
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
.PHONY: build test lint arch clean install

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) $(PKG)

test:
	go test ./...

lint:
	go vet ./...

arch:
	go test ./internal/architecture

clean:
	rm -f $(BINARY)

install:
	go install -ldflags "-X main.version=$(VERSION)" $(PKG)
