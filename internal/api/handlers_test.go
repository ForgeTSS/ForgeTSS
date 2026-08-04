package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/ForgeTSS/ForgeTSS/internal/config"
	"github.com/ForgeTSS/ForgeTSS/internal/store"
	"github.com/google/uuid"
)

// errTxNotFound is a sentinel error for missing transactions in tests.
var errTxNotFound = errors.New("transaction not found")

// mockTxStore implements store methods needed by handlers for testing.
type mockTxStore struct {
	mu       sync.Mutex
	txs      map[uuid.UUID]store.Transaction
	saveErr  error
	getErr   error
}

func newMockTxStore() *mockTxStore {
	return &mockTxStore{txs: make(map[uuid.UUID]store.Transaction)}
}

func (m *mockTxStore) SaveTransaction(ctx context.Context, tx store.Transaction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saveErr != nil {
		return m.saveErr
	}
	m.txs[tx.ID] = tx
	return nil
}

func (m *mockTxStore) GetTransaction(ctx context.Context, id uuid.UUID) (store.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return store.Transaction{}, m.getErr
	}
	tx, ok := m.txs[id]
	if !ok {
		return store.Transaction{}, errTxNotFound
	}
	return tx, nil
}

// Stub remaining interface methods — handlers only call the two above.
func (m *mockTxStore) UpdateTransactionStatus(ctx context.Context, id uuid.UUID, status store.TxStatus, channelAccount string, lastError string) error { return nil }
func (m *mockTxStore) IncrementRetry(ctx context.Context, id uuid.UUID) error { return nil }
func (m *mockTxStore) GetPendingTransactions(ctx context.Context, limit int) ([]store.Transaction, error) { return nil, nil }
func (m *mockTxStore) GetTransactionForRetry(ctx context.Context, id uuid.UUID) (store.Transaction, error) {
	return store.Transaction{}, nil
}
func (m *mockTxStore) SaveChannelAccount(ctx context.Context, acct store.ChannelAccount) error { return nil }
func (m *mockTxStore) GetChannelAccountByID(ctx context.Context, id uuid.UUID) (store.ChannelAccount, error) {
	return store.ChannelAccount{}, nil
}
func (m *mockTxStore) ListChannelAccounts(ctx context.Context) ([]store.ChannelAccount, error) { return nil, nil }
func (m *mockTxStore) ReleaseChannelAccount(ctx context.Context, id uuid.UUID) error { return nil }
func (m *mockTxStore) LeaseChannelAccount(ctx context.Context) (store.ChannelAccount, error) {
	return store.ChannelAccount{}, nil
}
func (m *mockTxStore) UpdateSequenceNumber(ctx context.Context, publicKey string, seq int64) error { return nil }
func (m *mockTxStore) UpdateSequenceNumberIfIdle(ctx context.Context, publicKey string, seq int64) error {
	return nil
}

// TestEnqueueHandler_Valid tests successful enqueue with valid XDR.
func TestEnqueueHandler_Valid(t *testing.T) {
	ms := newMockTxStore()
	srv := &Server{store: ms, cfg: &config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/transactions", bytes.NewBufferString(`{"envelope_xdr":"AAAA.test"}`))
	rec := httptest.NewRecorder()

	srv.enqueueHandler(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rec.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["id"] == "" {
		t.Error("expected non-empty id in response")
	}
}

// TestEnqueueHandler_EmptyEnvelope tests 400 for missing XDR.
func TestEnqueueHandler_EmptyEnvelope(t *testing.T) {
	ms := newMockTxStore()
	srv := &Server{store: ms, cfg: &config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/transactions", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()

	srv.enqueueHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// TestEnqueueHandler_InvalidJSON tests 400 for malformed JSON.
func TestEnqueueHandler_InvalidJSON(t *testing.T) {
	ms := newMockTxStore()
	srv := &Server{store: ms, cfg: &config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/transactions", bytes.NewBufferString(`not json`))
	rec := httptest.NewRecorder()

	srv.enqueueHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// TestEnqueueHandler_SaveError tests 500 when store save fails.
func TestEnqueueHandler_SaveError(t *testing.T) {
	ms := newMockTxStore()
	ms.saveErr = errors.New("save failed")
	srv := &Server{store: ms, cfg: &config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/transactions", bytes.NewBufferString(`{"envelope_xdr":"AAAA.test"}`))
	rec := httptest.NewRecorder()

	srv.enqueueHandler(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// TestGetTxHandler_Found tests 200 with transaction details.
func TestGetTxHandler_Found(t *testing.T) {
	ms := newMockTxStore()
	id := uuid.New()
	ms.txs[id] = store.Transaction{
		ID:       id,
		Status:   store.TxStatusSubmitted,
		RetryCount: 1,
		LastError: "timeout",
	}
	srv := &Server{store: ms, cfg: &config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/transactions/"+id.String(), nil)
	rec := httptest.NewRecorder()

	srv.getTxHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "submitted" {
		t.Errorf("expected status 'submitted', got %v", resp["status"])
	}
}

// TestGetTxHandler_NotFound tests 404 for unknown ID.
func TestGetTxHandler_NotFound(t *testing.T) {
	ms := newMockTxStore()
	id := uuid.New()
	srv := &Server{store: ms, cfg: &config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/transactions/"+id.String(), nil)
	rec := httptest.NewRecorder()

	srv.getTxHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// TestGetTxHandler_InvalidID tests 400 for malformed UUID.
func TestGetTxHandler_InvalidID(t *testing.T) {
	ms := newMockTxStore()
	srv := &Server{store: ms, cfg: &config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/transactions/not-a-uuid", nil)
	rec := httptest.NewRecorder()

	srv.getTxHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// TestHealthHandler tests 200 OK with JSON body.
func TestHealthHandler(t *testing.T) {
	srv := &Server{cfg: &config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	srv.healthHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", resp["status"])
	}
}

// TestWriteJSON tests the writeJSON helper.
func TestWriteJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	writeJSON(rec, http.StatusCreated, map[string]string{"key": "value"})

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}
