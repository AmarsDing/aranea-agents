-- Version 20261308: 03 方案 P0（外部编码工具看板）——coding_agents 增加图标/能力声明/
-- 默认自治级别（注册表驱动前端渲染），coding_tasks 增加发起来源与验收字段，
-- 新建 coding_task_events 统一步骤事件表（raw-SQL 管理，不加 ent schema，
-- 先例：20261295_board_run_leases）。
-- Idempotent: "duplicate column" / "already exists" errors are treated as success by the migration runner (DB-N6)。

ALTER TABLE coding_agents ADD COLUMN icon_url TEXT NOT NULL DEFAULT '';
ALTER TABLE coding_agents ADD COLUMN capabilities TEXT NOT NULL DEFAULT '';
ALTER TABLE coding_agents ADD COLUMN default_autonomy VARCHAR(32) NOT NULL DEFAULT '';

ALTER TABLE coding_tasks ADD COLUMN origin VARCHAR(16) NOT NULL DEFAULT '';
ALTER TABLE coding_tasks ADD COLUMN review_status VARCHAR(16) NOT NULL DEFAULT '';
ALTER TABLE coding_tasks ADD COLUMN review_comment TEXT NOT NULL DEFAULT '';
ALTER TABLE coding_tasks ADD COLUMN reviewed_at TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS coding_task_events (
  id VARCHAR(64) PRIMARY KEY,
  task_id VARCHAR(64) NOT NULL,
  seq BIGINT NOT NULL,
  kind VARCHAR(32) NOT NULL DEFAULT '',
  title VARCHAR(256) NOT NULL DEFAULT '',
  text TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_coding_task_events_task ON coding_task_events (task_id, seq);
