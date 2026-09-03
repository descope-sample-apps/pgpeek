# Test Matrix

This matrix is the checklist for answering “is the product behavior covered?” Coverage gates still matter, but they only prove statements, branches, functions, and lines were executed. This file maps user-visible behavior to tests that assert the intended outcomes.

## Gates

| Gate | Command | What it proves |
| --- | --- | --- |
| Go unit + race | `go test -race -count=1 ./...` | Go packages pass under race detector. |
| Go integration coverage | `make cover-check` | `internal/...` remains at 100% statement coverage, including integration-tagged tests. |
| Web coverage | `make web-test` | Web statements, branches, functions, and lines remain at 100%. |
| Go build | `go build ./...` | All Go packages compile. |
| Go vet | `go vet ./...` | Standard Go static checks pass. |
| Lint | `make lint` | `golangci-lint` policy passes. |
| Vulnerabilities | `make vulncheck` | Called dependency symbols have no known vulnerabilities for the active Go toolchain. |

## Feature coverage

| Feature / surface | Covered behavior | Primary tests |
| --- | --- | --- |
| App startup and shutdown | config errors, DB connect errors, IAM path, TLS, graceful shutdown, store open errors | `main_test.go` |
| Configuration | single DB, multi-DB env/list/file parsing, defaults, bad values, secret redaction inputs | `internal/config/*_test.go` |
| Runtime DB safeguards | pool creation, DSN validation, ping/close, statement timeout, read-only session, row cap, value normalization | `internal/db/pool_test.go`, `internal/db/pool_integration_test.go` |
| Database registry | default selection, known/unknown DB IDs, private DSN handling, multi-DB metadata | `internal/db/registry_test.go`, `internal/server/registry_unit_test.go`, `internal/server/multi_database_integration_test.go` |
| Catalog listing | tables, columns, foreign keys, empty arrays, sanitized error responses | `internal/db/catalog_test.go`, `internal/db/pool_integration_test.go`, `internal/server/catalog_handlers_test.go` |
| Table data paging | limit/offset handling, row cap, negative offset clamp, query errors, rows errors | `internal/db/catalog_test.go`, `internal/db/pool_test.go`, `internal/db/pool_integration_test.go`, `web/app.test.js` |
| Table data search | global search across columns, text casting, search + filter composition, no-match path | `internal/db/catalog_test.go`, `internal/db/catalog_filter_test.go`, `web/app.test.js` |
| Table filters | `eq`, `ne`, `lt`, `lte`, `gt`, `gte`, `like`, `ilike`, `is_null`, `is_not_null`, bad column, bad operator | `internal/db/catalog_filter_test.go`, `internal/server/catalog_filter_handler_test.go`, `web/app.test.js`, `web/url-helpers.test.js` |
| Sorting | ascending/descending SQL generation, UI sort state, export URL state | `internal/db/catalog_test.go`, `web/app.test.js`, `web/url-helpers.test.js` |
| Query export | uncapped row iteration, gzip integrity, 512 MiB raw CSV cap, single active export, direct browser download, inline errors, CSRF and content-type rejection | `internal/db/pool_test.go`, `internal/server/query_handlers_test.go`, `internal/server/saved_query_errors_test.go`, `web/app.test.js`, `web/db-sql.test.js` |
| SQL execution and count | capped result rows, exact count without rows, per-statement gutter controls, selection/current-statement execution, cancellation, trailing comments/terminators, unsupported EXPLAIN count, stale responses, structured PostgreSQL errors, invalid JSON, DB errors | `internal/db/pool_test.go`, `internal/guard/guard_test.go`, `internal/server/query_handlers_test.go`, `web/app.test.js`, `web/db-sql.test.js`, `web/codemirror6-entry.test.js` |
| SQL guard | allowed read forms, rejected writes/multiple statements, comments, strings, dollar quotes, keyword masking | `internal/guard/guard_test.go` |
| MCP transport and tools | stateless initialize/version metadata, four structured read-only tools, database selection, SQL guardrails, sanitized failures, cross-origin rejection, 32 KiB request cap, batch rejection, 448 KiB output cap and truncation | `internal/server/mcp_test.go`, `internal/server/mcp_errors_test.go`, `internal/server/mcp_hardening_test.go`, `internal/server/mcp_output_test.go` |
| MCP Descope OAuth and DCR | canonical and alias env parsing, exact public `/mcp` resource, secure discovery/JWKS/DCR validation, asymmetric signing algorithms, redirect policy, issuer/audience/expiry/scope enforcement, public RFC 9728 metadata, Cloudflare composition, startup failure | `internal/config/config_test.go`, `internal/server/mcp_auth_test.go`, `main_test.go` |
| SQL editor autocomplete | CodeMirror mount, per-statement gutter updates/clicks, nested schema/alias completion, single bounded catalog request per database, cache/abort/fallback behavior, formatting, diagnostics, and fallback textarea mode | `internal/db/catalog_test.go`, `internal/server/catalog_handlers_test.go`, `web/app.test.js`, `web/codemirror6-entry.test.js`, `web/sql-tab-concurrency.test.js` |
| Structure tab | lazy column loading, loading/empty/error states, nullable/default variants, stale response handling | `web/app.test.js` |
| Foreign-key navigation | FK buttons, eq filter navigation, non-browsable refs, stale/failing FK introspection | `web/app.test.js` |
| Saved queries | SQLite CRUD, presets, ordering, validation, not found, bad IDs, closed-store errors | `internal/store/store_test.go`, `internal/server/saved_query_handlers_test.go`, `internal/server/saved_query_errors_test.go`, `web/app.test.js` |
| Database selector | render/no-render states, DB switching, stale response handling, compressed URL and API propagation, malformed responses | `web/db-selector.test.js`, `web/db-sql.test.js`, `web/url-state.test.js` |
| URL state | single-payload compression, legacy links, tab/table/db restoration, filters, sort, pagination, FK destinations, SQL round-trips, popstate, no SQL auto-run, malformed filters | `web/url-state.test.js`, `web/url-helpers.test.js`, `web/app.test.js` |
| Theme selector | default theme, persistence, clearing, localStorage failures | `web/app.test.js` |
| Large schema UI | many tables/columns remain scrollable, overflow boundaries are defined | `web/large-schema.test.js` |
| Health/readiness/static UI | `/healthz`, `/readyz`, static UI serving, security headers, default presets pass guard | `internal/server/server_routes_test.go` |
| AWS IAM auth | token generation success and loader/signer error paths | `internal/awsauth/awsauth_test.go` |

## How to add coverage for a new behavior

1. Add or update one row in this file before changing code.
2. Add the smallest test that would fail without the behavior.
3. If the behavior crosses layers, cover the lowest logic layer and the user-facing boundary.
4. Run the relevant focused test, then the gates above.

For example, a new table filter operator needs a DB SQL-builder test, an HTTP parser test, and a UI/API parameter test. Statement coverage alone is not enough because one executed branch can still miss an unsupported semantic combination.
