import psycopg2, json

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor()

print("== memory_entities 全量 ==")
cur.execute("SELECT id, scope_type, scope_id, workspace_id, user_id, name, entity_type, status, deleted_at FROM memory_entities")
for r in cur.fetchall():
    print(r)

print("\n== memory_facts scope 分布 ==")
cur.execute("SELECT scope_type, scope_id, workspace_id, status, count(*) FROM memory_facts GROUP BY 1,2,3,4")
for r in cur.fetchall():
    print(r)

print("\n== memory_facts index_status 分布 ==")
cur.execute("SELECT index_status, count(*) FROM memory_facts GROUP BY 1")
for r in cur.fetchall():
    print(r)

print("\n== memory_l1_tasks 全量 ==")
cur.execute("SELECT count(*) FROM memory_l1_tasks")
print("l1_tasks count:", cur.fetchone())

print("\n== memory_episodes scope 分布 ==")
cur.execute("SELECT scope_type, scope_id, consolidation_status, count(*) FROM memory_episodes GROUP BY 1,2,3")
for r in cur.fetchall():
    print(r)

print("\n== memory_l0_assembly_snapshots 计数 ==")
cur.execute("SELECT count(*) FROM memory_l0_assembly_snapshots")
print(cur.fetchone())

cur.close()
conn.close()
