# ForgeTSS API Reference

Base URL: `http://localhost:8080`

All endpoints except `/health` require a `Bearer` API key in the `Authorization` header.

## Authentication

Include an API key with every authenticated request:

```
Authorization: Bearer your-api-key
```

If no `API_KEYS` are configured, authentication is disabled (development mode).

---

## Enqueue Transaction

**POST** `/transactions`

Submit a Stellar transaction envelope for queued submission.

### Request

```json
{
  "envelope_xdr": "AAAA..."
}
```

### Response

**201 Created**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000"
}
```

**400 Bad Request** — missing or empty `envelope_xdr`

**500 Internal Server Error** — persistence failure

---

## Get Transaction Status

**GET** `/transactions/{id}`

Retrieve the current status of a transaction by UUID.

### Response

**200 OK**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "submitted",
  "retry": 1,
  "last_error": "timeout"
}
```

Status values: `pending`, `submitting`, `submitted`, `failed`, `confirmed`.

**404 Not Found** — transaction not found

---

## Stream Status (SSE)

**GET** `/transactions/{id}/stream`

Opens a Server-Sent Events connection that streams status updates every 2 seconds.

### Event Format

```
event: status
data: {"id":"550e8400-...","status":"submitted","retry":0,"last_error":""}
```

Events:

- `status` — updated transaction state
- `error` — transaction not found

The connection stays open until the client disconnects or the server closes it.

---

## Health Check

**GET** `/health`

Returns `200 OK` when the service is healthy.

### Response

**200 OK**

```json
{"status": "ok"}
```

---

## Prometheus Metrics

**GET** `/metrics`

Prometheus scrape endpoint.

### Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `forgetss_queue_depth{status}` | Gauge | Pending transactions by status |
| `forgetss_submission_duration_seconds` | Histogram | Submission attempt durations |
| `forgetss_submission_total{status}` | Counter | Submission attempts by result |
| `forgetss_channel_accounts_available` | Gauge | Idle channel accounts in pool |
| `forgetss_retry_total` | Counter | Total retry count |

---

## Error Responses

All error responses return plain text with an HTTP status code:

| Code | Meaning |
|------|---------|
| 400 | Bad request — invalid input |
| 401 | Unauthorized — missing or invalid API key |
| 404 | Not found — transaction not found |
| 500 | Internal error — server or database failure |
