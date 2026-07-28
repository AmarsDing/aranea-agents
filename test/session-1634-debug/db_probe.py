"""Full probe for the 16:34 spirit session install-probe (f3511b7a)."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(
    host="127.0.0.1", port=5432, dbname="aranea",
    user="postgres", password="Hangshan@123",
)
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

SID = "f3511b7a-345c-410e-8596-8ab2b0913fcb"
TASK = "baba1cba-4c2e-4621-be39-d2fd18f651bc"

def q(label, sql, args=()):
    print(f"\n===== {label} =====")
    try:
        cur.execute(sql, args)
        rows = cur.fetchall()
        for r in rows:
            print({k: (str(v)[:260] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("tasks_v2 for session", """
SELECT id, session_id, status, title, error_message, version, created_at, updated_at
FROM tasks_v2 WHERE session_id = %s ORDER BY created_at
""", (SID,))

q("task by id baba1cba", """
SELECT id, session_id, status, title, error_message, version, created_at, updated_at
FROM tasks_v2 WHERE id = %s
""", (TASK,))

q("turns_v2 for task", """
SELECT id, task_id, session_id, status, turn_index, role, created_at
FROM turns_v2 WHERE task_id = %s ORDER BY turn_index
""", (TASK,))

q("plan_boards_v2 for task", """
SELECT id, task_id, session_id, status, version, created_at
FROM plan_boards_v2 WHERE task_id = %s
""", (TASK,))

q("plan_steps_v2 for task", """
SELECT id, plan_board_id, task_id, status, title, depends_on, created_at
FROM plan_steps_v2 WHERE task_id = %s ORDER BY created_at
""", (TASK,))

q("graph_stages_v2 for task", """
SELECT id, task_id, session_id, plan_board_id, status, error_message, version, started_at, finished_at
FROM graph_stages_v2 WHERE task_id = %s
""", (TASK,))

q("graph_nodes_v2 for task", """
SELECT id, graph_stage_id, dag_node_id, name, status, team_id, team_stage_id, error_message, started_at, finished_at
FROM graph_nodes_v2 WHERE graph_stage_id IN (SELECT id FROM graph_stages_v2 WHERE task_id = %s)
ORDER BY created_at
""", (TASK,))

q("team_stages_v2 for task", """
SELECT id, task_id, session_id, team_id, status, stage, error_message, version, started_at, finished_at
FROM team_stages_v2 WHERE task_id = %s
""", (TASK,))

q("team_runs_v2 for task", """
SELECT id, team_stage_id, task_id, status, error_message, version, started_at, finished_at
FROM team_runs_v2 WHERE task_id = %s
""", (TASK,))

q("member_sessions_v2 for task", """
SELECT id, team_run_id, team_stage_id, task_id, session_id, spirit_session_id, agent_key, agent_name, status, started_at, finished_at
FROM member_sessions_v2 WHERE task_id = %s OR spirit_session_id = %s
ORDER BY started_at
""", (TASK, SID))

q("member_sessions_v2 all cols sample", """
SELECT * FROM member_sessions_v2 WHERE spirit_session_id = %s LIMIT 1
""", (SID,))

q("steps_v2 for task", """
SELECT id, task_id, session_id, turn_id, kind, status, title, seq, created_at
FROM steps_v2 WHERE task_id = %s ORDER BY seq LIMIT 50
""", (TASK,))

q("steps_v2 for session (latest 30)", """
SELECT id, task_id, session_id, kind, status, title, seq, created_at
FROM steps_v2 WHERE session_id = %s ORDER BY seq DESC LIMIT 30
""", (SID,))
