-- ============================================================
-- Event store: persisted Envelope snapshots for replay API
-- ============================================================

CREATE TABLE IF NOT EXISTS event_store (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL DEFAULT '',
  author TEXT NOT NULL DEFAULT '',
  channel TEXT NOT NULL DEFAULT '',
  envelope_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_event_store_session_created ON event_store(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_event_store_type ON event_store(type);
CREATE INDEX IF NOT EXISTS idx_event_store_created_at ON event_store(created_at);
