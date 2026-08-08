-- Version 20261201: Self-improvement risk rule configuration columns (P5 console).
-- Stores the admin-configurable D6/D10 risk-classification rules on the
-- system_settings singleton. 0 / '' means "inherit code defaults"
-- (100 / 300 / default D6 core-path globs / daily auto-apply quota 5).
-- 重编号注记：原版本 20261121 与数据迁移 team_copy_ownership_to_user 的
-- 历史编号碰撞——该数据迁移在重编号为 20261124 之前曾以 20261121 落库，
-- 凡在此窗口跑过它的库都会把本 DDL 误判为"已应用"而永久跳过（si_risk_*
-- 列缺失，GetRiskRules 500）。20261201 重新入列后：缺列库正常补列；已建列
-- 库经 AlreadyExistsErr 逐句跳过。语句天然幂等，重复执行安全。
ALTER TABLE system_settings ADD COLUMN si_risk_low_max_lines INTEGER NOT NULL DEFAULT 0;
ALTER TABLE system_settings ADD COLUMN si_risk_medium_max_lines INTEGER NOT NULL DEFAULT 0;
ALTER TABLE system_settings ADD COLUMN si_risk_core_path_globs TEXT NOT NULL DEFAULT '';
ALTER TABLE system_settings ADD COLUMN si_risk_daily_auto_quota INTEGER NOT NULL DEFAULT 0;
