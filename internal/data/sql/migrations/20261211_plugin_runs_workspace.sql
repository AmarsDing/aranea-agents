-- N-B5: plugin_runs 工作区隔离（运行审计租户可见性）。
-- empty workspace_id = 共享行（所有租户可见；当前写入侧语义）；
-- non-empty = 租户私有行（仅归属租户可见）。存量行默认 ''（共享，行为不变）。
-- 索引命名沿用 sql/plugin_run.sql 的 idx_plugin_runs_* 风格。

ALTER TABLE plugin_runs ADD COLUMN IF NOT EXISTS workspace_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_plugin_runs_workspace ON plugin_runs(workspace_id);
