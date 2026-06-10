.DEFAULT_GOAL := help

IMAGE_NAME ?= ghcr.io/aetherpak/cli
TAG ?= local
CONTAINER_TOOL ?= $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)

.PHONY: build test test/integration test/container fmt vet clean release/build container container/cli container/builder setup lint help

##@ Build & Quality

VERSION ?= git+$(shell git rev-parse --short HEAD 2>/dev/null || echo dev)

build: ## Build the aetherpak CLI binary into bin/
	@mkdir -p bin
	go build -ldflags="-X github.com/aetherpak/aetherpak/cmd.Version=$(VERSION)" -o bin/aetherpak main.go

test: ## Run the unit test suite
	go test -v ./...

test/integration: ## Run E2E integration tests (requires docker/podman and compose)
	go test -tags=integration -v ./tests/...

test/container: container/builder ## Run E2E container smoke tests using locally built builder image
	./tests/smoke_container.sh $(IMAGE_NAME):$(TAG)-builder

fmt: ## Format Go source files
	go fmt ./...

vet: ## Report suspicious constructs
	go vet ./...

clean: ## Remove build artifacts
	rm -rf bin/ dist/

##@ Quality

setup: ## Install the pre-commit git hooks
	pre-commit install || uvx pre-commit install

lint: ## Run all pre-commit checks
	pre-commit run --all-files || uvx pre-commit run --all-files

##@ Containers

container: | container/cli container/builder ## Build both CLI and Builder container images

container/cli: ## Build local single-architecture CLI image
	@[ -n "$(CONTAINER_TOOL)" ] || { printf "Error: Neither podman nor docker found in PATH\n"; exit 1; }
	$(CONTAINER_TOOL) build --target cli --build-arg VERSION=$(VERSION) -t $(IMAGE_NAME):$(TAG) -f Containerfile .

container/builder: ## Build local single-architecture builder image
	@[ -n "$(CONTAINER_TOOL)" ] || { printf "Error: Neither podman nor docker found in PATH\n"; exit 1; }
	$(CONTAINER_TOOL) build --target cli-builder --build-arg VERSION=$(VERSION) -t $(IMAGE_NAME):$(TAG)-builder -f Containerfile .

##@ Release

release/build: ## Cross-compile release binaries and archives into dist/ (Linux only)
	@mkdir -p dist
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X github.com/aetherpak/aetherpak/cmd.Version=$(VERSION)" -o dist/aetherpak-linux-amd64 main.go
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w -X github.com/aetherpak/aetherpak/cmd.Version=$(VERSION)" -o dist/aetherpak-linux-arm64 main.go
	cd dist && tar -czf aetherpak-linux-amd64.tar.gz aetherpak-linux-amd64
	cd dist && tar -czf aetherpak-linux-arm64.tar.gz aetherpak-linux-arm64

##@ Utilities

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} \
	  /^[a-zA-Z0-9_/-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } \
	  /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
