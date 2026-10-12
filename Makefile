.PHONY: help build clean run run-trace check fmt-check vet test tidy proto proto-lint proto-breaking docker-build docker-run docker-stop test-integration

.DEFAULT_GOAL := help

BIN := bin/z21-sim
ADDR ?= 127.0.0.1:21105
ALT_ADDR ?= 127.0.0.1:21106
GRPC_ADDR ?= 127.0.0.1:50051
IMAGE ?= ghcr.io/trains-io/z21-sim
TAG ?= local
CONTAINER ?= z21-sim
BUF_AGAINST ?= .git\#branch=main,subdir=api
export Z21_TESTSERVER_IMAGE ?= $(IMAGE):$(TAG)

help: ## Show targets
	@grep -E '^[a-zA-Z0-9_-]+:.*##' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*## "}; {printf "  %-18s %s\n", $$1, $$2}'

proto: ## Regenerate gRPC code from api/simctl/v1/simctl.proto (buf + protoc-gen-go, protoc-gen-go-grpc)
	buf generate

proto-lint: ## Lint the control API with buf
	buf lint

proto-breaking: ## Fail on incompatible control API changes against BUF_AGAINST (default: main)
	buf breaking --against '$(BUF_AGAINST)'

build: ## Compile binary to bin/
	go build -o $(BIN) ./cmd/z21-sim

clean: ## Remove build artifacts in bin/
	rm -rf bin/

run: build ## Run the simulator locally
	$(BIN) -addr $(ADDR) -alt-addr '$(ALT_ADDR)' -grpc-addr $(GRPC_ADDR)

run-trace: build ## Run with LAN and gRPC trace dumps on stderr
	$(BIN) -addr $(ADDR) -alt-addr '$(ALT_ADDR)' -grpc-addr $(GRPC_ADDR) -trace

check: fmt-check vet test ## Run the same checks as CI (gofmt, go vet, unit tests with -race)

fmt-check: ## Fail if any Go file needs gofmt
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "Files need gofmt:"; echo "$$unformatted"; exit 1; \
	fi

vet: ## Run go vet, including integration-tagged files
	go vet ./...
	go vet -tags=integration ./...

test: ## Run unit tests with the race detector
	go test -race ./... -count=1

test-integration: docker-build ## Run Docker integration tests
	go test -tags=integration ./integration/... -count=1 -timeout=10m

tidy: ## Tidy module dependencies
	go mod tidy

docker-build: ## Build the Docker image
	docker build -t $(IMAGE):$(TAG) .

docker-run: ## Start the simulator container detached
	docker run --rm -d --name $(CONTAINER) -p 21105:21105/udp -p 21106:21106/udp $(IMAGE):$(TAG)

docker-stop: ## Stop and remove the simulator container
	@docker rm -f $(CONTAINER) >/dev/null 2>&1 && echo "Stopped $(CONTAINER)" || echo "No container named $(CONTAINER)"
