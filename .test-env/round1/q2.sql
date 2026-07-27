\echo '=== tasks_v2 ==='
SELECT id, left(title,30) AS title, status, created_at, completed_at FROM tasks_v2 WHERE session_id='49dd810d-87f8-4ebf-9636-8a433164e479' ORDER BY created_at;
\echo '=== team_runs_v2 ==='
SELECT id, team_stage_id, status, started_at, completed_at FROM team_runs_v2 WHERE session_id='49dd810d-87f8-4ebf-9636-8a433164e479' ORDER BY started_at;
\echo '=== steps_v2 ==='
SELECT id, left(name,26) AS name, status, dag_node_id FROM steps_v2 WHERE session_id='49dd810d-87f8-4ebf-9636-8a433164e479' ORDER BY created_at LIMIT 20;
