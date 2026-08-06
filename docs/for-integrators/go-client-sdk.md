# Go Client SDK

The `pkg/client` package is a typed Go wrapper over the ForgeTSS REST API. It covers the three operations you need: enqueue an envelope, poll a transaction's status, and stream status changes over SSE. This page shows the full API with runnable examples that include error handling — not just the happy path.

## Installation

```bash
go get github.com/ForgeTSS/ForgeTSS/pkg/client
```

```go
import "github.com/ForgeTSS/ForgeTSS/pkg/client"
```

## Creating a client

```go
c := client.New("http://localhost:8080", "your-api-key")
```

`New` takes the base URL and an API key. Pass an empty string for the key when the server runs in development mode (`API_KEYS` unset). The client uses a 30-second HTTP timeout, set internally; there is no option to change it in the current API, so if you need a different timeout you construct your own `*http.Client` and call the API directly.

```go
type Client struct {
	// unexported fields: baseURL, apiKey, httpClient
}

func New(baseURL string, apiKey string) *Client
```

## Enqueue

`Enqueue` submits a base64 XDR envelope and returns the transaction ID.

```go
func (c *Client) Enqueue(envelopeXDR string) (string, error)
```

Full example with error handling:

```go
package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/ForgeTSS/ForgeTSS/pkg/client"
)

func main() {
	c := client.New("http://localhost:8080", "your-api-key")

	envelope := "AAAAAgAAAAC7JAuE3Xvqu..." // signed base64 XDR

	id, err := c.Enqueue(envelope)
	if err != nil {
		// Enqueue returns a wrapped error. The message includes the HTTP
		// status and the server's plain-text body for non-201 responses,
		// e.g. "API error 400: envelope_xdr is required".
		log.Fatalf("enqueue failed: %v", err)
	}

	fmt.Println("enqueued:", id)
	_ = errors.Is // placeholder; see note below on error inspection
}
```

A note on error inspection: the SDK returns errors created with `fmt.Errorf`, carrying a human-readable message but no typed sentinel errors or status-code accessor. You cannot cleanly `errors.Is` against, say, a "not found" sentinel — the status code is embedded in the message string (`"API error 404: ..."`). If your application needs to branch on the status code, that is a current limitation; you would parse the message or call the REST API directly. This is worth knowing before you build retry logic around the SDK.

## Get status

`GetStatus` fetches the current state of a transaction.

```go
func (c *Client) GetStatus(id string) (*TransactionStatus, error)

type TransactionStatus struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Retry     int    `json:"retry"`
	LastError string `json:"last_error"`
}
```

Example that polls until the transaction reaches a terminal state:

```go
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/ForgeTSS/ForgeTSS/pkg/client"
)

// pollUntilTerminal polls GetStatus until the transaction is confirmed or
// failed, or until the deadline is reached. It returns the final status.
func pollUntilTerminal(c *client.Client, id string, deadline time.Time) (*client.TransactionStatus, error) {
	for {
		status, err := c.GetStatus(id)
		if err != nil {
			return nil, fmt.Errorf("polling %s: %w", id, err)
		}

		switch status.Status {
		case "confirmed":
			return status, nil
		case "failed":
			// Terminal failure — surface the error trail to the caller.
			return status, fmt.Errorf("transaction %s failed: %s", id, status.LastError)
		}

		if time.Now().After(deadline) {
			return status, fmt.Errorf("transaction %s did not reach terminal state before deadline (last status: %s)", id, status.Status)
		}

		// Stellar ledger close is ~5-6s; polling every 2s is responsive
		// without hammering the API.
		time.Sleep(2 * time.Second)
	}
}

func main() {
	c := client.New("http://localhost:8080", "your-api-key")

	id, err := c.Enqueue("AAAAAgAAAAC7JAuE...")
	if err != nil {
		log.Fatalf("enqueue: %v", err)
	}

	status, err := pollUntilTerminal(c, id, time.Now().Add(2*time.Minute))
	if err != nil {
		log.Fatalf("await confirmation: %v", err)
	}

	fmt.Printf("done: %s (retries: %d)\n", status.Status, status.Retry)
}
```

## Stream status (SSE)

`StreamStatus` opens a Server-Sent Events connection and invokes your callback for each event. It blocks until the stream ends (the server closes it, the connection drops, or a read error occurs) and returns the error that ended it.

```go
func (c *Client) StreamStatus(id string, onEvent func(event string, data map[string]interface{})) error
```

Example:

```go
package main

import (
	"fmt"
	"log"

	"github.com/ForgeTSS/ForgeTSS/pkg/client"
)

func main() {
	c := client.New("http://localhost:8080", "your-api-key")

	id, err := c.Enqueue("AAAAAgAAAAC7JAuE...")
	if err != nil {
		log.Fatalf("enqueue: %v", err)
	}

	// StreamStatus blocks. The callback fires for each SSE event: the
	// event type ("status" or "error") and the parsed JSON payload.
	err = c.StreamStatus(id, func(event string, data map[string]interface{}) {
		switch event {
		case "status":
			fmt.Printf("status=%v retry=%v last_error=%q\n",
				data["status"], data["retry"], data["last_error"])
		case "error":
			fmt.Printf("stream error: %v\n", data["message"])
		}
	})

	// StreamStatus always returns a non-nil error when it stops, because
	// the stream ends by hitting a read error (including normal EOF when
	// the connection closes). Don't treat a returned error as necessarily
	// a failure — inspect it.
	if err != nil {
		log.Printf("stream ended: %v", err)
	}
}
```

Two behaviors of `StreamStatus` you need to design around:

1. **It never returns `nil`.** The read loop only exits via an error from `resp.Body.Read`, so even a clean end of stream comes back as an error (wrapping `io.EOF` or similar). Log it, don't panic on it, and don't assume a non-nil return means something went wrong.

2. **The stream does not self-terminate on a terminal status.** The server keeps pushing `status` events every 2 seconds even after the transaction is `confirmed` or `failed`. If you only want to watch until the transaction settles, track that inside your callback and close the connection yourself — which, given the current SDK signature, means cancelling from outside (the SDK does not expose a stop channel). A common pattern is to run `StreamStatus` in a goroutine and stop consuming once you've seen a terminal status, then let the process move on. If you need precise stream teardown, prefer `GetStatus` polling (shown above) over the SSE stream, since polling gives you explicit loop control.

## Which to use: polling or streaming?

- **Polling (`GetStatus`)** gives you full control over the loop, clean termination, and simple error handling. It's the better fit for "submit and wait for this one transaction," and it's what the polling example above does.
- **Streaming (`StreamStatus`)** is useful when you want to react to changes as they happen and you're comfortable managing the connection lifecycle yourself. Given the two caveats above, it's currently the sharper-edged of the two. For most integrations, polling is the more predictable choice.

## Error handling summary

| Call | Returns error when | Error contains |
|------|--------------------|----------------|
| `Enqueue` | Non-201 response, network failure, or decode failure | Wrapped message with `"API error <code>: <body>"` for HTTP errors |
| `GetStatus` | Non-200 response (including 404), network failure, or decode failure | Same format |
| `StreamStatus` | Always, when the stream ends (including clean EOF) | Wrapped read error |

None of these expose the HTTP status code as a typed field. If your application logic depends on distinguishing, for example, a 404 from a 500, that's a gap in the current SDK — track it against the raw REST API instead. See [Handling Failure States](handling-failure-states.md) for what to do with each terminal transaction status once you have it.
