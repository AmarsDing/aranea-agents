import psycopg2

conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()
cur.execute("SELECT id, name, document_count, chunk_count, sync_state, root_path FROM knowledge_collections ORDER BY created_at")
for r in cur.fetchall():
    print(r)
print('---documents---')
cur.execute("SELECT collection_id, count(*) FROM knowledge_documents GROUP BY collection_id")
for r in cur.fetchall():
    print(r)
conn.close()
