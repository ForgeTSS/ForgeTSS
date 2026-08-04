// Package rpc provides Stellar RPC clients and routing logic.
package rpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"

	stellar "github.com/stellar/go-stellar-sdk"
	"github.com/stellar/go-stellar-sdk/pkg/txnbuild"
)

// Backend identifies which Stellar backend to use.
type Backend int

const (
	BackendHorizon Backend = iota
	BackendSoroban
)

// Router selects the appropriate Stellar backend based on transaction operations
// and handles failover across configured endpoints.
type Router struct {
	horizonClients []*HorizonClient
	sorobanClients []*SorobanClient
	horizonIdx     atomic.Int64
	sorobanIdx     atomic.Int64
}

// NewRouter creates a router from lists of Horizon and Soroban endpoint URLs.
func NewRouter(horizonURLs []string, sorobanURLs []string) *Router {
	r := &Router{}

	for _, url := range horizonURLs {
		r.horizonClients = append(r.horizonClients, NewHorizonClient(url))
	}
	for _, url := range sorobanURLs {
		sc, err := NewSorobanClient(url)
		if err != nil {
			slog.Warn("failed to create soroban client", "url", url, "error", err)
			continue
		}
		r.sorobanClients = append(r.sorobanClients, sc)
	}

	return r
}

// route selects the backend based on the transaction envelope XDR.
// If any operation is an InvokeHostFunction, the transaction is routed to Soroban.
// Otherwise it goes to Horizon.
func (r *Router) route(envelopeXDR string) Backend {
	tx, err := stellar.ParseEnvelopeXDR(envelopeXDR, stellar.NetworkPublicTestnet)
	if err != nil {
		slog.Warn("failed to parse envelope for routing, defaulting to horizon", "error", err)
		return BackendHorizon
	}

	for _, op := range tx.Operations() {
		if _, ok := op.(*txnbuild.InvokeHostFunction); ok {
			return BackendSoroban
		}
	}
	return BackendHorizon
}

// SubmitTransaction routes the envelope to the correct backend and submits it,
// failing over across endpoints on 5xx or timeout.
func (r *Router) SubmitTransaction(ctx context.Context, envelopeXDR string) (*stellar.TransactionResult, error) {
	backend := r.route(envelopeXDR)
	switch backend {
	case BackendHorizon:
		return r.submitWithFailover(ctx, envelopeXDR)
	case BackendSoroban:
		return r.submitSoroban(ctx, envelopeXDR)
	default:
		return nil, fmt.Errorf("unknown backend %d", backend)
	}
}

// GetTransactionStatus queries Horizon for transaction status, with failover.
func (r *Router) GetTransactionStatus(ctx context.Context, hash string) (*stellar.TransactionResult, error) {
	for i := 0; i < len(r.horizonClients); i++ {
		idx := int(r.horizonIdx.Add(1)) % len(r.horizonClients)
		client := r.horizonClients[idx]

		result, err := client.GetTransactionStatus(ctx, hash)
		if err != nil {
			if isRetryable(err) {
				slog.Warn("status query endpoint failed, trying next", "endpoint", client.baseURL, "error", err)
				continue
			}
			return nil, fmt.Errorf("querying horizon (%s): %w", client.baseURL, err)
		}
		return result, nil
	}
	return nil, fmt.Errorf("all horizon endpoints failed for status query of %s", hash)
}

// submitWithFailover tries each Horizon endpoint in round-robin order, skipping
// endpoints that return 5xx responses or context deadline exceeded errors.
func (r *Router) submitWithFailover(ctx context.Context, envelopeXDR string) (*stellar.TransactionResult, error) {
	for i := 0; i < len(r.horizonClients); i++ {
		idx := int(r.horizonIdx.Add(1)) % len(r.horizonClients)
		client := r.horizonClients[idx]

		result, err := client.SubmitTransaction(ctx, envelopeXDR)
		if err != nil {
			if isRetryable(err) {
				slog.Warn("submit endpoint failed, trying next", "endpoint", client.baseURL, "error", err)
				continue
			}
			return nil, fmt.Errorf("submitting via horizon (%s): %w", client.baseURL, err)
		}
		return result, nil
	}
	return nil, fmt.Errorf("all horizon endpoints failed after %d attempts", len(r.horizonClients))
}

// submitSoroban simulates then submits via the next available Soroban endpoint.
func (r *Router) submitSoroban(ctx context.Context, envelopeXDR string) (*stellar.TransactionResult, error) {
	for i := 0; i < len(r.sorobanClients); i++ {
		idx := int(r.sorobanIdx.Add(1)) % len(r.sorobanClients)
		client := r.sorobanClients[idx]

		// Simulate first (required for Soroban).
		if _, err := client.SimulateTransaction(ctx, envelopeXDR); err != nil {
			if isRetryable(err) {
				slog.Warn("sim endpoint failed, trying next", "endpoint", client.baseURL, "error", err)
				continue
			}
			return nil, fmt.Errorf("simulating on soroban (%s): %w", client.baseURL, err)
		}

		resp, err := client.SubmitTransaction(ctx, envelopeXDR)
		if err != nil {
			if isRetryable(err) {
				slog.Warn("submit soroban endpoint failed, trying next", "endpoint", client.baseURL, "error", err)
				continue
			}
			return nil, fmt.Errorf("submitting soroban transaction (%s): %w", client.baseURL, err)
		}
		_ = resp
		return nil, fmt.Errorf("soroban submission succeeded")
	}
	return nil, fmt.Errorf("all soroban endpoints failed after %d attempts", len(r.sorobanClients))
}

// HorizonClient returns the primary Horizon client (first configured endpoint).
// Used by the channel account pool for sequence syncing.
func (r *Router) HorizonClient() *HorizonClient {
	if len(r.horizonClients) == 0 {
		return nil
	}
	return r.horizonClients[0]
}

// isRetryable checks if an error is transient (5xx, timeout, network error)
// and warrants failover to another endpoint.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check for HTTP 5xx status codes.
	var httpErr stellar.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode >= 500 {
		return true
	}

	// Context deadline exceeded (timeout) is retryable.
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, context.Canceled) {
		return true
	}

	// Generic network-level errors.
	if errors.Is(err, io.EOF) {
		return true
	}

	return false
}

// Ensure http and io imports are used.
var _ = http.StatusOK
var _ = io.EOF
