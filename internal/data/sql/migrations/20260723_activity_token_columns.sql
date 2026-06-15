-- Add token usage columns to activities table.
-- These columns store LLM token consumption per turn (root task Activity only),
-- enabling future independence from the merged assistant ChatMessage for usage stats.
ALTER TABLE activities ADD COLUMN prompt_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE activities ADD COLUMN completion_tokens INTEGER NOT NULL DEFAULT 0;
