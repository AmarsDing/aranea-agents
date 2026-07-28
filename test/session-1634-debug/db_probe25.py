"""Probe 25: 16:34 session task failure detail — tasks_v2, team definitions, member messages."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)
SID = "f3511b7a-345c-410e-8596-8ab2b0913fcb"

def q(label, sql, args=()):
    print(f"\n===== {label} =====")
    try:
        cur.execute(sql, args)
        rows = cur.fetchall()
        for r in rows:
            print({k: (str(v)[:600] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("1. tasks_v2 of session", """
SELECT id, title, left(description, 400) AS description, status, error, created_at, completed_at
FROM tasks_v2 WHERE session_id = %s ORDER BY created_at
""", (SID,))

q("2. team_stages_v2 of session", """
SELECT id, team_id, task_id, status, stage, left(error, 300) AS error, version, started_at, finished_at
FROM team_stages_v2 WHERE session_id = %s ORDER BY started_at
""", (SID,))

q("3. teams created under session (definition members)", """
SELECT id, display_name, status, left(definition_json, 700) AS definition_json,
       left(task_description, 400) AS task_description, created_at
FROM teams
WHERE spirit_session_id = %s ORDER BY created_at
""", (SID,))

q("4. plan board steps (planner output)", """
SELECT pb.id AS board_id, ps.step_index, ps.title, left(ps.description, 500) AS description,
       ps.status, ps.dag_node_id
FROM plan_boards_v2 pb JOIN plan_steps_v2 ps ON ps.board_id = pb.id
WHERE pb.session_id = %s ORDER BY ps.step_index
""", (SID,))
