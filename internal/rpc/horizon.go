// Package rpc provides Stellar RPC clients and routing logic.
package rpc

import (
	"context"
	"fmt"
	"log/slog"

	stellar "github.com/stellar/go-stellar-sdk"
)

// HorizonClient wraps the Stellar Horizon API for fetching account state,
// submitting transactions, and polling transaction status.
type HorizonClient struct {
	baseURL string
	backend *stellar.Client
}

// NewHorizonClient creates a HorizonClient for the given base URL.
func NewHorizonClient(baseURL string) *HorizonClient {
	return &HorizonClient{
		baseURL: baseURL,
		backend: &stellar.Client{
			URI: baseURL,
		},
	}
}

// Account fetches the current state of a Stellar account by public key.
// It returns the sequence number as an int64.
func (c *HorizonClient) Account(ctx context.Context, accountID string) (int64, error) {
	resp, err := c.backend.LoadAccount(ctx, accountID)
	if err != nil {
		return 0, fmt.Errorf("loading account %s from horizon (%s): %w", accountID, c.baseURL, err)
	}
	return resp.SequenceNum, nil
}

// SubmitTransaction submits a transaction envelope (as XDR string) to Horizon.
func (c *HorizonClient) SubmitTransaction(ctx context.Context, envelopeXDR string) (*stellar.TransactionResult, error) {
	result, err := c.backend.SubmitTransactionXDR(ctx, envelopeXDR)
	if err != nil {
		return nil, fmt.Errorf("submitting transaction via horizon (%s): %w", c.baseURL, err)
	}
	return result, nil
}

// GetTransactionStatus returns the transaction result by hash.
func (c *HorizonClient) GetTransactionStatus(ctx context.Context, hash string) (*stellar.TransactionResult, error) {
	result, err := c.backend.Transaction(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("querying transaction %s from horizon (%s): %w", hash, c.baseURL, err)
	}
	return result, nil
}

// SimulateTransaction is a no-op wrapper for Horizon — Horizon doesn't support
// simulation. Callers should use the Soroban client for simulate.
func (c *HorizonClient) SimulateTransaction(ctx context.Context, envelopeXDR string) (*stellar.SimulateHostFunctionResult, error) {
	slog.Debug("simulate called on horizon client — skipping")
	return nil, nil
}

// BaseURL returns the Horizon endpoint URL.
func (c *HorizonClient) BaseURL() string {
	return c.baseURL
}
