-- Migration: add team_run_steps.tool_call_count for TEAM-04 per-member tool call stats.
ALTER TABLE team_run_steps ADD COLUMN tool_call_count INTEGER NOT NULL DEFAULT 0;
