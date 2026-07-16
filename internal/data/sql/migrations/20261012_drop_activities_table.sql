-- Version 20261012: Drop activities table (v1 Activity-First persistence).
-- Chat truth is v2 steps_v2 / tasks_v2 / turns_v2. ListActivities RPC already
-- reads steps_v2 via StepToActivity adapter. ActivityRepo was unwired and
-- Create/Update/UpsertActivity had zero production callers (2026-07-16).
--
-- Idempotent: DROP IF EXISTS.

DROP INDEX IF EXISTS idx_activities_session_turn;
DROP INDEX IF EXISTS idx_activities_parent;
DROP INDEX IF EXISTS idx_activities_spirit_session;
DROP INDEX IF EXISTS idx_activities_team;

DROP TABLE IF EXISTS activities;
