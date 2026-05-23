.PHONY: build test lint clean release-dry run-ls

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/eslusarenko/port-client/internal/version.Version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/port ./cmd/port-client

test:
	go test -race ./...

lint:
	golangci-lint run

clean:
	rm -rf bin/ dist/

release-dry:
	goreleaser release --snapshot --clean

run-ls:
	go run ./cmd/port-client ls
