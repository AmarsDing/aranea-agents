import psycopg2

conn = psycopg2.connect(
    host="127.0.0.1", port=5432, dbname="aranea",
    user="postgres", password="Hangshan@123",
)
cur = conn.cursor()
cur.execute("""
    SELECT id, title, owner_type, status, team_id, agent_id, user_id
    FROM sessions
    WHERE team_id = '9755a723338ec796b02c36c9' AND deleted_at = ''
    ORDER BY created_at DESC LIMIT 10
""")
for r in cur.fetchall():
    print(r)
cur.close()
conn.close()
