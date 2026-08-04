package channelaccounts

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockStore is a minimal Store implementation for testing Lease concurrency.
type mockStore struct {
	mu       sync.Mutex
	accounts []ChannelAccountRecord
	leased   map[string]bool
	leaseErr error
}

func newMockStore(accts []ChannelAccountRecord) *mockStore {
	m := &mockStore{leased: make(map[string]bool)}
	for _, a := range accts {
		m.leased[a.PublicKey] = false
	}
	return m
}

func (m *mockStore) LeaseChannelAccount(ctx context.Context) (*ChannelAccountRecord, error) {
	if m.leaseErr != nil {
		return nil, m.leaseErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.accounts {
		if m.accounts[i].Status == AccountStatusIdle && !m.leased[m.accounts[i].PublicKey] {
			m.leased[m.accounts[i].PublicKey] = true
			return &m.accounts[i], nil
		}
	}
	return nil, nil
}

func (m *mockStore) ReleaseChannelAccount(ctx context.Context, publicKey string, lastUsed time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.leased[publicKey] = false
	return nil
}

func (m *mockStore) ListChannelAccounts(ctx context.Context) ([]ChannelAccountRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cpy := make([]ChannelAccountRecord, len(m.accounts))
	copy(cpy, m.accounts)
	return cpy, nil
}

func (m *mockStore) UpdateSequenceNumberIfIdle(ctx context.Context, publicKey string, seq int64) error {
	return nil
}

func (m *mockStore) SaveChannelAccount(ctx context.Context, acct ChannelAccountRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.accounts = append(m.accounts, acct)
	m.leased[acct.PublicKey] = false
	return nil
}

func TestLease_Concurrent(t *testing.T) {
	accts := []ChannelAccountRecord{
		{PublicKey: "G1", EncryptedSecret: "S1", Status: AccountStatusIdle, SequenceNumber: 1},
		{PublicKey: "G2", EncryptedSecret: "S2", Status: AccountStatusIdle, SequenceNumber: 2},
		{PublicKey: "G3", EncryptedSecret: "S3", Status: AccountStatusIdle, SequenceNumber: 3},
	}

	ms := newMockStore(accts)
	pool := NewPool(ms, nil, "", 0)

	const goroutines = 20
	var wg sync.WaitGroup
	var successCount int64
	var mu sync.Mutex
	seen := make(map[string]bool)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			acct, err := pool.Lease(context.Background())
			if err != nil {
				return
			}
			atomic.AddInt64(&successCount, 1)
			mu.Lock()
			defer mu.Unlock()
			if seen[acct.PublicKey] {
				t.Errorf("duplicate lease: %s leased by two goroutines", acct.PublicKey)
			}
			seen[acct.PublicKey] = true
		}()
	}

	wg.Wait()

	if int(successCount) != len(accts) {
		t.Errorf("expected %d successful leases, got %d", len(accts), successCount)
	}
	if len(seen) != len(accts) {
		t.Errorf("expected %d unique accounts leased, got %d", len(accts), len(seen))
	}
}

func TestLease_NoAccounts(t *testing.T) {
	ms := newMockStore(nil)
	pool := NewPool(ms, nil, "", 0)

	_, err := pool.Lease(context.Background())
	if err == nil {
		t.Fatal("expected error when no accounts available")
	}
}
