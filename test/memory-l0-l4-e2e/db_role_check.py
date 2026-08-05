import psycopg2

SID = '48f311e3-ab69-4823-ba35-fd63767471bd'
conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

cur.execute("SELECT DISTINCT role FROM session_messages WHERE session_id=%s", (SID,))
print('distinct roles:', cur.fetchall())

cur.execute("SELECT role, left(content_markdown, 40) FROM session_messages WHERE session_id=%s ORDER BY created_at LIMIT 12", (SID,))
for r in cur.fetchall():
    print(' ', r)
conn.close()
