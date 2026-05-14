-- S44 itw-crud Phase 2b — add UNIQUE constraint on md5_url for ON CONFLICT UPSERT.
-- The UpsertArticle handler uses ON CONFLICT (md5_url) DO UPDATE which requires
-- a unique or exclusion constraint on the conflict target column.
-- S44 G21: missing constraint caused "no unique or exclusion constraint" SQLSTATE 42P10.
-- S44 G22: article_records is an append-log (record_run inserts per run per URL) so
-- md5_url has ~639 duplicate groups. Must deduplicate first, keeping the row with the
-- highest id (latest run) per md5_url, before creating the UNIQUE INDEX.

-- migrate:up

-- Step 1: delete duplicate rows, keep highest record_id per md5_url (latest pipeline run).
-- article_records PK is record_id (bigint sequence), not id.
DELETE FROM article_records
WHERE record_id NOT IN (
  SELECT MAX(record_id) FROM article_records GROUP BY md5_url
);

-- Step 2: create UNIQUE INDEX now that duplicates are removed.
CREATE UNIQUE INDEX IF NOT EXISTS idx_article_records_md5_url_unique
  ON article_records (md5_url);

-- migrate:down
DROP INDEX IF EXISTS idx_article_records_md5_url_unique;
