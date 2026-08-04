package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestEnqueue_Success tests successful enqueue returns transaction ID.
func TestEnqueue_Success(t *testing.T) {
	ms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Error("expected POST")
		}
		if auth := r.Header.Get("Authorization"); !strings.HasPrefix(auth, "Bearer ") {
			t.Error("expected Bearer auth")
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": "tx-123"})
	}))
	defer ms.Close()

	c := New(ms.URL, "test-key")
	id, err := c.Enqueue("AAAA.test")
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	if id != "tx-123" {
		t.Errorf("expected tx-123, got %s", id)
	}
}

// TestEnqueue_EmptyBody tests error on non-201 response.
func TestEnqueue_EmptyBody(t *testing.T) {
	ms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ms.Close()

	c := New(ms.URL, "test-key")
	_, err := c.Enqueue("AAAA.test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGetStatus_Success tests retrieving transaction status.
func TestGetStatus_Success(t *testing.T) {
	ms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":         "tx-123",
			"status":     "submitted",
			"retry":      1,
			"last_error": "timeout",
		})
	}))
	defer ms.Close()

	c := New(ms.URL, "test-key")
	status, err := c.GetStatus("tx-123")
	if err != nil {
		t.Fatalf("get status failed: %v", err)
	}
	if status.Status != "submitted" {
		t.Errorf("expected submitted, got %s", status.Status)
	}
}

// TestGetStatus_NotFound tests 404 returns error.
func TestGetStatus_NotFound(t *testing.T) {
	ms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ms.Close()

	c := New(ms.URL, "test-key")
	_, err := c.GetStatus("tx-123")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestNew_Defaults tests client initialization.
func TestNew_Defaults(t *testing.T) {
	c := New("https://api.example.com", "key123")
	if c.baseURL != "https://api.example.com" {
		t.Errorf("unexpected baseURL: %s", c.baseURL)
	}
	if c.apiKey != "key123" {
		t.Errorf("unexpected apiKey: %s", c.apiKey)
	}
	if c.httpClient.Timeout != 30_000_000_000 { // 30s in nanoseconds
		t.Errorf("unexpected timeout: %v", c.httpClient.Timeout)
	}
}
