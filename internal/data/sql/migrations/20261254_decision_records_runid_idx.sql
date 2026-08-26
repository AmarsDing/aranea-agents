-- 20261254 decision_records_runid_idx（M80 1.10，Postgres only）
-- source_run_id 过滤（设计 §4.1 高频条件）生产表达式为
-- COALESCE(NULLIF(source_ref::text,'')::jsonb,'{}'::jsonb) ->> 'run_id'，
-- GIN（20261252）只服务 @> 包含不服务 ->> 等值，1M 行实测并行全表扫
-- P95≈125ms；补 btree 表达式索引后 <5ms（NFR-80-04 余量 40 倍）。
-- 由 registry Func ddlDecisionRecordsRunIDIndex 门控执行；SQLite CLI/测试跳过。
-- 幂等，重跑安全。
CREATE INDEX IF NOT EXISTS idx_decision_records_source_run_id
  ON decision_records ((COALESCE(NULLIF(source_ref::text, '')::jsonb, '{}'::jsonb) ->> 'run_id'));
