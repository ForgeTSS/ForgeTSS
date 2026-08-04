package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// AccountStatus represents the state of a channel account.
type AccountStatus string

const (
	AccountStatusIdle    AccountStatus = "idle"
	AccountStatusLeased  AccountStatus = "leased"
)

// ChannelAccount is a Stellar account used for submitting transactions.
type ChannelAccount struct {
	PublicKey      string
	EncryptedSecret string
	Status         AccountStatus
	SequenceNumber int64
	LastUsedAt     *time.Time
}

// SaveChannelAccount inserts a new channel account record.
func (s *Store) SaveChannelAccount(ctx context.Context, acct ChannelAccount) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO channel_accounts
			(public_key, encrypted_secret, status, sequence_number, last_used_at)
		VALUES ($1, $2, $3, $4, $5)
	`, acct.PublicKey, acct.EncryptedSecret, acct.Status, acct.SequenceNumber, acct.LastUsedAt)
	if err != nil {
		return fmt.Errorf("saving channel account %s: %w", acct.PublicKey, err)
	}
	return nil
}

// GetChannelAccountByID returns a single channel account by public key.
func (s *Store) GetChannelAccountByID(ctx context.Context, publicKey string) (ChannelAccount, error) {
	var acct ChannelAccount
	err := s.pool.QueryRow(ctx, `
		SELECT public_key, encrypted_secret, status, sequence_number, last_used_at
		FROM channel_accounts
		WHERE public_key = $1
	`, publicKey).Scan(
		&acct.PublicKey, &acct.EncryptedSecret, &acct.Status,
		&acct.SequenceNumber, &acct.LastUsedAt,
	)
	if err != nil {
		return acct, fmt.Errorf("fetching channel account %s: %w", publicKey, err)
	}
	return acct, nil
}

// ListChannelAccounts returns all channel accounts, optionally filtered by status.
func (s *Store) ListChannelAccounts(ctx context.Context, status *AccountStatus) ([]ChannelAccount, error) {
	var query string
	var args []interface{}

	if status == nil {
		query = `
			SELECT public_key, encrypted_secret, status, sequence_number, last_used_at
			FROM channel_accounts
			ORDER BY public_key
		`
	} else {
		query = `
			SELECT public_key, encrypted_secret, status, sequence_number, last_used_at
			FROM channel_accounts
			WHERE status = $1
			ORDER BY public_key
		`
		args = append(args, *status)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing channel accounts: %w", err)
	}
	defer rows.Close()

	accts, err := pgx.CollectRows(rows, pgx.RowToStructByName[ChannelAccount])
	if err != nil {
		return nil, fmt.Errorf("collecting channel accounts: %w", err)
	}
	return accts, nil
}

// ReleaseChannelAccount sets the status of a channel account back to idle and updates last_used_at.
func (s *Store) ReleaseChannelAccount(ctx context.Context, publicKey string, lastUsed time.Time) error {
	result, err := s.pool.Exec(ctx, `
		UPDATE channel_accounts
		SET status = 'idle', last_used_at = $2
		WHERE public_key = $1
	`, publicKey, lastUsed)
	if err != nil {
		return fmt.Errorf("releasing channel account %s: %w", publicKey, err)
	}
	if result.RowsAffected() == 0 {
		return errors.New("channel account not found")
	}
	return nil
}

// LeaseChannelAccount atomically picks an idle channel account and marks it leased.
// It uses SELECT ... FOR UPDATE SKIP LOCKED so multiple replicas never lease the same account.
func (s *Store) LeaseChannelAccount(ctx context.Context) (*ChannelAccount, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning lease transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var acct ChannelAccount
	err = tx.QueryRow(ctx, `
		SELECT public_key, encrypted_secret, status, sequence_number, last_used_at
		FROM channel_accounts
		WHERE status = 'idle'
		ORDER BY last_used_at ASC NULLS FIRST
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`).Scan(
		&acct.PublicKey, &acct.EncryptedSecret, &acct.Status,
		&acct.SequenceNumber, &acct.LastUsedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("no idle channel account available: %w", err)
		}
		return nil, fmt.Errorf("leasing channel account: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE channel_accounts SET status = 'leased' WHERE public_key = $1
	`, acct.PublicKey); err != nil {
		return nil, fmt.Errorf("marking account %s leased: %w", acct.PublicKey, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing lease: %w", err)
	}

	return &acct, nil
}

// UpdateSequenceNumber sets the sequence number for a channel account.
func (s *Store) UpdateSequenceNumber(ctx context.Context, publicKey string, seq int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE channel_accounts SET sequence_number = $2 WHERE public_key = $1
	`, publicKey, seq)
	if err != nil {
		return fmt.Errorf("updating sequence number for %s: %w", publicKey, err)
	}
	return nil
}

// UpdateSequenceNumberIfIdle updates the sequence number only for idle accounts,
// used during startup reconciliation so we don't overwrite sequence numbers of
// in-flight accounts leased by other replicas.
func (s *Store) UpdateSequenceNumberIfIdle(ctx context.Context, publicKey string, seq int64) error {
	result, err := s.pool.Exec(ctx, `
		UPDATE channel_accounts SET sequence_number = $2
		WHERE public_key = $1 AND status = 'idle'
	`, publicKey, seq)
	if err != nil {
		return fmt.Errorf("updating sequence number for idle account %s: %w", publicKey, err)
	}
	_ = result.RowsAffected()
	return nil
}
