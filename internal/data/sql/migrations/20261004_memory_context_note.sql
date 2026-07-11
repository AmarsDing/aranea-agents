-- Version 20261004: Add context_note column to memory_facts for A-MEM style
-- memory evolution (Phase 6A-03). Stores LLM-generated contextual annotation
-- explaining how this fact relates to / evolved from related memories.
-- Empty string means no evolution context has been attached yet.
ALTER TABLE memory_facts ADD COLUMN context_note TEXT NOT NULL DEFAULT '';
