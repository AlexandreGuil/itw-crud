// Package storage owns the PG repository implementation for article_records.
package storage

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository wraps the pgx pool + provides typed CRUD methods over article_records.
type Repository struct {
	pool *pgxpool.Pool
}

// New constructs a Repository bound to the given pgx pool.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Ping verifies the PG connection is alive — used by /readyz handler.
func (r *Repository) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}
