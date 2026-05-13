-- S43 itw-crud Phase 1 — extend article_records for itw-crud ownership.
-- Adds reader_payload_pending_at (orphan detection), reader_tags (full Reader tags
-- pre-Phase 6 composite), version (optimistic locking via ETag HTTP).
-- Migration is additive — preserves backward-compat with PostgresArticleHistory
-- Python in-process during transition.

-- migrate:up
ALTER TABLE article_records
  ADD COLUMN IF NOT EXISTS reader_payload_pending_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS reader_tags TEXT[],
  ADD COLUMN IF NOT EXISTS version INT NOT NULL DEFAULT 1;

CREATE INDEX IF NOT EXISTS idx_article_records_orphans
  ON article_records (reader_payload_pending_at)
  WHERE reader_payload_pending_at IS NOT NULL AND translated_at IS NULL;

-- migrate:down
DROP INDEX IF EXISTS idx_article_records_orphans;
ALTER TABLE article_records
  DROP COLUMN IF EXISTS reader_payload_pending_at,
  DROP COLUMN IF EXISTS reader_tags,
  DROP COLUMN IF EXISTS version;
