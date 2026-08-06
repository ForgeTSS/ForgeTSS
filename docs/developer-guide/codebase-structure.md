# Codebase Structure

The repository is organized by function, not by layer. Each directory under `internal/` owns a single responsibility; the `cmd/`, `pkg/`, and `migrations/` directories handle entry points, public clients, and database schema respectively.

## Top-level layout

```
.
├── cmd/forgetss        # Main binary entry point
├── internal/
│   ├── api             # HTTP handlers, auth middleware, SSE streams
│   ├── channelaccounts # Channel account management (not a separate directory; lives in store)
│   ├── config          # Environment variable parsing
│   ├── metrics         # Prometheus metric definitions
│   ├── rpc             # Horizon / Soroban endpoint routing and failover
│   ├── store           # Postgres persistence (transactions, channel accounts, migrations)
│   └── submission      # Background engine: polling, retry, fee bump
├── pkg/client          # Go SDK for external callers
├── migrations/         # SQL schema files (applied automatically on startup)
├── scripts/            # Deployment and setup scripts
├── docs/               # GitBook-compatible documentation (this site)
└── docker-compose.yml  # Development services (Postgres + ForgeTSS)
```

## Key internal packages

- `internal/config/config.go` — the single source of truth for all environment variables and their defaults. If a variable name or default is wrong, fix it here; all documentation should reference this file.
- `internal/store/store.go` and `postgres.go` — database access. All durable state (transaction statuses, channel account pool including sequence numbers) lives here.
- `internal/store/channelaccounts.go` — leasing logic using `SELECT ... FOR UPDATE SKIP LOCKED`. The concurrency mechanism that allows multiple replicas without duplicate leases.
- `internal/submission/engine.go` — the background worker loop. Polls `GetPendingTransactions(ctx, 1)`, leases an account, submits, retries on failure.
- `internal/submission/retry.go` — exponential backoff with fee bump. The multiplier and base delay are configured in `internal/config/config.go`.
- `internal/api/auth.go` — bearer token validation. When `API_KEYS` is empty (`len(s.cfg.APIKeys) == 0`), authentication is disabled entirely.
- `internal/rpc/router.go` — selects between Horizon and Soroban based on operation types, with multi-endpoint failover on 5xx or timeout.
- `internal/metrics/metrics.go` — Prometheus metric definitions (`forgetss_queue_depth`, `forgetss_channel_accounts_available`, etc.).
- `pkg/client/client.go` — the typed Go wrapper over the REST API used by external integrators.

## Migration files

`migrations/0001_init.sql` creates the `transactions` table; `migrations/0002_channel_accounts.sql` creates the `channel_accounts` table with the leasing and status columns. The service applies them on startup automatically; `scripts/deploy.sh` runs them explicitly via `go run ./cmd/forgetss migrate`. They are idempotent — safe to run multiple times.

## Where new work goes

- A new endpoint or handler change → `internal/api/`
- A new retry policy or fee-bump behavior → `internal/submission/retry.go` and `submission/engine.go`
- A new database query or schema change → `internal/store/` and `migrations/`
- A new metric → `internal/metrics/metrics.go`
- A new environment variable → `internal/config/config.go` and the documentation pages (`docs/for-operators/environment-variables.md`, `docs/developer-guide/local-setup.md`)
- A public SDK feature → `pkg/client/client.go`
