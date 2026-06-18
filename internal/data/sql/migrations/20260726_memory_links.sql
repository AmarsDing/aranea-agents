-- Version 20260726: Memory links/keywords/tags columns for A-MEM style
-- memory evolution (P3-12). Links stores related memory IDs as JSON array.
-- Keywords/Tags are LLM-generated metadata. All stored as TEXT (JSON)
-- consistent with existing memory_facts schema (bi-temporal uses TEXT).
ALTER TABLE memory_facts ADD COLUMN links TEXT NOT NULL DEFAULT '[]';
ALTER TABLE memory_facts ADD COLUMN keywords TEXT NOT NULL DEFAULT '[]';
ALTER TABLE memory_facts ADD COLUMN tags TEXT NOT NULL DEFAULT '[]';
