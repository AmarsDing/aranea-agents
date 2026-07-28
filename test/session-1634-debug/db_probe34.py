"""Probe 34: count sessions by parent/deleted state."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

cur.execute("""
SELECT (parent_session_id <> '') AS is_child, (deleted_at <> '') AS deleted, COUNT(*)
FROM sessions GROUP BY 1, 2 ORDER BY 1, 2
""")
for r in cur.fetchall():
    print(r)

print("\nchild sessions (parent <> '', not deleted):")
cur.execute("""
SELECT id, session_type, owner_type, agent_id, left(title, 50) AS title, created_at
FROM sessions WHERE parent_session_id <> '' AND deleted_at = ''
ORDER BY created_at DESC LIMIT 10
""")
for r in cur.fetchall():
    print({k: str(v) for k, v in r.items()})
