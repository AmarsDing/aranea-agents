"""Probe 31: graph/team state + outbox backlog (correct columns)."""
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
            print({k: (str(v)[:280] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("1. team_stages_v2 (16:34)", """
SELECT id, team_id, team_name, status, stage, dag_node_id, started_at, completed_at
FROM team_stages_v2 WHERE session_id = %s ORDER BY started_at
""", (SID,))

q("2. team_runs_v2 (16:34)", """
SELECT id, team_stage_id, status, left(COALESCE(error,''),250) AS error, started_at, completed_at
FROM team_runs_v2 WHERE spirit_session_id = %s ORDER BY started_at
""", (SID,))

q("3. tasks_v2 (16:34)", """
SELECT id, status, left(user_message,120) AS msg, created_at, completed_at
FROM tasks_v2 WHERE session_id = %s ORDER BY created_at
""", (SID,))

q("4. member_sessions_v2 (16:34)", """
SELECT id, agent_key, status, task_id, left(COALESCE(error,''),200) AS error, started_at, finished_at
FROM member_sessions_v2 WHERE spirit_session_id = %s ORDER BY started_at
""", (SID,))

q("5. outbox unpublished backlog (all-time)", """
SELECT id, kind, entity_id, left(COALESCE(payload,''),150) AS payload, created_at
FROM event_delivery_outbox WHERE published_at IS NULL
ORDER BY created_at DESC LIMIT 20
""")

q("6. outbox for 16:34 session (published state)", """
SELECT id, kind, entity_id, published_at IS NOT NULL AS published, created_at
FROM event_delivery_outbox WHERE session_id = %s
ORDER BY created_at
""", (SID,))
