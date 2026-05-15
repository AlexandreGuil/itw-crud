-- S43-bis: sync_state key-value table for Reader feedback loop cursor (migrated from Python).

-- migrate:up
CREATE TABLE IF NOT EXISTS sync_state (
  state_key   TEXT        NOT NULL PRIMARY KEY,
  state_value TEXT        NOT NULL,
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- migrate:down
DROP TABLE IF EXISTS sync_state;
