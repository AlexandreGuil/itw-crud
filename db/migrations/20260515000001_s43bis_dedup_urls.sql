-- S43-bis: dedup_urls table for URL deduplication (migrated from Python PostgresDedupStore).

-- migrate:up
CREATE TABLE IF NOT EXISTS dedup_urls (
  md5_url       TEXT        NOT NULL PRIMARY KEY,
  url           TEXT        NOT NULL,
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dedup_urls_last_seen ON dedup_urls(last_seen_at DESC);

-- migrate:down
DROP TABLE IF EXISTS dedup_urls;
