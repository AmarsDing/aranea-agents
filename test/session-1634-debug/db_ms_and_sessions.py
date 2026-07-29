"""Verify member_sessions_v2 task_id fill + session root_only filter via API."""
import psycopg2

conn = psycopg2.connect(
    host="127.0.0.1", port=5432, dbname="aranea",
    user="postgres", password="Hangshan@123",
)
cur = conn.cursor()

print("=== member_sessions_v2: task_id fill state (last 15) ===")
cur.execute("""
    SELECT id, agent_key, task_id <> '' AS has_task, status, started_at
    FROM member_sessions_v2 ORDER BY started_at DESC LIMIT 15
""")
for r in cur.fetchall():
    print(r)

print("\n=== sessions: root vs child ===")
cur.execute("""
    SELECT count(*) FILTER (WHERE parent_session_id = ''),
           count(*) FILTER (WHERE parent_session_id <> '')
    FROM sessions WHERE deleted_at = ''
""")
root, child = cur.fetchone()
print(f"root={root} child={child}")

print("\n=== child sessions sample (should be excluded by root_only) ===")
cur.execute("""
    SELECT id, title, owner_type, parent_session_id, created_at
    FROM sessions
    WHERE deleted_at = '' AND parent_session_id <> ''
    ORDER BY created_at DESC LIMIT 5
""")
for r in cur.fetchall():
    print(r)

cur.close()
conn.close()
