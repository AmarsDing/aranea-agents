import psycopg2

conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

cur.execute("SELECT id, scope_type, scope_id, entity_type, name, source_kind, metadata_json, created_at FROM memory_entities ORDER BY created_at DESC LIMIT 5")
print('entities:')
for r in cur.fetchall():
    print(' ', r)

cur.execute("SELECT count(*) FROM memory_relations")
print('relations:', cur.fetchone())

# 该 session 的 activity 角色分布
SID = '48f311e3-ab69-4823-ba35-fd63767471bd'
cur.execute("SELECT kind, count(*) FROM activities WHERE session_id=%s GROUP BY kind", (SID,))
print('activity kinds:', cur.fetchall())
conn.close()
