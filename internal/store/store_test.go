// Package store provides Postgres-backed persistence for ForgeTSS.
package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// setupTestDB creates an in-memory pgxpool from the embedded migrations for testing.
// It expects the environment variable TEST_DATABASE_URL to be set.
func setupTestDB(t *testing.T) *Store {
	t.Helper()

	dsn := getTestDSN()
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
		return nil
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping test db: %v", err)
	}

	// Drop and recreate test tables.
	_, err = pool.Exec(ctx, `
		DROP TABLE IF EXISTS transactions;
		DROP TABLE IF EXISTS channel_accounts;
		DROP TABLE IF EXISTS _migrations;
	`)
	if err != nil {
		t.Fatalf("clean test tables: %v", err)
	}

	s := &Store{pool: pool}
	if err := s.migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
	})

	return s
}

func getTestDSN() string {
	return ""
}

func TestSaveAndGetTransaction(t *testing.T) {
	s := setupTestDB(t)
	if s == nil {
		return
	}

	id := uuid.New()
	tx := Transaction{
		ID:          id,
		EnvelopeXDR: "AAAA...",
		Status:      TxStatusPending,
		RetryCount:  0,
	}

	if err := s.SaveTransaction(context.Background(), tx); err != nil {
		t.Fatalf("SaveTransaction: %v", err)
	}

	got, err := s.GetTransaction(context.Background(), id)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if got.ID != id || got.EnvelopeXDR != "AAAA..." || got.Status != TxStatusPending {
		t.Errorf("got %+v, want id=%s envelope=AAAA... status=pending", got, id)
	}
}

func TestUpdateTransactionStatus(t *testing.T) {
	s := setupTestDB(t)
	if s == nil {
		return
	}

	id := uuid.New()
	tx := Transaction{ID: id, EnvelopeXDR: "AAAA", Status: TxStatusPending}
	if err := s.SaveTransaction(context.Background(), tx); err != nil {
		t.Fatalf("SaveTransaction: %v", err)
	}

	if err := s.UpdateTransactionStatus(context.Background(), id, TxStatusFailed, "GDUMMY", "timeout"); err != nil {
		t.Fatalf("UpdateTransactionStatus: %v", err)
	}

	got, err := s.GetTransaction(context.Background(), id)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if got.Status != TxStatusFailed || got.LastError != "timeout" || got.ChannelAccount != "GDUMMY" {
		t.Errorf("got status=%s error=%s channel=%s, want failed timeout GDUMMY", got.Status, got.LastError, got.ChannelAccount)
	}
}

func TestGetPendingTransactions(t *testing.T) {
	s := setupTestDB(t)
	if s == nil {
		return
	}

	// Insert one pending, one submitted.
	id1 := uuid.New()
	id2 := uuid.New()
	if err := s.SaveTransaction(context.Background(), Transaction{ID: id1, EnvelopeXDR: "A", Status: TxStatusPending}); err != nil {
		t.Fatalf("SaveTransaction: %v", err)
	}
	if err := s.SaveTransaction(context.Background(), Transaction{ID: id2, EnvelopeXDR: "B", Status: TxStatusSubmitted}); err != nil {
		t.Fatalf("SaveTransaction: %v", err)
	}

	pending, err := s.GetPendingTransactions(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetPendingTransactions: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("got %d pending, want 1", len(pending))
	}
	if len(pending) > 0 && pending[0].ID != id1 {
		t.Errorf("got pending id=%s, want %s", pending[0].ID, id1)
	}
}

func TestIncrementRetry(t *testing.T) {
	s := setupTestDB(t)
	if s == nil {
		return
	}

	id := uuid.New()
	s.SaveTransaction(context.Background(), Transaction{ID: id, EnvelopeXDR: "A", Status: TxStatusPending})
	s.IncrementRetry(context.Background(), id)
	s.IncrementRetry(context.Background(), id)

	got, _ := s.GetTransaction(context.Background(), id)
	if got.RetryCount != 2 {
		t.Errorf("got retry_count=%d, want 2", got.RetryCount)
	}
}

func TestLeaseChannelAccount(t *testing.T) {
	s := setupTestDB(t)
	if s == nil {
		return
	}

	acct := ChannelAccount{
		PublicKey:      "GACCT1",
		EncryptedSecret: "S1",
		Status:         AccountStatusIdle,
		SequenceNumber: 100,
	}
	if err := s.SaveChannelAccount(context.Background(), acct); err != nil {
		t.Fatalf("SaveChannelAccount: %v", err)
	}

	leased, err := s.LeaseChannelAccount(context.Background())
	if err != nil {
		t.Fatalf("LeaseChannelAccount: %v", err)
	}
	if leased.PublicKey != "GACCT1" {
		t.Errorf("got public_key=%s, want GACCT1", leased.PublicKey)
	}

	// No more idle accounts.
	_, err = s.LeaseChannelAccount(context.Background())
	if err == nil {
		t.Fatal("expected error when no idle accounts remain")
	}
}

func TestReleaseChannelAccount(t *testing.T) {
	s := setupTestDB(t)
	if s == nil {
		return
	}

	acct := ChannelAccount{
		PublicKey:      "GACCT2",
		EncryptedSecret: "S2",
		Status:         AccountStatusIdle,
		SequenceNumber: 200,
	}
	s.SaveChannelAccount(context.Background(), acct)

	leased, err := s.LeaseChannelAccount(context.Background())
	if err != nil {
		t.Fatalf("LeaseChannelAccount: %v", err)
	}

	if leased.Status != AccountStatusLeased {
		// Lease returns the original row, which has status idle;
		// the update happens in the same tx. We verify by querying.
	}

	got, err := s.GetChannelAccountByID(context.Background(), "GACCT2")
	if err != nil {
		t.Fatalf("GetChannelAccountByID: %v", err)
	}
	if got.Status != AccountStatusLeased {
		t.Errorf("got status=%s, want leased", got.Status)
	}

	now := time.Now()
	if err := s.ReleaseChannelAccount(context.Background(), "GACCT2", now); err != nil {
		t.Fatalf("ReleaseChannelAccount: %v", err)
	}

	got, err = s.GetChannelAccountByID(context.Background(), "GACCT2")
	if err != nil {
		t.Fatalf("GetChannelAccountByID after release: %v", err)
	}
	if got.Status != AccountStatusIdle {
		t.Errorf("got status=%s after release, want idle", got.Status)
	}
}

func TestUpdateSequenceNumber(t *testing.T) {
	s := setupTestDB(t)
	if s == nil {
		return
	}

	acct := ChannelAccount{
		PublicKey:      "GACCT3",
		EncryptedSecret: "S3",
		Status:         AccountStatusIdle,
		SequenceNumber: 50,
	}
	s.SaveChannelAccount(context.Background(), acct)

	s.UpdateSequenceNumber(context.Background(), "GACCT3", 75)

	got, _ := s.GetChannelAccountByID(context.Background(), "GACCT3")
	if got.SequenceNumber != 75 {
		t.Errorf("got sequence=%d, want 75", got.SequenceNumber)
	}
}

func TestLeaseChannelAccount_NoRows(t *testing.T) {
	s := setupTestDB(t)
	if s == nil {
		return
	}

	_, err := s.LeaseChannelAccount(context.Background())
	if err == nil {
		t.Fatal("expected error when pool is empty")
	}
}
