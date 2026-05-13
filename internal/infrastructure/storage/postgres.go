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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreGuil/itw-crud/internal/domain"
)

// Sentinel errors returned by Repository methods.
var ErrNotFound = errors.New("article not found")
var ErrConflict = errors.New("article already exists")
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

// CreateArticle inserts a new article_records row with version=1 and
// reader_payload_pending_at=NOW(). Returns ErrConflict on duplicate URL.
func (r *Repository) CreateArticle(ctx context.Context, in domain.CreateArticleInput) (int, error) {
	const q = `
INSERT INTO article_records (
  md5_url, url, title, content, summary,
  axes, tags, source_url,
  reader_tags, reader_payload_pending_at, version, ingested_at
) VALUES (
  $1, $2, $3, $4, $5,
  $6, $7, $8,
  $9, NOW(), 1, NOW()
)
RETURNING version
`
	var version int
	err := r.pool.QueryRow(ctx, q,
		md5URL(in.URL), in.URL, in.TitleVO, in.Content, in.Summary,
		in.Axes, in.Tags, in.SourceURL,
		in.ReaderTags,
	).Scan(&version)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return 0, ErrConflict
		}
		return 0, fmt.Errorf("create article: %w", err)
	}
	return version, nil
}

// GetArticleByURL fetches a single article by URL. Returns ErrNotFound if absent.
func (r *Repository) GetArticleByURL(ctx context.Context, url string) (*domain.Article, error) {
	const q = `
SELECT
  url, md5_url, COALESCE(title, ''), COALESCE(content, ''), COALESCE(summary, ''),
  COALESCE(axes, '{}'), COALESCE(tags, '{}'), COALESCE(source_url, ''),
  final_score, COALESCE(final_decision, ''), ingested_at,
  title_fr, summary_fr, translation_model,
  translation_tokens_input, translation_tokens_output, translation_duration_ms,
  translated_at, readwise_id, reader_pushed_at,
  reader_payload_pending_at, COALESCE(reader_tags, '{}'), version
FROM article_records
WHERE md5_url = $1
`
	var a domain.Article
	err := r.pool.QueryRow(ctx, q, md5URL(url)).Scan(
		&a.URL, &a.MD5URL, &a.TitleVO, &a.Content, &a.Summary,
		&a.Axes, &a.Tags, &a.SourceURL,
		&a.FinalScore, &a.FinalDecision, &a.IngestedAt,
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
