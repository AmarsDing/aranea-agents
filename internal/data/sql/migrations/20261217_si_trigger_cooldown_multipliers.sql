-- Version 20261217: Persist D8 self-improvement trigger cooldown multipliers
-- on the system_settings singleton (JSON object keyed by trigger_source).
-- Restarts must not reset the adaptive cooldown (×2, cap 8×); empty '{}'
-- means all sources at 1×. Same raw-SQL column pattern as si_risk_* (Ent
-- generator cannot cover these extra columns).
-- '{}' default is valid JSON on both Postgres and SQLite.
ALTER TABLE system_settings ADD COLUMN si_trigger_cooldown_multipliers TEXT NOT NULL DEFAULT '{}';
