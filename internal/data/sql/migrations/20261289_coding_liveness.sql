-- Version 20261289: coding_agents / coding_tasks 活跃性监管与孤儿收割扩列（M4，ADR-88）
-- coding_tasks 增 child_pid（Windows 进程组根 PID，重启收割 taskkill /T /F）＋
-- liveness 四字段（last_active_at / last_excerpt / heartbeat_miss / liveness_state）；
-- coding_agents 增 liveness_override（按 key 覆盖 liveness 阈值，JSON 文本，空=全局默认）。
-- P0-2b（child_pid）先于 P3 活跃性监管落地，同批迁移一次到位。
-- Idempotent: "duplicate column" errors are treated as success by the migration runner (DB-N6)。
ALTER TABLE coding_tasks ADD COLUMN child_pid INTEGER NOT NULL DEFAULT 0;
ALTER TABLE coding_tasks ADD COLUMN last_active_at TEXT NOT NULL DEFAULT '';
ALTER TABLE coding_tasks ADD COLUMN last_excerpt VARCHAR(256) NOT NULL DEFAULT '';
ALTER TABLE coding_tasks ADD COLUMN heartbeat_miss INTEGER NOT NULL DEFAULT 0;
ALTER TABLE coding_tasks ADD COLUMN liveness_state VARCHAR(16) NOT NULL DEFAULT 'healthy';
ALTER TABLE coding_agents ADD COLUMN liveness_override JSONB;
-- 回填（76 设计「活跃性监管」§B）：存量行 last_active_at 以 updated_at 为基线。
UPDATE coding_tasks SET last_active_at = updated_at WHERE last_active_at = '';
