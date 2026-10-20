-- Version 20261282: knowledge_access_log 扩列（G-15 审计足迹）
-- 加 user/agent/rank/score/latency 列：session_id 列在 20261220 建表时已存在但从未
-- 被写入，自本版本起由检索命中路径（Retriever/Router）连同新列一起接通。
-- 为「我的足迹」与按用户的审计查询建 (user_id, accessed_at DESC) 索引。
-- Idempotent: "duplicate column" errors are treated as success by the migration runner (DB-N6).
-- rank 为窗口函数关键字，双方言统一加双引号。
ALTER TABLE knowledge_access_log ADD COLUMN user_id TEXT;
ALTER TABLE knowledge_access_log ADD COLUMN agent_id TEXT;
ALTER TABLE knowledge_access_log ADD COLUMN "rank" INTEGER NOT NULL DEFAULT 0;
ALTER TABLE knowledge_access_log ADD COLUMN score DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE knowledge_access_log ADD COLUMN latency_ms INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS knowledge_access_log_user_idx
  ON knowledge_access_log (user_id, accessed_at DESC);
