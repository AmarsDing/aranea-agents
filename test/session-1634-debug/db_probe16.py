"""Probe 16: sessions around 16:34 + spirit agent session list."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

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

q("sessions cols", """
SELECT column_name FROM information_schema.columns
WHERE table_name='sessions' ORDER BY ordinal_position""")

q("all sessions created 16:00-18:00 today", """
SELECT id, title, owner_type, session_type, agent_id, team_id,
       parent_session_id, root_session_id, user_id, status, created_at
FROM sessions
WHERE created_at >= '2026-07-28T08:00:00Z' AND created_at <= '2026-07-28T10:30:00Z'
ORDER BY created_at
""")

q("spirit agent sessions (sidebar view)", """
SELECT id, title, owner_type, session_type, status, parent_session_id, created_at, updated_at
FROM sessions
WHERE agent_id = 'agent___spirit__' AND deleted_at = ''
ORDER BY updated_at DESC LIMIT 15
""")

conn.close()
