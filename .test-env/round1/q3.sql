\echo '=== tasks_v2 cols ==='
SELECT column_name FROM information_schema.columns WHERE table_name='tasks_v2' ORDER BY ordinal_position;
\echo '=== steps_v2 cols ==='
SELECT column_name FROM information_schema.columns WHERE table_name='steps_v2' ORDER BY ordinal_position;
\echo '=== team_runs_v2 cols ==='
SELECT column_name FROM information_schema.columns WHERE table_name='team_runs_v2' ORDER BY ordinal_position;
