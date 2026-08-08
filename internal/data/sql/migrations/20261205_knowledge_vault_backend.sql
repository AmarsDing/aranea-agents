-- 20261205_knowledge_vault_backend: SP1-F 团队库后端维度（设计 S6）。
-- 幂等：IF NOT EXISTS；重复应用安全。
-- 语义锚点：
--   - vault_backend = 'local'：文件系统即真相源（root_path 必填唯一，biz 层约束）；
--   - vault_backend = 'team'：PG 即真相源（knowledge_documents.content_text 承载文档本体，
--     root_path 必须为空，无 SyncEngine 文件监听）。
--   - 存量行默认 'local' 与迁移前语义一致（此前所有 vault 均为本地库），无需回填。
--   - root_path 部分唯一索引（WHERE root_path <> ''）对 team 行天然不生效，无需调整。

ALTER TABLE knowledge_collections ADD COLUMN IF NOT EXISTS vault_backend TEXT NOT NULL DEFAULT 'local';
