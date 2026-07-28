"""Probe v2 entities with correct columns."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

SID = "f3511b7a-345c-410e-8596-8ab2b0913fcb"
TASK = "baba1cba-4c2e-4621-be39-d2fd18f651bc"

def q(label, sql, args=()):
    print(f"\n===== {label} =====")
    try:
        cur.execute(sql, args)
        rows = cur.fetchall()
        for r in rows:
            print({k: (str(v)[:300] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("turns_v2", "SELECT * FROM turns_v2 WHERE task_id = %s ORDER BY seq", (TASK,))
q("plan_boards_v2", "SELECT * FROM plan_boards_v2 WHERE task_id = %s", (TASK,))
q("plan_steps_v2", """
SELECT id, plan_id, task_id, label, depends_on, mapped_team_stage_id, status, started_at, completed_at, seq, version, substr(coalesce(error,''),1,200) AS error
FROM plan_steps_v2 WHERE task_id = %s ORDER BY seq
""", (TASK,))
q("graph_stages_v2", "SELECT * FROM graph_stages_v2 WHERE task_id = %s", (TASK,))
q("graph_nodes_v2", """
SELECT gn.* FROM graph_nodes_v2 gn
JOIN graph_stages_v2 gs ON gs.id = gn.graph_stage_id
WHERE gs.task_id = %s
""", (TASK,))
q("team_stages_v2", """
SELECT id, task_id, turn_id, session_id, team_id, dag_node_id, depends_on, status, stage, members, started_at, completed_at, seq, version, team_name
FROM team_stages_v2 WHERE task_id = %s ORDER BY seq
""", (TASK,))
q("team_runs_v2", """
SELECT id, team_stage_id, task_id, session_id, spirit_session_id, dag_node_id, status, started_at, completed_at, seq, version, substr(coalesce(error,''),1,200) AS error
FROM team_runs_v2 WHERE task_id = %s OR spirit_session_id = %s ORDER BY seq
""", (TASK, SID))
q("steps_v2 for task", """
SELECT id, turn_id, kind, author_agent_key, seq, status, is_final, tool_name, tool_error_code,
       substr(coalesce(content,''),1,120) AS content, started_at, completed_at
FROM steps_v2 WHERE task_id = %s ORDER BY seq LIMIT 80
""", (TASK,))
