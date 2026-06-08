-- Version 20260608: Configurable subagent runes and verification truncate chars
ALTER TABLE agent_runtime_settings ADD COLUMN subagents_stored_result_runes INTEGER NOT NULL DEFAULT 4000;
ALTER TABLE agent_runtime_settings ADD COLUMN subagents_stored_summary_runes INTEGER NOT NULL DEFAULT 240;
ALTER TABLE agent_runtime_settings ADD COLUMN verification_truncate_chars INTEGER NOT NULL DEFAULT 2000;
