-- 20261336 office_task_lifecycle（工作台任务生命周期，设计
-- docs/aranea-agents/design/2026-11-20-工作台任务生命周期.md §5）
--
-- office_tasks：
--   session_id  AI 执行的承载会话。原来交办行把会话放在 source_ref 里，任务一旦
--               派给 AI 就丢了来源（邮件转来的任务 source_ref 是邮件 id）。从此执行
--               会话单独一列，source / source_ref 只表示「从哪来」。存量交办行回填。
--   due_at      到期时间（可空）；今天到期的任务进今日时间线。
--   status      取值扩为 open | doing | done（列是 TEXT，无需改约束）。
--
-- office_task_files 任务附件：与邮件附件同款 BYTEA 入库，单个文件上限与每任务
-- 文件数在 biz 层卡口；user_id 冗余存一份，属主校验不必 JOIN。任务删除级联。
-- 幂等：IF NOT EXISTS / 回填只改空值，重跑安全。

ALTER TABLE office_tasks ADD COLUMN IF NOT EXISTS "session_id" TEXT NOT NULL DEFAULT '';
ALTER TABLE office_tasks ADD COLUMN IF NOT EXISTS "due_at" TIMESTAMPTZ;

UPDATE office_tasks SET session_id = source_ref
 WHERE source = 'delegate' AND session_id = '' AND source_ref <> '';

CREATE TABLE IF NOT EXISTS office_task_files (
  "id"           TEXT PRIMARY KEY,
  "task_id"      TEXT NOT NULL REFERENCES office_tasks("id") ON DELETE CASCADE,
  "user_id"      TEXT NOT NULL,
  "filename"     TEXT NOT NULL DEFAULT '',
  "content_type" TEXT NOT NULL DEFAULT '',
  "size"         BIGINT NOT NULL DEFAULT 0,
  "content"      BYTEA NOT NULL,
  "created_at"   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_office_task_files_task ON office_task_files ("task_id", "created_at");
