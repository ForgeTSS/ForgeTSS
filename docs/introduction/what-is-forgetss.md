# What is ForgeTSS?

ForgeTSS is a standalone service that submits Stellar transactions on behalf of another product. You hand it a signed transaction envelope over a REST call; it takes responsibility for getting that transaction onto the network — picking a channel account so sequence numbers don't collide, routing to Horizon or Soroban depending on what the transaction does, retrying with a fee bump when the network is congested, and reporting a final status you can poll or stream.

The name stands for Transaction Submission Service. That is the whole scope. ForgeTSS does not build transactions, hold your signing keys for the source account, run KYC, manage disbursement campaigns, or talk to end users. It does one job: reliable, high-throughput submission.

## Who it is for

A product that pays a lot of people in XLM or Stellar assets — payroll, remittance, marketplace payouts, batch airdrops — hits the same wall. Stellar accounts have a sequence number that increments by one per transaction, and two transactions submitted with the same sequence number conflict. If you submit 500 payouts from one account in a burst, they serialize on that single sequence number and you spend your time reconciling `tx_bad_seq` errors instead of shipping product.

The standard fix is channel accounts: a pool of accounts that each supply their own sequence number, so many transactions can be in flight at once. Running that pool correctly — leasing accounts without two workers grabbing the same one, syncing sequence numbers against the network after a crash, funding new accounts, releasing them when a submission finishes — is fiddly infrastructure that has nothing to do with your product. ForgeTSS is that infrastructure, extracted so you can call it instead of rebuilding it.

## What you get

- A REST API to enqueue an envelope, check a transaction's status, and stream status changes over Server-Sent Events.
- A Postgres-backed channel account pool that leases accounts with `FOR UPDATE SKIP LOCKED`, so you can run more than one ForgeTSS replica against the same database without two of them leasing the same account.
- An RPC router that inspects each envelope, sends Soroban contract invocations to a Soroban RPC endpoint and everything else to Horizon, and fails over across multiple endpoints when one returns a 5xx or times out.
- Exponential-backoff retries with a fee-bump on each attempt, bounded by a configurable maximum.
- A Go client SDK in `pkg/client` covering enqueue, status polling, and SSE streaming.
- Prometheus metrics for queue depth, submission outcomes, pool availability, and retries.

## What it is not

ForgeTSS is not a wallet, not a custody solution, and not a disbursement platform. It assumes the envelope you send is already built and signed by whatever holds the source account's keys. The one seed ForgeTSS does hold is `MASTER_SEED`, used only to fund newly created channel accounts — and channel accounts hold trivial balances, just enough for base reserves and fees.

If you need the surrounding product — tenant management, SEP-10 authentication, a disbursement dashboard — ForgeTSS is a component you would put underneath that, not a replacement for it. The next page explains why that separation exists and what the alternatives cost you.

## A note on honesty about origins

Part of ForgeTSS is derived from the Stellar Disbursement Platform's transaction submission module (Apache 2.0, Stellar Development Foundation). The channel-account-pooling approach and some of the submission logic trace back to that code. This is stated plainly in the [LICENSE](https://github.com/ForgeTSS/ForgeTSS/blob/main/LICENSE) and the README, and it is worth stating here too: ForgeTSS is as much an extraction-and-repackaging effort as it is original work. The value it adds is making that submission engine usable on its own, without deploying the rest of SDP. Whether that framing holds up over time is discussed candidly in [The Problem](the-problem.md).
