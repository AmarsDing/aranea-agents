-- 20261341 office_task_executions（任务 ↔ 执行关联，设计
-- docs/aranea-agents/plan/2026-11-21-任务入口与执行统筹方案.md §6）
--
-- 一个工作台任务可以有多次执行，每次执行是别处的一条记录：一段会话、一次团队运行、
-- 一次流程运行、一个编码任务、运行台的一个终端或一次编排、一轮定时任务。这张表只记
-- 「哪条记录属于哪个任务」和派出时的交接单，执行本身的状态仍在各自的表里。
-- 之前这层关系记在会话 metadata_json（cc_task / cc_links）里，换不了会话的执行挂不上。
--
--   kind         session | team_run | graph_execution | coding_task | fleet_terminal | fleet_run | cron_run
--   ref_id       该类记录自己的 id
--   executor     执行者键（agent:<id> / team:<id> / flow:<graphId> / 工具键），只用于展示与筛选
--   hand         派出时发给执行者的交接单原文
-- user_id 冗余存一份，属主校验不必 JOIN。任务删除级联。同一任务下同一条记录只挂一次。
-- 幂等：IF NOT EXISTS，重跑安全。

CREATE TABLE IF NOT EXISTS office_task_executions (
  "id"            TEXT PRIMARY KEY,
  "task_id"       TEXT NOT NULL REFERENCES office_tasks("id") ON DELETE CASCADE,
  "user_id"       TEXT NOT NULL,
  "kind"          TEXT NOT NULL,
  "ref_id"        TEXT NOT NULL,
  "executor"      TEXT NOT NULL DEFAULT '',
  "executor_name" TEXT NOT NULL DEFAULT '',
  "hand"          TEXT NOT NULL DEFAULT '',
  "created_at"    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_office_task_executions_ref ON office_task_executions ("task_id", "kind", "ref_id");
CREATE INDEX IF NOT EXISTS idx_office_task_executions_user ON office_task_executions ("user_id", "created_at");
