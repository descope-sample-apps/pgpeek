# pgpeek

A minimal, **read-only**, team-shared Postgres browser. Built to replace Adminer
for the support team. pgweb-style browsing — a **sidebar of tables/views** you
click to page through rows, a **Structure** tab showing each table's columns, and
a **SQL** tab for free-form `SELECT`s with saved/preset queries — plus CSV export
everywhere. Read-only by design: no row editing, schema management, or migrations.

## What it looks like

```
┌─ tables ──────┬─ Data │ Structure │ SQL ─────────────┐
│ 🔍 filter…    │  id   email          created_at       │
│ public        │  1    a@x.com        2026-01-02…      │
│  • users  ◀   │  2    b@y.com        2026-01-03…      │
│  • companies  │  …                                    │
│ auth          │  ◀ Prev   1–100   Next ▶   [Export]   │
│  • sessions   │                                       │
└───────────────┴───────────────────────────────────────┘
```

- **Data** tab — click a table → paged rows (Prev/Next). A global search box
  (matches any column), per-column filters with operators (`=`, `≠`, `<`, `>`,
  `ILIKE`, `IS NULL`, …), and click-to-sort headers. CSV export respects the
  active search/filters/sort.
- **Structure** tab — column name, type, nullable, default.
- **SQL** tab — CodeMirror editor, saved/preset queries, CSV export.

Filtering is safe by construction: column names are validated against the
relation's real columns and emitted via `pgx.Identifier`, operators come from a
fixed allowlist, values are bound as query parameters, and sort is `ASC`/`DESC`
only — no user input is ever concatenated into SQL.

It exists because Adminer kept falling over. pgpeek avoids those failure modes
on purpose:

- **Connection pooling** (`pgx`/pgxpool) — not a new connection per request.
- **Row cap** — results stop at `PGPEEK_ROW_CAP` rows; an enormous result set is
  never fully buffered into memory. The UI tells you when output was capped.
- **Statement timeout** — `statement_timeout` is set on every pooled session, so
  a runaway query can't wedge a pod.
- **Stateless pods** — saved queries live in a small SQLite file on a PVC, not in
  pod memory. The app process holds no per-user state.

## Architecture

```
browser ── HTTP ──> pgpeek (Go, single static binary)
                       │  pgx pool ──> Postgres (Aurora, read-only role)
                       └  SQLite file ──> saved/preset queries  (on a PVC)
```

- **Backend**: Go, `jackc/pgx/v5` for Postgres, `modernc.org/sqlite` (pure Go, no
  cgo) for the saved-query store → static binary, ~25 MB distroless image.
- **Frontend**: one `web/index.html` — CodeMirror editor (CDN, degrades to a
  textarea), results table, saved-query dropdown, CSV button. Embedded into the
  binary via `go:embed`.

## Read-only enforcement (defense in depth)

1. **The real boundary**: the DB role (`descoperead`) has no write privileges.
   That's what actually keeps the data safe.
2. **Session-level**: pgpeek sets `default_transaction_read_only=on` on every
   pooled connection.
3. **App-layer guardrail** (`internal/guard`): rejects anything that isn't a
   single `SELECT`/`WITH`/`VALUES`/`TABLE`/`EXPLAIN` statement — blocks multiple
   statements and DML/DDL keywords, ignoring keywords that appear inside comments
   or string literals. This is a guardrail against fat-fingering, **not** the
   security boundary. Don't rely on it as one.

## Configuration (env vars)

Everything is configured via the environment. Any value can also be supplied
from a **mounted file** by setting `<VAR>_FILE` to a path (Docker secrets / k8s
projected volumes); the file's trimmed contents become the value. This is wired
for the secret-bearing `DATABASE_URL` (use `DATABASE_URL_FILE`).

| Variable                     | Default              | Notes                                                                 |
| ---------------------------- | -------------------- | --------------------------------------------------------------------- |
| `DATABASE_URL`               | _(required)_         | Postgres DSN. Use the read-only role. **Never logged.** Aurora: include `?sslmode=require`. |
| `DATABASE_URL_FILE`          | —                    | Path to a file holding the DSN (mounted-secret alternative).          |
| `PGPEEK_LISTEN`              | `:8080`              | Listen address.                                                       |
| `PGPEEK_ROW_CAP`             | `1000`               | Max rows returned/exported per query.                                 |
| `PGPEEK_STATEMENT_TIMEOUT`   | `30s`                | Per-query DB statement timeout.                                       |
| `PGPEEK_IDLE_TX_TIMEOUT`     | `30s`                | `idle_in_transaction_session_timeout`.                                |
| `PGPEEK_MAX_CONNS`           | `8`                  | Max pool size (caps DB connection usage).                             |
| `PGPEEK_STORE_PATH`          | `/data/pgpeek.db`    | SQLite file for saved queries.                                        |
| `PGPEEK_READ_HEADER_TIMEOUT` | `10s`                | HTTP read-header timeout.                                             |
| `PGPEEK_WRITE_TIMEOUT`       | `statementTimeout+30s` | HTTP write timeout (must exceed statement timeout for big exports). |
| `PGPEEK_IDLE_TIMEOUT`        | `120s`               | HTTP keep-alive idle timeout.                                         |
| `PGPEEK_SHUTDOWN_TIMEOUT`    | `15s`                | Graceful-shutdown grace period.                                       |
| `PGPEEK_TLS_CERT_FILE`       | —                    | Enable HTTPS (set together with the key). Otherwise serve plain HTTP behind a TLS-terminating ingress. |
| `PGPEEK_TLS_KEY_FILE`        | —                    | TLS private key path.                                                 |
| `PGPEEK_DB_IAM_AUTH`         | `false`              | Use RDS/Aurora IAM auth instead of a password (see below).            |
| `PGPEEK_AWS_REGION`          | `$AWS_REGION`        | AWS region for IAM token signing (required when IAM auth is on).      |

### RDS / Aurora IAM authentication

Set `PGPEEK_DB_IAM_AUTH=true` and `PGPEEK_AWS_REGION`. The `DATABASE_URL` then
needs only host/port/user/dbname (no password) and `sslmode=require`. pgpeek
mints a short-lived IAM auth token from the default AWS credential chain
(env / web-identity / **IRSA** / instance role) **before every new connection**,
so tokens never go stale and no static DB password is stored anywhere.

```bash
export PGPEEK_DB_IAM_AUTH=true
export PGPEEK_AWS_REGION=us-east-1
export DATABASE_URL='postgres://descoperead@your-cluster.cluster-xxxx.us-east-1.rds.amazonaws.com:5432/yourdb?sslmode=require'
```

In k8s, attach an IRSA-annotated ServiceAccount (see `k8s/serviceaccount.yaml`)
whose role has `rds-db:connect` on the `descoperead` DB user.

## Run locally

```bash
export DATABASE_URL='postgres://descoperead:PASSWORD@host:5432/db?sslmode=require'
export PGPEEK_STORE_PATH=./pgpeek.db
go run .
# open http://localhost:8080
```

Keyboard: **Ctrl/Cmd + Enter** runs the query.

## Build

```bash
make build                 # static binary (CGO disabled)
make image                 # snapshot distroless image via goreleaser + ko
docker build -t pgpeek .   # alternative: hand-written multi-stage Dockerfile
```

Release images are built with [ko](https://ko.build) inside goreleaser
(distroless, multi-arch, reproducible, with SBOMs) — see [Releases](#releases).

## Testing & quality

The backend is at **100% statement coverage** on every package under
`internal/` (guard, db, store, server, config, awsauth); the front-end
(`web/app.js`) is at **100%** lines/branches/functions via vitest. `package main`
is thin bootstrap, exercised by integration tests.

```bash
make test               # unit tests, race detector
make test-integration   # + db/main integration tests (needs Postgres)
make cover-check        # full coverage profile, fail if internal/ < 100%
make lint               # golangci-lint (errcheck, gosec, revive, …)
make vulncheck          # govulncheck
make web-test           # vitest --coverage (100% thresholds)
make ci                 # everything above
```

A throwaway Postgres for integration/coverage:

```bash
docker run -d --name pg -e POSTGRES_PASSWORD=secret -e POSTGRES_DB=testdb -p 55432:5432 postgres:16
make cover-check        # uses PGPEEK_TEST_DATABASE_URL (default points at :55432)
```

## Releases

- **release-please** watches `main` for [Conventional Commits](https://www.conventionalcommits.org)
  and maintains a release PR (version bump + `CHANGELOG.md`). Merging it tags
  `vX.Y.Z` and cuts a GitHub Release.
- The tag triggers **goreleaser** (`.goreleaser.yaml`), which builds the
  binaries and uses **ko** to publish multi-arch distroless images to
  `ghcr.io/descope/pgpeek:{version,major.minor,latest}` with SBOMs.

CI (`.github/workflows/ci.yml`) runs lint, vet, race tests with a Postgres
service, the 100% coverage gate, govulncheck, the vitest suite, and a snapshot
image build on every PR.

## Deploy to k8s

Manifests live in [`k8s/`](k8s/): `Deployment`, `Service`, `PersistentVolumeClaim`,
an optional `Ingress`, and a `secret.example.yaml`.

```bash
# 1. Create the DB secret out-of-band (do NOT commit it):
kubectl create secret generic pgpeek-db \
  --from-literal=DATABASE_URL='postgres://descoperead:PASSWORD@your-aurora-host:5432/yourdb?sslmode=require'

# 2. Apply the rest:
kubectl apply -k k8s/
```

The pod runs as non-root with a read-only root filesystem (only `/data` is
writable), drops all capabilities, and has liveness (`/healthz`) and readiness
(`/readyz`) probes.

### A note on scaling

The saved-query store is a SQLite file on a **ReadWriteOnce** PVC, so the
Deployment ships with `replicas: 1` and a `Recreate` strategy. The query path
itself is stateless. To scale horizontally, move the saved-query store to a
shared backend (a dedicated schema in Postgres, or an RWX volume) and bump
`replicas` — see comments in `k8s/pvc.yaml`.

### Auth

pgpeek is intentionally **auth-thin** — put it behind your existing SSO. The
example `Ingress` assumes oauth2-proxy (Entra/Google SAML). **Do not expose
pgpeek without an auth layer in front of it.**

## Managing preset queries

Two ways:

- **From the UI**: write a query, click **Save**. Saved queries appear in the
  dropdown (grouped "Presets" vs "Saved") and persist in the SQLite store.
- **Seeded on first boot**: edit `internal/store/presets.go` and rebuild. These
  seed only when the store is empty, so they never clobber the team's edits. The
  shipped presets (custom-domains-per-company, recent signups, table sizes) are
  illustrative — adjust table/column names to your actual schema.

## Endpoints

| Method & path                                 | Purpose                                        |
| --------------------------------------------- | ---------------------------------------------- |
| `POST /api/query`                             | Run a query → JSON `{columns, rows, …}`.       |
| `POST /api/export`                            | Run a query → CSV download.                    |
| `GET /api/meta`                               | Server limits the UI needs (`{rowCap}`).       |
| `GET /api/tables`                             | List browsable tables/views (+ row estimate).  |
| `GET /api/tables/{schema}/{table}/columns`    | Column structure (name, type, nullable, default). |
| `GET /api/tables/{schema}/{table}/data`       | Paged rows; `?limit=&offset=&search=&sort=&dir=&f=col:op:val` (`&format=csv`). |
| `GET /api/queries`                            | List saved/preset queries.                     |
| `POST /api/queries`         | Create a saved query.                     |
| `PUT /api/queries/{id}`     | Update a saved query.                     |
| `DELETE /api/queries/{id}`  | Delete a saved query.                     |
| `GET /healthz`              | Liveness (always 200 if process is up).   |
| `GET /readyz`               | Readiness (pings the DB).                 |
| `GET /`                     | The UI.                                   |
