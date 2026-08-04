# ForgeTSS — Stellar Transaction Submission Service

An independent service that any Stellar payroll, remittance, or batch-payout
product calls to reliably submit large volumes of transactions.

## Architecture

ForgeTSS owns the full lifecycle of a Stellar transaction:

1. **Enqueue** — Caller submits a transaction envelope via REST API; persisted to Postgres
2. **Lease** — A channel account is leased from a DB-backed pool (row-locked via `FOR UPDATE SKIP LOCKED`)
3. **Submit** — Transaction is routed to Horizon or Soroban based on operation type
4. **Retry** — Failed transactions are fee-bumped and retried with exponential backoff
5. **Report** — Terminal status is available via REST or SSE stream

### Components

- **Store** — Postgres-backed persistence for transactions and channel accounts
- **Channel Account Pool** — Concurrent leasing with `FOR UPDATE SKIP LOCKED`
- **RPC Router** — Selects Horizon or Soroban backend based on transaction operations, with multi-endpoint failover
- **Submission Engine** — Background worker loop polling for pending transactions
- **API** — REST API with bearer auth, SSE streams, and Prometheus metrics
- **Go Client SDK** — `pkg/client` for enqueue, status polling, and SSE streaming

## Quick Start

```bash
# Start Postgres and ForgeTSS via docker-compose
docker compose up -d

# Wait for postgres to be ready
docker compose exec postgres pg_isready

# Submit a transaction
curl -X POST http://localhost:8080/transactions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"envelope_xdr":"AAAA..."}'

# Check status
curl http://localhost:8080/transactions/{id} \
  -H "Authorization: Bearer your-api-key"

# Stream status updates (SSE)
curl http://localhost:8080/transactions/{id}/stream \
  -H "Authorization: Bearer your-api-key"
```

## API

See [API.md](API.md) for full API documentation.

## Configuration

All configuration comes from environment variables (see `internal/config/config.go`):

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | Postgres connection string | `postgres://forge:forgepass@localhost:5432/forge` |
| `LISTEN_ADDR` | HTTP listen address | `:8080` |
| `API_KEYS` | Comma-separated list of valid API keys | (none — auth disabled in dev mode) |
| `HORIZON_ENDPOINTS` | Comma-separated Horizon endpoints | `https://horizon-testnet.stellar.org` |
| `SOROBAN_ENDPOINTS` | Comma-separated Soroban RPC endpoints | `https://soroban-testnet.stellar.org` |
| `MASTER_SEED` | Master distribution seed for funding channel accounts | (required for Refill) |
| `MAX_RETRY_ATTEMPTS` | Max retry attempts before marking failed | `5` |
| `RETRY_BASE_DELAY` | Base delay for exponential backoff (duration) | `2s` |
| `RETRY_MULTIPLIER` | Multiplier for exponential backoff | `2.0` |
| `QUEUE_POLL_INTERVAL` | Interval between queue polls (duration) | `1s` |
| `REFILL_BATCH_SIZE` | Number of channel accounts to create per refill | `10` |
| `LOG_LEVEL` | Log level: `debug`, `info`, `warn`, `error` | `info` |

## Go Client SDK

```go
import "github.com/gamp/forgetss/pkg/client"

c := client.New("http://localhost:8080", "your-api-key")

// Enqueue a transaction
id, err := c.Enqueue("AAAA...")

// Check status
status, err := c.GetStatus(id)
fmt.Println(status.Status) // "submitted", "confirmed", "failed"

// Stream real-time updates
err := c.StreamStatus(id, func(event string, data map[string]interface{}) {
    fmt.Printf("%s: %v\n", event, data)
})
```

## Testing

```bash
# Run all unit tests
go test ./...

# Run with a test Postgres
docker compose up postgres -d
TEST_DATABASE_URL="postgresql://forge:forgepass@localhost:5432/forge?sslmode=disable" go test -count=1 ./...
```

## License

Proprietary.
