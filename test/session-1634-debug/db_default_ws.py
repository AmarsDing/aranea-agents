"""What do the 46 default-workspace sessions look like — find 'external message' pollution."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

def q(label, sql, args=()):
    print(f"\n===== {label} =====")
    cur.execute(sql, args)
    for r in cur.fetchall():
        print({k: (str(v)[:160] if v is not None else None) for k, v in r.items()})

q("default workspace: owner/agent/type breakdown", """
SELECT owner_type, agent_id, session_type, COUNT(*) AS n
FROM sessions WHERE deleted_at = '' AND workspace_id = 'default'
GROUP BY owner_type, agent_id, session_type ORDER BY n DESC
""")

q("default workspace: all sessions list", """
SELECT id, owner_type, agent_id, session_type, title, created_at
FROM sessions WHERE deleted_at = '' AND workspace_id = 'default'
ORDER BY created_at DESC LIMIT 50
""")
