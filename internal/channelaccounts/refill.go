package channelaccounts

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"

	"github.com/gamp/forgetss/internal/store"
	"github.com/stellar/go-stellar-sdk/keypair"
)

// Refill creates count new Stellar accounts and inserts them into the pool.
//
// In production the accounts should be funded from a master distribution account
// before they become usable. The caller is expected to run a funding workflow
// (or use the Stellar faucet on testnet) after Refill completes.
func (p *Pool) Refill(ctx context.Context, count int) error {
	if count < 1 {
		return fmt.Errorf("refill count must be >= 1, got %d", count)
	}

	for i := 0; i < count; i++ {
		if err := p.refillOne(ctx); err != nil {
			slog.Error("failed to refill channel account", "index", i, "error", err)
			return fmt.Errorf("refilling account %d: %w", i, err)
		}
	}

	slog.Info("refilled channel accounts", "count", count)
	return nil
}

// refillOne generates a random Stellar keypair, derives the public key and
// encrypted secret, and inserts the record into the store.
func (p *Pool) refillOne(ctx context.Context) error {
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return fmt.Errorf("generating seed: %w", err)
	}

	kp, err := keypair.FromRawEntropy(seed)
	if err != nil {
		return fmt.Errorf("deriving keypair from seed: %w", err)
	}

	acct := store.ChannelAccount{
		PublicKey:       kp.Address(),
		EncryptedSecret: kp.Private(),
		Status:          store.AccountStatusIdle,
		SequenceNumber:  0,
	}

	if err := p.store.SaveChannelAccount(ctx, acct); err != nil {
		return fmt.Errorf("saving channel account %s: %w", kp.Address(), err)
	}

	slog.Info("created channel account", "public_key", kp.Address())
	return nil
}
