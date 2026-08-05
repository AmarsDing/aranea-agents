import psycopg2

conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()
cur.execute("""
    SELECT s.id, s.title, a.agent_key, s.created_at
    FROM sessions s JOIN agents a ON a.id = s.agent_id
    WHERE a.agent_key = 'memtest-agent'
    ORDER BY s.created_at DESC LIMIT 5
""")
for r in cur.fetchall():
    print(r)
conn.close()
