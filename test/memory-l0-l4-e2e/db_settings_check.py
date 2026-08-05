import psycopg2

AID = 'f2e5a24ab0756d6413d6a1a3'
conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

cur.execute("""SELECT memory_enabled, l1_enabled, l2_episode_enabled, l2_recall_enabled,
                      l3_enabled, l4_enabled,
                      l0_inject_l1, l0_inject_l3, l0_inject_l4
               FROM agent_runtime_settings WHERE agent_id=%s""", (AID,))
row = cur.fetchone()
print('settings row:', row)

cur.execute("SELECT count(*) FROM memory_entities WHERE scope_id=%s", (AID,))
print('entities for agent:', cur.fetchone())
cur.execute("SELECT count(*) FROM memory_entities")
print('entities total:', cur.fetchone())
conn.close()
