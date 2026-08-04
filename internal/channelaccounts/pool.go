// Package channelaccounts provides a pool manager for Stellar channel accounts.
//
// The pool uses a DB-level row lock (SELECT ... FOR UPDATE SKIP LOCKED) so that
// multiple ForgeTSS replicas can safely lease accounts without contention.
package channelaccounts

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gamp/forgetss/internal/rpc"
	"github.com/gamp/forgetss/internal/store"
)

// Account represents a leased channel account.
type Account struct {
	PublicKey      string
	EncryptedSecret string
	SequenceNumber int64
}

// Pool manages channel accounts: leasing, releasing, and syncing sequence numbers.
type Pool struct {
	store    *store.Store
	router   *rpc.Router
	master   string
	refillN  int
}

// NewPool creates a new pool backed by the given store and router.
// master is the master seed used for funding new accounts during Refill.
func NewPool(store *store.Store, router *rpc.Router, master string, refillN int) *Pool {
	return &Pool{store: store, router: router, master: master, refillN: refillN}
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
		PublicKey:      acct.PublicKey,
		EncryptedSecret: acct.EncryptedSecret,
		SequenceNumber: acct.SequenceNumber,
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
