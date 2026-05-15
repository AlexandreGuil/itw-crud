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

// md5URL mirrors storage.md5URL for test setup (unexported in storage, duplicated here).
func md5URL(s string) string {
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
	// Includes S44 columns: source_url + axes.
	// pipeline_runs includes S43-bis columns: source_type, mode, finished_at, articles_count.
	bootstrapSQL := `
CREATE TABLE IF NOT EXISTS pipeline_runs (
  run_id         TEXT        PRIMARY KEY,
  started_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  finished_at    TIMESTAMPTZ,
  source_type    TEXT        NOT NULL DEFAULT '',
  mode           TEXT        NOT NULL DEFAULT 'normal',
  articles_count INT
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
  source_url TEXT NOT NULL DEFAULT '',
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
	migrationS43 := `
ALTER TABLE article_records
  ADD COLUMN IF NOT EXISTS reader_payload_pending_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS reader_tags TEXT[],
  ADD COLUMN IF NOT EXISTS version INT NOT NULL DEFAULT 1;

CREATE INDEX IF NOT EXISTS idx_article_records_orphans
  ON article_records (reader_payload_pending_at)
  WHERE reader_payload_pending_at IS NOT NULL AND translated_at IS NULL;
`
	if _, err := pool.Exec(ctx, migrationS43); err != nil {
		pool.Close()
		cleanup()
		t.Fatal(err)
	}

	// Apply S44 migration — axes column (source_url already in bootstrap above).
	migrationS44 := `
ALTER TABLE article_records
  ADD COLUMN IF NOT EXISTS axes TEXT[] NOT NULL DEFAULT '{}';
`
	if _, err := pool.Exec(ctx, migrationS44); err != nil {
		pool.Close()
		cleanup()
		t.Fatal(err)
	}

	// Apply S43-bis migrations — dedup_urls + sync_state tables.
	migrationS43Bis := `
CREATE TABLE IF NOT EXISTS dedup_urls (
  md5_url       TEXT        NOT NULL PRIMARY KEY,
  url           TEXT        NOT NULL,
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dedup_urls_last_seen ON dedup_urls(last_seen_at DESC);

CREATE TABLE IF NOT EXISTS sync_state (
  state_key   TEXT        NOT NULL PRIMARY KEY,
  state_value TEXT        NOT NULL,
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`
	if _, err := pool.Exec(ctx, migrationS43Bis); err != nil {
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
		"test-run-1", url, md5URL(url),
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

func TestUpsertArticle_InsertsIfNotExists(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()

	repo := storage.New(pool)
	ctx := context.Background()

	in := domain.UpsertArticleInput{
		URL:           "https://example.com/upsert-new",
		MD5URL:        md5URL("https://example.com/upsert-new"),
		ArticleID:     "art-new-001",
		RunID:         "test-run-1",
		Title:         "Original Title",
		Content:       "lorem ipsum",
		Summary:       "short",
		Tags:          "tag1,tag2",
		Source:        "https://example.com/feed",
		Axes:          []string{"ai-ml-data"},
		ReaderTags:    []string{"axis:ai-ml-data", "source:rss", "veille-validee"},
		FinalDecision: "accepted",
	}

	version, err := repo.UpsertArticle(ctx, in)
	if err != nil {
		t.Fatalf("UpsertArticle: %v", err)
	}
	if version != 1 {
		t.Errorf("version=%d, want 1", version)
	}

	got, err := repo.GetArticleByURL(ctx, in.URL)
	if err != nil {
		t.Fatalf("GetArticleByURL: %v", err)
	}
	if got.Title != "Original Title" {
		t.Errorf("title=%q, want %q", got.Title, "Original Title")
	}
	if got.ReaderPayloadPendingAt == nil {
		t.Error("reader_payload_pending_at should be set on insert")
	}
	if len(got.ReaderTags) != 3 {
		t.Errorf("reader_tags len=%d, want 3", len(got.ReaderTags))
	}
}

func TestUpsertArticle_UpdatesIfExists(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()

	repo := storage.New(pool)
	ctx := context.Background()

	in := domain.UpsertArticleInput{
		URL:           "https://example.com/upsert-update",
		MD5URL:        md5URL("https://example.com/upsert-update"),
		ArticleID:     "art-upd-001",
		RunID:         "test-run-1",
		Title:         "First",
		FinalDecision: "accepted",
		Axes:          []string{"ai-ml-data"},
		ReaderTags:    []string{"veille-validee"},
	}
	v1, err := repo.UpsertArticle(ctx, in)
	if err != nil {
		t.Fatalf("UpsertArticle first: %v", err)
	}

	in.Title = "Updated"
	v2, err := repo.UpsertArticle(ctx, in)
	if err != nil {
		t.Fatalf("UpsertArticle second: %v", err)
	}
	if v2 != v1+1 {
		t.Errorf("version=%d, want %d (v1+1)", v2, v1+1)
	}

	got, _ := repo.GetArticleByURL(ctx, in.URL)
	if got.Title != "Updated" {
		t.Errorf("title=%q, want %q", got.Title, "Updated")
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

func TestWriteTranslationState_UpdatesArticleByMD5(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()

	repo := storage.New(pool)
	ctx := context.Background()

	// Bootstrap article via UpsertArticle
	articleMD5 := md5URL("https://example.com/translated")
	_, err := repo.UpsertArticle(ctx, domain.UpsertArticleInput{
		URL: "https://example.com/translated", MD5URL: articleMD5,
		ArticleID: "art-trans-001", RunID: "test-run-1",
		Title: "Hello", FinalDecision: "accepted",
		Axes: []string{}, ReaderTags: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}

	titleFR := "Bonjour"
	summaryFR := "Résumé FR"
	model := "gemma4:31b-cloud"
	tokensIn := 100
	tokensOut := 50
	durationMs := int64(4500)
	err = repo.WriteTranslationState(ctx, domain.TranslationResponseInput{
		RequestID:             "itw-tech:" + articleMD5,
		Status:                "ok",
		SourceLanguage:        "en",
		TargetLanguage:        "fr",
		TitleFR:               &titleFR,
		SummaryFR:             &summaryFR,
		TranslationModel:      &model,
		TranslationTokensIn:   &tokensIn,
		TranslationTokensOut:  &tokensOut,
		TranslationDurationMs: &durationMs,
	})
	if err != nil {
		t.Fatalf("WriteTranslationState: %v", err)
	}

	got, _ := repo.GetArticleByURL(ctx, "https://example.com/translated")
	if got.TitleFR == nil || *got.TitleFR != "Bonjour" {
		t.Errorf("title_fr=%v, want Bonjour", got.TitleFR)
	}
	if got.TranslatedAt == nil {
		t.Error("translated_at should be set")
	}
}

func TestWriteTranslationState_SkipsIfStatusNotOk(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()

	repo := storage.New(pool)
	ctx := context.Background()

	articleMD5 := md5URL("https://example.com/skipped")
	if _, err := repo.UpsertArticle(ctx, domain.UpsertArticleInput{
		URL: "https://example.com/skipped", MD5URL: articleMD5,
		ArticleID: "art-skip-001", RunID: "test-run-1",
		Title: "Skip Me", FinalDecision: "accepted",
		Axes: []string{}, ReaderTags: []string{},
	}); err != nil {
		t.Fatal(err)
	}

	// status=skipped_french — no update expected, no error
	err := repo.WriteTranslationState(ctx, domain.TranslationResponseInput{
		RequestID: "itw-tech:" + articleMD5,
		Status:    "skipped_french",
	})
	if err != nil {
		t.Fatalf("WriteTranslationState skipped_french: %v", err)
	}

	got, _ := repo.GetArticleByURL(ctx, "https://example.com/skipped")
	if got.TranslatedAt != nil {
		t.Error("translated_at should NOT be set for skipped_french")
	}
}

func TestGetArticleByMD5_Found(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()
	repo := storage.New(pool)
	ctx := context.Background()

	url := "https://example.com/by-md5"
	md5 := md5URL(url)
	_, err := repo.UpsertArticle(ctx, domain.UpsertArticleInput{
		URL: url, MD5URL: md5, ArticleID: "art-md5-001", RunID: "test-run-1",
		Title: "Hello", FinalDecision: "accepted",
		Axes: []string{}, ReaderTags: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetArticleByMD5(ctx, md5)
	if err != nil {
		t.Fatalf("GetArticleByMD5: %v", err)
	}
	if got.URL != url {
		t.Errorf("url=%q want %q", got.URL, url)
	}
	if got.MD5URL != md5 {
		t.Errorf("md5_url mismatch")
	}
}

func TestGetArticleByMD5_NotFound(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()
	repo := storage.New(pool)
	ctx := context.Background()

	_, err := repo.GetArticleByMD5(ctx, "deadbeef00000000000000000000000000")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestCreateRun_Inserts(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()
	repo := storage.New(pool)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	in := domain.CreateRunInput{
		RunID:      "run-test-001",
		StartedAt:  &now,
		SourceType: "rss",
		Mode:       "normal",
	}
	run, err := repo.CreateRun(ctx, in)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if run.RunID != "run-test-001" {
		t.Errorf("run_id=%q", run.RunID)
	}
}

func TestCreateRun_Idempotent(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()
	repo := storage.New(pool)
	ctx := context.Background()

	in := domain.CreateRunInput{RunID: "run-idem-001", SourceType: "rss", Mode: "normal"}
	_, err := repo.CreateRun(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	// Second call must not fail (ON CONFLICT DO NOTHING)
	_, err = repo.CreateRun(ctx, in)
	if err != nil {
		t.Fatalf("CreateRun idempotent: %v", err)
	}
}

func TestPatchRun_Updates(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()
	repo := storage.New(pool)
	ctx := context.Background()

	in := domain.CreateRunInput{RunID: "run-patch-001", SourceType: "rss", Mode: "normal"}
	_, err := repo.CreateRun(ctx, in)
	if err != nil {
		t.Fatal(err)
	}

	count := 14
	patch := domain.PatchRunInput{ArticlesCount: &count}
	if err := repo.PatchRun(ctx, "run-patch-001", patch); err != nil {
		t.Fatalf("PatchRun: %v", err)
	}
}

func TestPatchRun_NotFound(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()
	repo := storage.New(pool)
	ctx := context.Background()

	count := 5
	err := repo.PatchRun(ctx, "run-nonexistent", domain.PatchRunInput{ArticlesCount: &count})
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestDedupCheck_Empty(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()
	repo := storage.New(pool)
	ctx := context.Background()

	result, err := repo.DedupCheck(ctx, []string{"abc123", "def456"})
	if err != nil {
		t.Fatalf("DedupCheck: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("want empty, got %v", result)
	}
}

func TestDedupMark_ThenCheck(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()
	repo := storage.New(pool)
	ctx := context.Background()

	urls := []domain.DedupURL{
		{URL: "https://example.com/a", MD5: "md5aaa"},
		{URL: "https://example.com/b", MD5: "md5bbb"},
	}
	count, err := repo.DedupMark(ctx, urls)
	if err != nil {
		t.Fatalf("DedupMark: %v", err)
	}
	if count != 2 {
		t.Errorf("count=%d, want 2", count)
	}

	seen, err := repo.DedupCheck(ctx, []string{"md5aaa", "md5bbb", "md5ccc"})
	if err != nil {
		t.Fatalf("DedupCheck after mark: %v", err)
	}
	if len(seen) != 2 {
		t.Errorf("seen=%v, want 2 items", seen)
	}
}

func TestDedupMark_Idempotent(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()
	repo := storage.New(pool)
	ctx := context.Background()

	urls := []domain.DedupURL{{URL: "https://example.com/x", MD5: "md5xxx"}}
	_, _ = repo.DedupMark(ctx, urls)
	count, err := repo.DedupMark(ctx, urls)
	if err != nil {
		t.Fatalf("DedupMark idempotent: %v", err)
	}
	if count != 1 {
		t.Errorf("count=%d, want 1", count)
	}
}

func TestSyncStateGet_NotFound(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()
	repo := storage.New(pool)
	ctx := context.Background()

	_, err := repo.GetSyncState(ctx, "last_sync_at")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestSyncStatePut_ThenGet(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()
	repo := storage.New(pool)
	ctx := context.Background()

	if err := repo.SetSyncState(ctx, "last_sync_at", "2026-05-15T04:11:00Z"); err != nil {
		t.Fatalf("SetSyncState: %v", err)
	}

	s, err := repo.GetSyncState(ctx, "last_sync_at")
	if err != nil {
		t.Fatalf("GetSyncState: %v", err)
	}
	if s.Value != "2026-05-15T04:11:00Z" {
		t.Errorf("value=%q", s.Value)
	}
	if s.Key != "last_sync_at" {
		t.Errorf("key=%q", s.Key)
	}
}

func TestSyncStatePut_Idempotent(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()
	repo := storage.New(pool)
	ctx := context.Background()

	_ = repo.SetSyncState(ctx, "cursor", "v1")
	if err := repo.SetSyncState(ctx, "cursor", "v2"); err != nil {
		t.Fatalf("SetSyncState overwrite: %v", err)
	}
	s, _ := repo.GetSyncState(ctx, "cursor")
	if s.Value != "v2" {
		t.Errorf("value=%q, want v2", s.Value)
	}
}

func TestListOrphans_FiltersByPendingNotTranslated(t *testing.T) {
	pool, cleanup := startTestPG(t)
	defer cleanup()
	repo := storage.New(pool)
	ctx := context.Background()

	// Article 1: pending (via UPSERT) + not translated → ORPHAN
	url1 := "https://e/orphan"
	_, _ = repo.UpsertArticle(ctx, domain.UpsertArticleInput{
		URL: url1, MD5URL: md5URL(url1),
		ArticleID: "art-orphan-001", RunID: "test-run-1",
		FinalDecision: "accepted",
		Axes:          []string{},
		ReaderTags:    []string{"veille-validee"},
	})

	// Article 2: pending (via UPSERT) + translated → NOT orphan
	url2 := "https://e/done"
	v1, _ := repo.UpsertArticle(ctx, domain.UpsertArticleInput{
		URL: url2, MD5URL: md5URL(url2),
		ArticleID: "art-done-001", RunID: "test-run-1",
		FinalDecision: "accepted",
		Axes:          []string{},
		ReaderTags:    []string{"veille-validee"},
	})
	titleFR := "FR"
	_, _ = repo.PatchTranslationState(ctx, url2, v1, domain.PatchTranslationStateInput{
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
