// Package channelaccounts provides a pool manager for Stellar channel accounts.
//
// The pool uses a DB-level row lock (SELECT ... FOR UPDATE SKIP LOCKED) so that
// multiple ForgeTSS replicas can safely lease accounts without contention.
package channelaccounts

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"
)

// Account represents a leased channel account.
type Account struct {
	PublicKey       string
	EncryptedSecret string
	SequenceNumber  int64
}

// HorizonClient abstracts Horizon queries for sequence number fetching.
type HorizonClient interface {
	// Account fetches the current state of a Stellar account by public key.
	Account(ctx context.Context, accountID string) (int64, error)
}

// Pool manages channel accounts: leasing, releasing, and syncing sequence numbers.
type Pool struct {
	store      *Store
	horizonCli HorizonClient
	master     string
	refillN    int
}

// Store abstracts the store package to avoid circular imports.
type Store interface {
	LeaseChannelAccount(ctx context.Context) (*ChannelAccountRecord, error)
	ReleaseChannelAccount(ctx context.Context, publicKey string, lastUsed time.Time) error
	ListChannelAccounts(ctx context.Context) ([]ChannelAccountRecord, error)
	UpdateSequenceNumberIfIdle(ctx context.Context, publicKey string, seq int64) error
	SaveChannelAccount(ctx context.Context, acct ChannelAccountRecord) error
}

// ChannelAccountRecord is a channel account row from the store.
type ChannelAccountRecord struct {
	PublicKey       string
	EncryptedSecret string
	Status          AccountStatus
	SequenceNumber  int64
	LastUsedAt      *time.Time
}

// AccountStatus represents the state of a channel account.
type AccountStatus string

const (
	AccountStatusIdle  AccountStatus = "idle"
	AccountStatusLeased AccountStatus = "leased"
)

// NewPool creates a new pool backed by the given store and Horizon client.
// master is the master seed used for funding new accounts during Refill.
func NewPool(store Store, horizonCli HorizonClient, master string, refillN int) *Pool {
	return &Pool{store: store, horizonCli: horizonCli, master: master, refillN: refillN}
}

// Lease picks an idle channel account from the pool and marks it leased.
// It returns an error when no idle accounts are available; callers should
// trigger Refill in that case.
func (p *Pool) Lease(ctx context.Context) (Account, error) {
	acct, err := p.store.LeaseChannelAccount(ctx)
	if err != nil {
		return Account{}, fmt.Errorf("leasing channel account from pool: %w", err)
	}
	return Account{
		PublicKey:       acct.PublicKey,
		EncryptedSecret: acct.EncryptedSecret,
		SequenceNumber:  acct.SequenceNumber,
	}, nil
}

// Release marks a previously leased account as idle again and updates last_used_at.
func (p *Pool) Release(ctx context.Context, acct Account) error {
	now := time.Now()
	if err := p.store.ReleaseChannelAccount(ctx, acct.PublicKey, now); err != nil {
		return fmt.Errorf("releasing channel account %s: %w", acct.PublicKey, err)
	}
	slog.Info("released channel account", "account", acct.PublicKey)
	return nil
}

// parseSeqToInt64 converts a string sequence number to int64.
func parseSeqToInt64(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
