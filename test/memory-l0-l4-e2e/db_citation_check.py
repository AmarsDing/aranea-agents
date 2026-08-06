import psycopg2
conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()
cur.execute("SELECT fact_id, turn_id, created_at FROM memory_fact_citations ORDER BY created_at DESC LIMIT 10")
print('recent citations:')
for r in cur.fetchall(): print(' ', r)
cur.execute("SELECT COUNT(*) FROM memory_fact_citations")
print('total citations:', cur.fetchone())
# any fact with cited_count > 0?
cur.execute("SELECT id, cited_count FROM memory_facts WHERE cited_count > 0 LIMIT 5")
print('facts with cited_count>0:', cur.fetchall())
# any fact with injected_count > 0 at all?
cur.execute("SELECT id, injected_count FROM memory_facts WHERE injected_count > 0 LIMIT 5")
print('facts with injected_count>0:', cur.fetchall())
conn.close()
