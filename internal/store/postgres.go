// Package store provides Postgres-backed persistence for ForgeTSS.
package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the central persistence layer, holding a connection pool and
// providing access to transaction and channel account records.
type Store struct {
	pool *pgxpool.Pool
}

// NewPostgres opens a connection pool and runs pending migrations.
// It accepts a database URL string and returns a fully configured Store.
func NewPostgres(ctx context.Context, databaseURL string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 10
	cfg.MinConns = 2

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}

	s := &Store{pool: pool}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return s, nil
}

// New returns a Store backed by the given pool. It skips migration
// — used primarily in tests where the caller manages schema setup.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Pool returns the underlying pgxpool for direct access.
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// Close shuts down the connection pool.
func (s *Store) Close() {
	s.pool.Close()
}
