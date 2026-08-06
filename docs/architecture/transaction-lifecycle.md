# Transaction Lifecycle

Every transaction moves through a state machine from the moment you enqueue it to the moment it reaches a terminal status. This page documents the exact states, the transitions, and which component triggers each one.

## The states

### `pending`

**What it means:** The transaction envelope has been validated, assigned a UUID, and written to the `transactions` table. No worker has picked it up yet. This is the entry state.

**Who sets it:** The API handler in `internal/api/handlers.go`, on `POST /transactions`. The write happens in `internal/submission/engine.go` (`Enqueue` method).

**What happens to the channel account:** None is leased yet.

**Next transitions:**
- → `submitting` when a background worker claims it for processing
- → stays `pending` indefinitely if the pool is exhausted and no worker can lease a channel account

### `submitting`

**What it means:** A worker has claimed this transaction and is in the process of leasing a channel account, incrementing the sequence number, and calling the RPC router. This state exists to prevent two workers (or two replicas polling the same database) from picking up the same transaction.

**Who sets it:** The submission engine in `internal/submission/engine.go`, in the `processBatch` method, immediately after fetching a `pending` transaction and before leasing a channel account.

**What happens to the channel account:** About to be leased. The transition to `submitting` is the lock that makes the transaction exclusive to one worker; the lease happens in the next line of code.

**Next transitions:**
- → `confirmed` when submission succeeds
- → `failed` when submission fails and no retry is scheduled, or when the channel account pool is empty

### `confirmed`

**What it means:** The transaction was accepted by the network. This is a terminal state. The channel account that was used is released back to the pool.

**Who sets it:** The submission engine, after `router.SubmitTransaction` returns success.

**What happens to the channel account:** Released (status set back to `idle`, `last_used_at` updated). The release happens in `internal/submission/engine.go` line 142–144, immediately after submission, whether the result is success or failure.

**Next transitions:** None. Terminal.

### `failed`

**What it means:** The transaction was rejected by the network, or submission failed for a reason that will not be retried (e.g., the pool is exhausted, or the maximum retry count has been reached). This is a terminal state. The `last_error` column preserves the final error message.

**Who sets it:** The submission engine, in the `failTransaction` helper (line 162), or after a retry attempt exhausts `MAX_RETRY_ATTEMPTS` (in `internal/submission/retry.go` line 28–29).

**What happens to the channel account:** Released, same as `confirmed`. Release happens before the status update.

**Next transitions:** None. Terminal.

## States that are referenced but not persisted

The README and some handler code mention `submitted` as a state between submission and confirmation. The current engine does not actually write `submitted` to the database — it goes directly from `submitting` to `confirmed` or `failed` based on the RPC result. The `submitted` status appears in the enum (`internal/store/transactions.go` line 20) but is not set in the engine. The SSE stream handler checks for it, but the engine never writes it. This is an honest gap in the implementation: `submitted` would mean "sent to the network, awaiting ledger close," but the current code collapses that state into the `submitting` → `confirmed` jump.

## The transition table

| From | To | Trigger | Component |
|------|------|---------|-----------|
| (none) | `pending` | Enqueue via REST API | `internal/api/handlers.go` + `internal/submission/engine.go` (`Enqueue`) |
| `pending` | `submitting` | Background worker claims the transaction | `internal/submission/engine.go` (`processBatch`) |
| `submitting` | `confirmed` | Submission succeeds, network accepts | `internal/submission/engine.go` (line 157) |
| `submitting` | `failed` | Submission fails, or pool exhausted, or max retries hit | `internal/submission/engine.go` (`failTransaction`) |
| `failed` | `submitting` | Retry logic (not automatic — requires explicit `Retry(id)` call) | `internal/submission/retry.go` (line 56) |
| `submitting` | `failed` | Retry submission fails after fee-bump | `internal/submission/retry.go` (line 68–69) |
| `submitting` | `confirmed` | Retry submission succeeds | `internal/submission/retry.go` (line 72) |

## What "leased" means for the channel account

When a transaction is `submitting`, it holds an exclusive lease on one channel account. The lease is acquired via `SELECT ... FOR UPDATE SKIP LOCKED` in `internal/store/channelaccounts.go` (line 119–127), which atomically picks one idle account and marks it `leased`. The account remains `leased` until the transaction finishes — either `confirmed` or `failed` — at which point the engine releases it (sets `status = 'idle'`, updates `last_used_at`) so another transaction can use it.

The lease exists in the database, not in process memory. If the ForgeTSS process crashes while a transaction is `submitting`, that account stays `leased` in the database. On restart, the pool's `SyncSequenceNumbers` deliberately skips `leased` accounts (line 20 in `internal/channelaccounts/sync.go`), assuming they may be in flight on another replica. The current implementation does not have automatic lease timeout or orphaned-lease cleanup; a crashed-in-flight transaction's account stays `leased` until manually released or the row is updated by hand. This is a known simplification, called out here rather than hidden.

## Retries and the state loop

The retry flow does not happen automatically in the background. The current `Retry` method in `internal/submission/retry.go` is a synchronous function that:

1. Fetches the transaction (must be in `failed` state)
2. Checks `retry_count` against `MAX_RETRY_ATTEMPTS`
3. Waits inline for the backoff delay (`time.After(delay)` on line 41)
4. Increments `retry_count`
5. Marks the transaction `submitting` again
6. Creates a fee-bump envelope (10× the original fee, line 94)
7. Submits the fee-bumped transaction
8. Updates status to `confirmed` or `failed` based on the result

The wait is inline, not scheduled. A real deployment would have a separate retry worker that polls for retriable failures and schedules the backoff without blocking a submission thread. The current code is honest about this: it works, but it ties up the caller for the duration of the backoff.

## Reading the state from outside

- `GET /transactions/{id}` returns a snapshot: `{"id": "...", "status": "confirmed", "retry": 0, "last_error": ""}`.
- `GET /transactions/{id}/stream` opens an SSE connection that polls the database every 2 seconds and pushes the current state as a `status` event. The stream stays open until the client disconnects or the server shuts down; there is no automatic close on terminal status. That's a choice: some callers want to know *when* it reached `confirmed` by watching the timestamp of the event, so closing the stream early would lose that signal.

The combination of a durable database state and the SSE polling model means status is always eventually consistent — a replica crash doesn't lose a transaction's state, and a client reconnecting to a different replica sees the same status.
