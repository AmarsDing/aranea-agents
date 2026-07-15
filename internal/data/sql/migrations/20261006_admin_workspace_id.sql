-- P2-A-2: 为 admins 表添加 workspace_id 列与索引。
-- 审计 P2-A 修复：将 admin 用户与 workspace 绑定（一对一模型），消除
-- "X-Workspace-ID 纯客户端可控"的安全风险。
--
-- 默认值 "default" 与 workspace.DefaultWorkspaceID 一致，确保 legacy
-- admin（迁移前已存在）自动归属 default workspace，避免历史数据成为孤儿。
--
-- 该列由 Ent Schema.Create() 在新数据库上自动创建；此迁移用于补齐已部署
-- 数据库缺失的列。ALTER TABLE ADD COLUMN 不支持 IF NOT EXISTS，重复执行
-- 会报 "duplicate column name" 错误，由迁移运行器按 DB-N6 规则视为成功。
--
-- 索引使用 IF NOT EXISTS，原生支持幂等。

ALTER TABLE admins ADD COLUMN workspace_id TEXT NOT NULL DEFAULT 'default';

CREATE INDEX IF NOT EXISTS idx_admins_workspace_id ON admins(workspace_id);
