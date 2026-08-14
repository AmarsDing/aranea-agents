-- Y4/Y5（53-team-graph-orchestration P1 恢复路径正确性）：
-- graph_executions 增加：
--   spirit_session_id — 跨会话聚合键（root spirit session）。此前仅内存持有，
--     进程重启后从 DB 加载的执行丢失该字段，resume 时事件归属错乱。
--   definition_hash — 执行启动时 GraphBuildConfig 的 SHA256。C1 全量物化后
--     team 图是真实资产，定义被编辑后 resume 旧 checkpoint 会按新图路由，
--     节点/边不一致导致错乱；resume 前比对 hash，不一致拒绝恢复。
-- team_graph_sessions 增加：
--   spirit_session_id — coordinator RecoverSessions 重建 step-watch 订阅的
--     过滤键（EventSubscribeOptions.SpiritSessionID），此前恢复后为空导致
--     waiting_human 会话收不到 graph_stage 通知。

ALTER TABLE graph_executions ADD COLUMN spirit_session_id TEXT NOT NULL DEFAULT '';
ALTER TABLE graph_executions ADD COLUMN definition_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE team_graph_sessions ADD COLUMN spirit_session_id TEXT NOT NULL DEFAULT '';

-- 回填：standalone 执行的 spirit_session_id 等于其 session_id（直接 spirit 运行）。
UPDATE graph_executions SET spirit_session_id = session_id
WHERE spirit_session_id = '' AND session_id != '';
