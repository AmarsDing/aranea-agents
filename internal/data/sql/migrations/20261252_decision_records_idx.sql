-- Version 20261252: decision_records 查询索引（M80 决策智能 Phase 1，NFR-80-04）
-- 双方言常规索引：列表筛选（category+时间倒序）、actor 维度、父链追溯、
-- workspace 隔离筛选。PG 侧 source_ref/related_entities 的 GIN 表达式索引
-- 由同版本条目的 Func（ddlDecisionRecordsGINIndexes）门控补建，SQLite 跳过。
-- 幂等，重跑安全。
CREATE INDEX IF NOT EXISTS idx_decision_records_category_created
  ON decision_records(category, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_decision_records_actor
  ON decision_records(actor_key);
CREATE INDEX IF NOT EXISTS idx_decision_records_parent
  ON decision_records(parent_decision_id);
CREATE INDEX IF NOT EXISTS idx_decision_records_ws_category
  ON decision_records(workspace_id, category);
