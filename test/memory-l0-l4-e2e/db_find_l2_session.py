import psycopg2
conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()
cur.execute("""SELECT s.id, s.title, COUNT(e.id) AS eps
               FROM sessions s LEFT JOIN memory_episodes e ON e.session_id = s.id
               WHERE s.agent_id = 'agent___spirit__'
               GROUP BY s.id, s.title HAVING COUNT(e.id) > 0
               ORDER BY COUNT(e.id) DESC LIMIT 5""")
for r in cur.fetchall():
    print(r)
conn.close()
