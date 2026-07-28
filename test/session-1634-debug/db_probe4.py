"""Probe plan_steps + outbox + child sessions."""
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
            print({k: (str(v)[:400] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("plan_steps_v2", """
SELECT id, plan_id, label, status, seq, version,
       left(description, 300) AS description,
       left(coalesce(error::text,''), 300) AS error,
       agent_keys::text AS agent_keys
FROM plan_steps_v2 WHERE task_id = %s ORDER BY seq
""", (TASK,))

q("member child sessions", """
SELECT id, agent_id, session_type, owner_type, parent_session_id, title, created_at
FROM sessions WHERE id IN ('3c730e21-c061-44d6-a56f-99fc8c48f8c9','bdeda93d-f5e4-441c-8dd1-c747cd702cca')
""")

q("sessions cols", """
SELECT column_name FROM information_schema.columns
WHERE table_name = 'sessions' ORDER BY ordinal_position
""")

q("messages in member session (pdf)", """
SELECT id, session_id, role, left(content_markdown, 200) AS content, status, error_message, created_at
FROM chat_messages WHERE session_id = '3c730e21-c061-44d6-a56f-99fc8c48f8c9' ORDER BY created_at LIMIT 10
""")
