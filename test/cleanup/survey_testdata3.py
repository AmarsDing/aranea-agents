# 测试中间数据普查 v4（只读；LIKE 中 % 已转义）
import psycopg2

conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

def q(label, sql, limit=50):
    try:
        cur.execute(sql)
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

# memtest agent
q("memtest/test agents", "SELECT id, agent_key, display_name, created_at FROM agents WHERE agent_key LIKE '%%memtest%%' OR agent_key LIKE '%%test%%' OR agent_key LIKE '%%e2e%%'")

# f2e5a24a... 是哪个 agent
q("agent f2e5a24a", "SELECT id, agent_key, display_name, created_at FROM agents WHERE id = 'f2e5a24ab0756d6413d6a1a3'")

# memtest sessions（按 agent_key）
q("memtest sessions by agent", """
  SELECT s.id, s.title, s.user_id, s.message_count, s.created_at
  FROM sessions s JOIN agents a ON a.id = s.agent_id
  WHERE a.agent_key LIKE '%%memtest%%' ORDER BY s.created_at DESC
""")

# user_id 含 memtest 的 sessions
q("sessions user memtest", "SELECT id, title, user_id, agent_id, created_at FROM sessions WHERE user_id LIKE '%%memtest%%' ORDER BY created_at DESC LIMIT 20")

# 其余记忆表
for t in ['memory_relations','memory_items','memory_profile_cards','memory_l0_assembly_snapshots','memory_l1_tasks','memory_l1_fields','agent_memory_1536','memory_fact_index','memory_entity_facts','entity_reinforcements','memory_action_log']:
    q(f"count {t}", f"SELECT count(*) FROM {t}")

# memtest sessions 的关联数据量
q("memtest session ids", "SELECT s.id FROM sessions s JOIN agents a ON a.id=s.agent_id WHERE a.agent_key LIKE '%%memtest%%'")

q("session_turns memtest", """
  SELECT count(*) FROM session_turns t JOIN sessions s ON s.id=t.session_id JOIN agents a ON a.id=s.agent_id WHERE a.agent_key LIKE '%%memtest%%'
""")
q("steps_v2 memtest", """
  SELECT count(*) FROM steps_v2 st JOIN sessions s ON s.id=st.session_id JOIN agents a ON a.id=s.agent_id WHERE a.agent_key LIKE '%%memtest%%'
""")
q("tasks_v2 memtest", """
  SELECT count(*) FROM tasks_v2 t JOIN sessions s ON s.id=t.session_id JOIN agents a ON a.id=s.agent_id WHERE a.agent_key LIKE '%%memtest%%'
""")
q("activities memtest", """
  SELECT count(*) FROM activities ac JOIN sessions s ON s.id=ac.session_id JOIN agents a ON a.id=s.agent_id WHERE a.agent_key LIKE '%%memtest%%'
""")

# activities 总量
q("activities total", "SELECT count(*) FROM activities")

# knowledge chunks/links/entities 按 collection
q("knowledge_chunks by collection", """
  SELECT d.collection_id, count(*) FROM knowledge_chunks c JOIN knowledge_documents d ON d.id=c.document_id GROUP BY 1
""")
q("knowledge_links by collection", "SELECT collection_id, count(*) FROM knowledge_links GROUP BY 1")
q("knowledge_entities", "SELECT count(*) FROM knowledge_entities")
q("knowledge_doc_entities", "SELECT count(*) FROM knowledge_doc_entities")

# facts ('','') 94 条是什么（来源）
q("facts no-user no-agent sample", "SELECT id, statement, source_kind, fact_kind, created_at FROM memory_facts WHERE user_id='' AND agent_id='' ORDER BY created_at DESC LIMIT 15")

# f2e5 facts sample
q("facts f2e5 sample", "SELECT id, statement, source_kind, created_at FROM memory_facts WHERE agent_id='f2e5a24ab0756d6413d6a1a3' ORDER BY created_at DESC LIMIT 15")

# spirit 的吃日料实体上下文（是否测试产生）
q("spirit pref entity", "SELECT id, name, entity_type, source_kind, created_at FROM memory_entities WHERE scope_id='agent___spirit__' AND entity_type='preference'")

conn.close()
