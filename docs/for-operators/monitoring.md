# Monitoring

The `/metrics` endpoint exposes Prometheus-formatted metrics. No authentication required. The endpoint returns `metrics not registered` (500) if the process started without calling `metrics.Register()` — that points to an initialization failure, not a metrics-format issue.

## Metrics

| Metric | Type | Labels | Meaning |
|--------|------|--------|---------|
| `forgetss_queue_depth` | Gauge | `status` (e.g., `pending`) | How many transactions are in the queue, broken out by status. A growing `pending` value means the worker is saturated or not running. |
| `forgetss_channel_accounts_available` | Gauge | (none) | Number of idle channel accounts in the pool. If this hits zero, new submissions will fail with `no channel account available`. Monitor and alert on this. |
| `forgetss_submission_total` | Counter | `status` (`confirmed`, `failed`) | Cumulative submission attempts by result. A rising `failed` rate signals either bad transactions (fix the client) or infrastructure problems (fix the deployment). |
| `forgetss_retry_total` | Counter | (none) | Total retries attempted across all transactions. A sharp increase indicates network instability or endpoint failures. |
| `forgetss_submission_duration_seconds` | Histogram | (none) | Duration of each submission attempt. Exponential buckets (0.01 to ~10s). Use for latency analysis and SLO tracking. |

## Alerting suggestions

These thresholds are not configured in the repository; add them to your Prometheus/Alertmanager rules:

- `forgetss_channel_accounts_available == 0` for more than 1 minute — pool exhausted; submissions will fail.
- `forgetss_queue_depth{status="pending"}` growing over multiple poll cycles — worker not keeping up.
- `rate(forgetss_submission_total{status="failed"}[5m])` significantly above baseline — either bad transactions or endpoint issues.
- `forgetss_retry_total` increasing rapidly — network or endpoint instability; check endpoint health.

## What `/health` does not tell you

`GET /health` returns `{"status":"ok"}` whenever the HTTP server is up. It does **not** check the database, the channel account pool, or endpoint reachability. A `200` from `/health` with `forgetss_channel_accounts_available == 0` and an unreachable Horizon endpoint is a broken deployment that still reports healthy. Use `/metrics` (not `/health`) for readiness signals, and combine it with database and endpoint probes at the load balancer or orchestrator level (see [Deployment](deployment.md)).

## Scraping

Point your Prometheus scraper at the service's `/metrics`. The endpoint needs no authorization header. If you run multiple replicas, scrape each replica individually (the metrics are per-process, not aggregated across a fleet). Aggregate at query time with `sum()` or `avg()` over your instance labels.
