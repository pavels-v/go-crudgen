# go-crudgen developer tasks. Run `make help` for the list.

BINARY := go-crudgen
CMD    := ./cmd/go-crudgen

.DEFAULT_GOAL := help

.PHONY: help build test e2e integration verify-examples lint lint-fix fmt tidy vet clean

help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

build: ## Build the CLI binary (named go-crudgen)
	go build -o $(BINARY) $(CMD)

test: ## Run tests
	go test ./...

e2e: ## Run end-to-end tests (regenerates examples/blog from its spec, then builds & tests it)
	go test -tags e2e -count=1 -v ./test/e2e

integration: ## Run examples/blog integration tests against real Postgres via testcontainers (requires Docker)
	cd examples/blog/integration && go test -count=1 -v ./...

verify-examples: e2e ## Regenerate examples via e2e and fail if the committed output is stale (CI gate)
	git diff --exit-code examples/blog

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
