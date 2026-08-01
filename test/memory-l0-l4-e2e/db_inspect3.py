import psycopg2
conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()
cur.execute("SELECT id, scope_type, scope_id, workspace_id, name, entity_type, status, created_at FROM memory_entities WHERE scope_id='f2e5a24ab0756d6413d6a1a3' ORDER BY created_at DESC")
print('entities for memtest agent:')
for r in cur.fetchall():
    print(' ', r)
cur.execute("SELECT count(*) FROM memory_facts WHERE scope_id='f2e5a24ab0756d6413d6a1a3'")
print('facts for memtest agent:', cur.fetchone())
cur.execute("SELECT count(*) FROM memory_episodes WHERE agent_id='f2e5a24ab0756d6413d6a1a3'")
print('episodes for memtest agent:', cur.fetchone())
cur.execute("SELECT count(*) FROM memory_l1_tasks WHERE session_id='33461c44-fd97-44c4-b755-a98597876d66'")
print('l1 tasks for session:', cur.fetchone())
cur.execute("SELECT count(*) FROM memory_l0_assembly_snapshots WHERE session_id='33461c44-fd97-44c4-b755-a98597876d66'")
print('l0 snapshots for session:', cur.fetchone())
# 死信检查
cur.execute("SELECT count(*) FROM memory_job_deadletter")
print('dead letters total:', cur.fetchone())
conn.close()
