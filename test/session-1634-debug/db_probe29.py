"""Probe 29: graph/team state + outbox backlog for the 16:34 session."""
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
            print({k: (str(v)[:300] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("1. team_stages_v2 for 16:34 session", """
SELECT id, team_id, team_name, status, error_message, started_at, finished_at, version
FROM team_stages_v2 WHERE spirit_session_id = %s ORDER BY started_at
""", (SID,))

q("2. team_runs_v2 for 16:34 session", """
SELECT id, team_stage_id, agent_key, status, error_message, started_at, finished_at
FROM team_runs_v2 WHERE team_stage_id IN (
  SELECT id FROM team_stages_v2 WHERE spirit_session_id = %s
) ORDER BY started_at
""", (SID,))

q("3. tasks_v2 for 16:34 session", """
SELECT id, title, status, left(COALESCE(error_message,''),200) AS err, created_at
FROM tasks_v2 WHERE session_id = %s ORDER BY created_at
""", (SID,))

q("4. outbox unpublished backlog (all)", """
SELECT id, event_type, aggregate_id, retry_count, created_at, published_at
FROM event_delivery_outbox
WHERE published_at IS NULL
ORDER BY created_at DESC LIMIT 20
""")

q("5. outbox recent published (verify mark published works now)", """
SELECT id, event_type, aggregate_id, retry_count, created_at, published_at
FROM event_delivery_outbox
ORDER BY created_at DESC LIMIT 15
""")
