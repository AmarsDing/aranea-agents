-- Add source column to agents table (aligned with teams.source)
ALTER TABLE agents ADD COLUMN source TEXT NOT NULL DEFAULT 'user';
