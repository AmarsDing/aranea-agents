# 普查 v4：canary/L0L1/action_log/chunks/cron（只读）
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

# canary facts 计数
q("canary facts count", "SELECT count(*) FROM memory_facts WHERE source_kind='memory_canary'")
q("canary facts by date", "SELECT date(created_at), count(*) FROM memory_facts WHERE source_kind='memory_canary' GROUP BY 1 ORDER BY 1")

# ('','') 组非 canary 的
q("facts ('','') non-canary", "SELECT source_kind, count(*) FROM memory_facts WHERE user_id='' AND agent_id='' AND source_kind<>'memory_canary' GROUP BY 1")

# cron 任务（canary 是否还在跑）
q("cron_task memory", "SELECT id, name, enabled, schedule FROM cron_task WHERE name ILIKE '%%memory%%' OR name ILIKE '%%canary%%' OR id ILIKE '%%memory%%' OR id ILIKE '%%canary%%'")
q("cron_task all", "SELECT id, name, enabled FROM cron_task LIMIT 40")

# memory_action_log 分布
q("action_log by action", "SELECT action, count(*) FROM memory_action_log GROUP BY 1 ORDER BY 2 DESC LIMIT 20")

# L0 快照按 session
q("L0 snapshots by session", "SELECT session_id, count(*) FROM memory_l0_assembly_snapshots GROUP BY 1 ORDER BY 2 DESC LIMIT 30")

# L1 tasks 按 session
q("L1 tasks by session", "SELECT session_id, count(*) FROM memory_l1_tasks GROUP BY 1 ORDER BY 2 DESC LIMIT 30")

# knowledge_chunks 结构
q("chunks cols", "SELECT column_name FROM information_schema.columns WHERE table_name='knowledge_chunks' ORDER BY ordinal_position")

# episodes memtest agent
q("episodes memtest agent", "SELECT id, title, session_id, created_at FROM memory_episodes WHERE agent_id='f2e5a24ab0756d6413d6a1a3' ORDER BY created_at")

# relations 详情
q("memory_relations all", "SELECT * FROM memory_relations LIMIT 10")
q("relations cols", "SELECT column_name FROM information_schema.columns WHERE table_name='memory_relations' ORDER BY ordinal_position")

# facts user_id='1' 是什么
q("facts user 1", "SELECT id, statement, source_kind, created_at FROM memory_facts WHERE user_id='1' LIMIT 10")

# sessions_v2 / member_sessions_v2 是否有 memtest
q("sessions_v2 count", "SELECT count(*) FROM sessions_v2")

# memtest agent 的 runtime settings / prompt files
q("memtest runtime_settings", "SELECT count(*) FROM agent_runtime_settings WHERE agent_id='f2e5a24ab0756d6413d6a1a3'")

conn.close()
