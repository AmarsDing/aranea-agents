-- Version 20261323: approval_risk_matrix 分级审批矩阵配置表（E 阶段 W1b，
-- 需求文档 §5.3 拍板 A-4）。
--
-- 语义：三级风险（low/medium/high）× approval-policy 四档（never/auto/ask/
-- trusted）的默认档映射 + 中风险同类自动批准次数上限。matcher_key 为匹配键
-- （tool_pattern/动作类名，消费点=approval-policy 求值默认档建议，W3 接线）。
--
-- 幂等：IF NOT EXISTS；双方言（pg/sqlite）语法一致。重跑安全。
CREATE TABLE IF NOT EXISTS approval_risk_matrix (
  "matcher_key"        TEXT PRIMARY KEY,
  "risk"               TEXT NOT NULL DEFAULT 'medium',
  "default_mode"       TEXT NOT NULL DEFAULT 'ask',
  "auto_approve_times" INTEGER NOT NULL DEFAULT 0,
  "updated_by"         TEXT NOT NULL DEFAULT '',
  "updated_at"         TEXT NOT NULL DEFAULT ''
);
