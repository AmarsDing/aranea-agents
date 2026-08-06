import psycopg2
conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()
cur.execute("""SELECT session_id, left(title,60), left(outcome_summary,80)
               FROM memory_episodes ORDER BY created_at DESC LIMIT 8""")
for r in cur.fetchall():
    print(r)
conn.close()
