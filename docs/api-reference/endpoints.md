# API Reference: Endpoints

Every endpoint, with its method, path, required headers, request body, and every response status code it can return — each with an example body. Error bodies are plain text (see [Overview](overview.md#content-types)); they are shown here exactly as the server writes them.

---

## Enqueue a transaction

Submit a signed transaction envelope for queued submission.

```
POST /transactions
```

### Headers

| Header | Value | Required |
|--------|-------|----------|
| `Authorization` | `Bearer <api-key>` | Yes, when `API_KEYS` is set |
| `Content-Type` | `application/json` | Yes |

### Request body

```json
{
  "envelope_xdr": "AAAAAgAAAAC7JAuE3XvquOYyLBqQR..."
}
```

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `envelope_xdr` | string | Yes | Base64-encoded, signed Stellar transaction envelope XDR |

The handler validates that the JSON parses and that `envelope_xdr` is non-empty. Note a current gap: the HTTP handler does **not** re-parse the XDR for structural validity before persisting — that deeper check lives in the engine's own `Enqueue` path. In practice a malformed but non-empty `envelope_xdr` is accepted at the API layer and will surface as a submission failure later, visible on the status endpoint. Send a properly built envelope.

### Responses

**201 Created** — the transaction was persisted as `pending`.

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000"
}
```

The `id` is the UUID you use for every subsequent status query.

**400 Bad Request** — the JSON body was malformed:

```
invalid request body
```

**400 Bad Request** — `envelope_xdr` was missing or empty:

```
envelope_xdr is required
```

**401 Unauthorized** — missing or invalid bearer token (only when `API_KEYS` is configured):

```
authorization required
```

or, for a present-but-unrecognized key:

```
invalid API key
```

**500 Internal Server Error** — the database write failed:

```
failed to save transaction
```

### Example

```bash
curl -sS -X POST http://localhost:8080/transactions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"envelope_xdr":"AAAAAgAAAAC7JAuE3Xvqu..."}'
# → {"id":"550e8400-e29b-41d4-a716-446655440000"}
```

---

## Get transaction status

Return the current status of a transaction by UUID.

```
GET /transactions/{id}
```

### Headers

| Header | Value | Required |
|--------|-------|----------|
| `Authorization` | `Bearer <api-key>` | Yes, when `API_KEYS` is set |

### Path parameters

| Parameter | Type | Notes |
|-----------|------|-------|
| `id` | UUID | The ID returned by the enqueue call |

### Responses

**200 OK** — the transaction exists:

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "confirmed",
  "retry": 0,
  "last_error": ""
}
```

| Field | Type | Notes |
|-------|------|-------|
| `id` | string | The transaction UUID |
| `status` | string | One of `pending`, `submitting`, `submitted`, `confirmed`, `failed` (see [Transaction Lifecycle](../architecture/transaction-lifecycle.md)) |
| `retry` | integer | Number of retries attempted so far |
| `last_error` | string | The most recent error message; empty string when there has been no error |

A failed transaction carries its error trail:

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "failed",
  "retry": 5,
  "last_error": "operation rejected by horizon"
}
```

**400 Bad Request** — the path parameter is not a valid UUID:

```
invalid transaction ID
```

**401 Unauthorized** — missing or invalid bearer token:

```
invalid API key
```

**404 Not Found** — no transaction with that ID:

```
transaction not found
```

### Example

```bash
curl -sS http://localhost:8080/transactions/550e8400-e29b-41d4-a716-446655440000 \
  -H "Authorization: Bearer your-api-key"
# → {"id":"550e8400-...","status":"confirmed","retry":0,"last_error":""}
```

---

## Stream transaction status (SSE)

Open a Server-Sent Events connection that pushes the transaction's status every 2 seconds.

```
GET /transactions/{id}/stream
```

### Headers

| Header | Value | Required |
|--------|-------|----------|
| `Authorization` | `Bearer <api-key>` | Yes, when `API_KEYS` is set |

The response is `text/event-stream`; the connection stays open until the client disconnects or the server shuts down. It does **not** close automatically when the transaction reaches a terminal status — watch for `confirmed` or `failed` in the event data and close the connection from the client side when you're done.

### Events

The first event is sent immediately with a `pending` placeholder status, then an updated event follows every 2 seconds.

**`status` event** — the current transaction state:

```
event: status
data: {"id":"550e8400-e29b-41d4-a716-446655440000","status":"submitting","retry":0,"last_error":""}
```

**`error` event** — the transaction could not be read (e.g., it was deleted, or the ID never existed after the stream opened):

```
event: error
data: {"message":"transaction not found"}
```

After an `error` event the server closes the stream.

### Responses

**200 OK** — the stream opened; events follow in the body as shown above.

**400 Bad Request** — the path parameter is not a valid UUID:

```
invalid transaction ID
```

**401 Unauthorized** — missing or invalid bearer token:

```
invalid API key
```

**500 Internal Server Error** — the underlying HTTP writer does not support flushing (streaming unsupported by the server environment):

```
streaming unsupported
```

### Example

```bash
curl -sS -N http://localhost:8080/transactions/550e8400-e29b-41d4-a716-446655440000/stream \
  -H "Authorization: Bearer your-api-key"
# event: status
# data: {"id":"550e8400-...","status":"pending"}
#
# event: status
# data: {"id":"550e8400-...","status":"confirmed","retry":0,"last_error":""}
```

The `-N` flag disables curl's buffering so events print as they arrive.

---

## Health check

Liveness endpoint. No authentication.

```
GET /health
```

### Responses

**200 OK**:

```json
{"status": "ok"}
```

This endpoint reports only that the HTTP server is up. It does not check database connectivity or pool availability — a `200` here means the process is serving, not that it can currently submit transactions. For deeper health, scrape `/metrics` and alert on pool availability (see [Monitoring](../for-operators/monitoring.md)).

### Example

```bash
curl -sS http://localhost:8080/health
# → {"status":"ok"}
```

---

## Prometheus metrics

Prometheus scrape endpoint. No authentication.

```
GET /metrics
```

### Responses

**200 OK** — OpenMetrics/Prometheus exposition format (text). Abbreviated example:

```
# HELP forgetss_queue_depth Number of pending transactions waiting in the queue
# TYPE forgetss_queue_depth gauge
forgetss_queue_depth{status="pending"} 3
# HELP forgetss_channel_accounts_available Number of idle channel accounts in the pool
# TYPE forgetss_channel_accounts_available gauge
forgetss_channel_accounts_available 18
# HELP forgetss_submission_total Total number of submission attempts, labeled by result
# TYPE forgetss_submission_total counter
forgetss_submission_total{status="confirmed"} 1240
forgetss_submission_total{status="failed"} 12
```

**500 Internal Server Error** — the metrics registry was never initialized (the process did not call `metrics.Register()` at startup):

```
metrics not registered
```

The full metric list, meanings, and suggested alert thresholds are on the [Monitoring](../for-operators/monitoring.md) page.

### Example

```bash
curl -sS http://localhost:8080/metrics
```
