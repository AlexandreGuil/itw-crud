// Package storage owns the PG repository implementation for article_records.
package storage

import (
	"context"
	"crypto/md5" //nolint:gosec // MD5 used as URL lookup key, not for security
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreGuil/itw-crud/internal/domain"
)

// Sentinel errors returned by Repository methods.
var ErrNotFound = errors.New("article not found")
var ErrVersionMismatch = errors.New("version mismatch — stale ETag")

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

// md5URL returns the hex-encoded MD5 of the URL — primary lookup key (not a security primitive).
func md5URL(url string) string {
	h := md5.Sum([]byte(url)) //nolint:gosec // MD5 as deterministic lookup key, aligned with existing ITW Python schema
	return hex.EncodeToString(h[:])
}

// UpsertArticle does INSERT ON CONFLICT (md5_url) DO UPDATE.
// Sets reader_payload_pending_at=NOW() on insert (article entering translation pipeline).
// Increments version on update. Idempotent.
func (r *Repository) UpsertArticle(ctx context.Context, in domain.UpsertArticleInput) (int, error) {
	const q = `
INSERT INTO article_records (
  md5_url, url, article_id, run_id, title, content, summary,
  tags, source, source_url, author, published_date, word_count,
  axes, reader_tags, final_decision, final_score, decisions,
  ingested_at, reader_payload_pending_at, version
) VALUES (
  $1, $2, $3, $4, $5, $6, $7,
  $8, $9, $10, $11, $12, $13,
  $14, $15, $16, $17, COALESCE($18, '[]'::jsonb)::jsonb,
  COALESCE($19, NOW()), NOW(), 1
)
ON CONFLICT (md5_url) DO UPDATE SET
  title = EXCLUDED.title,
  content = EXCLUDED.content,
  summary = EXCLUDED.summary,
  tags = EXCLUDED.tags,
  source = EXCLUDED.source,
  source_url = EXCLUDED.source_url,
  axes = EXCLUDED.axes,
  reader_tags = EXCLUDED.reader_tags,
  final_decision = EXCLUDED.final_decision,
  final_score = EXCLUDED.final_score,
  reader_payload_pending_at = NOW(),
  version = article_records.version + 1
RETURNING version
`
	var decisionsArg interface{}
	if in.Decisions != "" {
		decisionsArg = in.Decisions
	}
	var version int
	err := r.pool.QueryRow(ctx, q,
		in.MD5URL, in.URL, in.ArticleID, in.RunID, in.Title, in.Content, in.Summary,
		in.Tags, in.Source, in.SourceURL, in.Author, in.PublishedDate, in.WordCount,
		in.Axes, in.ReaderTags, in.FinalDecision, in.FinalScore, decisionsArg,
		in.IngestedAt,
	).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("upsert article: %w", err)
	}
	return version, nil
}

// GetArticleByURL fetches a single article by URL. Returns ErrNotFound if absent.
func (r *Repository) GetArticleByURL(ctx context.Context, url string) (*domain.Article, error) {
	const q = `
SELECT
  article_id, url, md5_url,
  COALESCE(title, ''), COALESCE(final_decision, ''),
  final_score, ingested_at, tags, source, content, summary,
  title_fr, summary_fr, translation_model,
  translation_tokens_input, translation_tokens_output, translation_duration_ms,
  translated_at, readwise_id, reader_pushed_at,
  reader_payload_pending_at, COALESCE(reader_tags, '{}'), version
FROM article_records
WHERE md5_url = $1
LIMIT 1
`
	var a domain.Article
	err := r.pool.QueryRow(ctx, q, md5URL(url)).Scan(
		&a.ArticleID, &a.URL, &a.MD5URL,
		&a.Title, &a.FinalDecision,
		&a.FinalScore, &a.IngestedAt, &a.Tags, &a.Source, &a.Content, &a.Summary,
		&a.TitleFR, &a.SummaryFR, &a.TranslationModel,
		&a.TranslationTokensIn, &a.TranslationTokensOut, &a.TranslationDurationMs,
		&a.TranslatedAt, &a.ReadwiseID, &a.ReaderPushedAt,
		&a.ReaderPayloadPendingAt, &a.ReaderTags, &a.Version,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get article: %w", err)
	}
	return &a, nil
}

// GetArticleByMD5 fetches a single article by its md5_url hex string.
// Used by reader-pusher to resolve a push trigger to the full article payload.
// Returns ErrNotFound if absent.
func (r *Repository) GetArticleByMD5(ctx context.Context, md5Hex string) (*domain.Article, error) {
	const q = `
SELECT
  article_id, url, md5_url,
  COALESCE(title, ''), COALESCE(final_decision, ''),
  final_score, ingested_at, tags, source, content, summary,
  title_fr, summary_fr, translation_model,
  translation_tokens_input, translation_tokens_output, translation_duration_ms,
  translated_at, readwise_id, reader_pushed_at,
  reader_payload_pending_at, COALESCE(reader_tags, '{}'), version
FROM article_records
WHERE md5_url = $1
LIMIT 1
`
	var a domain.Article
	err := r.pool.QueryRow(ctx, q, md5Hex).Scan(
		&a.ArticleID, &a.URL, &a.MD5URL,
		&a.Title, &a.FinalDecision,
		&a.FinalScore, &a.IngestedAt, &a.Tags, &a.Source, &a.Content, &a.Summary,
		&a.TitleFR, &a.SummaryFR, &a.TranslationModel,
		&a.TranslationTokensIn, &a.TranslationTokensOut, &a.TranslationDurationMs,
		&a.TranslatedAt, &a.ReadwiseID, &a.ReaderPushedAt,
		&a.ReaderPayloadPendingAt, &a.ReaderTags, &a.Version,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get article by md5: %w", err)
	}
	return &a, nil
}

// PatchTranslationState applies a column-level UPDATE on translation fields.
// ifMatchVersion must equal the current version; otherwise ErrVersionMismatch is returned.
// Returns the incremented version on success.
func (r *Repository) PatchTranslationState(
	ctx context.Context,
	url string,
	ifMatchVersion int,
	in domain.PatchTranslationStateInput,
) (int, error) {
	sets := []string{"version = version + 1"}
	args := []any{}
	argN := 1

	add := func(col string, value any) {
		sets = append(sets, fmt.Sprintf("%s = $%d", col, argN))
		args = append(args, value)
		argN++
	}

	if in.TitleFR != nil {
		add("title_fr", *in.TitleFR)
	}
	if in.SummaryFR != nil {
		add("summary_fr", *in.SummaryFR)
	}
	if in.TranslationModel != nil {
		add("translation_model", *in.TranslationModel)
	}
	if in.TranslationTokensIn != nil {
		add("translation_tokens_input", *in.TranslationTokensIn)
	}
	if in.TranslationTokensOut != nil {
		add("translation_tokens_output", *in.TranslationTokensOut)
	}
	if in.TranslationDurationMs != nil {
		add("translation_duration_ms", *in.TranslationDurationMs)
	}
	if in.MarkTranslated {
		sets = append(sets, "translated_at = NOW()")
	}
	if in.ReadwiseID != nil {
		add("readwise_id", *in.ReadwiseID)
	}
	if in.MarkPushedToReader {
		sets = append(sets, "reader_pushed_at = NOW()")
	}

	args = append(args, md5URL(url))
	md5Arg := argN
	argN++
	args = append(args, ifMatchVersion)
	versionArg := argN

	q := fmt.Sprintf(`
UPDATE article_records
SET %s
WHERE md5_url = $%d AND version = $%d
RETURNING version
`, strings.Join(sets, ", "), md5Arg, versionArg)

	var newVersion int
	err := r.pool.QueryRow(ctx, q, args...).Scan(&newVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		_ = r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM article_records WHERE md5_url = $1)`, md5URL(url)).Scan(&exists)
		if !exists {
			return 0, ErrNotFound
		}
		return 0, ErrVersionMismatch
	}
	if err != nil {
		return 0, fmt.Errorf("patch translation state: %w", err)
	}
	return newVersion, nil
}

// WriteTranslationState UPDATE article row matched by md5_url extracted from request_id.
// request_id format: "<source>:<md5_url_hex>" — split on ":" take index 1.
// Idempotent. If status != "ok", no-op (skipped/failed translation).
func (r *Repository) WriteTranslationState(ctx context.Context, in domain.TranslationResponseInput) error {
	parts := strings.SplitN(in.RequestID, ":", 2)
	if len(parts) != 2 || parts[1] == "" {
		return fmt.Errorf("invalid request_id %q: expected <source>:<md5_url>", in.RequestID)
	}
	if in.Status != "ok" {
		return nil
	}
	md5 := parts[1]

	const q = `
UPDATE article_records
SET title_fr = $1, summary_fr = $2,
    translation_model = $3,
    translation_tokens_input = $4,
    translation_tokens_output = $5,
    translation_duration_ms = $6,
    translated_at = COALESCE(translated_at, NOW()),
    version = version + 1
WHERE md5_url = $7
RETURNING version
`
	var version int
	err := r.pool.QueryRow(ctx, q,
		in.TitleFR, in.SummaryFR, in.TranslationModel,
		in.TranslationTokensIn, in.TranslationTokensOut, in.TranslationDurationMs,
		md5,
	).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("write translation state: %w", err)
	}
	return nil
}

// CreateRun inserts a pipeline_runs row. Idempotent via ON CONFLICT (run_id) DO NOTHING.
// Returns the run row (fetches it back if conflict).
func (r *Repository) CreateRun(ctx context.Context, in domain.CreateRunInput) (*domain.PipelineRun, error) {
	mode := in.Mode
	if mode == "" {
		mode = "normal"
	}
	const q = `
INSERT INTO pipeline_runs (run_id, started_at, source_type, mode)
VALUES ($1, COALESCE($2, NOW()), $3, $4)
ON CONFLICT (run_id) DO NOTHING
RETURNING run_id, started_at
`
	var run domain.PipelineRun
	err := r.pool.QueryRow(ctx, q, in.RunID, in.StartedAt, in.SourceType, mode).Scan(
		&run.RunID, &run.StartedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		// Conflict — row already existed, fetch it
		const qGet = `SELECT run_id, started_at FROM pipeline_runs WHERE run_id = $1`
		err2 := r.pool.QueryRow(ctx, qGet, in.RunID).Scan(&run.RunID, &run.StartedAt)
		if err2 != nil {
			return nil, fmt.Errorf("create run (fetch existing): %w", err2)
		}
		return &run, nil
	}
	if err != nil {
		return nil, fmt.Errorf("create run: %w", err)
	}
	return &run, nil
}

// PatchRun updates finished_at and/or articles_count on pipeline_runs.
// Returns ErrNotFound if run_id does not exist.
func (r *Repository) PatchRun(ctx context.Context, runID string, in domain.PatchRunInput) error {
	const q = `
UPDATE pipeline_runs
SET
  finished_at    = COALESCE($1, finished_at),
  articles_count = COALESCE($2, articles_count)
WHERE run_id = $3
RETURNING run_id
`
	var id string
	err := r.pool.QueryRow(ctx, q, in.FinishedAt, in.ArticlesCount, runID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("patch run: %w", err)
	}
	return nil
}

// DedupCheck returns the subset of md5s that are already in dedup_urls.
func (r *Repository) DedupCheck(ctx context.Context, md5s []string) ([]string, error) {
	if len(md5s) == 0 {
		return nil, nil
	}
	const q = `SELECT md5_url FROM dedup_urls WHERE md5_url = ANY($1)`
	rows, err := r.pool.Query(ctx, q, md5s)
	if err != nil {
		return nil, fmt.Errorf("dedup check: %w", err)
	}
	defer rows.Close()

	var seen []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, fmt.Errorf("dedup check scan: %w", err)
		}
		seen = append(seen, m)
	}
	return seen, rows.Err()
}

// DedupMark upserts a batch of URL+md5 pairs into dedup_urls.
// Returns the number of rows inserted or updated.
func (r *Repository) DedupMark(ctx context.Context, urls []domain.DedupURL) (int, error) {
	if len(urls) == 0 {
		return 0, nil
	}

	batch := &pgx.Batch{}
	const q = `
INSERT INTO dedup_urls (md5_url, url, first_seen_at, last_seen_at)
VALUES ($1, $2, NOW(), NOW())
ON CONFLICT (md5_url) DO UPDATE SET last_seen_at = NOW()
`
	for _, u := range urls {
		batch.Queue(q, u.MD5, u.URL)
	}
	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()

	count := 0
	for range urls {
		if _, err := results.Exec(); err != nil {
			return count, fmt.Errorf("dedup mark batch: %w", err)
		}
		count++
	}
	return count, nil
}

// ListOrphans returns URLs of articles where reader_payload_pending_at IS NOT NULL,
// translated_at IS NULL, and the pending timestamp is older than olderThan.
// Pass 0 to return all pending-not-translated regardless of age.
func (r *Repository) ListOrphans(ctx context.Context, olderThan time.Duration) ([]string, error) {
	const q = `
SELECT url
FROM article_records
WHERE reader_payload_pending_at IS NOT NULL
  AND translated_at IS NULL
  AND NOW() - reader_payload_pending_at > $1
ORDER BY reader_payload_pending_at ASC
`
	rows, err := r.pool.Query(ctx, q, olderThan)
	if err != nil {
		return nil, fmt.Errorf("list orphans: %w", err)
	}
	defer rows.Close()
	var urls []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		urls = append(urls, u)
	}
	return urls, rows.Err()
}
