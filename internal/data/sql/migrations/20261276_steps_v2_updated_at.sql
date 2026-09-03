-- 2026-09-03 P4-1 观测面：steps_v2 添加 updated_at。
-- ent UpdateDefault 在每次写入时自动刷新（零业务调用点改动），既有行回填
-- started_at。idle 探测（step_v2_repo.LatestStepActivityAt）取
-- max(started_at, updated_at)，覆盖「step 原地更新但无新 step 启动」的
-- 活跃形态，提升 F1 探测信号粒度。
ALTER TABLE steps_v2 ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
-- 既有行回填 started_at（保留原始新鲜度语义，而非全部记为迁移时刻）。
-- 全表 UPDATE 无 WHERE：迁移在 schema_migrations 版本守卫下仅执行一次，
-- 执行窗口内不存在「已按新语义写入的 updated_at」（列刚创建），全表覆盖
-- 安全且幂等（重跑结果相同）。
UPDATE steps_v2 SET updated_at = started_at;
