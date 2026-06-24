.DEFAULT_GOAL := help
SHELL := bash
GO ?= go
PROFILE ?= cover.out

# PGPEEK_TEST_DATABASE_URL enables the db/main integration tests. Override as needed:
#   make test-integration PGPEEK_TEST_DATABASE_URL=postgres://...
# The default is applied only by integration/coverage targets so `make test`
# remains unit-only unless the caller exports PGPEEK_TEST_DATABASE_URL.
PGPEEK_TEST_DATABASE_URL ?= postgres://postgres:secret@localhost:55432/testdb?sslmode=disable
INTEGRATION_ENV := PGPEEK_TEST_DATABASE_URL="$(PGPEEK_TEST_DATABASE_URL)"

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: fmt
fmt: ## Format Go code
	gofmt -w .

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

.PHONY: vet
vet: ## go vet
	$(GO) vet ./...

.PHONY: test
test: ## Unit tests with race detector
	$(GO) test -race ./...

.PHONY: test-integration
test-integration: ## Tests incl. integration (needs Postgres)
	$(INTEGRATION_ENV) $(GO) test -race -tags=integration ./...

.PHONY: cover
cover: ## Full coverage profile (incl. integration)
	$(INTEGRATION_ENV) $(GO) test -race -tags=integration -coverpkg=./... -coverprofile=$(PROFILE) ./...
	$(GO) tool cover -func=$(PROFILE) | tail -1

.PHONY: cover-check
cover-check: cover ## Enforce 100% coverage on internal/...
	./scripts/check-coverage.sh $(PROFILE)

.PHONY: cover-html
cover-html: cover ## Open the HTML coverage report
	$(GO) tool cover -html=$(PROFILE)

.PHONY: vulncheck
vulncheck: ## Scan for known vulnerabilities
	govulncheck ./...

.PHONY: web-test
web-test: ## Front-end tests (vitest, 100% thresholds)
	npm ci && npx vitest run --coverage

.PHONY: web-vendor
web-vendor: ## Regenerate the vendored CodeMirror 6 bundle (esbuild)
	npm ci && npm run vendor

.PHONY: build
build: ## Build the static binary
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="-s -w" -o pgpeek .

.PHONY: image
image: ## Build a snapshot image via goreleaser+ko
	goreleaser build --snapshot --clean --single-target

.PHONY: run
run: ## Run locally (requires DATABASE_URL)
	$(GO) run .

.PHONY: ci
ci: lint vet cover-check vulncheck web-test ## Everything CI runs
