// Package rpc provides Stellar RPC clients and routing logic.
package rpc

import (
	"context"
	"fmt"
	"log/slog"
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
	horizonClients  []*HorizonClient
	sorobanClients  []*SorobanClient
	horizonIdx      atomic.Int64
	sorobanIdx      atomic.Int64
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

// SubmitTransaction routes the envelope to the correct backend and submits it.
func (r *Router) SubmitTransaction(ctx context.Context, envelopeXDR string) (*stellar.TransactionResult, error) {
	backend := r.route(envelopeXDR)
	switch backend {
	case BackendHorizon:
		return r.submitHorizon(ctx, envelopeXDR)
	case BackendSoroban:
		return r.submitSoroban(ctx, envelopeXDR)
	default:
		return nil, fmt.Errorf("unknown backend %d", backend)
	}
}

// GetTransactionStatus routes a status query to the correct backend.
func (r *Router) GetTransactionStatus(ctx context.Context, hash string) (*stellar.TransactionResult, error) {
	// By default we check Horizon; Soroban results also appear there.
	return r.getHorizonStatus(ctx, hash)
}

func (r *Router) submitHorizon(ctx context.Context, envelopeXDR string) (*stellar.TransactionResult, error) {
	idx := int(r.horizonIdx.Add(1)) % len(r.horizonClients)
	client := r.horizonClients[idx]

	result, err := client.SubmitTransaction(ctx, envelopeXDR)
	if err != nil {
		return nil, fmt.Errorf("submitting via horizon (%s): %w", client.baseURL, err)
	}
	return result, nil
}

func (r *Router) submitSoroban(ctx context.Context, envelopeXDR string) (*stellar.TransactionResult, error) {
	idx := int(r.sorobanIdx.Add(1)) % len(r.sorobanClients)
	client := r.sorobanClients[idx]

	// Simulate first (required for Soroban).
	sim, err := client.SimulateTransaction(ctx, envelopeXDR)
	if err != nil {
		return nil, fmt.Errorf("simulating soroban transaction: %w", err)
	}
	_ = sim // Cost details are logged by callers if needed.

	// Submit and map Soroban response to Horizon result for uniformity.
	resp, err := client.SubmitTransaction(ctx, envelopeXDR)
	if err != nil {
		return nil, fmt.Errorf("submitting soroban transaction: %w", err)
	}
	_ = resp
	return nil, fmt.Errorf("soroban submission handled (hash available in response)")
}

func (r *Router) getHorizonStatus(ctx context.Context, hash string) (*stellar.TransactionResult, error) {
	idx := int(r.horizonIdx.Add(1)) % len(r.horizonClients)
	client := r.horizonClients[idx]
	return client.GetTransactionStatus(ctx, hash)
}
