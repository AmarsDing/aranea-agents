-- 20261331 office_task_assignee（工作台重新设计第 3 步：指令栏与委派）
-- 任务对象要能同时指派给人或 Agent：assignee_kind 为空 = 负责人是任务属主
-- 本人；agent / team = 交办给某个 Agent 或团队，assignee_id 为其主键，
-- assignee_name 是交办当时的显示名快照（对方改名或被删后列表仍可读）。
-- 交办任务的 source='delegate'，source_ref 为承载这次交办的会话 id；
-- 随手记录的 source='note'。存量行默认空串，行为不变。幂等 IF NOT EXISTS。
ALTER TABLE office_tasks ADD COLUMN IF NOT EXISTS assignee_kind TEXT NOT NULL DEFAULT '';
ALTER TABLE office_tasks ADD COLUMN IF NOT EXISTS assignee_id TEXT NOT NULL DEFAULT '';
ALTER TABLE office_tasks ADD COLUMN IF NOT EXISTS assignee_name TEXT NOT NULL DEFAULT '';
-- 列表恒按 (user_id, source, status) 过滤后取最近若干条。
CREATE INDEX IF NOT EXISTS idx_office_tasks_user_source ON office_tasks ("user_id", "source", "status", "created_at" DESC);
