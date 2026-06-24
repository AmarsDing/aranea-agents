-- Version 20260804: Planner model configuration columns
-- Stores the plan_and_execute planner model resolution mode and specify-mode values.
-- Mode: 'specify' (admin picks a model) or 'inherit' (use session's selected model).
ALTER TABLE system_settings ADD COLUMN planner_model_mode TEXT NOT NULL DEFAULT 'inherit';
ALTER TABLE system_settings ADD COLUMN planner_model_provider TEXT NOT NULL DEFAULT '';
ALTER TABLE system_settings ADD COLUMN planner_model_model TEXT NOT NULL DEFAULT '';
