// Package store provides Postgres-backed persistence for ForgeTSS.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TxStatus represents the lifecycle state of a submitted transaction.
type TxStatus string

const (
	TxStatusPending   TxStatus = "pending"
	TxStatusSubmitting TxStatus = "submitting"
	TxStatusSubmitted  TxStatus = "submitted"
	TxStatusFailed    TxStatus = "failed"
	TxStatusConfirmed TxStatus = "confirmed"
)

// Transaction is a stored transaction envelope awaiting submission.
type Transaction struct {
	ID            uuid.UUID
	EnvelopeXDR   string
	Status        TxStatus
	ChannelAccount string
	RetryCount    int
	LastError    string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// SaveTransaction writes a new transaction record with status pending.
func (s *Store) SaveTransaction(ctx context.Context, tx Transaction) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO transactions
			(id, envelope_xdr, status, retry_count)
		VALUES ($1, $2, $3, $4)
	`, tx.ID, tx.EnvelopeXDR, tx.Status, tx.RetryCount)
	if err != nil {
		return fmt.Errorf("saving transaction %s: %w", tx.ID, err)
	}
	return nil
}

// UpdateTransactionStatus updates the status, error, and channel account of an existing record.
func (s *Store) UpdateTransactionStatus(ctx context.Context, id uuid.UUID, status TxStatus, channelAccount string, lastError string) error {
	query := `
		UPDATE transactions
		SET status = $2, channel_account = $3, last_error = $4, updated_at = now()
		WHERE id = $1
	`
	result, err := s.pool.Exec(ctx, query, id, status, channelAccount, lastError)
	if err != nil {
		return fmt.Errorf("updating transaction %s status: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return errors.New("transaction not found")
	}
	return nil
}

// IncrementRetry bumps retry_count by one for the given transaction.
func (s *Store) IncrementRetry(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE transactions
		SET retry_count = retry_count + 1, updated_at = now()
		WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("incrementing retry for %s: %w", id, err)
	}
	return nil
}

// GetPendingTransactions returns up to limit transactions that are in pending state,
// ordered by creation time so oldest are processed first.
func (s *Store) GetPendingTransactions(ctx context.Context, limit int) ([]Transaction, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, envelope_xdr, status, channel_account, retry_count,
		       last_error, created_at, updated_at
		FROM transactions
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying pending transactions: %w", err)
	}
	defer rows.Close()

	txs, err := pgx.CollectRows(rows, pgx.RowToStructByName[Transaction])
	if err != nil {
		return nil, fmt.Errorf("collecting pending transactions: %w", err)
	}
	return txs, nil
}

// GetTransaction returns a single transaction by ID.
func (s *Store) GetTransaction(ctx context.Context, id uuid.UUID) (Transaction, error) {
	var tx Transaction
	err := s.pool.QueryRow(ctx, `
		SELECT id, envelope_xdr, status, channel_account, retry_count,
		       last_error, created_at, updated_at
		FROM transactions
		WHERE id = $1
	`, id).Scan(
		&tx.ID, &tx.EnvelopeXDR, &tx.Status, &tx.ChannelAccount,
		&tx.RetryCount, &tx.LastError, &tx.CreatedAt, &tx.UpdatedAt,
	)
	if err != nil {
		return tx, fmt.Errorf("fetching transaction %s: %w", id, err)
	}
	return tx, nil
}

// GetTransactionForRetry returns a pending transaction with pessimistic row lock
// so no other worker steals it during retry processing.
func (s *Store) GetTransactionForRetry(ctx context.Context, id uuid.UUID) (Transaction, error) {
	var tx Transaction
	err := s.pool.QueryRow(ctx, `
		SELECT id, envelope_xdr, status, channel_account, retry_count,
		       last_error, created_at, updated_at
		FROM transactions
		WHERE id = $1
		FOR UPDATE SKIP LOCKED
	`, id).Scan(
		&tx.ID, &tx.EnvelopeXDR, &tx.Status, &tx.ChannelAccount,
		&tx.RetryCount, &tx.LastError, &tx.CreatedAt, &tx.UpdatedAt,
	)
	if err != nil {
		return tx, fmt.Errorf("fetching transaction %s for retry: %w", id, err)
	}
	return tx, nil
}
