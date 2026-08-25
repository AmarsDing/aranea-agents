-- Version 20261245: Agent runtime settings — 包B 意图门禁（session-eval-20260825 B1）
-- intent_skip_enabled：简单轮 skip 快路径开关（shouldSkipIntentPass 的 agent 维度闸）。
-- default TRUE 保持现状（存量行零行为变化）；管理层 agent（3 GM + 部门主管 + spirit）
-- 经 SQL 置 FALSE——任务型消息被 QuickAssess 误判 simple 导致 intent pass 跳过、
-- 组织路由失效（P-INTENT-SKIP），校准原则「宁重勿轻」（R4 RouteLLM 非对称代价）。
-- INTEGER 0/1 兼容双方言（同 20261244 先例）。幂等，重跑安全（DB-N6 duplicate column 视为成功）。
ALTER TABLE agent_runtime_settings ADD COLUMN intent_skip_enabled INTEGER NOT NULL DEFAULT 1;
