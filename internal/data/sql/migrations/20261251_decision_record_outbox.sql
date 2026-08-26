-- Version 20261251: decision_record_outbox（M80 决策智能 Phase 1，E5）
-- 决策记录异步落库 outbox：采集侧（主 Turn 链路）只做内存入队，worker 批量
-- flush 到 decision_records，DB 故障不阻塞/不反压业务链路。decision_key 为
-- 幂等键（worker 重试/重复入队去重）。模式复用 event_delivery_outbox（20261010）
-- 但不共用该表——决策 outbox 无 session/seq 语义，按 created_at 顺序拉取。
-- 双方言通用，幂等，重跑安全。
CREATE TABLE IF NOT EXISTS decision_record_outbox (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  decision_key TEXT NOT NULL,
  payload TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  published_at TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_decision_record_outbox_key
  ON decision_record_outbox(decision_key);
CREATE INDEX IF NOT EXISTS idx_decision_record_outbox_status_created
  ON decision_record_outbox(status, created_at);
