-- 20261326 cron_run_resolved（个人工作台告警 lane 核销）
-- cron 定时任务失败在 7 天窗口内持续计入告警计数与信任态 backlog，缺少
-- 「已确认/忽略」语义导致计数无法回到 live。resolved_at 非空 = 该次运行已被
-- 人工核销（一次核销按 task_id 覆盖该任务全部未核销失败，避免历史失败逐条
-- 复现）。resolved_by 记录操作者，供审计回溯。
ALTER TABLE cron_task_run ADD COLUMN IF NOT EXISTS resolved_at TEXT NOT NULL DEFAULT '';
ALTER TABLE cron_task_run ADD COLUMN IF NOT EXISTS resolved_by TEXT NOT NULL DEFAULT '';
-- 列表查询恒为 (status='failure' AND resolved_at='') 形态；组合索引让窗口内
-- 未核销失败的扫描面收敛到索引区间。
CREATE INDEX IF NOT EXISTS idx_cron_run_status ON cron_task_run (status, resolved_at);
