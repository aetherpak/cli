.DEFAULT_GOAL := help

.PHONY: build test test/integration fmt vet clean release/build help

##@ Build & Quality

build: ## Build the aetherpak CLI binary into bin/
	@mkdir -p bin
	go build -o bin/aetherpak main.go

test: ## Run the unit test suite
	go test -v ./...

test/integration: ## Run E2E integration tests (requires docker/podman and compose)
	go test -tags=integration -v ./tests/...

fmt: ## Format Go source files
	go fmt ./...

vet: ## Report suspicious constructs
	go vet ./...

clean: ## Remove build artifacts
	rm -rf bin/ dist/

##@ Release

release/build: ## Cross-compile release binaries and archives into dist/
	@mkdir -p dist
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/aetherpak-linux-amd64 main.go
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o dist/aetherpak-linux-arm64 main.go
	GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o dist/aetherpak-darwin-amd64 main.go
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o dist/aetherpak-darwin-arm64 main.go
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o dist/aetherpak-windows-amd64.exe main.go
	GOOS=windows GOARCH=arm64 go build -ldflags="-s -w" -o dist/aetherpak-windows-arm64.exe main.go
	cd dist && tar -czf aetherpak-linux-amd64.tar.gz aetherpak-linux-amd64
	cd dist && tar -czf aetherpak-linux-arm64.tar.gz aetherpak-linux-arm64
	cd dist && tar -czf aetherpak-darwin-amd64.tar.gz aetherpak-darwin-amd64
	cd dist && tar -czf aetherpak-darwin-arm64.tar.gz aetherpak-darwin-arm64
	cd dist && zip aetherpak-windows-amd64.zip aetherpak-windows-amd64.exe
	cd dist && zip aetherpak-windows-arm64.zip aetherpak-windows-arm64.exe

##@ Utilities

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} \
	  /^[a-zA-Z0-9_/-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } \
	  /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
