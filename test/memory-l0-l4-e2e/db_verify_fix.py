import psycopg2

AID = 'f2e5a24ab0756d6413d6a1a3'
SID = '97b36ce6-a581-42e9-a664-b65e6bc30ccd'
conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

cur.execute("SELECT count(*) FROM memory_facts WHERE agent_id=%s", (AID,))
print('facts for agent:', cur.fetchone())

cur.execute("SELECT id, statement, source_kind, source_session_id, created_at FROM memory_facts WHERE agent_id=%s ORDER BY created_at DESC LIMIT 6", (AID,))
for r in cur.fetchall():
    print(' ', r)

cur.execute("SELECT id, session_id, title, consolidation_status, consolidated_l3_count FROM memory_episodes WHERE session_id=%s", (SID,))
print('episodes for new session:', cur.fetchall())

cur.execute("SELECT id, entity_type, name, scope_id, created_at FROM memory_entities WHERE scope_id=%s", (AID,))
print('entities:')
for r in cur.fetchall():
    print(' ', r)

cur.execute("SELECT count(*) FROM memory_relations WHERE scope_id=%s", (AID,))
print('relations:', cur.fetchone())
conn.close()
