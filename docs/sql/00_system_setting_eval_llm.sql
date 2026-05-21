-- Evaluation UserSim / LLM-as-Judge defaults (system_settings singleton).
ALTER TABLE system_settings ADD COLUMN eval_sim_provider TEXT NOT NULL DEFAULT '';
ALTER TABLE system_settings ADD COLUMN eval_sim_model TEXT NOT NULL DEFAULT '';
ALTER TABLE system_settings ADD COLUMN eval_judge_provider TEXT NOT NULL DEFAULT '';
ALTER TABLE system_settings ADD COLUMN eval_judge_model TEXT NOT NULL DEFAULT '';
