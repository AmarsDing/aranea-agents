"""Check what the triggered turn created: tasks, turns, team stages, member sessions."""
import psycopg2

conn = psycopg2.connect(
    host="127.0.0.1", port=5432, dbname="aranea",
    user="postgres", password="Hangshan@123",
)
cur = conn.cursor()
SID = "02e2a664-bb47-4540-9e07-6b8c8b71cec0"

print("=== tasks_v2 for session (last 5) ===")
cur.execute("""
    SELECT id, status, version, created_at FROM tasks_v2
    WHERE session_id = %s ORDER BY created_at DESC LIMIT 5
""", (SID,))
for r in cur.fetchall():
    print(r)

print("\n=== turns_v2 for session tasks (last 5) ===")
cur.execute("""
    SELECT t.id, t.task_id, t.status, t.created_at FROM turns_v2 t
    JOIN tasks_v2 k ON k.id = t.task_id
    WHERE k.session_id = %s ORDER BY t.created_at DESC LIMIT 5
""", (SID,))
for r in cur.fetchall():
    print(r)

print("\n=== team_stages_v2 for session tasks (last 5) ===")
cur.execute("""
    SELECT ts.id, ts.team_id, ts.status, ts.created_at FROM team_stages_v2 ts
    JOIN tasks_v2 k ON k.id = ts.task_id
    WHERE k.session_id = %s ORDER BY ts.created_at DESC LIMIT 5
""", (SID,))
for r in cur.fetchall():
    print(r)

print("\n=== member_sessions_v2 newest 5 (any session) ===")
cur.execute("""
    SELECT id, agent_key, status, version, session_id, team_run_id, started_at
    FROM member_sessions_v2 ORDER BY started_at DESC LIMIT 5
""")
for r in cur.fetchall():
    print(r)

print("\n=== messages for session (last 3) ===")
cur.execute("""
    SELECT id, role, left(content_markdown, 80), created_at FROM messages
    WHERE session_id = %s ORDER BY created_at DESC LIMIT 3
""", (SID,))
for r in cur.fetchall():
    print(r)

cur.close()
conn.close()
