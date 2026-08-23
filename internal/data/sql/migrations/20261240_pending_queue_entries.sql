-- 20261240 pending_queue_entries: durable follow-up queue for process restart.
-- Complements pending_queue.json write-through. Idempotent CREATE IF NOT EXISTS.

CREATE TABLE IF NOT EXISTS pending_queue_entries (
  session_id TEXT NOT NULL,
  entry_id TEXT NOT NULL,
  content TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  created_at TEXT NOT NULL,
  priority INTEGER NOT NULL DEFAULT 0,
  kind TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (session_id, entry_id)
);

CREATE INDEX IF NOT EXISTS idx_pending_queue_session_created
  ON pending_queue_entries (session_id, created_at);
