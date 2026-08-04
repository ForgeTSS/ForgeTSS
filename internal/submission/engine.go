// Package submission provides the background engine that processes pending
// transactions from the queue, leasing channel accounts and submitting via RPC.
package submission

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gamp/forgetss/internal/channelaccounts"
	"github.com/gamp/forgetss/internal/config"
	"github.com/gamp/forgetss/internal/rpc"
	"github.com/gamp/forgetss/internal/store"
	"github.com/google/uuid"
	stellar "github.com/stellar/go-stellar-sdk"
)

// Request is the payload accepted by Enqueue.
type Request struct {
	EnvelopeXDR string
}

// Engine manages the transaction submission queue.
type Engine struct {
	store    *store.Store
	router   *rpc.Router
	pool     *channelaccounts.Pool
	cfg      *config.Config
}

// NewEngine creates a new submission engine.
func NewEngine(store *store.Store, router *rpc.Router, pool *channelaccounts.Pool, cfg *config.Config) *Engine {
	return &Engine{store: store, router: router, pool: pool, cfg: cfg}
}

// Enqueue validates and persists a transaction envelope as pending.
// It returns the transaction ID for later status polling.
func (e *Engine) Enqueue(ctx context.Context, req Request) (uuid.UUID, error) {
	if req.EnvelopeXDR == "" {
		return uuid.UUID{}, fmt.Errorf("envelope XDR is required")
	}

	// Validate that the envelope is parseable Stellar XDR.
	_, err := stellar.ParseEnvelopeXDR(req.EnvelopeXDR, stellar.NetworkPublicTestnet)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("envelope XDR is not valid: %w", err)
	}

	id := uuid.New()
	tx := store.Transaction{
		ID:          id,
		EnvelopeXDR: req.EnvelopeXDR,
		Status:      store.TxStatusPending,
		RetryCount:  0,
	}

	if err := e.store.SaveTransaction(ctx, tx); err != nil {
		return uuid.UUID{}, fmt.Errorf("enqueuing transaction: %w", err)
	}

	slog.Info("enqueued transaction", "tx_id", id)
	return id, nil
}
