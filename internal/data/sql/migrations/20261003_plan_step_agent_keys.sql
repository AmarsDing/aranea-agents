-- 2026-07-05 Step 2 修复：为 plan_steps_v2 添加 agent_keys 列。
-- PlanStep 携带 LLM 分配的 agent key 列表（来自 AllocationPlan），
-- RealTeamOrchestrator 优先使用此字段组建 team，避免查 DB 取到错误 agent。
-- 详见 docs/superpowers/plans/2026-07-05-fix-double-execution-plan-step-agent-keys.md

ALTER TABLE plan_steps_v2 ADD COLUMN agent_keys TEXT;
