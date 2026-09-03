-- 2026-09-03 P2-1 持久化心跳：team_runs_v2 添加 heartbeat_at。
-- runner 流式消费循环节流写入（单列 UPDATE，无版本守卫）；biz idle 探测
-- （spirit_orchestration.probeTeamActivity）取 max(steps_v2.started_at,
-- heartbeat_at) 作为团队活跃信号，根治「成员单 step 长流式生成（>idle 窗口
-- 无新 step 启动）被误判 idle 击杀」。
-- Ent Schema.Create() 不会为已存在表新增列，需要 ALTER TABLE 补列。
ALTER TABLE team_runs_v2 ADD COLUMN IF NOT EXISTS heartbeat_at TIMESTAMPTZ NULL;
