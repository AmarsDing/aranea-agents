-- 2026-09-01 取证断裂修复：team_runs 添加 team_run_v2_id 桥接列。
-- team_run_steps.run_id 指向 team_runs.id（v1 随机 UUID），而编排/评测取证
-- 查 team_runs_v2（确定性 SHA1 ID，agent.NewTeamRunV2ID 公式），两族 ID
-- 独立生成导致按 run_id join 为 0 行。runner 创建 v1 run 时用同一公式派生
-- v2 run ID 落本列；空串 = 无 root task 上下文无法派生（legacy 数据）。
-- Ent Schema.Create() 不会为已存在表新增列，需要 ALTER TABLE 补列。
ALTER TABLE team_runs ADD COLUMN IF NOT EXISTS team_run_v2_id TEXT NOT NULL DEFAULT '';
