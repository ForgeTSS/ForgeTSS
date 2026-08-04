// Package api provides HTTP handlers for ForgeTSS.
package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ForgeTSS/ForgeTSS/internal/metrics"
	"github.com/ForgeTSS/ForgeTSS/internal/store"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// enqueueHandler handles POST /transactions — enqueues a transaction envelope.
func (s *Server) enqueueHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EnvelopeXDR string `json:"envelope_xdr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.EnvelopeXDR == "" {
		http.Error(w, "envelope_xdr is required", http.StatusBadRequest)
		return
	}

	id := uuid.New()
	tx := store.Transaction{
		ID:            id,
		EnvelopeXDR:   req.EnvelopeXDR,
		Status:        store.TxStatusPending,
		RetryCount:    0,
	}
	if err := s.store.SaveTransaction(r.Context(), tx); err != nil {
		slog.Error("save transaction", "err", err)
		http.Error(w, "failed to save transaction", http.StatusInternalServerError)
		return
	}

	slog.Info("enqueued transaction", "tx_id", id, "envelope_length", len(req.EnvelopeXDR))

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": id.String()})
}

// getTxHandler handles GET /transactions/{id} — returns transaction status.
func (s *Server) getTxHandler(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid transaction ID", http.StatusBadRequest)
		return
	}

	tx, err := s.store.GetTransaction(r.Context(), id)
	if err != nil {
		http.Error(w, "transaction not found", http.StatusNotFound)
		return
	}

	slog.Info("get transaction", "tx_id", id, "status", tx.Status)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":         id.String(),
		"status":     string(tx.Status),
		"retry":      tx.RetryCount,
		"last_error": tx.LastError,
	})
}

// streamHandler handles GET /transactions/{id}/stream — SSE status stream.
func (s *Server) streamHandler(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid transaction ID", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Send initial status.
	if err := sendSSEEvent(w, flusher, "status", map[string]interface{}{
		"id":     id.String(),
		"status": "pending",
	}); err != nil {
		slog.Warn("sse write error", "tx_id", id, "err", err)
		return
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tx, err := s.store.GetTransaction(ctx, id)
			if err != nil {
				sendSSEEvent(w, flusher, "error", map[string]string{"message": "transaction not found"})
				return
			}
			if err := sendSSEEvent(w, flusher, "status", map[string]interface{}{
				"id":         id.String(),
				"status":     string(tx.Status),
				"retry":      tx.RetryCount,
				"last_error": tx.LastError,
			}); err != nil {
				slog.Warn("sse write error", "tx_id", id, "err", err)
				return
			}
		}
	}
}

// sendSSEEvent marshals data as an SSE event and flushes the writer.
func sendSSEEvent(w http.ResponseWriter, flusher http.Flusher, event string, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload)
	if err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

// healthHandler handles GET /health — health check endpoint.
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// metricsHandler handles GET /metrics — Prometheus scrape endpoint.
func (s *Server) metricsHandler(w http.ResponseWriter, r *http.Request) {
	if metrics.Registry() == nil {
		http.Error(w, "metrics not registered", http.StatusInternalServerError)
		return
	}
	handler := promhttp.HandlerFor(metrics.Registry(), promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
	handler.ServeHTTP(w, r)
}

// writeJSON marshals data and writes it as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

