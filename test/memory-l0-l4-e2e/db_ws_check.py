import psycopg2

AID = 'f2e5a24ab0756d6413d6a1a3'
conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()
cur.execute("SELECT workspace_id, source_kind, count(*) FROM memory_facts WHERE agent_id=%s GROUP BY workspace_id, source_kind", (AID,))
print('facts by workspace/source:', cur.fetchall())
conn.close()
