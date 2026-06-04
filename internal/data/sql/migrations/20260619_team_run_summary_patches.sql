-- Version 20260619: Team run summary column patches
ALTER TABLE team_runs ADD COLUMN summary_json TEXT NOT NULL DEFAULT '';
ALTER TABLE team_runs ADD COLUMN definition_snapshot_json TEXT NOT NULL DEFAULT '';
