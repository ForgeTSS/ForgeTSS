// Package client provides a Go SDK for interacting with the ForgeTSS API.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is the ForgeTSS API client.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// New creates a new API client.
func New(baseURL string, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Enqueue submits a transaction envelope to the ForgeTSS queue.
func (c *Client) Enqueue(envelopeXDR string) (string, error) {
	body, err := json.Marshal(map[string]string{"envelope_xdr": envelopeXDR})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/transactions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", c.statusError(resp)
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return result.ID, nil
}

// TransactionStatus represents the current status of a transaction.
type TransactionStatus struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Retry      int    `json:"retry"`
	LastError  string `json:"last_error"`
}

// GetStatus retrieves the current status of a transaction.
func (c *Client) GetStatus(id string) (*TransactionStatus, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/transactions/"+id, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.statusError(resp)
	}

	var status TransactionStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &status, nil
}

// EventSource represents an SSE event from the stream endpoint.
type EventSource struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// StreamStatus opens an SSE connection and calls onEvent for each event.
// onEvent is called with the event type and parsed data map.
func (c *Client) StreamStatus(id string, onEvent func(event string, data map[string]interface{})) error {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/transactions/"+id+"/stream", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.statusError(resp)
	}

	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if err != nil {
			return fmt.Errorf("read stream: %w", err)
		}
		lines := bytes.Split(bytes.TrimSpace(buf[:n]), []byte("\n"))
		var event, data string
		for _, line := range lines {
			if bytes.HasPrefix(line, []byte("event:")) {
				event = string(bytes.TrimPrefix(line, []byte("event:")))
			}
			if bytes.HasPrefix(line, []byte("data:")) {
				data = string(bytes.TrimPrefix(line, []byte("data:")))
			}
		}
		if event != "" && data != "" {
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(data), &parsed); err == nil {
				onEvent(event, parsed)
			}
		}
	}
}

// statusError returns a descriptive error from an HTTP error response.
func (c *Client) statusError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
}
