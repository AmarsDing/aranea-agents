"""Probe 24: session tree of the 16:34 session + spirit sessions with parents."""
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
            print({k: (str(v)[:260] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("1. full session tree under 16:34 session (root_session_id or parent)", """
SELECT id, title, session_type, owner_type, agent_id, team_id, member_agent_key,
       parent_session_id, root_session_id, status, created_at, updated_at
FROM sessions
WHERE root_session_id = %(sid)s OR parent_session_id = %(sid)s OR id = %(sid)s
ORDER BY created_at
""", {"sid": SID})

q("2. spirit-owned sessions WITH parent (leak candidates, all time)", """
SELECT id, title, session_type, owner_type, agent_id, member_agent_key,
       parent_session_id, root_session_id, status, created_at
FROM sessions
WHERE agent_id = 'agent___spirit__' AND parent_session_id <> ''
ORDER BY updated_at DESC LIMIT 15
""")

q("3. messages of the 16:34 session (what user sees in chat)", """
SELECT id, role, left(content_markdown, 120) AS content, status, created_at
FROM chat_messages
WHERE session_id = %s
ORDER BY created_at LIMIT 30
""", (SID,))
