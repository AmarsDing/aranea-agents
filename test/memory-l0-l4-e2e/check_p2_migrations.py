import psycopg2, json, sys

conn = psycopg2.connect(host='localhost', port=5432, dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

# 1. FTS index exists?
cur.execute("SELECT indexname FROM pg_indexes WHERE tablename='memory_facts' AND indexname='idx_memory_facts_fts'")
print('fts_index:', cur.fetchall())

# 2. L2 recall migration applied?
cur.execute("SELECT version, name FROM schema_migrations WHERE version IN (20261127, 20261128)")
print('migrations:', cur.fetchall())

# 3. L2 recall enabled count vs total
cur.execute("SELECT COUNT(*) FILTER (WHERE l2_recall_enabled) AS on_cnt, COUNT(*) FROM agent_runtime_settings")
print('l2_recall on/total:', cur.fetchall())

conn.close()
