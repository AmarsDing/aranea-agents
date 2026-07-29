"""Find recent root sessions (any hour) to locate the 16:34 session."""
import psycopg2

conn = psycopg2.connect(
    host="127.0.0.1", port=5432, dbname="aranea",
    user="postgres", password="Hangshan@123",
)
cur = conn.cursor()

print("=== latest 20 root sessions ===")
cur.execute("""
    SELECT id, left(title, 60), owner_type, created_at
    FROM sessions
    WHERE deleted_at = '' AND parent_session_id = ''
    ORDER BY created_at DESC LIMIT 20
""")
for r in cur.fetchall():
    print(r)

cur.close()
conn.close()
