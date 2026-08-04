package rpc

import (
	"context"
	"fmt"

	"github.com/stellar/go-stellar-sdk"
	sorobanrpc "github.com/stellar/go-stellar-sdk/soroban/rpc"
)

// SorobanClient wraps the Soroban JSON-RPC client for simulation, submission,
// and status polling of Soroban (smart contract) transactions.
type SorobanClient struct {
	backend *sorobanrpc.Client
}

// NewSorobanClient creates a SorobanClient for the given JSON-RPC URL.
func NewSorobanClient(rpcURL string) (*SorobanClient, error) {
	c, err := sorobanrpc.New(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("creating soroban rpc client (%s): %w", rpcURL, err)
	}
	return &SorobanClient{backend: c}, nil
}

// SimulateTransaction simulates a transaction on the Soroban network and returns
// the cost estimate and results. This must be called before SubmitTransaction
// for any InvokeHostFunction operation.
func (c *SorobanClient) SimulateTransaction(ctx context.Context, envelopeXDR string) (*sorobanrpc.SimulateHostFunctionResult, error) {
	tx, err := stellar.ParseEnvelopeXDR(envelopeXDR, stellar.NetworkPublicTestnet)
	if err != nil {
		return nil, fmt.Errorf("parsing envelope for simulation: %w", err)
	}

	result, err := c.backend.SimulateTransaction(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("simulating transaction on soroban: %w", err)
	}
	return result, nil
}

// SubmitTransaction submits a transaction envelope (as XDR string) to the Soroban network.
func (c *SorobanClient) SubmitTransaction(ctx context.Context, envelopeXDR string) (*sorobanrpc.SendTransactionResponse, error) {
	tx, err := stellar.ParseEnvelopeXDR(envelopeXDR, stellar.NetworkPublicTestnet)
	if err != nil {
		return nil, fmt.Errorf("parsing envelope for submission: %w", err)
	}

	result, err := c.backend.SendTransaction(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("submitting transaction on soroban: %w", err)
	}
	return result, nil
}

// GetTransactionStatus polls the Soroban network for the status of a transaction by hash.
func (c *SorobanClient) GetTransactionStatus(ctx context.Context, hash string) (*sorobanrpc.GetTransactionResponse, error) {
	result, err := c.backend.GetTransaction(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("querying transaction %s on soroban: %w", hash, err)
	}
	return result, nil
}

// BaseURL returns the Soroban RPC endpoint URL.
func (c *SorobanClient) BaseURL() string {
	return c.backend.URI()
}
