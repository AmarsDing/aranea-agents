-- Version 20261269: runtime-cost Wave 2 defaults
-- ReplyReminder: 20261230 added the column with DEFAULT 1; Go/ent later flipped
-- new-row default to false, but stock rows stayed on and fired an extra model
-- call after every tool (~+3.5s). Backfill to 0; keep the memory butler opt-in.
-- L3 provenance: 20261232 DEFAULT 1; turn off except __memory__ (memory-eval).
-- Idempotent: re-running the UPDATEs is a no-op once values are already 0.
UPDATE agent_runtime_settings
SET reply_reminder_enabled = 0
WHERE reply_reminder_enabled != 0
  AND agent_id NOT IN (
    SELECT id FROM agents WHERE agent_key = '__memory__'
  );

UPDATE agent_runtime_settings
SET l3_inject_provenance = 0
WHERE l3_inject_provenance != 0
  AND agent_id NOT IN (
    SELECT id FROM agents WHERE agent_key = '__memory__'
  );
