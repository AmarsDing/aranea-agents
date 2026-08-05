# 测试中间数据普查 v3（只读）
import psycopg2

conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

def q(label, sql, params=None, limit=50):
    try:
        cur.execute(sql, params or ())
        rows = cur.fetchall()
        print(f"\n=== {label} ===")
        for r in rows[:limit]:
            print(" ", r)
        if len(rows) > limit:
            print(f"  ...({len(rows)} rows total)")
        return rows
    except Exception as e:
        conn.rollback()
        print(f"\n=== {label} === ERROR: {e}")
        return []

# 全部表清单
q("all tables", "SELECT table_name FROM information_schema.tables WHERE table_schema='public' ORDER BY 1", limit=150)

# memtest agent 的 id
rows = q("memtest agent", "SELECT id, agent_key, display_name, created_at FROM agents WHERE agent_key LIKE '%memtest%' OR agent_key LIKE '%test%'")

# memtest 相关 sessions
q("memtest sessions", """
  SELECT s.id, s.title, s.user_id, s.message_count, s.created_at
  FROM sessions s JOIN agents a ON a.id = s.agent_id
  WHERE a.agent_key LIKE '%memtest%' OR s.user_id LIKE '%memtest%' OR s.title ILIKE '%memtest%'
  ORDER BY s.created_at DESC
""")

# L4 entities 全量
q("memory_entities all", "SELECT id, scope_type, scope_id, user_id, entity_type, name, use_count, activation, created_at FROM memory_entities ORDER BY created_at DESC", limit=60)

# L4 关系表?
q("relations table check", "SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_name LIKE '%relation%'")

# memory_facts 按 user_id/agent 分布
q("facts by user", "SELECT user_id, agent_id, count(*) FROM memory_facts GROUP BY user_id, agent_id ORDER BY 3 DESC LIMIT 30")

# episodes by session (memtest)
q("episodes memtest", """
  SELECT e.id, e.title, e.session_id, e.created_at FROM memory_episodes e
  JOIN sessions s ON s.id = e.session_id
  JOIN agents a ON a.id = s.agent_id
  WHERE a.agent_key LIKE '%memtest%' LIMIT 20
""")

# vector_embeddings 规模与 meta 分布
q("vector_embeddings total", "SELECT count(*) FROM vector_embeddings")
q("vector_embeddings meta keys", "SELECT meta->>'collection_id' AS cid, count(*) FROM vector_embeddings GROUP BY 1 ORDER BY 2 DESC LIMIT 20")

conn.close()
