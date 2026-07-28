"""Probe 40: all member sessions + team stages for the 16:34 task."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

def show(label, q, args=()):
    print(f"=== {label} ===")
    cur.execute(q, args)
    rows = cur.fetchall()
    if not rows:
        print("(empty)")
    for r in rows:
        print({k: (str(v)[:200] if v is not None else None) for k, v in r.items()})
    print()

show("member_sessions_v2 for spirit session f3511b7a",
     """SELECT id, agent_key, status, task_id, team_run_id, started_at, finished_at, version
        FROM member_sessions_v2 WHERE spirit_session_id = 'f3511b7a-345c-410e-8596-8ab2b0913fcb'
        ORDER BY started_at""")

show("ALL member_sessions_v2 stuck non-terminal",
     """SELECT id, agent_key, status, spirit_session_id, started_at
        FROM member_sessions_v2 WHERE status NOT IN ('completed','failed','cancelled','interrupted')
        ORDER BY started_at DESC LIMIT 20""")

show("team stages for task baba1cba",
     """SELECT id, dag_node_id, status, left(team_name,50) AS team_name, version
        FROM team_stages_v2 WHERE task_id = 'baba1cba-4c2e-4621-be39-d2fd18f651bc'""")

show("plan steps for task baba1cba",
     """SELECT ps.id, ps.status, left(ps.label,50) AS label, ps.version
        FROM plan_steps_v2 ps WHERE ps.plan_board_id IN (
          SELECT id FROM plan_boards_v2 WHERE task_id = 'baba1cba-4c2e-4621-be39-d2fd18f651bc')""")
