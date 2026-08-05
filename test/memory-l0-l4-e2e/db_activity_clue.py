import psycopg2

conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

cur.execute("SELECT column_name FROM information_schema.columns WHERE table_name='activities' ORDER BY ordinal_position")
print('columns:', [r[0] for r in cur.fetchall()])

cur.execute("SELECT session_id, kind, left(COALESCE(content,''),30) FROM activities ORDER BY timestamp DESC LIMIT 8")
print('latest activities:')
for r in cur.fetchall():
    print(' ', r)

cur.execute("SELECT count(*) FROM activities WHERE session_id='48f311e3-ab69-4823-ba35-fd63767471bd'")
print('for test session:', cur.fetchone())
conn.close()
