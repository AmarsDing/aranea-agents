import psycopg2

conn = psycopg2.connect(
    host="127.0.0.1", port=5432, dbname="aranea",
    user="postgres", password="Hangshan@123",
)
cur = conn.cursor()

print("=== messages for session 02e2a664 in last 3 hours ===")
cur.execute("""
    SELECT id, role, status, left(content_markdown, 80), created_at
    FROM messages
    WHERE session_id = '02e2a664-bb47-4540-9e07-6b8c8b71cec0'
      AND created_at >= now() - interval '3 hours'
    ORDER BY created_at DESC LIMIT 10
""")
for r in cur.fetchall():
    print(r)

print("\n=== pending queue for session ===")
cur.execute("""
    SELECT id, session_id, status, left(content, 60), created_at
    FROM chat_pending_messages
    WHERE session_id = '02e2a664-bb47-4540-9e07-6b8c8b71cec0'
    ORDER BY created_at DESC LIMIT 5
""")
try:
    for r in cur.fetchall():
        print(r)
except Exception as e:
    print("ERR", e)

print("\n=== session status now ===")
cur.execute("""
    SELECT id, status, owner_type, agent_id, team_id FROM sessions
    WHERE id = '02e2a664-bb47-4540-9e07-6b8c8b71cec0'
""")
for r in cur.fetchall():
    print(r)

cur.close()
conn.close()
