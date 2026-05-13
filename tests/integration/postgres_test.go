//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/AlexandreGuil/itw-crud/internal/infrastructure/storage"
)

func startTestPG(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"pgvector/pgvector:pg16",
		postgres.WithDatabase("itw-test"),
		postgres.WithUsername("itw"),
		postgres.WithPassword("itw"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		_ = pgContainer.Terminate(ctx)
	}

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		cleanup()
		t.Fatal(err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}

	// Bootstrap base article_records table (existing schema before S43 migration)
	bootstrapSQL := `
CREATE TABLE article_records (
  id BIGSERIAL PRIMARY KEY,
  md5_url TEXT UNIQUE NOT NULL,
  url TEXT NOT NULL,
  title TEXT,
  content TEXT,
  summary TEXT,
  axes TEXT[],
  tags TEXT[],
  source_url TEXT,
  final_score FLOAT,
  final_decision TEXT,
  ingested_at TIMESTAMPTZ,
  title_fr TEXT,
  summary_fr TEXT,
  translation_model TEXT,
  translation_tokens_input INT,
  translation_tokens_output INT,
  translation_duration_ms INT,
  translated_at TIMESTAMPTZ,
  readwise_id TEXT,
  reader_pushed_at TIMESTAMPTZ
);`
	if _, err := pool.Exec(ctx, bootstrapSQL); err != nil {
		pool.Close()
		cleanup()
		t.Fatal(err)
	}

	// Apply S43 migration (same SQL as migration file)
	migration := `
ALTER TABLE article_records
  ADD COLUMN IF NOT EXISTS reader_payload_pending_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS reader_tags TEXT[],
  ADD COLUMN IF NOT EXISTS version INT NOT NULL DEFAULT 1;

CREATE INDEX IF NOT EXISTS idx_article_records_orphans
  ON article_records (reader_payload_pending_at)
  WHERE reader_payload_pending_at IS NOT NULL AND translated_at IS NULL;
`
	if _, err := pool.Exec(ctx, migration); err != nil {
		pool.Close()
		cleanup()
		t.Fatal(err)
	}

	return pool, func() { pool.Close(); cleanup() }
}

func TestPostgresPingOK(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()

	repo := storage.New(pool)
	if err := repo.Ping(context.Background()); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
}
