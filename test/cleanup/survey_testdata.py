# 测试中间数据普查（只读，不删除）
import psycopg2

conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

def q(label, sql, params=None):
    try:
        cur.execute(sql, params or ())
        rows = cur.fetchall()
        print(f"\n=== {label} ===")
        for r in rows[:40]:
            print(" ", r)
        if len(rows) > 40:
            print(f"  ...({len(rows)} rows total)")
    except Exception as e:
        conn.rollback()
        print(f"\n=== {label} === ERROR: {e}")

# 1. agents 全量
q("agents", "SELECT id, agent_key, name, created_at FROM agents ORDER BY created_at DESC LIMIT 50")

# 2. sessions 按 agent 分布
q("sessions by agent", """
  SELECT a.agent_key, count(*) AS n
  FROM sessions s JOIN agents a ON a.id = s.agent_id
  GROUP BY a.agent_key ORDER BY n DESC LIMIT 30
""")

# 3. memory_entities 全量（L4）
q("memory_entities", "SELECT id, agent_id, name, type, importance, use_count, created_at FROM memory_entities ORDER BY created_at DESC LIMIT 60")

# 4. memory_facts 分布
q("memory_facts by agent/scope", "SELECT agent_id, scope, count(*) FROM memory_facts GROUP BY agent_id, scope ORDER BY 3 DESC LIMIT 30")

# 5. episodes 分布
q("episodes by agent", "SELECT agent_id, count(*) FROM memory_episodes GROUP BY agent_id ORDER BY 2 DESC LIMIT 20")

# 6. knowledge collections
q("knowledge collections", "SELECT id, name, root_path, created_at FROM knowledge_collections ORDER BY created_at DESC LIMIT 30")

# 7. knowledge documents 分布
q("knowledge docs by collection", "SELECT collection_id, count(*) FROM knowledge_documents GROUP BY collection_id LIMIT 30")

# 8. vector_embeddings 规模
q("vector_embeddings", "SELECT count(*), count(DISTINCT collection_id) FROM vector_embeddings")

# 9. 表清单（确认记忆相关表名）
q("memory tables", "SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND (table_name LIKE '%memory%' OR table_name LIKE '%episode%' OR table_name LIKE '%fact%' OR table_name LIKE '%entit%' OR table_name LIKE '%knowledge%') ORDER BY 1")

conn.close()
