# Retry and Fee Bump

When a submission fails, ForgeTSS can resubmit the transaction wrapped in a fee-bump envelope, after waiting an exponentially growing delay. This page states the exact backoff formula from the code, walks five attempts with real numbers, and documents the fee-bump multiplier that is actually applied.

## The backoff formula

The delay before a retry is computed by `backoffDuration` in `internal/submission/engine.go`:

```go
func backoffDuration(attempt int, base time.Duration, multiplier float64) time.Duration {
	if attempt <= 0 {
		return base
	}
	delay := float64(base) * math.Pow(multiplier, float64(attempt))
	return time.Duration(delay)
}
```

In words: `delay = RETRY_BASE_DELAY * RETRY_MULTIPLIER^attempt`, with one special case — when `attempt` is zero or negative, the delay is just `RETRY_BASE_DELAY` rather than running the exponent. Since `mult^0 = 1` anyway, the special case produces the same value the formula would; it's a guard, not a different rule.

The function is called with the transaction's current `retry_count`, which starts at `0` on the first retry. So `attempt` is 0-indexed: the first retry uses `attempt = 0`, the second uses `attempt = 1`, and so on.

The two knobs come from config (`internal/config/config.go`):

| Variable | Default | Meaning |
|----------|---------|---------|
| `RETRY_BASE_DELAY` | `2s` | The base delay (`attempt = 0`) |
| `RETRY_MULTIPLIER` | `2.0` | The exponential growth factor |
| `MAX_RETRY_ATTEMPTS` | `5` | Retries allowed before terminal failure |

## Five attempts, with the default numbers

Using the documented defaults — base `2s`, multiplier `2.0` — here is every retry delay, keyed to the `retry_count` value passed in as `attempt`:

| Attempt | `retry_count` (`attempt`) | Calculation | Delay |
|---------|---------------------------|-------------|-------|
| 1st retry | 0 | base (special case) | **2s** |
| 2nd retry | 1 | 2s × 2.0¹ | **4s** |
| 3rd retry | 2 | 2s × 2.0² | **8s** |
| 4th retry | 3 | 2s × 2.0³ | **16s** |
| 5th retry | 4 | 2s × 2.0⁴ | **32s** |
| — | 5 | `retry_count >= MAX_RETRY_ATTEMPTS` | **terminal failure** |

So the delays double each time: 2s → 4s → 8s → 16s → 32s, and the sixth time the transaction comes up for retry, `retry_count` has reached `MAX_RETRY_ATTEMPTS` (5) and `Retry` returns an error without resubmitting (`internal/submission/retry.go`, line 27–30). The transaction stays `failed` with its last error preserved.

The cumulative wait across all five retries is 2 + 4 + 8 + 16 + 32 = **62 seconds** of backoff, plus the submission time of each attempt. That total matters for the current implementation because, as noted below, the wait is inline.

This exact doubling is pinned by a unit test, `TestBackoffDuration` in `internal/submission/engine_test.go`, which asserts `backoffDuration(0..3, 1s, 2.0)` returns `1s, 2s, 4s, 8s` — the same shape at a 1-second base.

## The fee bump

Each retry does not resubmit the original transaction unchanged — it wraps it in a fee-bump transaction so the network has an incentive to include it even when the fee market has moved. The construction is in `createFeeBump` (`internal/submission/retry.go`):

```go
fbTx, err := txnbuild.NewFeeBumpTransaction(
    txnbuild.FeeBumpTransactionOptions{
        Envelope:      origTx,
        Fee:           origTx.Fee() * 10,
        SourceAccount: feeSource,
    },
)
```

The multiplier that is actually applied is **10× the original transaction's fee**, flat. Two details about that, both worth being precise on because they are easy to assume wrong:

1. **It is 10× the *original* fee every time, not an escalating bump.** The code always reads `origTx.Fee()` — the fee on the transaction as first submitted — and multiplies by 10. It does not compound: the third retry is still 10× the original fee, not 10× the previous fee-bumped fee. So the fee does not climb across attempts; only the *delay* between attempts grows. If you expected the fee to escalate alongside the backoff, it does not in the current code.

2. **The fee source is the original transaction's source account.** `feeSource := origTx.SourceAccount()` — the fee-bump names the same account that sourced the inner transaction as the fee payer. A production setup would more typically fee-bump from a dedicated, well-funded fee account, so this is a place where the current implementation is simpler than a hardened one would be.

## Honest limitations of the current retry path

The retry logic works, but it is deliberately simple, and pretending otherwise would not help anyone operating it:

- **The backoff wait is inline, not scheduled.** `Retry` calls `time.After(delay)` and blocks until the delay elapses (`internal/submission/retry.go`, line 38–42). Whatever goroutine invoked `Retry` is tied up for the full backoff — up to 32 seconds on the fifth attempt. A production design would record a "retry after" timestamp, return immediately, and let a separate worker pick the transaction back up when its delay expires. As written, retries do not run automatically in the background at all; `Retry(id)` is an explicit call.

- **The fee does not adapt to network conditions.** A flat 10× is a reasonable default, but it is not informed by the current fee market. Under sustained congestion a fixed multiple can still be too low; under calm conditions it overpays. There is no fee estimation feeding the multiplier.

- **A rejected fee bump is terminal for that attempt.** If the fee-bumped submission comes back with an operation-level rejection, the transaction is marked `failed` immediately with `"fee bump operation rejected"` (line 68–69) rather than distinguishing retryable from non-retryable rejection reasons.

These are the kinds of things you would tighten before running this at scale, and they are called out here so the retry behavior holds no surprises. The failover across *endpoints* — trying the next Horizon or Soroban URL on a 5xx or timeout — is separate from this retry loop and is handled inside the [RPC router](../introduction/how-it-works.md); that failover is automatic and does not consume a retry attempt.
