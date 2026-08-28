-- Version 20261269: runtime-cost Wave 2 defaults
-- ReplyReminder: 20261230 added the column with DEFAULT 1; Go/ent later flipped
-- new-row default to false, but stock rows stayed on and fired an extra model
-- call after every tool (~+3.5s). Backfill to false; keep the memory butler opt-in.
-- L3 provenance: 20261232 DEFAULT 1; turn off except __memory__ (memory-eval).
-- Boolean columns: use TRUE/FALSE (Postgres bool cannot compare to integer 0/1;
-- SQLite stores bool as 0/1 and accepts TRUE/FALSE). Idempotent UPDATE.
UPDATE agent_runtime_settings
SET reply_reminder_enabled = FALSE
WHERE reply_reminder_enabled IS TRUE
  AND agent_id NOT IN (
    SELECT id FROM agents WHERE agent_key = '__memory__'
  );

UPDATE agent_runtime_settings
SET l3_inject_provenance = FALSE
WHERE l3_inject_provenance IS TRUE
  AND agent_id NOT IN (
    SELECT id FROM agents WHERE agent_key = '__memory__'
  );
