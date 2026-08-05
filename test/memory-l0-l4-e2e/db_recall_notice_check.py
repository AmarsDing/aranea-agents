import psycopg2

conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

cur.execute("""SELECT id, turn_id, session_id, notice_type, status, left(content, 120), started_at
               FROM steps_v2 WHERE kind='notice' AND notice_type='memory_recalled'
               ORDER BY started_at DESC LIMIT 10""")
rows = cur.fetchall()
print('memory_recalled notices:', len(rows))
for r in rows:
    print(r)

# Any notice at all recently?
cur.execute("""SELECT notice_type, count(*) FROM steps_v2 WHERE kind='notice'
               GROUP BY notice_type ORDER BY 2 DESC""")
print('\nall notice types:', cur.fetchall())
conn.close()
