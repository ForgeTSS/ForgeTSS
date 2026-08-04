package store

import (
	"context"
	"embed"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
)

//go:embed ../../migrations/*.sql
var migrations embed.FS

// migrate executes all pending migration files in order.
// It creates a migrations schema table to track which migrations have run.
func (s *Store) migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS _migrations (
			id INT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	if err != nil {
		return fmt.Errorf("creating migration tracking table: %w", err)
	}

	var applied []struct {
		ID int
	}
	rows, err := s.pool.Query(ctx, `SELECT id FROM _migrations ORDER BY id`)
	if err != nil {
		return fmt.Errorf("querying applied migrations: %w", err)
	}
	applied, err = pgx.CollectRows(rows, pgx.RowToStructByName[struct {
		ID int
	}])
	if err != nil {
		return fmt.Errorf("collecting applied migrations: %w", err)
	}

	appliedSet := make(map[int]struct{}, len(applied))
	for _, a := range applied {
		appliedSet[a.ID] = struct{}{}
	}

	files, err := migrations.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("reading migration files: %w", err)
	}

	for _, f := range files {
		id, ext := extractMigrationID(f.Name())
		if !ext || id <= 0 {
			continue
		}
		if _, ok := appliedSet[id]; ok {
			continue
		}

		data, err := migrations.ReadFile("migrations/" + f.Name())
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", f.Name(), err)
		}

		if err := s.runMigration(ctx, id, f.Name(), string(data)); err != nil {
			return err
		}
	}

	return nil
}

func extractMigrationID(name string) (int, bool) {
	var id int
	n, err := fmt.Sscanf(name, "%d_", &id)
	if err != nil || n != 1 {
		return 0, false
	}
	return id, len(name) > 3 && name[len(name)-4:] == ".sql"
}

func (s *Store) runMigration(ctx context.Context, id int, name string, sql string) error {
	slog.Info("running migration", "id", id, "name", name)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning migration tx %d: %w", id, err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("executing migration %d: %w", id, err)
	}

	if _, err := tx.Exec(ctx, `INSERT INTO _migrations (id, name) VALUES ($1, $2)`, id, name); err != nil {
		return fmt.Errorf("recording migration %d: %w", id, err)
	}

	return tx.Commit(ctx)
}
