-- Version 20261010: B-06 durable critical-event outbox for WS last_event_id replay.
-- Primary durability for critical v2 delivery events (Postgres/SQLite).
-- critical_journal (JSONL) remains an optional secondary sink.

CREATE TABLE IF NOT EXISTS event_delivery_outbox (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  event_id TEXT NOT NULL,
  kind TEXT NOT NULL DEFAULT '',
  entity_id TEXT NOT NULL DEFAULT '',
  payload BLOB NOT NULL,
  published_at TEXT,
  created_at TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_event_delivery_outbox_session_seq
  ON event_delivery_outbox(session_id, seq);

CREATE UNIQUE INDEX IF NOT EXISTS idx_event_delivery_outbox_session_event_id
  ON event_delivery_outbox(session_id, event_id);

CREATE INDEX IF NOT EXISTS idx_event_delivery_outbox_session_id
  ON event_delivery_outbox(session_id);
