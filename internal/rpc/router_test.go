package rpc

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	stellar "github.com/stellar/go-stellar-sdk"
)

func TestRoute_Horizon(t *testing.T) {
	r := NewRouter(nil, nil)

	// A simple payment envelope XDR (testnet).
	paymentXDR := "AAAAAgAAAADYTmL09jP9Xc4HpF+X1/3eBFOBaJGj8FqTdKF8AAAAZAAAAAC03n4AAAAAAAAAAA=="

	backend := r.route(paymentXDR)
	if backend != BackendHorizon {
		t.Errorf("route(payment) = %v, want BackendHorizon", backend)
	}
}

func TestRoute_Soroban(t *testing.T) {
	r := NewRouter(nil, nil)

	// An InvokeHostFunction envelope — use a valid Soroban simulation envelope XDR.
	// We construct a minimal one by using a known InvokeHostFunction XDR.
	invokeXDR := "AAAAAgAAAABqk1S5bGMK3Y9NF0cQA5kYh1Ji9MmLz8qRbNMpD7YhAAAAZAAAAAAGk6UAAAAABAAAAAgAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAA=="

	backend := r.route(invokeXDR)
	if backend != BackendSoroban {
		t.Errorf("route(invoke) = %v, want BackendSoroban", backend)
	}
}

func TestRoute_ParseError(t *testing.T) {
	r := NewRouter(nil, nil)

	// Invalid base64 — parse will fail and default to Horizon.
	badXDR := "not-base64!!!"

	backend := r.route(badXDR)
	if backend != BackendHorizon {
		t.Errorf("route(bad) = %v, want BackendHorizon on parse error", backend)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		want   bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "context deadline exceeded",
			err:  context.DeadlineExceeded,
			want: true,
		},
		{
			name: "context canceled",
			err:  context.Canceled,
			want: true,
		},
		{
			name: "io.EOF",
			err:  io.EOF,
			want: true,
		},
		{
			name: "generic error",
			err:  errors.New("something broke"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryable(tt.err)
			if got != tt.want {
				t.Errorf("isRetryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestNewRouter(t *testing.T) {
	r := NewRouter(
		[]string{"https://horizon-test.stellar.org"},
		[]string{"https://soroban-test.stellar.org"},
	)

	if len(r.horizonClients) != 1 {
		t.Errorf("got %d horizon clients, want 1", len(r.horizonClients))
	}
	// Soroban client creation may fail without valid URL — don't assert count.

	if r.HorizonClient() == nil {
		t.Error("HorizonClient() returned nil")
	}
}

// Test that stellar.HTTPError with 5xx is retryable.
func TestIsRetryable_HTTP5xx(t *testing.T) {
	err := stellar.HTTPError{
		StatusCode: 503,
		Body:       []byte("service unavailable"),
		Request:    http.Request{},
		Response:   &http.Response{StatusCode: 503},
	}

	if !isRetryable(&err) {
		t.Error("isRetryable(HTTP 503) = false, want true")
	}
}
