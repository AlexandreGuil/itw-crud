-- S44 itw-crud Phase 2b — add UNIQUE constraint on md5_url for ON CONFLICT UPSERT.
-- The UpsertArticle handler uses ON CONFLICT (md5_url) DO UPDATE which requires
-- a unique or exclusion constraint on the conflict target column.
-- S44 G21: missing constraint caused "no unique or exclusion constraint" SQLSTATE 42P10.

-- migrate:up
CREATE UNIQUE INDEX IF NOT EXISTS idx_article_records_md5_url_unique
  ON article_records (md5_url);

-- migrate:down
DROP INDEX IF EXISTS idx_article_records_md5_url_unique;
