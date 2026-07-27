\echo '=== tasks_v2 (by chat or spirit session) ==='
SELECT id, session_id, left(user_message,30) AS msg, status, created_at, completed_at FROM tasks_v2 WHERE session_id IN ('49dd810d-87f8-4ebf-9636-8a433164e479','d78029b9-c305-4bc1-9583-ac9f743cdc60') ORDER BY created_at DESC LIMIT 10;
\echo '=== team_runs_v2 (by spirit_session_id) ==='
SELECT id, team_stage_id, dag_node_id, status, started_at, completed_at, left(coalesce(error,''),40) AS err FROM team_runs_v2 WHERE spirit_session_id='49dd810d-87f8-4ebf-9636-8a433164e479' OR session_id='49dd810d-87f8-4ebf-9636-8a433164e479' ORDER BY started_at;
\echo '=== steps_v2 (by spirit_session_id) ==='
SELECT id, kind, status, left(coalesce(tool_name,''),24) AS tool, started_at, completed_at FROM steps_v2 WHERE spirit_session_id='49dd810d-87f8-4ebf-9636-8a433164e479' ORDER BY started_at DESC LIMIT 25;
