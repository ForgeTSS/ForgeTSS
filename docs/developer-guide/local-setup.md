# Local Setup

The quickest way to get a working development environment is Docker Compose; the alternative is building from source with a local Postgres instance.

## Docker Compose (recommended)

Requires Docker and Docker Compose v2:

```bash
docker compose up -d
```

This starts:

- `postgres` on port 5432 (`forge` / `forgepass` / database `forge`)
- `forge-tss` on port 8080, waiting for Postgres to be healthy before starting

Verify:

```bash
docker compose ps
curl http://localhost:8080/health
```

A note on variable names: the `docker-compose.yml` uses names that do not match `internal/config/config.go`. The service falls back to hard-coded defaults in development mode, so it works locally. For a real deployment you must pass the correctly named variables (see [Environment Variables](../for-operators/environment-variables.md) and [Deployment](../for-operators/deployment.md)).

## From source

Requires Go 1.25+ and a running Postgres 16+:

```bash
go build ./cmd/forgetss
```

Run migrations manually before starting:

```bash
go run ./cmd/forgetss migrate
```

Or rely on the binary's automatic startup migration (idempotent, safe to re-run).

## Environment variables for local development

Only three matter locally:

- `MASTER_SEED` — to fund channel accounts via `scripts/setup-testnet.sh`
- `DATABASE_URL` — if you want to override the default (`postgres://forgetss:forgetss@localhost:5432/forgetss`)
- `API_KEYS` — leave empty for development (auth disabled); set a value to test authentication

All other variables fall back to sensible defaults. See [Environment Variables](../for-operators/environment-variables.md) for the full table.
