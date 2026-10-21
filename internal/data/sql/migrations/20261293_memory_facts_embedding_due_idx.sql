-- 20261293 memory_facts_embedding_due_idx (AMC C3 修复 #7, Postgres only)
-- MemoryFactIndexSweeper（2026-10-22 AMC C3-Q2 引入）每 30s 拾取一行批：
--   WHERE status='active' AND deleted_at=''
--     AND embedding_status IN ('stale','pending','failed')
--     AND index_attempts < 8 AND 退避到期
--   ORDER BY updated_at ASC LIMIT 50
-- 无支撑索引时对 38 万行/8.2GB memory_facts 每 30s 全表顺序扫描 + 排序，
-- 并与 32 并发评测搜索/Add 争抢 I/O。本部分索引只对 due 候选谓词生效：
-- 行数趋零时索引体积极小，写放大仅在 embedding_status 变化时发生一次。
-- index_attempts/退避表达式随行变化，查询计划按部分索引取 (embedding_status,
-- updated_at) 有序前缀后做行内过滤。
-- Idempotent: IF NOT EXISTS；存量库重跑安全。
CREATE INDEX IF NOT EXISTS idx_memory_facts_embedding_due
  ON memory_facts (embedding_status, updated_at)
  WHERE status = 'active' AND deleted_at = ''
    AND embedding_status IN ('stale', 'pending', 'failed');
