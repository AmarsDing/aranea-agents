-- Tool invocations: streaming telemetry (23 tools.design.md §9.5)
ALTER TABLE tool_invocations ADD COLUMN streaming INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tool_invocations ADD COLUMN chunk_count INTEGER NOT NULL DEFAULT 0;
