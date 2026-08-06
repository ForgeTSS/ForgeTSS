# Channel Account Pool

The channel account pool is the mechanism that lets ForgeTSS have many transactions in flight at once. This page explains what a channel account is, how leasing works, and — with real arithmetic — how many transactions per minute a pool of a given size can clear.

## Why channel accounts exist

Every Stellar account has a sequence number that increments by exactly one per transaction it sources. Two transactions built with the same source account and the same sequence number conflict: at most one lands, the other fails with `tx_bad_seq`. This means a single account can only have one transaction in flight at a time without careful sequencing.

A channel account sidesteps this. It is a Stellar account whose only job is to supply a sequence number for a transaction it doesn't otherwise care about. Put a pool of them in front of your submission path and each concurrent transaction borrows a different channel account, hence a different sequence number, so they don't collide. The account paying and the account authorizing the operation can stay the same across all of them; only the sequence source rotates.

## The schema

Channel accounts live in one table (`migrations/0002_channel_accounts.sql`):

```sql
CREATE TABLE channel_accounts (
    public_key TEXT PRIMARY KEY,
    encrypted_secret TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'idle',
    sequence_number BIGINT NOT NULL,
    last_used_at TIMESTAMPTZ
);
```

`status` is either `idle` or `leased`. `sequence_number` is the pool's local view of the account's sequence, reconciled against the network at startup. `last_used_at` drives fair rotation — the pool always leases the least-recently-used idle account first.

## How a lease works

Leasing is a single database transaction in `internal/store/channelaccounts.go`:

```sql
SELECT public_key, encrypted_secret, status, sequence_number, last_used_at
FROM channel_accounts
WHERE status = 'idle'
ORDER BY last_used_at ASC NULLS FIRST
LIMIT 1
FOR UPDATE SKIP LOCKED
```

Three clauses carry the weight:

- **`FOR UPDATE`** locks the selected row so no other database transaction can modify it until this one commits.
- **`SKIP LOCKED`** means if a row is already locked by another worker, this query skips past it to the next idle account instead of blocking. This is what makes concurrent leasing work: two workers running this query at the same instant lock two *different* rows, and neither waits on the other.
- **`ORDER BY last_used_at ASC NULLS FIRST`** picks the account that has been idle longest (never-used accounts, with a `NULL` timestamp, come first). This spreads load evenly and gives each account's network sequence the most time to settle between uses.

Once selected, the row is updated to `status = 'leased'` and the transaction commits. Releasing (`ReleaseChannelAccount`) sets `status = 'idle'` and stamps `last_used_at = now()`.

Because both lease and release are ordinary database writes, the pool state is shared across every ForgeTSS replica pointed at the same database. You can run three replicas and they cooperate on one pool without any coordination beyond Postgres row locks.

## A worked throughput example

Numbers make this concrete. Suppose you configure a pool of **20 channel accounts** and enqueue **50 transactions** simultaneously.

- The first **20** transactions each lease an account immediately. All 20 accounts are now `leased`.
- The remaining **30** transactions find no idle account when a worker tries to lease. In the current implementation the worker logs "no channel account available" and marks that transaction `failed` — it does not automatically wait and requeue. So with a pool of 20 and a burst of 50, 20 proceed and 30 fail unless the pool is larger or the burst is metered. (This is a real limitation of the present code, discussed at the end of this page — a fuller implementation would leave over-burst transactions `pending` and retry the lease.)

Now assume the pool is sized so it isn't exhausted, and look at steady-state throughput. The time an account stays leased is roughly the time to submit and confirm one transaction. For a classic Horizon payment, confirmation tracks the Stellar ledger close time, which averages about **5 to 6 seconds**.

Take 5.5 seconds as the average lease duration. One account can therefore turn over:

```
60 seconds / 5.5 seconds per transaction ≈ 10.9 transactions per account per minute
```

Across a 20-account pool, if the queue keeps every account busy:

```
20 accounts × 10.9 tx/account/min ≈ 218 transactions per minute
```

So a 20-account pool clears **roughly 200–240 transactions per minute** at classic-payment confirmation times, before the queue depth starts growing faster than the pool can drain it. If you need 500 tx/min sustained, you need a pool closer to 46–50 accounts (500 ÷ 10.9 ≈ 46), plus headroom.

Two honesty notes on those numbers:

- The 5–6 second figure is the *typical* Stellar ledger close and confirmation window, not a guarantee. Under network congestion, confirmation is slower, lease duration rises, and per-account throughput drops proportionally. Halve the confirmation speed and you halve the throughput.
- The current engine processes one transaction per poll cycle (`GetPendingTransactions(ctx, 1)` in `internal/submission/engine.go`), with a poll interval of `QUEUE_POLL_INTERVAL` (default 1 second). That serial-per-poll design, not the pool size, is the actual throughput ceiling in the code as written — a single worker leasing one account per second tops out near 60 tx/min regardless of pool size. The pool arithmetic above describes what the *pool* supports; realizing it requires processing transactions concurrently, which is the natural next step for the engine.

## Sizing guidance

- **Match pool size to peak burst, not average load.** If you pay everyone at 9am on the 1st of the month, size for that spike. An idle account costs only its base reserve (a fraction of an XLM), so over-provisioning is cheap insurance against `no channel account available` failures.
- **Account for confirmation time under load.** If your network conditions push confirmation to 10–12 seconds during busy periods, halve your per-account throughput estimate when sizing.
- **Refill ahead of exhaustion.** `Refill` (in `internal/channelaccounts/refill.go`) creates new keypairs and inserts them as `idle`. Newly created accounts must be funded from `MASTER_SEED` before they are usable on the network — an unfunded account can't source a transaction. Refill creates the rows; funding is a separate step the operator runs (or the testnet faucet on testnet).

## Startup reconciliation

When ForgeTSS starts, `SyncSequenceNumbers` (in `internal/channelaccounts/sync.go`) walks every channel account and, for the `idle` ones, fetches the current sequence from the network and updates the local value if the network is ahead. It deliberately **skips** `leased` accounts — a leased account might be in flight on another replica, and overwriting its sequence mid-submission would corrupt that transaction. This is why a crash that leaves accounts `leased` needs care: those accounts are not reconciled on restart, by design. See [Transaction Lifecycle](transaction-lifecycle.md) for the orphaned-lease caveat.
