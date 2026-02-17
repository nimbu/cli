SHELL := /bin/bash

.DEFAULT_GOAL := build

.PHONY: build run help fmt fmt-check lint test ci tools clean

BIN_DIR := $(CURDIR)/bin
BIN := $(BIN_DIR)/nimbu-cli
CMD := ./cmd/nimbu-cli

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo "")
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/nimbu/cli/internal/cmd.version=$(VERSION) \
           -X github.com/nimbu/cli/internal/cmd.commit=$(COMMIT) \
           -X github.com/nimbu/cli/internal/cmd.date=$(DATE)

TOOLS_DIR := $(CURDIR)/.tools
GOFUMPT := $(TOOLS_DIR)/gofumpt
GOIMPORTS := $(TOOLS_DIR)/goimports
GOLANGCI_LINT := $(TOOLS_DIR)/golangci-lint

# Allow passing CLI args:
#   make run -- --help
#   make run -- auth login
ifneq ($(filter run,$(MAKECMDGOALS)),)
RUN_ARGS := $(wordlist 2,$(words $(MAKECMDGOALS)),$(MAKECMDGOALS))
$(eval $(RUN_ARGS):;@:)
endif

build:
	@mkdir -p $(BIN_DIR)
	@go build -ldflags "$(LDFLAGS)" -o $(BIN) $(CMD)

run: build
	@if [ -n "$(RUN_ARGS)" ]; then \
		$(BIN) $(RUN_ARGS); \
	elif [ -z "$(ARGS)" ]; then \
		$(BIN) --help; \
	else \
		$(BIN) $(ARGS); \
	fi

help: build
	@$(BIN) --help

install: build
	@cp $(BIN) $(GOPATH)/bin/nimbu-cli
	@ln -sf $(GOPATH)/bin/nimbu-cli $(GOPATH)/bin/nb 2>/dev/null || true

tools:
	@mkdir -p $(TOOLS_DIR)
	@GOTOOLCHAIN=go1.24.0 GOBIN=$(TOOLS_DIR) go install mvdan.cc/gofumpt@v0.7.0
	@GOTOOLCHAIN=go1.24.0 GOBIN=$(TOOLS_DIR) go install golang.org/x/tools/cmd/goimports@v0.28.0
	@GOTOOLCHAIN=go1.24.0 GOBIN=$(TOOLS_DIR) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6

fmt: tools
	@$(GOIMPORTS) -local github.com/nimbu/cli -w .
	@$(GOFUMPT) -w .

fmt-check: tools
	@$(GOIMPORTS) -local github.com/nimbu/cli -w .
	@$(GOFUMPT) -w .
	@git diff --exit-code -- '*.go' go.mod go.sum

lint: tools
	@$(GOLANGCI_LINT) run

test:
	@go test ./...

ci: fmt-check lint test

clean:
	@rm -rf $(BIN_DIR) $(TOOLS_DIR)
