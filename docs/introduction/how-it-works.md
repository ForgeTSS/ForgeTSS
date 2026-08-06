# How It Works

This page walks the path a transaction takes through ForgeTSS at a high level. The detailed state machine is in [Transaction Lifecycle](../architecture/transaction-lifecycle.md); the pool mechanics are in [Channel Account Pool](../architecture/channel-account-pool.md); retries are in [Retry and Fee Bump](../architecture/retry-and-fee-bump.md). Here the goal is just to see the whole shape.

## The five stages

ForgeTSS owns a transaction from the moment you hand it over to the moment it reaches a terminal status.

1. **Enqueue.** You POST a signed transaction envelope (base64 XDR) to `/transactions`. The service validates that the envelope parses as Stellar XDR, assigns a UUID, writes a row to Postgres with status `pending`, and returns the UUID. Nothing has touched the network yet. The write is the durability boundary: once you have an ID, the transaction survives a restart.

2. **Lease.** A background worker polls Postgres for the oldest `pending` transaction. When it finds one, it claims a channel account from the pool. The claim is a single SQL statement — `SELECT ... FOR UPDATE SKIP LOCKED` — that atomically picks one idle account and marks it `leased`. `SKIP LOCKED` is the important part: if two workers (or two replicas) poll at the same instant, each locks a different row and neither blocks, so you can scale ForgeTSS horizontally against one database without two workers grabbing the same account.

3. **Submit.** The router inspects the envelope's operations. If any operation is an `InvokeHostFunction`, the transaction is a Soroban contract call and goes to a Soroban RPC endpoint, which requires a simulate step before submission. Everything else goes to Horizon. Either backend is a list of endpoints, tried in round-robin order; a 5xx, a timeout, or a dropped connection makes the router fail over to the next endpoint rather than failing the transaction outright.

4. **Retry.** If submission fails, the transaction can be retried. Each retry wraps the original in a fee-bump transaction with a higher fee and waits an exponentially growing backoff delay before resubmitting — `RETRY_BASE_DELAY * RETRY_MULTIPLIER^attempt`. Retries stop at `MAX_RETRY_ATTEMPTS`, after which the transaction is marked `failed` with the last error preserved. The exact numbers are on the [Retry and Fee Bump](../architecture/retry-and-fee-bump.md) page.

5. **Report.** At every step the transaction's row is updated, so its status is always readable. You can `GET /transactions/{id}` for a snapshot or open `GET /transactions/{id}/stream` for a Server-Sent Events feed that pushes the current status every couple of seconds until you disconnect. Terminal statuses are `confirmed` (it landed) and `failed` (it didn't, and won't be retried further).

## The components behind those stages

The stages map onto a small set of packages, each with one responsibility:

- **Store** (`internal/store`) — Postgres persistence for both transactions and channel accounts. Every state transition is a row update here, which is why status survives restarts.
- **Channel Account Pool** (`internal/channelaccounts`) — leasing, releasing, sequence-number syncing on startup, and refilling the pool with newly created accounts.
- **RPC Router** (`internal/rpc`) — backend selection (Horizon vs Soroban) and multi-endpoint failover.
- **Submission Engine** (`internal/submission`) — the background loop that ties it together: poll, lease, submit, release, update status, and handle retries.
- **API** (`internal/api`) — the REST surface, bearer-token auth, the SSE stream, and the Prometheus metrics endpoint.
- **Go Client SDK** (`pkg/client`) — a typed wrapper over the REST API for Go callers.

## Where the durability comes from

The single idea holding this together is that Postgres is the source of truth, not process memory. A transaction's status, a channel account's lease state, and each account's last-known sequence number all live in the database. If a ForgeTSS process dies mid-submission, a restarted process reads the same rows and continues. On startup the pool reconciles every *idle* account's sequence number against the network — it deliberately leaves `leased` accounts alone, because those may be in flight on another replica. That is the mechanism that lets ForgeTSS recover from a crash without double-submitting or corrupting sequence numbers.

One current simplification worth knowing up front: the queue worker processes one transaction per poll to avoid exhausting the pool, and the retry backoff is an inline wait rather than a scheduled job. Both are honest limitations of the present implementation, called out where relevant rather than dressed up. The [architecture section](../architecture/transaction-lifecycle.md) is precise about what the code does today versus what a fuller implementation would do.
