-- 20261107_drop_micro_compact: drop micro_compact_enabled from agent_runtime_settings.
-- L1 MicroCompact was retired on 2026-07-20 (loadCompressBody only keeps user/assistant
-- messages, so the tool-message filtering logic never triggered — dead feature).
-- Idempotent: "no such column" errors are treated as success by the migration runner.
ALTER TABLE agent_runtime_settings DROP COLUMN micro_compact_enabled;
