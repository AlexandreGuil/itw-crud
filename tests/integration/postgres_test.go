//go:build integration

package integration

import (
	"context"
	"crypto/md5" //nolint:gosec
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/AlexandreGuil/itw-crud/internal/domain"
	"github.com/AlexandreGuil/itw-crud/internal/infrastructure/storage"
)

// md5Hex mirrors storage.md5URL for test setup (unexported in storage, duplicated here).
func md5Hex(s string) string {
	h := md5.Sum([]byte(s)) //nolint:gosec
	return hex.EncodeToString(h[:])
}

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

	// Bootstrap article_records table matching actual prod schema (relevant columns).
	bootstrapSQL := `
CREATE TABLE IF NOT EXISTS pipeline_runs (
  run_id TEXT PRIMARY KEY,
  started_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO pipeline_runs (run_id) VALUES ('test-run-1');

CREATE TABLE article_records (
  article_id TEXT NOT NULL PRIMARY KEY DEFAULT gen_random_uuid()::text,
  record_id BIGSERIAL UNIQUE,
  run_id TEXT NOT NULL REFERENCES pipeline_runs(run_id),
  url TEXT NOT NULL,
  md5_url TEXT NOT NULL UNIQUE,
  title TEXT,
  final_decision TEXT,
  final_score FLOAT,
  ingested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  decisions JSONB NOT NULL DEFAULT '[]'::jsonb,
  tags TEXT,
  author TEXT,
  source TEXT,
  published_date TIMESTAMPTZ,
  word_count INT NOT NULL DEFAULT 0,
  content TEXT,
  summary TEXT,
  llm_summary TEXT,
  llm_tech_tags TEXT,
  llm_level TEXT,
  llm_domain TEXT,
  title_fr TEXT,
  summary_fr TEXT,
  translation_model TEXT,
  translation_tokens_input INT,
  translation_tokens_output INT,
  translation_duration_ms BIGINT,
  translated_at TIMESTAMPTZ,
  readwise_id TEXT,
  reader_pushed_at TIMESTAMPTZ
);`
	if _, err := pool.Exec(ctx, bootstrapSQL); err != nil {
		pool.Close()
		cleanup()
		t.Fatal(err)
	}

	// Apply S43 migration (same SQL as migration file).
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

// insertMinimalRow inserts a minimal article_records row (simulating ITW cron record_run).
func insertMinimalRow(t *testing.T, pool *pgxpool.Pool, url string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO article_records (run_id, url, md5_url, ingested_at) VALUES ($1, $2, $3, NOW())`,
		"test-run-1", url, md5Hex(url),
	)
	if err != nil {
		t.Fatalf("insertMinimalRow: %v", err)
	}
}

func TestPostgresPingOK(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()

	repo := storage.New(pool)
	if err := repo.Ping(context.Background()); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
}

func TestSetReaderPayload_UpdatesExistingRow(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()

	repo := storage.New(pool)
	ctx := context.Background()

	url := "https://example.com/foo"
	insertMinimalRow(t, pool, url)

	input := domain.SetReaderPayloadInput{
		URL:        url,
		ReaderTags: []string{"axis:ai-ml-data", "source:rss", "veille-validee"},
	}

	version, err := repo.SetReaderPayload(ctx, input)
	if err != nil {
		t.Fatalf("SetReaderPayload: %v", err)
	}
	if version != 2 {
		t.Errorf("version=%d, want 2", version)
	}

	got, err := repo.GetArticleByURL(ctx, url)
	if err != nil {
		t.Fatalf("GetArticleByURL: %v", err)
	}
	if got.Version != 2 {
		t.Errorf("version=%d", got.Version)
	}
	if got.ReaderPayloadPendingAt == nil {
		t.Error("reader_payload_pending_at should be set")
	}
	if len(got.ReaderTags) != 3 {
		t.Errorf("reader_tags len=%d", len(got.ReaderTags))
	}
}

func TestSetReaderPayload_NotFound_ReturnsErrNotFound(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()

	repo := storage.New(pool)
	ctx := context.Background()

	_, err := repo.SetReaderPayload(ctx, domain.SetReaderPayloadInput{
		URL:        "https://example.com/does-not-exist",
		ReaderTags: []string{"veille-validee"},
	})
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("err=%v, want ErrNotFound", err)
	}
}

func TestPatchTranslationState_HappyPath(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()
	repo := storage.New(pool)
	ctx := context.Background()

	url := "https://e/x"
	insertMinimalRow(t, pool, url)

	titleFR := "Titre FR"
	summaryFR := "Résumé FR"
	model := "gemma4:31b-cloud"
	tokensIn := 100
	tokensOut := 50
	newVersion, err := repo.PatchTranslationState(ctx, url, 1, domain.PatchTranslationStateInput{
		TitleFR: &titleFR, SummaryFR: &summaryFR, TranslationModel: &model,
		TranslationTokensIn: &tokensIn, TranslationTokensOut: &tokensOut, MarkTranslated: true,
	})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if newVersion != 2 {
		t.Errorf("newVersion=%d, want 2", newVersion)
	}
	got, _ := repo.GetArticleByURL(ctx, url)
	if got.TitleFR == nil || *got.TitleFR != "Titre FR" {
		t.Errorf("title_fr=%v", got.TitleFR)
	}
	if got.TranslatedAt == nil {
		t.Error("translated_at should be set")
	}
	if got.Version != 2 {
		t.Errorf("version=%d", got.Version)
	}
}

func TestPatchTranslationState_StaleVersionReturns412(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()
	repo := storage.New(pool)
	ctx := context.Background()

	url := "https://e/x"
	insertMinimalRow(t, pool, url)

	titleFR := "first"
	_, _ = repo.PatchTranslationState(ctx, url, 1, domain.PatchTranslationStateInput{TitleFR: &titleFR})
	titleFR2 := "second"
	_, err := repo.PatchTranslationState(ctx, url, 1, domain.PatchTranslationStateInput{TitleFR: &titleFR2})
	if !errors.Is(err, storage.ErrVersionMismatch) {
		t.Errorf("err=%v, want ErrVersionMismatch", err)
	}
}

func TestPatchTranslationState_MarkPushedToReader(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()
	repo := storage.New(pool)
	ctx := context.Background()

	url := "https://e/x"
	insertMinimalRow(t, pool, url)

	rwID := "rw_abc123"
	_, err := repo.PatchTranslationState(ctx, url, 1, domain.PatchTranslationStateInput{
		ReadwiseID: &rwID, MarkPushedToReader: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetArticleByURL(ctx, url)
	if got.ReadwiseID == nil || *got.ReadwiseID != "rw_abc123" {
		t.Errorf("readwise_id=%v", got.ReadwiseID)
	}
	if got.ReaderPushedAt == nil {
		t.Error("reader_pushed_at should be set")
	}
}

func TestListOrphans_FiltersByPendingNotTranslated(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()
	repo := storage.New(pool)
	ctx := context.Background()

	// Article 1: pending + not translated → ORPHAN
	url1 := "https://e/orphan"
	insertMinimalRow(t, pool, url1)
	_, _ = repo.SetReaderPayload(ctx, domain.SetReaderPayloadInput{URL: url1, ReaderTags: []string{"veille-validee"}})

	// Article 2: pending + translated → NOT orphan
	url2 := "https://e/done"
	insertMinimalRow(t, pool, url2)
	_, _ = repo.SetReaderPayload(ctx, domain.SetReaderPayloadInput{URL: url2, ReaderTags: []string{"veille-validee"}})
	titleFR := "FR"
	// version is 2 after SetReaderPayload (started at 1, incremented once)
	_, _ = repo.PatchTranslationState(ctx, url2, 2, domain.PatchTranslationStateInput{
		TitleFR: &titleFR, MarkTranslated: true,
	})

	urls, err := repo.ListOrphans(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(urls) != 1 {
		t.Errorf("orphans=%v", urls)
	}
	if len(urls) == 1 && urls[0] != url1 {
		t.Errorf("orphan url=%q", urls[0])
	}
}
