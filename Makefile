.PHONY: help build run test tidy proto

.DEFAULT_GOAL := help

BIN := bin/z21-sim-go
ADDR ?= 127.0.0.1:21105
GRPC_ADDR ?= 127.0.0.1:50051

help: ## Show targets
	@grep -E '^[a-zA-Z0-9_-]+:.*##' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*## "}; {printf "  %-12s %s\n", $$1, $$2}'

proto: ## Regenerate gRPC code from api/control/v1/control.proto
	protoc --go_out=. --go_opt=module=github.com/trains-io/z21-sim-go \
		--go-grpc_out=. --go-grpc_opt=module=github.com/trains-io/z21-sim-go \
		api/control/v1/control.proto

build: ## Compile binary to bin/
	go build -o $(BIN) ./cmd/z21-sim-go

run: build ## Run the simulator (default 127.0.0.1:21105, gRPC 127.0.0.1:50051)
	$(BIN) -addr $(ADDR) -grpc-addr $(GRPC_ADDR)

test: ## Run unit tests
	go test ./... -count=1

tidy: ## Tidy module dependencies
	go mod tidy
