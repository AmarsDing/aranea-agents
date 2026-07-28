"""Comprehensive probe for the 16:34 session issues."""
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
            print({k: (str(v)[:200] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("outbox column types", """
SELECT column_name, data_type, udt_name FROM information_schema.columns
WHERE table_name = 'event_delivery_outbox' ORDER BY ordinal_position
""")

q("outbox rows for 16:34 session", """
SELECT seq, kind, entity_id, published_at, created_at
FROM event_delivery_outbox WHERE session_id = %s ORDER BY seq
""", (SID,))

q("outbox unpublished count overall", """
SELECT count(*) AS total,
       count(*) FILTER (WHERE published_at IS NULL) AS unpublished
FROM event_delivery_outbox
""")

q("member_sessions_v2 for 16:34 session", """
SELECT id, team_run_id, team_stage_id, session_id, spirit_session_id,
       agent_key, status, task_id, version, started_at, finished_at
FROM member_sessions_v2 WHERE spirit_session_id = %s
""", (SID,))

q("team_stages_v2 for 16:34 session", """
SELECT id, team_id, team_name, status, version, task_id, session_id, created_at, updated_at
FROM team_stages_v2 WHERE session_id = %s
""", (SID,))

q("team_runs_v2 for 16:34 session", """
SELECT id, team_stage_id, status, version, started_at, finished_at
FROM team_runs_v2 WHERE team_stage_id IN
  (SELECT id FROM team_stages_v2 WHERE session_id = %s)
""", (SID,))

q("graph_stages_v2 for 16:34 session", """
SELECT id, plan_board_id, session_id, status, version, created_at, updated_at
FROM graph_stages_v2 WHERE session_id = %s
""", (SID,))

q("graph_nodes_v2 for 16:34 session", """
SELECT id, graph_stage_id, label, status, team_stage_id, version, updated_at
FROM graph_nodes_v2 WHERE graph_stage_id IN
  (SELECT id FROM graph_stages_v2 WHERE session_id = %s)
""", (SID,))

q("tasks_v2 for 16:34 session", """
SELECT id, session_id, parent_task_id, kind, status, title, created_at
FROM tasks_v2 WHERE session_id = %s ORDER BY created_at
""", (SID,))

q("plan_boards_v2 for 16:34 session", """
SELECT id, session_id, task_id, status, version, created_at
FROM plan_boards_v2 WHERE session_id = %s
""", (SID,))

q("plan_steps_v2 for 16:34 session", """
SELECT id, plan_board_id, label, status, team_id, agent_keys, created_at
FROM plan_steps_v2 WHERE plan_board_id IN
  (SELECT id FROM plan_boards_v2 WHERE session_id = %s)
""", (SID,))
