//go:build integration

package integration

import (
	"context"
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

func TestCreateArticle_InsertsRowWithVersion1(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()

	repo := storage.New(pool)
	ctx := context.Background()

	input := domain.CreateArticleInput{
		URL:        "https://example.com/foo",
		TitleVO:    "Hello world",
		Content:    "lorem ipsum",
		Summary:    "short summary",
		Axes:       []string{"ai-ml-data"},
		Tags:       []string{"ml"},
		SourceURL:  "https://example.com/feed",
		ReaderTags: []string{"axis:ai-ml-data", "source:rss", "veille-validee"},
	}

	version, err := repo.CreateArticle(ctx, input)
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}
	if version != 1 {
		t.Errorf("version=%d, want 1", version)
	}

	got, err := repo.GetArticleByURL(ctx, input.URL)
	if err != nil {
		t.Fatalf("GetArticleByURL: %v", err)
	}
	if got.TitleVO != "Hello world" {
		t.Errorf("title_vo=%q", got.TitleVO)
	}
	if got.Version != 1 {
		t.Errorf("version=%d", got.Version)
	}
	if got.ReaderPayloadPendingAt == nil {
		t.Error("reader_payload_pending_at should be set")
	}
	if len(got.ReaderTags) != 3 {
		t.Errorf("reader_tags len=%d", len(got.ReaderTags))
	}
}

func TestCreateArticle_DuplicateReturnsConflict(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()

	repo := storage.New(pool)
	ctx := context.Background()

	input := domain.CreateArticleInput{URL: "https://example.com/foo", TitleVO: "Hello"}
	if _, err := repo.CreateArticle(ctx, input); err != nil {
		t.Fatal(err)
	}
	_, err := repo.CreateArticle(ctx, input)
	if !errors.Is(err, storage.ErrConflict) {
		t.Errorf("err=%v, want ErrConflict", err)
	}
}

func TestPatchTranslationState_HappyPath(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()
	repo := storage.New(pool)
	ctx := context.Background()
	_, _ = repo.CreateArticle(ctx, domain.CreateArticleInput{URL: "https://e/x", TitleVO: "T"})
	titleFR := "Titre FR"
	summaryFR := "Résumé FR"
	model := "gemma4:31b-cloud"
	tokensIn := 100
	tokensOut := 50
	newVersion, err := repo.PatchTranslationState(ctx, "https://e/x", 1, domain.PatchTranslationStateInput{
		TitleFR: &titleFR, SummaryFR: &summaryFR, TranslationModel: &model,
		TranslationTokensIn: &tokensIn, TranslationTokensOut: &tokensOut, MarkTranslated: true,
	})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if newVersion != 2 {
		t.Errorf("newVersion=%d, want 2", newVersion)
	}
	got, _ := repo.GetArticleByURL(ctx, "https://e/x")
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
	_, _ = repo.CreateArticle(ctx, domain.CreateArticleInput{URL: "https://e/x", TitleVO: "T"})
	titleFR := "first"
	_, _ = repo.PatchTranslationState(ctx, "https://e/x", 1, domain.PatchTranslationStateInput{TitleFR: &titleFR})
	titleFR2 := "second"
	_, err := repo.PatchTranslationState(ctx, "https://e/x", 1, domain.PatchTranslationStateInput{TitleFR: &titleFR2})
	if !errors.Is(err, storage.ErrVersionMismatch) {
		t.Errorf("err=%v, want ErrVersionMismatch", err)
	}
}

func TestPatchTranslationState_MarkPushedToReader(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()
	repo := storage.New(pool)
	ctx := context.Background()
	_, _ = repo.CreateArticle(ctx, domain.CreateArticleInput{URL: "https://e/x", TitleVO: "T"})
	rwID := "rw_abc123"
	_, err := repo.PatchTranslationState(ctx, "https://e/x", 1, domain.PatchTranslationStateInput{
		ReadwiseID: &rwID, MarkPushedToReader: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetArticleByURL(ctx, "https://e/x")
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
	_, _ = repo.CreateArticle(ctx, domain.CreateArticleInput{URL: "https://e/orphan", TitleVO: "T"})

	// Article 2: pending + translated → NOT orphan
	_, _ = repo.CreateArticle(ctx, domain.CreateArticleInput{URL: "https://e/done", TitleVO: "T"})
	titleFR := "FR"
	_, _ = repo.PatchTranslationState(ctx, "https://e/done", 1, domain.PatchTranslationStateInput{
		TitleFR: &titleFR, MarkTranslated: true,
	})

	urls, err := repo.ListOrphans(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(urls) != 1 {
		t.Errorf("orphans=%v", urls)
	}
	if len(urls) == 1 && urls[0] != "https://e/orphan" {
		t.Errorf("orphan url=%q", urls[0])
	}
}
