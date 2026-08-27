-- 20261268 decision_records_sessionid_idx（T5 chat 侧闸事件聚合面，Postgres only）
-- source_session_id 过滤 / SessionGateStats 聚合的生产表达式为
-- COALESCE(
--   COALESCE(NULLIF(source_ref::text,'')::jsonb,'{}'::jsonb) ->> 'session_id',
--   COALESCE(NULLIF(metadata::text,'')::jsonb,'{}'::jsonb) ->> 'session_id'
-- )——新记录 session_id 是 source_ref 一等公民，旧记录（Extra 注入时期）
-- 仅在 metadata，两路 COALESCE 兼容。表达式与 data 层 gateSessionExpr 生成
-- 串精确一致方可命中（20261252/20261254 两度实测：COALESCE 防御壳位置差
-- 异即索引失配全表扫）。
-- 由 registry Func ddlDecisionRecordsSessionIDIndex 门控执行；SQLite CLI/测试跳过。
-- 幂等，重跑安全。
CREATE INDEX IF NOT EXISTS idx_decision_records_source_session_id
  ON decision_records ((COALESCE(
    COALESCE(NULLIF(source_ref::text, '')::jsonb, '{}'::jsonb) ->> 'session_id',
    COALESCE(NULLIF(metadata::text, '')::jsonb, '{}'::jsonb) ->> 'session_id'
  )));
