-- MCP per-user credentials (when config_json.require_user_credentials is true)

CREATE TABLE IF NOT EXISTS mcp_server_user_credential (
  id TEXT PRIMARY KEY,
  mcp_server_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  credential_key TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  secret_ref TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT '',
  UNIQUE(mcp_server_id, user_id, credential_key)
);

CREATE INDEX IF NOT EXISTS idx_mcp_user_cred_server ON mcp_server_user_credential(mcp_server_id);
CREATE INDEX IF NOT EXISTS idx_mcp_user_cred_user ON mcp_server_user_credential(user_id);
