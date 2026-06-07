# go-crudgen developer tasks. Run `make help` for the list.

BINARY := go-crudgen
CMD    := ./cmd/cli

.DEFAULT_GOAL := help

.PHONY: help build test lint lint-fix fmt tidy vet clean

help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

build: ## Build the CLI binary (named go-crudgen)
	go build -o $(BINARY) $(CMD)

test: ## Run tests
	go test ./...

lint: ## Run golangci-lint (requires golangci-lint v2.x)
	golangci-lint run ./... -v

lint-fix: ## Run golangci-lint with autofixes applied
	golangci-lint run --fix ./...

fmt: ## Format code (gofmt + goimports via golangci-lint formatters)
	golangci-lint fmt ./... -v

vet: ## Run go vet
	go vet ./...

tidy: ## Tidy and verify module dependencies
	go mod tidy
	go mod verify

clean: ## Remove build artifacts
	go clean
	rm -f $(BINARY) $(BINARY).exe
