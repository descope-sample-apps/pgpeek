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
  `ILIKE`, `IS NULL`, …), and click-to-sort headers. Foreign-key cells are
  **click-through links** that jump to the referenced row. CSV export respects
  the active search/filters/sort.
- **Structure** tab — column name, type, nullable, default.
- **SQL** tab — CodeMirror editor with table/field autocomplete, saved/preset
  queries, CSV export.

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

Everything is configured via the environment. Single-database deployments can
keep using `DATABASE_URL`; multi-database deployments can use a URL list,
numbered env vars, or a mounted JSON config file. Secret-bearing URLs can be
supplied from mounted files so they do not live in manifests.

| Variable                     | Default              | Notes                                                                 |
| ---------------------------- | -------------------- | --------------------------------------------------------------------- |
| `DATABASE_URL`               | single-DB required   | Postgres DSN for single-database installs. Use the read-only role. **Never logged.** Aurora: include `?sslmode=require`. |
| `DATABASE_URL_FILE`          | —                    | Path to a file holding the DSN (mounted-secret alternative).          |
| `PGPEEK_DATABASE_URLS`       | —                    | Comma- or semicolon-separated DSNs for multiple databases. Quoted CSV values are supported. |
| `PGPEEK_DATABASE_IDS`        | `db1`, `db2`, …      | Optional comma/semicolon IDs matching `PGPEEK_DATABASE_URLS`; URL-safe (`A-Z`, `a-z`, `0-9`, `_`, `-`, `.`). |
| `PGPEEK_DATABASE_NAMES`      | `Database N`         | Optional display names matching `PGPEEK_DATABASE_URLS`.               |
| `PGPEEK_DATABASE_URL_1`      | —                    | Numbered DSN form. Continue with `_2`, `_3`, …; each also supports `_FILE`. |
| `PGPEEK_DATABASE_ID_1`       | `db1`                | Optional ID for numbered database 1.                                  |
| `PGPEEK_DATABASE_NAME_1`     | `Database 1`         | Optional display name for numbered database 1.                        |
| `PGPEEK_DATABASES_FILE`      | —                    | Path to a mounted JSON config file with database entries.             |
| `PGPEEK_DEFAULT_DATABASE`    | first configured DB  | Default database ID when the URL has no `db=` parameter.              |
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

### Multiple databases / clusters

The UI shows a database selector. The selected ID is kept in the URL as
`?db=<id>` alongside table, tab, filter, sort, and pagination state, so links are
bookmarkable and shareable.

Same-env list form:

```bash
export PGPEEK_DATABASE_URLS='postgres://reader:PASSWORD@prod:5432/app?sslmode=require;postgres://reader:PASSWORD@analytics:5432/warehouse?sslmode=require'
export PGPEEK_DATABASE_IDS='prod;analytics'
export PGPEEK_DATABASE_NAMES='Production;Analytics'
export PGPEEK_DEFAULT_DATABASE=prod
```

Numbered env var form:

```bash
export PGPEEK_DATABASE_URL_1_FILE=/run/secrets/prod-url
export PGPEEK_DATABASE_ID_1=prod
export PGPEEK_DATABASE_NAME_1=Production
export PGPEEK_DATABASE_URL_2_FILE=/run/secrets/analytics-url
export PGPEEK_DATABASE_ID_2=analytics
export PGPEEK_DATABASE_NAME_2=Analytics
```

Mounted config file form (`PGPEEK_DATABASES_FILE=/config/pgpeek/databases.json`):

```json
{
  "default": "prod",
  "databases": [
    { "id": "prod", "name": "Production", "urlFile": "/secrets/prod-url" },
    { "id": "analytics", "name": "Analytics", "urlFile": "/secrets/analytics-url" }
  ]
}
```

Kubernetes example (ConfigMap-mounted config + Secret-mounted DSNs; illustrative
only, not an extra manifest to commit):

```yaml
env:
  - name: PGPEEK_DATABASES_FILE
    value: /config/pgpeek/databases.json
volumeMounts:
  - name: pgpeek-db-config
    mountPath: /config/pgpeek
    readOnly: true
  - name: pgpeek-db-urls
    mountPath: /secrets
    readOnly: true
volumes:
  - name: pgpeek-db-config
    configMap:
      name: pgpeek-db-config
  - name: pgpeek-db-urls
    secret:
      secretName: pgpeek-db-urls
```

Docker Compose example (volume-mounted JSON + secret files; illustrative only):

```yaml
services:
  pgpeek:
    environment:
      PGPEEK_DATABASES_FILE: /config/pgpeek/databases.json
    volumes:
      - ./pgpeek-config:/config/pgpeek:ro
      - ./pgpeek-secrets:/secrets:ro
```

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
  `ghcr.io/descope-sample-apps/pgpeek:{version,major.minor,latest}` with SBOMs.

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
| `GET /api/databases`                          | List configured databases → `{defaultId, databases:[{id,name}]}`. |
| `POST /api/query?db=<id>`                     | Run a query → JSON `{columns, rows, …}`.       |
| `POST /api/export?db=<id>`                    | Run a query → CSV download.                    |
| `GET /api/meta?db=<id>`                       | Server limits the UI needs (`{rowCap}`).       |
| `GET /api/tables?db=<id>`                     | List browsable tables/views (+ row estimate).  |
| `GET /api/tables/{schema}/{table}/columns?db=<id>` | Column structure (name, type, nullable, default). |
| `GET /api/tables/{schema}/{table}/fks?db=<id>` | Single-column foreign keys (for click-through).   |
| `GET /api/tables/{schema}/{table}/data?db=<id>` | Paged rows; `&limit=&offset=&search=&sort=&dir=&f=col:op:val` (`&format=csv`). |
| `GET /api/queries`                            | List saved/preset queries.                     |
| `POST /api/queries`         | Create a saved query.                     |
| `PUT /api/queries/{id}`     | Update a saved query.                     |
| `DELETE /api/queries/{id}`  | Delete a saved query.                     |
| `GET /healthz`              | Liveness (always 200 if process is up).   |
| `GET /readyz`               | Readiness (pings the DB).                 |
| `GET /`                     | The UI.                                   |
