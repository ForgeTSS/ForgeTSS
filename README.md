# ForgeTSS — Stellar Transaction Submission Service

An independent service that any Stellar payroll, remittance, or batch-payout
product calls to reliably submit large volumes of transactions.

[![CI](https://github.com/ForgeTSS/ForgeTSS/actions/workflows/ci.yml/badge.svg)](https://github.com/ForgeTSS/ForgeTSS/actions/workflows/ci.yml)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/ForgeTSS/ForgeTSS.svg)](https://pkg.go.dev/github.com/ForgeTSS/ForgeTSS)
![Go Version](https://img.shields.io/badge/go-1.25+-00ADD8?logo=go)

| Name | Role | GitHub | Telegram |
|---|---|---|---|
| Fuhad | Maintainer | @K1NGD4VID | |

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

Note: `MASTER_SEED` funds channel account creation and controls real
Stellar funds. For any non-local deployment, source it from a secrets
manager (e.g. Vault, AWS Secrets Manager) rather than a plain environment
variable.

## Go Client SDK

```go
import "github.com/ForgeTSS/ForgeTSS/pkg/client"

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

## Contributing

We welcome contributions. Here is how to get started:

### Finding an Issue

Browse [open issues](https://github.com/ForgeTSS/ForgeTSS/issues) — look for the
[`good first issue`](https://github.com/ForgeTSS/ForgeTSS/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22)
label if you are new to the codebase.

### Branch Naming

Use descriptive prefixes for your branches:

| Prefix | Use Case | Example |
|--------|----------|---------|
| `feat/` | New feature | `feat/sse-streaming` |
| `fix/` | Bug fix | `fix/channel-lease-race` |
| `docs/` | Documentation changes | `docs/api-reference` |

### Commit Format

Follow the established convention:

```
type(scope): description

feat(api): wire store into handlers — SSE polling
test(submission): add engine tests covering retry
docs: fix typo in architecture section
```

### Pull Request Checklist

- [ ] Tests pass: `go test ./...`
- [ ] Code builds: `go build ./...`
- [ ] `golangci-lint` is clean (when installed)
- [ ] One logical change per commit
- [ ] PR title follows commit format

## License

Apache License 2.0. See [LICENSE](LICENSE) for full text.

ForgeTSS is derived in part from Stellar Disbursement Platform's
transaction submission module (also Apache 2.0, Stellar Development
Foundation).

[![Contributors](https://contrib.rocks/image?repo=ForgeTSS/ForgeTSS)](https://github.com/ForgeTSS/ForgeTSS/graphs/contributors)
