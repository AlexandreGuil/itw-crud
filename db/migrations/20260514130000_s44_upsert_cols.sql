-- S44 itw-crud Phase 2 — add columns required by UpsertArticle (POST /articles UPSERT).
-- axes: propagated from ITW cron payload for reader tag building.
-- source_url: canonical feed URL (separate from article source).
-- Migration is additive — no existing data affected.

-- migrate:up
ALTER TABLE article_records
  ADD COLUMN IF NOT EXISTS axes TEXT[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS source_url TEXT NOT NULL DEFAULT '';

-- migrate:down
ALTER TABLE article_records
  DROP COLUMN IF EXISTS axes,
  DROP COLUMN IF EXISTS source_url;
