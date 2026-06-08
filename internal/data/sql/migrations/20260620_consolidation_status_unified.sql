-- Sprint 3B-4: Unify consolidation_status to 'consolidated'
-- All episodes should use 'consolidated' instead of 'pending' or 'done'.

-- Update existing 'pending' episodes to 'consolidated'
UPDATE memory_episodes SET consolidation_status = 'consolidated' WHERE consolidation_status = 'pending';

-- Update existing 'done' episodes to 'consolidated'
UPDATE memory_episodes SET consolidation_status = 'consolidated' WHERE consolidation_status = 'done';

-- Sprint 3B-5: Drop unused memory_l2_index_meta table
DROP TABLE IF EXISTS memory_l2_index_meta;
