// Package api provides HTTP handlers for ForgeTSS.
package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/gamp/forgetss/internal/metrics"
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

	// TODO: accept submission engine via server.
	slog.Info("enqueue request", "envelope_length", len(req.EnvelopeXDR))

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": "pending"})
}

// getTxHandler handles GET /transactions/{id} — returns transaction status.
func (s *Server) getTxHandler(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid transaction ID", http.StatusBadRequest)
		return
	}

	slog.Info("get transaction", "tx_id", id)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"id":     id.String(),
		"status": "pending",
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

	// TODO: implement real SSE subscription via store events.
	fmt.Fprintf(w, "event: status\ndata: {\"id\":\"%s\",\"status\":\"pending\"}\n\n", id)
	flusher.Flush()
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

