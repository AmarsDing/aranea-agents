-- P1 形式契约（B.10.15.2）：为 plan_steps_v2 添加 deliverables / input_contract 列。
-- PlanStep 携带团队间交付物形式契约（Planner LLM 输出 + 确定性兜底派生），
-- crash recovery 从 DB 重建 dagRun 时契约不丢失。与 agent_keys 同模式（JSON TEXT）。
-- 详见 docs/development/1-chat.design.md §B.10.15

ALTER TABLE plan_steps_v2 ADD COLUMN deliverables TEXT;
ALTER TABLE plan_steps_v2 ADD COLUMN input_contract TEXT;
