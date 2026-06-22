# pgpeek

A minimal, **read-only**, team-shared Postgres browser. Built to replace Adminer
for the support team — a query box, a results table, saved/preset queries, and
CSV export. Nothing else (no row editing, no schema management, no migrations,
no charts).

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

| Variable                   | Default            | Notes                                              |
| -------------------------- | ------------------ | -------------------------------------------------- |
| `DATABASE_URL`             | _(required)_       | Postgres DSN. Use the read-only role. **Never logged.** Aurora: include `?sslmode=require`. |
| `PGPEEK_LISTEN`            | `:8080`            | Listen address.                                    |
| `PGPEEK_ROW_CAP`           | `1000`             | Max rows returned/exported per query.              |
| `PGPEEK_STATEMENT_TIMEOUT` | `30s`              | Per-query DB statement timeout.                    |
| `PGPEEK_IDLE_TX_TIMEOUT`   | `30s`              | `idle_in_transaction_session_timeout`.             |
| `PGPEEK_MAX_CONNS`         | `8`                | Max pool size (caps DB connection usage).          |
| `PGPEEK_STORE_PATH`        | `/data/pgpeek.db`  | SQLite file for saved queries.                     |

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
go test ./...
CGO_ENABLED=0 go build -o pgpeek .          # static binary
docker build -t ghcr.io/descope/pgpeek:latest .   # ~25MB distroless image
```

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

| Method & path               | Purpose                                   |
| --------------------------- | ----------------------------------------- |
| `POST /api/query`           | Run a query → JSON `{columns, rows, …}`.  |
| `POST /api/export`          | Run a query → CSV download.               |
| `GET /api/queries`          | List saved/preset queries.                |
| `POST /api/queries`         | Create a saved query.                     |
| `PUT /api/queries/{id}`     | Update a saved query.                     |
| `DELETE /api/queries/{id}`  | Delete a saved query.                     |
| `GET /healthz`              | Liveness (always 200 if process is up).   |
| `GET /readyz`               | Readiness (pings the DB).                 |
| `GET /`                     | The UI.                                   |
