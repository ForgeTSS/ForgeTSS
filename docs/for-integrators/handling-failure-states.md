# Handling Failure States

When you enqueue a transaction, it eventually reaches a status you have to act on. This page tells you, for each status a transaction can hold, exactly what it means and what your application should do. The goal is that you never have to guess.

The five statuses are `pending`, `submitting`, `submitted`, `confirmed`, and `failed`. Two are terminal (`confirmed`, `failed`); the rest are in-flight. The [Transaction Lifecycle](../architecture/transaction-lifecycle.md) page covers the internal transitions; this page is about what *you*, the integrator, do when you observe each one.

## `confirmed` — success, terminal

**What it means:** The transaction was accepted by the network. The channel account has been released back to the pool. This is the outcome you're waiting for.

**What your application should do:**

- Mark the corresponding payment/payout as settled in your own system.
- Record the transaction ID for reconciliation. If you need the on-chain transaction hash for an audit trail, note that the current API does **not** return the Horizon transaction hash in the status response — it returns your ForgeTSS UUID, status, retry count, and last error. If you need the ledger hash, you would query Horizon separately using the source account and sequence, or extend ForgeTSS to persist and expose it. Plan for that gap if hash-level reconciliation matters to you.
- Stop polling or close the SSE stream. `confirmed` is terminal; the status will not change again.

**What you should not do:** Do not resubmit. The transaction is on the network. Resubmitting the same envelope will fail with a sequence error at best, or double-pay at worst if you rebuilt it with a fresh sequence.

## `failed` — terminal failure, will not retry further

**What it means:** The transaction did not land, and ForgeTSS is not going to try again on its own. The channel account has been released. The `last_error` field holds the most recent error message — this is your primary diagnostic. A transaction reaches `failed` for one of several reasons, and the right response depends on which:

**Read `last_error` and branch on it:**

- **`"no channel account available"`** — the pool was exhausted when the worker tried to lease an account. This is not a problem with your transaction; it's a capacity problem with the ForgeTSS deployment. **Your application should retry the enqueue** after a short delay, ideally with backoff, because the condition is transient — accounts free up as other transactions settle. If it persists, the operator needs to grow the pool ([Channel Account Pool](../architecture/channel-account-pool.md)). Treat this as "try again," not "give up."

- **`"operation rejected by horizon"`** (or an operation result code like `op_no_destination`, `op_underfunded`) — the transaction itself is invalid or cannot succeed as built. **Do not blindly resubmit** — it will fail the same way. Your application should surface this to whatever produced the transaction: the destination doesn't exist, the source is underfunded, the asset trustline is missing, etc. Fix the underlying condition, rebuild the envelope, and enqueue a new transaction. This is a "your input was wrong" failure, not a "the network was busy" failure.

- **`tx_bad_seq`** — the sequence number on the envelope didn't match the account's current sequence. This usually means the envelope was built against a stale sequence, or the same source account was used concurrently outside ForgeTSS. **Rebuild with the current sequence and enqueue again.** If you're routing all of an account's traffic through ForgeTSS and still seeing this, check that nothing else is submitting for the same source account.

- **`"fee bump operation rejected"`** — a retry's fee-bumped submission was rejected at the operation level. The inner operation is failing regardless of fee. Same response as `"operation rejected by horizon"`: fix the operation, don't just retry.

- **A timeout or endpoint error** (e.g., a wrapped `context deadline exceeded` or "all horizon endpoints failed") — the network or the RPC endpoints were unreachable, and ForgeTSS exhausted its retries. **This is retryable from your side.** The transaction may or may not have actually landed — a timeout is ambiguous. Before resubmitting, if you can, check the network directly (query Horizon for the source account's recent transactions) to avoid a double-submit. If you can't check, weigh the cost of a possible duplicate against the cost of a missed payment for your specific use case.

**The general rule:** `failed` with a *transient infrastructure* cause (`no channel account available`, timeout, endpoint failure) → your app retries the enqueue. `failed` with a *transaction-level* cause (operation rejected, bad sequence) → your app fixes the transaction and enqueues a corrected one. The `last_error` string is what tells you which bucket you're in, so always read it.

**On retry attempts:** the `retry` field tells you how many times ForgeTSS already fee-bumped and resubmitted before giving up. A `failed` transaction with `retry: 5` means the maximum was reached — the network had five chances. A `failed` transaction with `retry: 0` failed on the first attempt, often meaning a transaction-level rejection that retrying would never fix.

## `pending` — enqueued, not yet processed

**What it means:** The transaction is durably stored and waiting for a worker to pick it up. It has not touched the network.

**What your application should do:** Wait. Keep polling or streaming. Under normal load a transaction leaves `pending` within a poll cycle (default 1 second). If a transaction sits in `pending` for a long time, the worker is either not running or is saturated — that's an operational issue to investigate on the ForgeTSS side, not something your application can fix by resubmitting. **Do not enqueue a duplicate** because a transaction is slow to leave `pending`; you'll just create two.

## `submitting` — being processed right now

**What it means:** A worker has claimed the transaction, leased a channel account, and is in the middle of submitting it to the network. This is a brief, transient state.

**What your application should do:** Wait for it to resolve to `confirmed` or `failed`. Do not resubmit — a `submitting` transaction is actively in flight and holds a channel account lease. The one edge case to know: if the ForgeTSS process crashes while a transaction is `submitting`, the transaction can be left stuck in `submitting` and its channel account stuck `leased`, because the current implementation has no automatic orphaned-lease recovery ([Transaction Lifecycle](../architecture/transaction-lifecycle.md)). If you observe a transaction stuck in `submitting` far longer than a ledger close (say, more than a minute), that points to a crashed worker and needs operator intervention, not a client-side retry.

## `submitted` — defined but not currently emitted

**What it means:** In principle, "sent to the network, awaiting ledger confirmation." In practice, the current engine does not write this status — it jumps from `submitting` straight to `confirmed` or `failed`. You may see `submitted` in the status enum and in the SSE handler's code, but the submission engine never sets it. **Do not build application logic that waits for `submitted`** — you would wait forever, because it never arrives in the current implementation. Treat `submitting` as the "in flight" signal instead. This gap is documented honestly rather than papered over; if a future version wires up `submitted`, it would sit between `submitting` and `confirmed`.

## A decision table

| Observed status | `last_error` | Meaning | Your action |
|-----------------|--------------|---------|-------------|
| `confirmed` | — | Landed on network | Settle, stop polling, don't resubmit |
| `failed` | `no channel account available` | Pool exhausted (transient) | Retry enqueue with backoff |
| `failed` | timeout / endpoint failure | Network unreachable (ambiguous) | Check network, then maybe retry |
| `failed` | `operation rejected` / `op_*` | Transaction invalid | Fix operation, enqueue corrected tx |
| `failed` | `tx_bad_seq` | Stale sequence | Rebuild with current sequence, re-enqueue |
| `pending` | — | Waiting for worker | Wait, keep polling, don't duplicate |
| `submitting` | — | In flight now | Wait; if stuck >1min, flag to operator |
| `submitted` | — | Not emitted by current engine | Ignore; watch `submitting` instead |

## The one thing to get right

The most important distinction is **transient vs. permanent failure**, and `last_error` is the only field that tells you which. Retrying a permanently-failed transaction (bad destination, underfunded source) wastes effort and never succeeds. *Not* retrying a transiently-failed transaction (pool exhausted, endpoint blip) drops a payment that would have gone through. Read `last_error`, put it in one of the two buckets, and respond accordingly. Everything else on this page is detail around that one decision.
