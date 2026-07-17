.PHONY: help build run test tidy

.DEFAULT_GOAL := help

BIN := bin/z21-sim-go
ADDR ?= 127.0.0.1:21105

help: ## Show targets
	@grep -E '^[a-zA-Z0-9_-]+:.*##' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*## "}; {printf "  %-12s %s\n", $$1, $$2}'

build: ## Compile binary to bin/
	go build -o $(BIN) ./cmd/z21-sim-go

run: build ## Run the simulator (default 127.0.0.1:21105)
	$(BIN) -addr $(ADDR)

test: ## Run unit tests
	go test ./... -count=1

tidy: ## Tidy module dependencies
	go mod tidy
