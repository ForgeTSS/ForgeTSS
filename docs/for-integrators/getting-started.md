# Getting Started

This page walks you from zero to a running ForgeTSS that can accept your first transaction. Local development with Docker Compose, testnet, one database, no production hardening yet — that comes in [Deployment](../for-operators/deployment.md).

## Prerequisites

- Docker and Docker Compose (to run Postgres and ForgeTSS without installing Go locally)
- OR Go 1.25+ and a running Postgres 16+ instance (if you prefer building from source)
- A Stellar testnet account with some XLM (for funding channel accounts)

If you don't have a testnet account yet, create one at [Stellar Laboratory](https://laboratory.stellar.org/#account-creator?network=test) and fund it with the friendbot.

## Clone the repository

```bash
git clone https://github.com/ForgeTSS/ForgeTSS.git
cd ForgeTSS
```

## Start Postgres and ForgeTSS

The simplest path uses Docker Compose:

```bash
docker compose up -d
```

This starts two containers:

- **postgres** on port 5432, with a health check. Database name `forge`, user `forge`, password `forgepass`.
- **forge-tss** on port 8080, waiting for postgres to be healthy before starting.

Check that both are up:

```bash
docker compose ps
```

You should see both services in the "running" state. The `forge-tss` container runs migrations automatically on startup, so the `transactions` and `channel_accounts` tables are already created.

### A note on environment variable mismatch

The `docker-compose.yml` currently defines environment variables that **do not match** what `internal/config/config.go` actually reads. Specifically:

- `HORIZON_URL` in compose → config expects `HORIZON_ENDPOINTS` (comma-separated list)
- `SOROBAN_RPC_URL` in compose → config expects `SOROBAN_ENDPOINTS` (comma-separated list)
- `MAX_RETRIES` in compose → config expects `MAX_RETRY_ATTEMPTS`
- `RETRY_INTERVAL_MS` in compose → config expects `RETRY_BASE_DELAY` (a duration string like `2s`)
- `CHANNEL_ACCOUNT_REFILL` in compose → config expects `REFILL_BATCH_SIZE`
- `STELLAR_NETWORK` in compose → not read by config at all (the network is inferred from the endpoints)

This is a real bug, not a documentation error. Until it's fixed, the container falls back to the hard-coded defaults in `config.go`. For local testnet use those defaults work (Horizon testnet, Soroban testnet, 5 retries, 2s base delay), so the service runs. For a real deployment you would pass the correctly-named environment variables via `-e` flags or a `.env` file rather than relying on the compose file's names. The [Environment Variables](../for-operators/environment-variables.md) page documents what the code actually reads.

## Create and fund channel accounts

ForgeTSS needs a pool of channel accounts before it can submit anything. The setup script generates keypairs, inserts them as `idle` in the database, and funds them on testnet using the Stellar friendbot.

Set your master seed — this is the account that will fund the channel accounts. You should have received testnet XLM from the friendbot into this account already.

```bash
export MASTER_SEED="S..."  # your secret seed from Stellar Laboratory
export REFILL_COUNT=5      # how many channel accounts to create
```

Run the setup script:

```bash
./scripts/setup-testnet.sh
```

The script runs migrations (idempotent — safe to run again), then generates 5 new keypairs, saves them to the database, and funds each one with 1000 XLM from your `MASTER_SEED` via the friendbot. The accounts are now `idle` in the pool and ready to lease.

If the script fails with "MASTER_SEED is not set," you forgot the `export` step above. If it fails with a friendbot error, the friendbot may be rate-limiting you — wait 30 seconds and try again with a smaller `REFILL_COUNT`.

## Verify the service is running

The health endpoint requires no auth:

```bash
curl http://localhost:8080/health
# → {"status":"ok"}
```

Check Prometheus metrics to see the pool:

```bash
curl -sS http://localhost:8080/metrics | grep forgetss_channel_accounts_available
# forgetss_channel_accounts_available 5
```

That confirms 5 idle channel accounts are in the pool.

## Submit your first transaction

Build a signed transaction envelope. For a quick test, use [Stellar Laboratory](https://laboratory.stellar.org/#txbuilder?network=test):

1. **Source account**: your testnet account (the one you funded earlier, NOT a channel account)
2. **Operation**: Payment, destination = any other testnet account, amount = 1 XLM
3. **Sign** with your source account's secret
4. Copy the base64 XDR envelope from the "Submit to Post Transaction Endpoint" box

Now enqueue it:

```bash
curl -sS -X POST http://localhost:8080/transactions \
  -H "Content-Type: application/json" \
  -d '{"envelope_xdr":"AAAAAgAAAAC7JAuE..."}' \
  | jq .
```

No `Authorization` header — the default config has `API_KEYS` empty, so auth is disabled in development mode. Production deployment changes this; see [Deployment](../for-operators/deployment.md).

The response is a UUID:

```json
{"id":"550e8400-e29b-41d4-a716-446655440000"}
```

## Check the status

Poll the transaction:

```bash
curl -sS http://localhost:8080/transactions/550e8400-e29b-41d4-a716-446655440000 | jq .
# {
#   "id": "550e8400-e29b-41d4-a716-446655440000",
#   "status": "confirmed",
#   "retry": 0,
#   "last_error": ""
# }
```

If `status` is still `pending` or `submitting`, wait a few seconds and poll again. Stellar testnet ledger close is about 5-6 seconds, so `confirmed` usually arrives within that window.

If `status` is `failed`, check `last_error` for the reason. Common failures on first run:

- `"no channel account available"` — the pool is exhausted. Refill with `./scripts/setup-testnet.sh` again, or check that the `channel_accounts` table has rows with `status = 'idle'`.
- `"tx_bad_seq"` — the source account's sequence number is stale, or the envelope was built with the wrong sequence. Rebuild the envelope in Stellar Laboratory with the current sequence.
- `"operation rejected by horizon"` — the operation itself failed (insufficient balance, bad destination, etc.). Check the Horizon error in the ForgeTSS logs: `docker compose logs forge-tss`.

## Stream status changes (optional)

For real-time updates, open an SSE stream:

```bash
curl -sS -N http://localhost:8080/transactions/550e8400-e29b-41d4-a716-446655440000/stream
# event: status
# data: {"id":"550e8400-...","status":"pending","retry":0,"last_error":""}
#
# event: status
# data: {"id":"550e8400-...","status":"confirmed","retry":0,"last_error":""}
```

The stream pushes an event every 2 seconds until you close it (Ctrl+C).

## Using the Go client SDK

Instead of raw `curl`, you can use `pkg/client`:

```go
package main

import (
	"fmt"
	"log"

	"github.com/ForgeTSS/ForgeTSS/pkg/client"
)

func main() {
	c := client.New("http://localhost:8080", "")

	id, err := c.Enqueue("AAAAAgAAAAC7JAuE...")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Transaction ID:", id)

	status, err := c.GetStatus(id)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Status: %s, Retry: %d\n", status.Status, status.Retry)
}
```

The empty string for the API key works in development mode (auth disabled). In production you pass the real key as the second argument.

The SDK also has a `StreamStatus` method for SSE; see [Go Client SDK](go-client-sdk.md) for full examples including error handling.

## What's next

You have a working ForgeTSS. The next pages cover:

- [Go Client SDK](go-client-sdk.md) — full client examples including retries and streaming
- [Handling Failure States](handling-failure-states.md) — what each terminal status means and how your application should respond
- [Deployment](../for-operators/deployment.md) — production hardening (API keys, secrets, multiple replicas)
