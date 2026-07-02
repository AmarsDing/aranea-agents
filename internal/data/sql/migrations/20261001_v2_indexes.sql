-- V2 entity supplementary indexes (Phase 1: LLM activity ordering redesign)
-- Ent Schema.Create() already creates table columns, primary keys, and the
-- composite indexes declared in each Schema's Indexes() method. This migration
-- adds single-column indexes for common query patterns NOT already covered by
-- the leftmost prefix of existing composite indexes.
--
-- Column name notes (critical):
--   * team_stages_v2.session_id holds spirit_session_id value (see schema comment)
--   * plan_boards_v2.session_id holds spirit_session_id value (see schema comment)
--   * plan_steps_v2 has NO step_key / spirit_session_id columns
--   * plan_steps_v2 uses plan_id (not plan_board_id) and mapped_team_stage_id (not assigned_team_id)

-- sessions_v2: Ent has (user_id, status). Add spirit_agent_id for agent-based lookup.
CREATE INDEX IF NOT EXISTS idx_sessions_v2_spirit_agent_id ON sessions_v2 (spirit_agent_id);

-- tasks_v2: Ent has (session_id, seq), (status). No supplementary indexes needed.

-- turns_v2: Ent has (task_id, seq), (spirit_session_id, seq), (parent_turn_id).
-- Add session_id for current-session queries (session_id is the member/team session, not spirit).
CREATE INDEX IF NOT EXISTS idx_turns_v2_session_id ON turns_v2 (session_id);
CREATE INDEX IF NOT EXISTS idx_turns_v2_team_stage_id ON turns_v2 (team_stage_id);

-- steps_v2: Ent has (turn_id, seq), (task_id, seq), (spirit_session_id, seq).
-- Add status for status-based filtering and kind for activity-type queries.
CREATE INDEX IF NOT EXISTS idx_steps_v2_status ON steps_v2 (status);
CREATE INDEX IF NOT EXISTS idx_steps_v2_kind ON steps_v2 (kind);

-- team_stages_v2: Ent has (task_id, seq), (session_id, seq), (dag_node_id).
-- Add team_id and status for team-based and status-based queries.
CREATE INDEX IF NOT EXISTS idx_team_stages_v2_team_id ON team_stages_v2 (team_id);
CREATE INDEX IF NOT EXISTS idx_team_stages_v2_status ON team_stages_v2 (status);

-- team_runs_v2: Ent has (team_stage_id, seq), (dag_node_id).
-- Add spirit_session_id for WebSocket filtering and status for status queries.
CREATE INDEX IF NOT EXISTS idx_team_runs_v2_spirit_session_id ON team_runs_v2 (spirit_session_id);
CREATE INDEX IF NOT EXISTS idx_team_runs_v2_status ON team_runs_v2 (status);
CREATE INDEX IF NOT EXISTS idx_team_runs_v2_task_id ON team_runs_v2 (task_id);

-- member_sessions_v2: Ent has (team_run_id, seq), (agent_key).
-- Add spirit_session_id for WebSocket filtering and status for status queries.
CREATE INDEX IF NOT EXISTS idx_member_sessions_v2_spirit_session_id ON member_sessions_v2 (spirit_session_id);
CREATE INDEX IF NOT EXISTS idx_member_sessions_v2_status ON member_sessions_v2 (status);
CREATE INDEX IF NOT EXISTS idx_member_sessions_v2_team_stage_id ON member_sessions_v2 (team_stage_id);

-- plan_boards_v2: Ent has (task_id, seq).
-- Add status for status-based filtering.
CREATE INDEX IF NOT EXISTS idx_plan_boards_v2_status ON plan_boards_v2 (status);

-- plan_steps_v2: Ent has (plan_id, seq), (task_id), (mapped_team_stage_id).
-- Add status for status-based filtering.
-- NOTE: plan_steps_v2 has NO spirit_session_id, step_key, plan_board_id, or assigned_team_id columns.
CREATE INDEX IF NOT EXISTS idx_plan_steps_v2_status ON plan_steps_v2 (status);
