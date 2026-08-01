.DEFAULT_GOAL := help
SHELL := bash
GO ?= go
PROFILE ?= cover.out
VERSION ?= $(shell tag=$$(git describe --tags --exact-match 2>/dev/null) && printf '%s' "$${tag#v}" || { base=$$(git describe --tags --abbrev=0 2>/dev/null || printf 'v0.0.0'); printf '%s-dev+%s' "$${base#v}" "$$(git rev-parse --short=12 HEAD 2>/dev/null || printf 'unknown')"; })
COMMIT ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || printf 'unknown')
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)

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
	npm ci && npm run vendor && npx vitest run --coverage

.PHONY: web-vendor
web-vendor: ## Regenerate the vendored CodeMirror 6 bundle (esbuild)
	npm ci --ignore-scripts && npm run vendor

.PHONY: docs-check
docs-check: ## Validate agent-readable docs, counts, links, and full-context sync
	node scripts/check-agent-docs.mjs

.PHONY: docs-css
docs-css: ## Synchronize inline critical CSS and deferred documentation styles
	node scripts/build-docs-css.mjs

.PHONY: build
build: web-vendor ## Build the static binary
	CGO_ENABLED=0 GOFIPS140=v1.0.0 GODEBUG=fips140=on $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o pgpeek .

.PHONY: image
image: ## Build a snapshot image via goreleaser+ko
	goreleaser build --snapshot --clean --single-target

.PHONY: run
run: ## Run locally (requires DATABASE_URL)
	$(GO) run .

.PHONY: ci
ci: lint vet cover-check vulncheck web-test docs-check ## Everything CI runs
