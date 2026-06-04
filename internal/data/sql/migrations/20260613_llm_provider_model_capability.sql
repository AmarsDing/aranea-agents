-- Version 20260613: LLM provider model capability column patches
ALTER TABLE llm_provider_models ADD COLUMN capability_text INTEGER NOT NULL DEFAULT 0;
ALTER TABLE llm_provider_models ADD COLUMN capability_vision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE llm_provider_models ADD COLUMN capability_audio INTEGER NOT NULL DEFAULT 0;
ALTER TABLE llm_provider_models ADD COLUMN capability_file INTEGER NOT NULL DEFAULT 0;
ALTER TABLE llm_provider_models ADD COLUMN capability_tool_call INTEGER NOT NULL DEFAULT 0;
ALTER TABLE llm_provider_models ADD COLUMN capability_cache INTEGER NOT NULL DEFAULT 0;
ALTER TABLE llm_provider_models ADD COLUMN capability_thinking INTEGER NOT NULL DEFAULT 0;
ALTER TABLE llm_provider_models ADD COLUMN capability_text_only INTEGER NOT NULL DEFAULT 0;
ALTER TABLE llm_provider_models ADD COLUMN capabilities_explicit INTEGER NOT NULL DEFAULT 0;
