-- M53 Phase 5b: cross-domain trace on team runs
ALTER TABLE team_runs ADD COLUMN trace_id TEXT NOT NULL DEFAULT '';
