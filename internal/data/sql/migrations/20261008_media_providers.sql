-- media_providers: media generation provider configs (media generation observation view).
-- Table is also managed by Ent schema (media_provider.go). This DDL ensures existence
-- on deployments where Ent auto-migration ordering may differ.

CREATE TABLE IF NOT EXISTS media_providers (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  provider_type TEXT NOT NULL,
  base_url TEXT NOT NULL DEFAULT '',
  api_key TEXT NOT NULL DEFAULT '',
  config_json TEXT NOT NULL DEFAULT '{}',
  capabilities TEXT NOT NULL DEFAULT '[]',
  status TEXT NOT NULL DEFAULT 'active',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT ''
);
