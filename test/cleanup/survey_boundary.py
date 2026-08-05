# 边界项明细 + 关联表 FK 探测（只读）
import psycopg2

conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

def q(label, sql, limit=40):
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

# spirit 的 facts 明细（判断哪些是测试产生）
q("spirit facts", "SELECT id, statement, source_kind, user_id, created_at FROM memory_facts WHERE agent_id='agent___spirit__' ORDER BY created_at DESC LIMIT 15")

# skills 的 facts
q("skills facts", "SELECT id, statement, source_kind, created_at FROM memory_facts WHERE agent_id='agent___skills__' LIMIT 10")

# f2e5 facts user_id 分布确认
q("f2e5 facts by user", "SELECT user_id, count(*) FROM memory_facts WHERE agent_id='f2e5a24ab0756d6413d6a1a3' GROUP BY 1")

# sushi fact 完整行
q("sushi fact", "SELECT id, agent_id, user_id, statement, source_kind, source_episode_id, created_at FROM memory_facts WHERE statement ILIKE '%%sushi%%'")

# 引用 memory_facts 的关联表行数（fact_id 外键）
for t, c in [('memory_fact_versions','fact_id'), ('memory_fact_feedback','fact_id'), ('memory_fact_conflicts','fact_id'),
             ('memory_fact_citations','fact_id'), ('memory_entity_facts','fact_id'), ('memory_fact_index','fact_id')]:
    q(f"{t} count", f"SELECT count(*) FROM {t}")

# memory_fact_conflicts 列
q("conflicts cols", "SELECT column_name FROM information_schema.columns WHERE table_name='memory_fact_conflicts' ORDER BY ordinal_position")

# 引用 memory_entities 的表
q("entity_versions count", "SELECT count(*) FROM memory_entity_versions")

# memtest sessions 的关联表计数
sids = "('48f311e3-ab69-4823-ba35-fd63767471bd','97b36ce6-a581-42e9-a664-b65e6bc30ccd','f5c0d524-e719-478a-adb2-2b366acf4741','e16fe63c-26b3-4140-b6e8-a48f65d01924','685dfbcb-f7e0-40f7-9792-003bfe2405ca','33461c44-fd97-44c4-b755-a98597876d66')"
for t in ['session_run_checkpoints','session_runtime','session_metrics','session_summaries','member_sessions_v2','session_participants','task_plans','activities','session_runs','flow_log_events','monitor_traces']:
    q(f"{t} memtest", f"SELECT count(*) FROM {t} WHERE session_id IN {sids}")

# l1 fields/history 列（找 task 关联列）
q("l1_fields cols", "SELECT column_name FROM information_schema.columns WHERE table_name='memory_l1_fields' ORDER BY ordinal_position")
q("l1_field_history cols", "SELECT column_name FROM information_schema.columns WHERE table_name='memory_l1_field_history' ORDER BY ordinal_position")

# knowledge_links 列
q("links cols", "SELECT column_name FROM information_schema.columns WHERE table_name='knowledge_links' ORDER BY ordinal_position LIMIT 12")

# memory_action_log 列（确认可全清）
q("action_log cols", "SELECT column_name FROM information_schema.columns WHERE table_name='memory_action_log' ORDER BY ordinal_position LIMIT 15")

conn.close()
