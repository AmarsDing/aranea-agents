-- P0-3 fix: split the (session_id, title, agent_id) unique index into two
-- partial unique indexes so that L1-archive episodes and consolidation
-- episodes use different conflict targets.
--
-- Background: the old non-partial unique index (session_id, title, agent_id)
-- caused two problems:
--   1. Path A changes the title (from "L1 Archive: <taskID>" to the structured
--      title extracted from the snapshot), so ON CONFLICT(session_id, title,
--      agent_id) did not match the initial episode and a duplicate row was
--      inserted.
--   2. Two different L1 tasks sharing the same title would conflict, causing
--      the second task's snapshot to overwrite the first's (data loss).
--
-- Fix:
--   - Drop the old non-partial unique index.
--   - Create a partial unique index on (session_id, l1_task_id) for L1-archive
--     episodes (l1_task_id != ''). Each L1 task maps to exactly one episode,
--     regardless of title changes during Path A/B enrichment.
--   - Create a partial unique index on (session_id, title, agent_id) for
--     consolidation episodes (l1_task_id = ''). Consolidation episodes
--     (created by the Sleep-time Agent) are still deduplicated by title.

DROP INDEX IF EXISTS idx_memory_episodes_session_title_agent;

CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_episodes_session_l1_task
  ON memory_episodes(session_id, l1_task_id)
  WHERE l1_task_id != '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_episodes_session_title_agent
  ON memory_episodes(session_id, title, agent_id)
  WHERE l1_task_id = '';
