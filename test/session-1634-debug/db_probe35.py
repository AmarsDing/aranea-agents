"""Probe 35: workspace distribution of sessions."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

cur.execute("""
SELECT workspace_id, (parent_session_id <> '') AS is_child, COUNT(*)
FROM sessions WHERE deleted_at = ''
GROUP BY 1, 2 ORDER BY 1, 2
""")
for r in cur.fetchall():
    print(r)

print("\n16:34 session tree workspaces:")
cur.execute("""
SELECT id, workspace_id, session_type, parent_session_id <> '' AS is_child
FROM sessions
WHERE id IN ('f3511b7a-345c-410e-8596-8ab2b0913fcb','3c730e21-c061-44d6-a56f-99fc8c48f8c9','bdeda93d-f5e4-441c-8dd1-c747cd702cca')
""")
for r in cur.fetchall():
    print({k: str(v) for k, v in r.items()})
