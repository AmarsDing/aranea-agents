-- Version 20260707: Memory facts extra column patches
ALTER TABLE memory_facts ADD COLUMN pii_types TEXT NOT NULL DEFAULT '';
ALTER TABLE memory_facts ADD COLUMN quality_score REAL NOT NULL DEFAULT 0;
