-- Version 20261246: usage events (usage_kind, message_id) 复合索引（79-runtime-governance Phase 0 任务 0.1）
-- run 级 cache_hit_ratio 读路径引入两类新查询：team_turn 行按 message_id(=run_id) 直取、
-- genuine team_member 行按 message_id(=step_id) IN 子查询求和。events 表持续增长且无
-- message_id 相关索引（既有索引仅 date_key/session_id/agent_id/provider_code/trace_id 表达式），
-- 每次 run 详情读取都会 seq scan。复合索引以前导列 usage_kind 收窄种类、message_id 精确定位，
-- 同服务于两类查询。Idempotent: CREATE INDEX IF NOT EXISTS，双方言通用。
CREATE INDEX IF NOT EXISTS idx_model_token_usage_events_kind_message ON model_token_usage_events(usage_kind, message_id);
