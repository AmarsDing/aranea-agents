-- 20261109 steps_v2_session_seq: composite index for the paged session-history
-- query used by chat history lazy load Phase 1:
--   WHERE session_id = ? [AND seq < ?] ORDER BY seq DESC LIMIT n+1
-- Existing indexes cover (task_id, seq) / (spirit_session_id, seq) / (turn_id, seq)
-- but not (session_id, seq). Idempotent per DB-N6.
CREATE INDEX IF NOT EXISTS idx_steps_v2_session_seq ON steps_v2 (session_id, seq);
