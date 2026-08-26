-- Version 20261248: sessions 增加 fork_from_turn_id（79-runtime-governance R6 Phase 4.1）
-- 会话 Fork-from-Turn：新会话以 parent_session_id 记血缘（既有列），本列记录分叉点
-- turn id（= v2 turn id / invocation id），供血缘徽标「来源会话 + 轮次」展示与审计追溯。
-- TEXT 双方言通用。幂等，重跑安全（DB-N6 duplicate column 视为成功）。
ALTER TABLE sessions ADD COLUMN fork_from_turn_id TEXT NOT NULL DEFAULT '';
