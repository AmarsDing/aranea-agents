import psycopg2

conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

cur.execute("""SELECT s.id, s.agent_id, a.agent_key, s.title FROM sessions s
               LEFT JOIN agents a ON a.id = s.agent_id
               WHERE s.id IN ('e16fe63c-26b3-4140-b6e8-a48f65d01924','97b36ce6-a581-42e9-a664-b65e6bc30ccd')""")
for r in cur.fetchall():
    print(r)
conn.close()
