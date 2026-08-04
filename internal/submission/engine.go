// Package submission provides the background engine that processes pending
// transactions from the queue, leasing channel accounts and submitting via RPC.
package submission

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

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

// ProcessQueue is the main background loop. It polls for pending transactions,
// leases a channel account for each, increments the sequence number, submits,
// and updates status. It runs until ctx is cancelled.
func (e *Engine) ProcessQueue(ctx context.Context) error {
	ticker := time.NewTicker(e.cfg.QueuePollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("submission queue processor shutting down")
			return ctx.Err()
		case <-ticker.C:
			e.processBatch(ctx)
		}
	}
}

// processBatch fetches pending transactions and processes one at a time.
// It acquires a channel account per transaction, increments the sequence,
// and submits via the router.
func (e *Engine) processBatch(ctx context.Context) {
	txs, err := e.store.GetPendingTransactions(ctx, 1) // process one at a time to avoid pool exhaustion
	if err != nil {
		slog.Error("fetching pending transactions", "error", err)
		return
	}
	if len(txs) == 0 {
		return
	}

	tx := txs[0]

	// Mark as submitting so other workers don't pick it up.
	if err := e.store.UpdateTransactionStatus(ctx, tx.ID, store.TxStatusSubmitting, "", ""); err != nil {
		slog.Warn("marking transaction submitting (already picked up?)", "tx_id", tx.ID, "error", err)
		return
	}

	// Lease a channel account.
	acct, err := e.pool.Lease(ctx)
	if err != nil {
		slog.Error("no channel account available", "tx_id", tx.ID, "error", err)
		if err := e.store.UpdateTransactionStatus(ctx, tx.ID, store.TxStatusFailed, "", fmt.Errorf("no channel account: %w", err).Error()); err != nil {
			slog.Error("updating failed status", "tx_id", tx.ID, "error", err)
		}
		return
	}

	// Increment sequence number for this transaction.
	newSeq := acct.SequenceNumber + 1
	if err := e.store.UpdateSequenceNumber(ctx, acct.PublicKey, newSeq); err != nil {
		slog.Error("updating sequence number", "account", acct.PublicKey, "error", err)
	}

	// Submit the transaction.
	status, submitErr := e.router.SubmitTransaction(ctx, tx.EnvelopeXDR)

	// Release the channel account.
	if err := e.pool.Release(ctx, acct); err != nil {
		slog.Error("releasing channel account", "account", acct.PublicKey, "error", err)
	}

	if submitErr != nil {
		e.failTransaction(ctx, tx.ID, acct.PublicKey, submitErr)
		return
	}

	// Check submission result.
	if status != nil && status.ResultCodes.Operation {
		e.failTransaction(ctx, tx.ID, acct.PublicKey, errors.New("operation rejected by horizon"))
		return
	}

	e.store.UpdateTransactionStatus(ctx, tx.ID, store.TxStatusConfirmed, acct.PublicKey, "")
	slog.Info("transaction confirmed", "tx_id", tx.ID, "channel_account", acct.PublicKey)
}

// failTransaction marks a transaction as failed and persists the error.
func (e *Engine) failTransaction(ctx context.Context, id uuid.UUID, channelAccount string, err error) {
	msg := err.Error()
	if err := e.store.UpdateTransactionStatus(ctx, id, store.TxStatusFailed, channelAccount, msg); err != nil {
		slog.Error("updating failed status", "tx_id", id, "error", err)
	}
	slog.Error("transaction failed", "tx_id", id, "channel_account", channelAccount, "error", err)
}

// backoffDuration calculates the exponential backoff delay for a given retry count.
func backoffDuration(attempt int, base time.Duration, multiplier float64) time.Duration {
	if attempt <= 0 {
		return base
	}
	delay := float64(base) * math.Pow(multiplier, float64(attempt))
	return time.Duration(delay)
}
