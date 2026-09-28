# go-crudgen developer tasks. Run `make help` for the list.

BINARY := go-crudgen
CMD    := ./cmd/go-crudgen
TOOL   := go tool -modfile="$(CURDIR)/tools/go.mod"
LINT   := $(TOOL) golangci-lint
VULN   := $(TOOL) govulncheck
LINTC  := -c "$(CURDIR)/.golangci.yml"
MODS   := . examples/blogservice examples/blogservice/integration

.DEFAULT_GOAL := help

.PHONY: help build test e2e integration verify-examples lint lint-fix fmt vuln tidy vet clean

help: ## List available targets
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

build: ## Build the CLI binary (named go-crudgen)
	go build -o $(BINARY) $(CMD)

test: ## Run tests
	go test ./...

e2e: ## Run end-to-end tests (regenerates examples/blogservice; builds, vets, tests and lints a flag matrix)
	go test -tags e2e -count=1 -v ./test/e2e

integration: ## Run examples/blogservice integration tests against real Postgres via testcontainers (requires Docker)
	cd examples/blogservice/integration && go test -count=1 -v ./...

verify-examples: e2e ## Regenerate examples via e2e and fail if the committed output is stale
	@git status --short examples/blogservice
	@test -z "$$(git status --porcelain examples/blogservice)"

lint: ## Run golangci-lint (pinned in tools/go.mod) on every module
	@for m in $(MODS); do (cd $$m && $(LINT) run $(LINTC) ./...) || exit 1; done

lint-fix: ## Run golangci-lint with autofixes applied
	@for m in $(MODS); do (cd $$m && $(LINT) run $(LINTC) --fix ./...) || exit 1; done

fmt: ## Format code (gofmt + goimports via golangci-lint formatters)
	@for m in $(MODS); do (cd $$m && $(LINT) fmt $(LINTC) ./...) || exit 1; done

vuln: ## Scan every module for known vulnerabilities (govulncheck pinned in tools/go.mod)
	@for m in $(MODS); do (cd $$m && $(VULN) ./...) || exit 1; done

vet: ## Run go vet
	go vet ./...

tidy: ## Tidy and verify module dependencies
	go mod tidy
	go mod verify

clean: ## Remove build artifacts
	go clean
	rm -f $(BINARY) $(BINARY).exe
