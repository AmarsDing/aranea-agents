import psycopg2

conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()
cur.execute("""SELECT column_name FROM information_schema.columns
               WHERE table_name='memory_relations' ORDER BY ordinal_position""")
cols = [r[0] for r in cur.fetchall()]
print('cols:', cols)
cur.execute("""SELECT a.name, r.relation_type, r.weight, r.co_activation_count, r.last_reinforced_at, b.name
               FROM memory_relations r
               JOIN memory_entities a ON a.id = r.source_id
               JOIN memory_entities b ON b.id = r.target_id
               ORDER BY r.updated_at DESC LIMIT 8""")
for r in cur.fetchall():
    print(r)
conn.close()
