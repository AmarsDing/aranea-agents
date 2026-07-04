-- 2026-07-04 问题 3 修复：为 team_stages_v2 添加 team_name 列。
-- 前端 TeamStagePanel 需要展示团队名称（如"研究团队"）而非团队 ID（如
-- "team-uuid-xxx"），后端 TeamStage 事件携带 TeamName 后需要持久化到 DB，
-- 以便刷新页面后从 DB 加载历史数据时仍能显示团队名称。
-- 详见 docs/development/1-chat.md §A.4.3 团队展示规范。

ALTER TABLE team_stages_v2 ADD COLUMN team_name TEXT NOT NULL DEFAULT '';
