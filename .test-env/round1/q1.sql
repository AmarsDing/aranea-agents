SELECT dag_node_id, left(team_name,30) AS team, status, stage, members::text AS members
FROM team_stages_v2
WHERE session_id='49dd810d-87f8-4ebf-9636-8a433164e479'
ORDER BY started_at;
