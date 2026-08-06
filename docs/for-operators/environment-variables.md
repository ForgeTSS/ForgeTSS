# Environment Variables

Every variable the binary reads, what it defaults to, and what breaks if you get it wrong. These names come from `internal/config/config.go` — not from the names used in `docker-compose.yml` or `scripts/deploy.sh`, which are mismatched (see [Deployment](deployment.md)).

## Required (or strongly required)

| Variable | Required? | Default | What breaks if wrong |
|----------|-----------|---------|----------------------|
| `DATABASE_URL` | Yes in practice | `postgres://forgetss:forgetss@localhost:5432/forgetss` | Service cannot start; no durable state. Must include TLS (`sslmode=require`) for non-local Postgres. |
| `MASTER_SEED` | Only if running `Refill` | (empty) | No funding source for new channel accounts. If you pre-provision accounts out of band, leave it empty to reduce blast radius. Never hard-code in an image. |
| `API_KEYS` | Only for auth | (empty) | Empty = authentication disabled (`internal/api/auth.go`, `len(s.cfg.APIKeys) == 0`). Any client can submit transactions. On a network this is a serious risk; always set it for production. |

## Endpoints and network

| Variable | Default | Notes |
|----------|---------|-------|
| `HORIZON_ENDPOINTS` | `https://horizon-testnet.stellar.org` | Comma-separated list. The router tries each in order on 5xx or timeout. List more than one for resilience. |
| `SOROBAN_ENDPOINTS` | `https://soroban-testnet.stellar.org` | Same pattern: multiple endpoints = failover. |
| `STELLAR_NETWORK` | (not read) | The binary infers the network from endpoint URLs. Setting it has no effect. This is one of the mismatched compose variables. |

## Retry and submission behavior

| Variable | Default | Notes |
|----------|---------|-------|
| `MAX_RETRY_ATTEMPTS` | `5` | Maximum fee-bump retries before the transaction reaches `failed`. |
| `RETRY_BASE_DELAY` | `2s` | Initial delay before the first retry; doubles with the multiplier. Duration string (e.g., `2s`, `500ms`). |
| `RETRY_MULTIPLIER` | `2.0` | Must be `>= 1.0`. The backoff factor applied to the base delay on each retry attempt. |
| `QUEUE_POLL_INTERVAL` | `1s` | How often the engine polls the pending transaction queue. |

## Channel account pool

| Variable | Default | Notes |
|----------|---------|-------|
| `REFILL_BATCH_SIZE` | `10` | How many new channel accounts the `Refill` process creates at once. Must be `>= 1`. |

## Service settings

| Variable | Default | Notes |
|----------|---------|-------|
| `LISTEN_ADDR` | `:8080` | HTTP server bind address. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`. |

## What the compose and deploy files get wrong

Both `docker-compose.yml` and `scripts/deploy.sh` use variable names that do not match `config.go`:

- `HORIZON_URL` (compose) vs. `HORIZON_ENDPOINTS` (code)
- `SOROBAN_RPC_URL` vs. `SOROBAN_ENDPOINTS`
- `MAX_RETRIES` vs. `MAX_RETRY_ATTEMPTS`
- `RETRY_INTERVAL_MS` vs. `RETRY_BASE_DELAY` (and the value format differs: milliseconds vs. duration string)
- `CHANNEL_ACCOUNT_REFILL` vs. `REFILL_BATCH_SIZE`
- `STELLAR_NETWORK` has no effect

If you rely on the compose file's names, the service falls back to hard-coded defaults. For production, pass correctly named variables explicitly (see the checklist in [Deployment](deployment.md)).
