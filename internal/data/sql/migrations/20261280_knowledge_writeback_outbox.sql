-- Version 20261280: knowledge_writeback_outbox（G-10 写回飞轮持久化，2026-10-19）
-- AutoMemory 写回团队库失败此前仅 Warn 即永久丢失（cronrunner/jobs auto_memory.go
-- maybeWriteBack），与 AGENTS.md AS-EVT-01「Important = 异步持久化」事件可靠性
-- 分级相违。本表为写回批次重试队列：失败时落 payload，knowledge_writeback 后台
-- worker 周期重放至 WriteBackSessionFacts；attempts 触达上限翻 dead 终态留存排查。
-- 模式复用 decision_record_outbox（20261251）但不共用表——写回无 decision 语义，
-- batch_key（会话+事实内容哈希）为幂等键，同批事实反复失败不堆行。
-- 双方言通用（runner 自动翻译 SQLite→Postgres），幂等，重跑安全。
CREATE TABLE IF NOT EXISTS knowledge_writeback_outbox (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  batch_key TEXT NOT NULL,
  workspace_id TEXT NOT NULL DEFAULT '',
  payload TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  published_at TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_knowledge_writeback_outbox_key
  ON knowledge_writeback_outbox(batch_key);
CREATE INDEX IF NOT EXISTS idx_knowledge_writeback_outbox_status_created
  ON knowledge_writeback_outbox(status, created_at);
