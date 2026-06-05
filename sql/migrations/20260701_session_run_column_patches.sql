-- Version 20260701: Session run column patches
ALTER TABLE session_runs ADD COLUMN checkpoint_id TEXT NOT NULL DEFAULT '';
ALTER TABLE session_runs ADD COLUMN agent_id TEXT NOT NULL DEFAULT '';
ALTER TABLE session_runs ADD COLUMN resume_started_at TEXT NOT NULL DEFAULT '';
