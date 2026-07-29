"""Workspace split for the 16:34 session tree."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

def q(label, sql, args=()):
    print(f"\n===== {label} =====")
    cur.execute(sql, args)
    for r in cur.fetchall():
        print({k: (str(v)[:200] if v is not None else None) for k, v in r.items()})

q("workspace_id distribution (alive sessions)", """
SELECT workspace_id, COUNT(*) AS total,
  COUNT(*) FILTER (WHERE parent_session_id = '') AS roots,
  COUNT(*) FILTER (WHERE parent_session_id <> '') AS children
FROM sessions WHERE deleted_at = '' GROUP BY workspace_id ORDER BY total DESC
""")

q("16:34 session + direct children workspace", """
SELECT id, workspace_id, session_type, member_agent_key, parent_session_id
FROM sessions
WHERE id = %s OR parent_session_id = %s
""", ("f3511b7a-345c-410e-8596-8ab2b0913fcb", "f3511b7a-345c-410e-8596-8ab2b0913fcb"))

q("spirit agent sessions: total vs roots (alive, any workspace)", """
SELECT COUNT(*) AS total,
  COUNT(*) FILTER (WHERE parent_session_id = '') AS roots
FROM sessions WHERE deleted_at = '' AND agent_id = 'agent___spirit__'
""")
