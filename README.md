# ForgeTSS — Stellar Transaction Submission Service

An independent service that any Stellar payroll, remittance, or batch-payout
product calls to reliably submit large volumes of transactions.

## Architecture

ForgeTSS owns the full lifecycle of a Stellar transaction:

1. **Enqueue** — Caller submits a transaction envelope via REST API
2. **Lease** — A channel account is leased from a DB-backed pool (row-locked)
3. **Submit** — Transaction is routed to Horizon or Soroban based on operation type
4. **Retry** — Failed transactions are fee-bumped and retried with exponential backoff
5. **Report** — Terminal status is available via REST or SSE stream

### Components

- **Store** — Postgres-backed persistence for transactions and channel accounts
- **Channel Account Pool** — DB-level row-locking (`FOR UPDATE SKIP LOCKED`) for concurrent leasing
- **RPC Router** — Selects Horizon or Soroban backend based on transaction operations
- **Submission Engine** — Background worker loop polling for pending transactions
- **API** — REST API with bearer auth, SSE streams, and Prometheus metrics

## Quick Start

```bash
# Start Postgres via docker-compose
docker compose up postgres -d

# Run migrations
pgx -d postgres://forgetss:forgetss@localhost:5432/forgetss < migrations/0001_init.sql
pgx -d postgres://forgetss:forgetss@localhost:5432/forgetss < migrations/0002_channel_accounts.sql

# Run the service
go run ./cmd/forgetss
```

## API

See [docs/API.md](docs/API.md) for full API documentation.

## Configuration

All configuration comes from environment variables (see `internal/config/config.go`):

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | Postgres connection string | `postgres://forgetss:forgetss@localhost:5432/forgetss` |
| `LISTEN_ADDR` | HTTP listen address | `:8080` |
| `API_KEYS` | Comma-separated list of valid API keys | (none — auth required but no keys set) |
| `HORIZON_ENDPOINTS` | Comma-separated Horizon endpoints | `https://horizon-testnet.stellar.org` |
| `SOROBAN_ENDPOINTS` | Comma-separated Soroban RPC endpoints | `https://soroban-testnet.stellar.org` |
| `MASTER_SEED` | Master distribution seed for funding channel accounts | (required for Refill) |
| `MAX_RETRY_ATTEMPTS` | Max retry attempts before marking failed | `5` |
| `RETRY_BASE_DELAY` | Base delay for exponential backoff (duration) | `2s` |
| `RETRY_MULTIPLIER` | Multiplier for exponential backoff | `2.0` |
| `QUEUE_POLL_INTERVAL` | Interval between queue polls (duration) | `1s` |
| `REFILL_BATCH_SIZE` | Number of channel accounts to create per refill | `10` |
| `LOG_LEVEL` | Log level: `debug`, `info`, `warn`, `error` | `info` |

## Testing

```bash
# Run all tests
go test ./...

# Run with a test Postgres
docker compose up postgres -d
go test -tags=integration ./...
```

## License

Proprietary.
