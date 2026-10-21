-- Version 20261291: session_approval_policies（P0-5 审批工程化，批 A11）
-- 会话/Agent 作用域的确定性审批姿态表：ask|never|trusted × tool_pattern ×
-- command_prefix × args_pattern × priority × shadow_only × expires_at。
-- 消费链：BeforeTool 审批策略 gate（p3，paramRuleGate 之后、循环守卫之前）——
-- never 确定性拒绝（文案不给 pattern，镜像 param_rule_deny）；trusted 作为新
-- grant 档在确认门禁 decide() 的 computer-use danger floor 之后消费。
-- 编号顺延当时 migrations 最大值（20261290 chunk_embeddings）+1，无撞号。
-- 仅建表：不加 ent schema、不播种内置规则。幂等，重跑安全。
CREATE TABLE IF NOT EXISTS session_approval_policies (
  id             TEXT PRIMARY KEY,
  scope          TEXT NOT NULL,
  scope_key      TEXT NOT NULL,
  tool_pattern   TEXT NOT NULL,
  command_prefix TEXT NOT NULL DEFAULT '',
  args_pattern   TEXT NOT NULL DEFAULT '',
  mode           TEXT NOT NULL,
  priority       INTEGER NOT NULL DEFAULT 100,
  enabled        INTEGER NOT NULL DEFAULT 1,
  shadow_only    INTEGER NOT NULL DEFAULT 0,
  expires_at     INTEGER NOT NULL DEFAULT 0,
  created_at     INTEGER NOT NULL DEFAULT 0,
  created_by     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_sap_scope ON session_approval_policies(scope, scope_key, enabled, priority);
