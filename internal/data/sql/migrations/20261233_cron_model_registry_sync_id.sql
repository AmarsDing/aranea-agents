-- Version 20261233: 修复内置定时任务 model-registry-sync 空主键（2026-08-20 cron 深度审查 F1）
-- 根因：SeedModelRegistryCronTask 直插 cronRepo.CreateCronTask，绕过 Usecase.CreateTask 的
-- newCronTaskID()，导致 cron_task 行 id=''，其全部 cron_task_run.task_id 亦为 ''，
-- 进而引发 UI 启停/触发禁用碰撞、编辑弹窗死胡同、执行历史无法按该任务筛选。
-- 处置：统一改为显式 ID cron_model_registry_sync（与修复后的 seed 一致），并回填运行记录。
-- cron_task_run.task_id 无外键约束（纯 string 字段），两表可独立 UPDATE。
-- Idempotent: WHERE 条件幂等，重复执行影响 0 行。
UPDATE cron_task SET id = 'cron_model_registry_sync' WHERE task_key = 'model-registry-sync' AND id = '';
UPDATE cron_task_run SET task_id = 'cron_model_registry_sync' WHERE task_id = '';
