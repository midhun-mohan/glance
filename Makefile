BINARY_NAME=mygit
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X github.com/midhunmohan/mygit/internal/config.Version=$(VERSION)"

.PHONY: build run clean install test lint

build:
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/mygit

run: build
	./bin/$(BINARY_NAME)

install:
	go install $(LDFLAGS) ./cmd/mygit

clean:
	rm -rf bin/

test:
	go test ./...

lint:
	golangci-lint run ./...
