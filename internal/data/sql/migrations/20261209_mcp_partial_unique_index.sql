-- mcp_partial_unique_index: 软删除感知的部分唯一索引（MCP 管理模块 R1 修复）。
--
-- 背景：mcp_server.server_key 原为列级 UNIQUE（PG 隐式约束
-- mcp_server_server_key_key），mcp_server_user_credential 原为全表复合唯一
-- 索引 platformmcpusercredential_mcp_server_id_user_id_credential_key。
-- 二者均含软删除墓碑行（deleted_at != ''），导致同名 server_key / 同
-- (mcp_server_id, user_id, credential_key) 凭据软删后无法重建（23505）。
--
-- 修复：删除旧约束/索引，改为仅约束活跃行（deleted_at = ''）的部分唯一索引。
-- 新索引已在 Ent Schema 中声明（entsql.IndexWhere），新库由 Ent 自动创建；
-- 本迁移负责存量库的旧约束清理与索引补齐。全部语句幂等。
--
-- 谓词写 PG 规范形式 ''::text，与 Ent 声明及 pg_get_expr 输出保持一致。

-- ============ mcp_server.server_key ============

-- 旧约束可能是内联 UNIQUE（约束+索引同名）或独立唯一索引，两种形态都处理。
ALTER TABLE mcp_server DROP CONSTRAINT IF EXISTS mcp_server_server_key_key;
DROP INDEX IF EXISTS mcp_server_server_key_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_mcp_server_server_key_active
  ON mcp_server (server_key)
  WHERE deleted_at = ''::text;

-- ============ mcp_server_user_credential 复合唯一 ============

DROP INDEX IF EXISTS platformmcpusercredential_mcp_server_id_user_id_credential_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_mcp_credential_unique_active
  ON mcp_server_user_credential (mcp_server_id, user_id, credential_key)
  WHERE deleted_at = ''::text;
