BINARY_NAME=glance
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X github.com/midhun-mohan/glance/internal/config.Version=$(VERSION)"

.PHONY: build run clean install test lint

build:
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/glance

run: build
	./bin/$(BINARY_NAME)

install:
	go install $(LDFLAGS) ./cmd/glance

clean:
	rm -rf bin/

test:
	go test ./...

lint:
	golangci-lint run ./...
