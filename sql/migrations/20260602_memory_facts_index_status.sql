-- Version 20260602: Memory facts index status column patches
ALTER TABLE memory_facts ADD COLUMN index_status TEXT NOT NULL DEFAULT 'fresh';
ALTER TABLE memory_facts ADD COLUMN index_synced_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE memory_facts ADD COLUMN index_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE memory_facts ADD COLUMN index_last_error TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_memory_facts_index_status ON memory_facts(index_status, index_synced_at);
