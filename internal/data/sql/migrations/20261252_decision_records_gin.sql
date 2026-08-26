-- 20261252 decision_records_gin（M80，Postgres only）
-- source_ref / related_entities 为 TEXT 存 JSON（双方言惯例），PG 侧以
-- 表达式 GIN 索引支撑 source_ref 任意键指向查询（@> 包含）与实体交集检索
-- （Phase 2 先例检索）。列默认值 '{}' / '[]' 保证全行为合法 JSON，转换安全。
-- 由 registry Func ddlDecisionRecordsGINIndexes 门控执行；SQLite CLI/测试跳过。
-- 幂等，重跑安全。
CREATE INDEX IF NOT EXISTS idx_decision_records_source_ref_gin
  ON decision_records USING GIN ((source_ref::jsonb));
CREATE INDEX IF NOT EXISTS idx_decision_records_entities_gin
  ON decision_records USING GIN ((related_entities::jsonb));
