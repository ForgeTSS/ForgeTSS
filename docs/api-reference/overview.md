# API Reference: Overview

ForgeTSS exposes a small REST API. Five endpoints, one of them a Server-Sent Events stream, two of them unauthenticated (health and metrics). This page covers the cross-cutting details — base URL, authentication, content types, error format — so the [Endpoints](endpoints.md) page can focus on each route's specifics.

## Base URL

By default the server listens on `:8080` (configurable via `LISTEN_ADDR`). In local development that's:

```
http://localhost:8080
```

There is no version prefix in the path. The API is small enough that versioning, if it becomes necessary, would be introduced as a `/v1` prefix at that point rather than pre-emptively.

## Authentication

Authenticated endpoints expect a bearer token in the `Authorization` header:

```
Authorization: Bearer your-api-key
```

Valid keys are configured through the `API_KEYS` environment variable — a comma-separated list. The auth middleware (`internal/api/auth.go`) compares the presented token against each configured key and accepts an exact match.

Two behaviors are worth knowing:

- **If `API_KEYS` is empty, authentication is disabled.** The middleware sees an empty key list and passes every request through. This is the development-mode default. Do not run a network-reachable deployment without `API_KEYS` set — an empty list means anyone who can reach the port can submit transactions. This is called out again, with more force, in [Deployment](../for-operators/deployment.md).
- **Key comparison is a plain string equality check**, iterated over the configured list. There is no scoping, no per-key rate limiting, and no expiry in the current implementation. A key is all-or-nothing access to the authenticated endpoints.

`/health` and `/metrics` never require auth — they are mounted outside the authenticated route group so that liveness probes and Prometheus scrapers don't need a key.

## Which endpoints require auth

| Endpoint | Method | Auth required |
|----------|--------|---------------|
| `/transactions` | POST | Yes |
| `/transactions/{id}` | GET | Yes |
| `/transactions/{id}/stream` | GET | Yes |
| `/health` | GET | No |
| `/metrics` | GET | No |

## Content types

- Request bodies are JSON. Send `Content-Type: application/json` on `POST /transactions`.
- Successful JSON responses are returned as `application/json`.
- The SSE stream returns `text/event-stream`.
- **Error responses are plain text, not JSON.** Every error path uses Go's `http.Error`, which writes a plain-text message and the status code. So a 404 body is the literal string `transaction not found\n`, not a JSON object. If you are parsing responses, branch on the HTTP status code and treat the body as a human-readable message, not structured data. This asymmetry — JSON on success, text on error — is a real quirk of the current implementation rather than a design goal.

## Status codes at a glance

| Code | Meaning | When |
|------|---------|------|
| 200 | OK | Successful GET |
| 201 | Created | Successful enqueue |
| 400 | Bad Request | Malformed JSON body, empty `envelope_xdr`, or an invalid UUID in the path |
| 401 | Unauthorized | Missing, malformed, or unrecognized bearer token (only when `API_KEYS` is set) |
| 404 | Not Found | No transaction with the given ID |
| 500 | Internal Server Error | Database write failure, or streaming unsupported by the response writer |

## Middleware and timeouts

The server (`internal/api/server.go`) wraps every route in a standard chi middleware stack: request ID injection, real-IP resolution, request logging, panic recovery, and a **60-second request timeout**. That timeout applies to the ordinary request/response endpoints. The SSE stream is long-lived by design and holds the connection open past 60 seconds — it pushes an update every 2 seconds until the client disconnects or the server shuts down.

## Rate limiting

There is none. The current implementation does not rate-limit by key, IP, or globally. If you expose ForgeTSS beyond a trusted network boundary, put a rate limiter (a reverse proxy, an API gateway) in front of it. This is noted here so its absence is a decision you make, not a surprise you discover.
