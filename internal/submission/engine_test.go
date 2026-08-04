package submission

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ForgeTSS/ForgeTSS/internal/config"
	"github.com/ForgeTSS/ForgeTSS/internal/store"
	"github.com/google/uuid"
	stellar "github.com/stellar/go-stellar-sdk"
)

// fakeStore is a minimal TxStore implementation for Engine tests.
type fakeStore struct {
	mu           sync.Mutex
	txs          map[uuid.UUID]store.Transaction
	leaseErr     error
	releaseErr   error
	acctSeq      int64
	acctPubKey   string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		txs:   make(map[uuid.UUID]store.Transaction),
		acctSeq: 1,
	}
}

func (f *fakeStore) SaveTransaction(ctx context.Context, tx store.Transaction) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.txs[tx.ID] = tx
	return nil
}

func (f *fakeStore) UpdateTransactionStatus(ctx context.Context, id uuid.UUID, status store.TxStatus, channelAccount string, lastError string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.txs[id]; ok {
		t.Status = status
		t.ChannelAccount = channelAccount
		t.LastError = lastError
		f.txs[id] = t
	}
	return nil
}

func (f *fakeStore) IncrementRetry(ctx context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.txs[id]; ok {
		t.RetryCount++
		f.txs[id] = t
	}
	return nil
}

func (f *fakeStore) GetPendingTransactions(ctx context.Context, limit int) ([]store.Transaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []store.Transaction
	for _, tx := range f.txs {
		if tx.Status == store.TxStatusPending {
			result = append(result, tx)
		}
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (f *fakeStore) GetTransaction(ctx context.Context, id uuid.UUID) (store.Transaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.txs[id]
	if !ok {
		return t, errors.New("not found")
	}
	return t, nil
}

func (f *fakeStore) GetTransactionForRetry(ctx context.Context, id uuid.UUID) (store.Transaction, error) {
	return f.GetTransaction(ctx, id)
}

func (f *fakeStore) UpdateSequenceNumber(ctx context.Context, publicKey string, seq int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acctPubKey = publicKey
	f.acctSeq = seq
	return nil
}

// fakeSubmitter is a no-op Submitter for tests.
type fakeSubmitter struct {
	submitErr error
}

func (f *fakeSubmitter) SubmitTransaction(ctx context.Context, envelopeXDR string) (*stellar.TransactionResult, error) {
	if f.submitErr != nil {
		return nil, f.submitErr
	}
	return nil, nil
}

// TestEnqueue tests the Enqueue validation and persistence.
func TestEnqueue(t *testing.T) {
	fs := newFakeStore()
	cfg := &config.Config{}
	eng := NewEngine(fs, &fakeSubmitter{}, nil, cfg)

	id, err := eng.Enqueue(context.Background(), Request{EnvelopeXDR: ""})
	if err == nil {
		t.Error("expected error for empty envelope")
	}
	_ = id
}

// TestBackoffDuration tests the exponential backoff calculation.
func TestBackoffDuration(t *testing.T) {
	base := time.Second
	mult := 2.0

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
	}

	for _, tt := range tests {
		got := backoffDuration(tt.attempt, base, mult)
		if got != tt.want {
			t.Errorf("backoffDuration(%d, %v, %.1f) = %v, want %v", tt.attempt, base, mult, got, tt.want)
		}
	}
}

// TestBackoffDuration_Negative tests that negative attempt returns base.
func TestBackoffDuration_Negative(t *testing.T) {
	base := time.Second
	mult := 2.0

	got := backoffDuration(-1, base, mult)
	if got != base {
		t.Errorf("backoffDuration(-1) = %v, want %v", got, base)
	}
}
