package submission

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ForgeTSS/ForgeTSS/internal/store"
	"github.com/google/uuid"
	stellar "github.com/stellar/go-stellar-sdk"
	"github.com/stellar/go-stellar-sdk/pkg/txnbuild"
)

// Retry fee-bumps and resubmits a failed transaction with exponential backoff.
// Max retry attempts and the backoff multiplier are read from config.
func (e *Engine) Retry(ctx context.Context, id uuid.UUID) error {
	tx, err := e.store.GetTransactionForRetry(ctx, id)
	if err != nil {
		return fmt.Errorf("fetching transaction %s for retry: %w", id, err)
	}

	if tx.Status != store.TxStatusFailed {
		return fmt.Errorf("transaction %s is not in failed state (status=%s)", id, tx.Status)
	}

	if tx.RetryCount >= e.cfg.MaxRetryAttempts {
		slog.Warn("max retry attempts reached, giving up", "tx_id", id, "max", e.cfg.MaxRetryAttempts)
		return fmt.Errorf("max retry attempts (%d) reached for %s", e.cfg.MaxRetryAttempts, id)
	}

	// Calculate backoff delay.
	delay := backoffDuration(tx.RetryCount, e.cfg.RetryBaseDelay, e.cfg.RetryMultiplier)
	slog.Info("retrying transaction with backoff", "tx_id", id, "delay", delay, "attempt", tx.RetryCount+1)

	// Brief sleep to allow the backoff delay (in a production system this would
	// be scheduled; here we wait inline for simplicity).
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
	}

	// Create a fee-bump transaction.
	fbEnvelope, err := e.createFeeBump(ctx, tx)
	if err != nil {
		return fmt.Errorf("creating fee bump for %s: %w", id, err)
	}

	// Increment retry count.
	if err := e.store.IncrementRetry(ctx, id); err != nil {
		return fmt.Errorf("incrementing retry for %s: %w", id, err)
	}

	// Mark as submitting.
	if err := e.store.UpdateTransactionStatus(ctx, id, store.TxStatusSubmitting, "", ""); err != nil {
		return fmt.Errorf("marking %s as submitting for retry: %w", id, err)
	}

	// Submit the fee-bumped transaction.
	result, err := e.router.SubmitTransaction(ctx, fbEnvelope)
	if err != nil {
		return fmt.Errorf("submitting fee bump for %s: %w", id, err)
	}

	// Check result.
	if result != nil && result.ResultCodes.Operation {
		e.store.UpdateTransactionStatus(ctx, id, store.TxStatusFailed, "", "fee bump operation rejected")
		return fmt.Errorf("fee bump operation rejected")
	}

	e.store.UpdateTransactionStatus(ctx, id, store.TxStatusConfirmed, "", "")
	slog.Info("fee-bumped transaction confirmed", "tx_id", id)
	return nil
}

// createFeeBump wraps the original transaction envelope in a fee-bump envelope
// with a higher fee to incentivize faster inclusion.
func (e *Engine) createFeeBump(ctx context.Context, tx store.Transaction) (string, error) {
	// Parse the original envelope.
	origTx, err := stellar.ParseEnvelopeXDR(tx.EnvelopeXDR, stellar.NetworkPublicTestnet)
	if err != nil {
		return "", fmt.Errorf("parsing original envelope: %w", err)
	}

	// Use the channel account as the fee source (it's already known on the network).
	feeSource := origTx.SourceAccount()

	// Build a fee-bump transaction with 10x the original fee.
	// The fee source signs the fee-bump envelope.
	fbTx, err := txnbuild.NewFeeBumpTransaction(
		txnbuild.FeeBumpTransactionOptions{
			Envelope:     origTx,
			Fee:          origTx.Fee() * 10,
			SourceAccount: feeSource,
		},
	)
	if err != nil {
		return "", fmt.Errorf("building fee bump: %w", err)
	}

	// Serialize back to XDR.
	fbXDR, err := fbTx.MarshalXDR()
	if err != nil {
		return "", fmt.Errorf("serializing fee bump: %w", err)
	}

	return string(fbXDR), nil
}
