-- Version 20261300: knowledge_source_connector_credential（G-38 SourceSync W1，
-- 凭据行表——P1 拍板 A，channel_credential 契约 1 同构）
-- 铁律链（§G-38 ④）：secret_ref 仅存 enc: 信封（biz.CredentialCrypto 唯一
-- 通道）；recordAudit 禁记 secret；读侧 API 只返 configured+masked_preview，
-- 永不返明文、不建 Reveal 端点；解密失败置 auth_paused 不重试烧配额。
-- 索引形态取 MCP partial-unique 活性唯一（P1 终裁唯一实质修正，
-- 20261209 ''::text 谓词规范先例）：软删墓碑行豁免，软删重建天然放行。
-- metadata_json 承载 workspace_id/bot_id 等非密元数据与 OAuth per-user
-- refresh 预留落点（挂账欠账目，不卡 shape）。
CREATE TABLE IF NOT EXISTS knowledge_source_connector_credential (
  id TEXT PRIMARY KEY,
  connector_id TEXT NOT NULL DEFAULT '',
  credential_key TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '',
  secret_ref TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT ''
);
-- 软删感知部分唯一：同 (connector_id, credential_key) 活跃行唯一。
CREATE UNIQUE INDEX IF NOT EXISTS idx_src_credential_active
  ON knowledge_source_connector_credential (connector_id, credential_key)
  WHERE deleted_at = ''::text;
CREATE INDEX IF NOT EXISTS idx_src_credential_connector
  ON knowledge_source_connector_credential (connector_id);
