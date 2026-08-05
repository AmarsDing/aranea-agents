import psycopg2

SID = '48f311e3-ab69-4823-ba35-fd63767471bd'
conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

cur.execute("SELECT kind, count(*) FROM steps_v2 WHERE session_id=%s GROUP BY kind", (SID,))
print('step kinds for session:', cur.fetchall())

cur.execute("SELECT kind, left(COALESCE(content,''),50) FROM steps_v2 WHERE session_id=%s ORDER BY started_at LIMIT 15", (SID,))
print('steps:')
for r in cur.fetchall():
    print(' ', r)
conn.close()
