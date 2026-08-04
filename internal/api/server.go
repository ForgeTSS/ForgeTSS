// Package api provides the HTTP API server for ForgeTSS.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gamp/forgetss/internal/config"
	"github.com/gamp/forgetss/internal/store"
)

// Server wraps the HTTP router, HTTP server, and dependency references.
type Server struct {
	router *chi.Mux
	srv    *http.Server
	cfg    *config.Config
	store  *store.Store
	engine  interface{}
}

// NewServer creates the API server with routing and middleware.
func NewServer(cfg *config.Config, st *store.Store, _ interface{}, _ interface{}, _ interface{}) *Server {
	r := chi.NewMux()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	srv := &Server{router: r, srv: &http.Server{Addr: cfg.ListenAddr, Handler: r}, cfg: cfg, store: st}

	// Public endpoints — no auth required.
	r.Get("/health", srv.healthHandler)
	r.Get("/metrics", srv.metricsHandler)

	// Authenticated endpoints.
	r.Group(func(r chi.Router) {
		r.Use(srv.authMiddleware)
		r.Post("/transactions", srv.enqueueHandler)
		r.Get("/transactions/{id}", srv.getTxHandler)
		r.Get("/transactions/{id}/stream", srv.streamHandler)
	})

	return srv
}

// Start begins listening on the configured address. It blocks until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	srvCh := make(chan error, 1)
	go func() {
		slog.Info("starting API server", "addr", s.cfg.ListenAddr)
		srvCh <- s.srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutting down API server")
		return s.srv.Shutdown(context.Background())
	case err := <-srvCh:
		return err
	}
}
