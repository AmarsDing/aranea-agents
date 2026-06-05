-- Version 20260709: Add embedding_ref column to memory_facts
-- This column stores a reference to the vector in the VectorStore,
-- enabling the strategy pattern for vector operations.
ALTER TABLE memory_facts ADD COLUMN embedding_ref TEXT DEFAULT '';
