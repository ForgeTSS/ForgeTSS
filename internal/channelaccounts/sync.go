package channelaccounts

import (
	"context"
	"fmt"
	"log/slog"
)

// SyncSequenceNumbers reconciles every channel account's local sequence number
// against the network via the Horizon client. It is designed to run at startup
// so that leased accounts are not touched — only idle ones are corrected.
func (p *Pool) SyncSequenceNumbers(ctx context.Context) error {
	accts, err := p.store.ListChannelAccounts(ctx)
	if err != nil {
		return fmt.Errorf("listing channel accounts for sync: %w", err)
	}

	for _, acct := range accts {
		// Skip leased accounts — they may be in-flight on this replica or another.
		if acct.Status != AccountStatusIdle {
			slog.Info("skipping leased account during sync", "account", acct.PublicKey)
			continue
		}

		if err := p.syncOne(ctx, acct); err != nil {
			slog.Error("sync failed for account", "account", acct.PublicKey, "error", err)
			// Continue syncing remaining accounts.
		}
	}

	slog.Info("sequence number sync complete", "total", len(accts))
	return nil
}

// syncOne queries the network for the current sequence of a single account and
// updates the local store only if the network sequence is higher.
func (p *Pool) syncOne(ctx context.Context, acct ChannelAccountRecord) error {
	networkSeq, err := p.horizonCli.Account(ctx, acct.PublicKey)
	if err != nil {
		return fmt.Errorf("fetching network sequence for %s: %w", acct.PublicKey, err)
	}

	if networkSeq <= acct.SequenceNumber {
		slog.Debug("sequence already up to date",
			"account", acct.PublicKey,
			"local", acct.SequenceNumber,
			"network", networkSeq)
		return nil
	}

	if err := p.store.UpdateSequenceNumberIfIdle(ctx, acct.PublicKey, networkSeq); err != nil {
		return fmt.Errorf("updating sequence for %s: %w", acct.PublicKey, err)
	}

	slog.Info("synced sequence number",
		"account", acct.PublicKey,
		"from", acct.SequenceNumber,
		"to", networkSeq)
	return nil
}
