-- embedded by unified_evolution_schema.go
CREATE TABLE IF NOT EXISTS unified_evolution_suggestions (
  id TEXT PRIMARY KEY,
  target_type TEXT NOT NULL,
  target_id TEXT NOT NULL,
  action_type TEXT NOT NULL,
  trigger_source TEXT NOT NULL DEFAULT '',
  trigger_reason TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  priority INTEGER NOT NULL DEFAULT 0,
  draft_body TEXT NOT NULL DEFAULT '',
  draft_name TEXT NOT NULL DEFAULT '',
  merge_target_id TEXT NOT NULL DEFAULT '',
  lifecycle_status TEXT NOT NULL DEFAULT 'draft',
  sandbox_passed INTEGER NOT NULL DEFAULT 0,
  sandbox_result TEXT,
  metadata TEXT,
  created_at TEXT NOT NULL,
  approved_by TEXT NOT NULL DEFAULT '',
  applied_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_ues_target ON unified_evolution_suggestions(target_type, target_id);
CREATE INDEX IF NOT EXISTS idx_ues_target_status ON unified_evolution_suggestions(target_type, target_id, status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ues_pending_target ON unified_evolution_suggestions(target_type, target_id) WHERE status = 'pending';
