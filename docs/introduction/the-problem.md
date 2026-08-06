# The Problem

"Batch payments on Stellar are hard" is true and useless. Here is the specific problem ForgeTSS exists to solve, and why the existing options don't solve it for you.

## The one production-tested implementation is trapped inside a bigger product

If you want to submit a high volume of Stellar transactions reliably — with channel-account pooling, sequence-number management, fee-bump retries, and crash recovery — there is exactly one implementation that has run at scale in production and been maintained by people who operate it: the transaction submission module inside the Stellar Disbursement Platform, at `internal/transactionsubmission/`. That code is solid. The problem is not the code. The problem is that to use it, you deploy SDP.

SDP is a full disbursement product. Standing it up means running its tenant database and multi-tenant provisioning, its SEP-10 authentication and SEP-24 interactive deposit/withdrawal flows, its admin dashboard, and its messaging stack for recipient notifications. A payroll backend that already knows who to pay and how much, and just needs the transactions to land, needs none of that. You would be operating an entire product to reach one module inside it.

So teams face a bad menu. Deploy and operate all of SDP for the one piece you want. Or fork `internal/transactionsubmission/` out by hand and take on maintaining a divergent copy. Or write your own submission engine and rediscover, in production, every edge case the SDP maintainers already hit — the `tx_bad_seq` races, the lost-lease-after-crash reconciliation, the fee-bump timing. ForgeTSS is an attempt at a fourth option: the extracted module, packaged as a service with its own API, so you call it instead of adopting or rebuilding it.

## Why the general-purpose alternatives fall short

There are other ways to push transactions to Stellar. Each stops short of what a batch-payout product needs, and it's worth being precise about where.

- **The Horizon SDKs directly (Go, JS, Python).** These give you `SubmitTransaction` and nothing above it. There is no pool, no lease, no retry policy, no persistence. A single source account means a single sequence number, which means one in-flight transaction at a time before you start managing sequence collisions yourself. Fine for a script that pays ten people; a liability for a service paying ten thousand.

- **A single fee-payer / fee-bump setup.** Fee-bumping a transaction lets a different account pay the fee, which is useful, but it does not give you concurrency. The *inner* transaction still draws its sequence number from its own source account. Wrapping every payout in a fee bump from one payer does not let two payouts from the same source run at once — it just changes who pays. You still need distinct sequence sources, which is what a channel-account pool provides and a fee-payer alone does not.

- **Home-grown channel account scripts.** Plenty of teams write a pool "quickly." The parts that are quick are the happy paths. The parts that are not: leasing an account atomically so two workers on two machines never grab the same one (ForgeTSS uses `SELECT ... FOR UPDATE SKIP LOCKED` for exactly this), reconciling every idle account's sequence number against the network at startup after a crash left leases dangling, and deciding what to do when an account's local sequence has drifted from the chain. These are the failures that show up under load, not in the demo.

- **No Soroban awareness.** Most classic-payment tooling predates Soroban and treats every transaction as a Horizon submission. A Soroban contract invocation has to be *simulated* before it can be submitted, and it goes to a Soroban RPC endpoint, not Horizon. Tooling that doesn't distinguish the two either can't submit contract calls at all or does it wrong. ForgeTSS routes on the envelope: if any operation is an `InvokeHostFunction`, it goes to Soroban (with the required simulate-then-submit step); otherwise it goes to Horizon.

None of these are bad tools. They are aimed at a different job. The gap is specifically: a standalone, pooled, multi-backend submission service you can run without adopting a whole product around it.

## The honest caveat

There is a real risk worth naming rather than hiding. The reason the best submission code lives inside SDP is that the Stellar Development Foundation built and maintains it — and SDF could decide to extract and publish that module as a standalone service themselves. If they do, they do it with the team that wrote it and operates it at scale, which is a strong position. ForgeTSS's bet is that the need is real *now* and the extraction is worth doing *now*, and that a focused, independently useful service has value even if the upstream project eventually offers its own. That bet could be wrong on timing. It is stated here so you can weigh it, not buried.

What ForgeTSS does not gamble on is the underlying approach: channel-account pooling with row-locked leasing and fee-bump retries is the established, production-proven way to do this on Stellar. That part is not novel, and it is not supposed to be. See [How It Works](how-it-works.md) for the mechanics.
