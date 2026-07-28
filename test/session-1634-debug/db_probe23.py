"""Probe 23: what sessions would appear in the sidebar list — find 'external' leaks."""
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
            print({k: (str(v)[:220] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("0. sessions columns", """
SELECT column_name FROM information_schema.columns
WHERE table_name='sessions' ORDER BY ordinal_position
""")

# What does the sidebar query look like: owner_type='agent', ordered by updated_at
q("1. sessions owner_type='agent' (sidebar view, latest 25)", """
SELECT id, title, session_type, owner_type, agent_id,
       parent_session_id, root_session_id, status, created_at, updated_at
FROM sessions
WHERE owner_type = 'agent'
ORDER BY updated_at DESC LIMIT 25
""")

q("2. distinct (owner_type, session_type) counts last 24h", """
SELECT owner_type, session_type, count(*)
FROM sessions
WHERE created_at >= now() - interval '24 hours'
GROUP BY 1,2 ORDER BY 3 DESC
""")

q("3. non-agent owner sessions updated recently (potential leaks)", """
SELECT id, title, session_type, owner_type, agent_id, team_id,
       parent_session_id, status, created_at, updated_at
FROM sessions
WHERE owner_type <> 'agent'
ORDER BY updated_at DESC LIMIT 20
""")
