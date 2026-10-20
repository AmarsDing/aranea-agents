-- Version 20261283: 父子分块（G-06）
-- knowledge_chunks 增 parent_chunk_id（子块→父块行 ID，'' = 独立块/父块自身；
-- 应用层同一 DELETE+INSERT 事务维护，不加自引用 FK）与 chunk_role（child|parent，
-- 父块仅语境回填，不嵌入不进召回）。回填批量取父块走部分索引（仅非空父引用才入索引）。
-- knowledge_documents 增 chunk_strategy/chunk_size/chunk_overlap：摄取时落库生效值，
-- reembed 据此保形（修复重嵌恒 char+512/64 丢形状的存量 bug）。
-- Idempotent: "duplicate column" errors are treated as success by the migration runner (DB-N6).
ALTER TABLE knowledge_chunks ADD COLUMN parent_chunk_id TEXT NOT NULL DEFAULT '';
ALTER TABLE knowledge_chunks ADD COLUMN chunk_role TEXT NOT NULL DEFAULT 'child';
CREATE INDEX IF NOT EXISTS knowledge_chunks_parent_idx
  ON knowledge_chunks (parent_chunk_id) WHERE parent_chunk_id <> '';
ALTER TABLE knowledge_documents ADD COLUMN chunk_strategy TEXT NOT NULL DEFAULT '';
ALTER TABLE knowledge_documents ADD COLUMN chunk_size INT NOT NULL DEFAULT 0;
ALTER TABLE knowledge_documents ADD COLUMN chunk_overlap INT NOT NULL DEFAULT 0;
