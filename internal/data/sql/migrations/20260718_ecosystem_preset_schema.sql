-- ecosystem_preset_schema: add ecosystem_loaded to system_settings, add kind to teams
ALTER TABLE system_settings ADD COLUMN ecosystem_loaded TEXT NOT NULL DEFAULT '{}';
ALTER TABLE teams ADD COLUMN kind TEXT NOT NULL DEFAULT 'user';
