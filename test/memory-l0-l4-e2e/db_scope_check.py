import psycopg2

AID = 'f2e5a24ab0756d6413d6a1a3'
conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

cur.execute("SELECT scope_type, scope_id, workspace_id, user_id, agent_id, fact_kind, statement, source_kind, created_at FROM memory_facts WHERE agent_id=%s ORDER BY created_at DESC", (AID,))
print('facts rows:')
for r in cur.fetchall():
    print(' ', r)

print()
cur.execute("SELECT scope_type, scope_id, workspace_id, name, entity_type, status, created_at FROM memory_entities ORDER BY created_at DESC LIMIT 10")
print('latest entities (any scope):')
for r in cur.fetchall():
    print(' ', r)

conn.close()
