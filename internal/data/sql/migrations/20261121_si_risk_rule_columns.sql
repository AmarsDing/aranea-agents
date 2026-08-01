-- Version 20261121: Self-improvement risk rule configuration columns (P5 console).
-- Stores the admin-configurable D6/D10 risk-classification rules on the
-- system_settings singleton. 0 / '' means "inherit code defaults"
-- (100 / 300 / default D6 core-path globs / daily auto-apply quota 5).
ALTER TABLE system_settings ADD COLUMN si_risk_low_max_lines INTEGER NOT NULL DEFAULT 0;
ALTER TABLE system_settings ADD COLUMN si_risk_medium_max_lines INTEGER NOT NULL DEFAULT 0;
ALTER TABLE system_settings ADD COLUMN si_risk_core_path_globs TEXT NOT NULL DEFAULT '';
ALTER TABLE system_settings ADD COLUMN si_risk_daily_auto_quota INTEGER NOT NULL DEFAULT 0;
