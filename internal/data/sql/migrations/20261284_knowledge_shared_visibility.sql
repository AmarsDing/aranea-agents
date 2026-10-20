-- Version 20261284: 部门级可见性（G-08）
-- knowledge_documents 增 shared_with：visibility='shared' 时允许读取的 org 节点 ID
-- （部门/岗位）名单；集合谓词 `shared_with && $n::text[]`（TEXT[] + GIN，ADR-KN-20261020-E）。
-- 存量行默认 '{}'（名单为空 = shared 档仅属主可见， fail-closed）。
-- Idempotent: "duplicate column" errors are treated as success by the migration runner (DB-N6).
ALTER TABLE knowledge_documents ADD COLUMN shared_with TEXT[] NOT NULL DEFAULT '{}';
CREATE INDEX IF NOT EXISTS knowledge_documents_shared_with_gin
  ON knowledge_documents USING GIN (shared_with);
