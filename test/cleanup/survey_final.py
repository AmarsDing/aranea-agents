# 最终关联探测（只读）
import psycopg2
conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

def q(label, sql, limit=30):
    try:
        cur.execute(sql)
        rows = cur.fetchall()
        print(f"\n=== {label} ===")
        for r in rows[:limit]:
            print(" ", r)
        return rows
    except Exception as e:
        conn.rollback()
        print(f"\n=== {label} === ERROR: {e}")
        return []

# spirit 测试 episode
q("spirit episodes", "SELECT id, title, session_id, created_at FROM memory_episodes WHERE agent_id='agent___spirit__' ORDER BY created_at DESC")

# monitor_trace_spans 列
q("spans cols", "SELECT column_name FROM information_schema.columns WHERE table_name='monitor_trace_spans' ORDER BY ordinal_position LIMIT 8")

# agent_prompt_files
q("prompt_files memtest", "SELECT count(*) FROM agent_prompt_files WHERE agent_id='f2e5a24ab0756d6413d6a1a3'")

# plan_steps_v2 / plan_boards_v2 关联列
q("plan_steps cols", "SELECT column_name FROM information_schema.columns WHERE table_name='plan_steps_v2' ORDER BY ordinal_position LIMIT 10")
q("plan_boards cols", "SELECT column_name FROM information_schema.columns WHERE table_name='plan_boards_v2' ORDER BY ordinal_position LIMIT 10")

# tasks_v2 列（找子表键）
q("tasks_v2 cols", "SELECT column_name FROM information_schema.columns WHERE table_name='tasks_v2' ORDER BY ordinal_position LIMIT 12")

# steps_v2 按 turn 关联?
q("steps_v2 cols", "SELECT column_name FROM information_schema.columns WHERE table_name='steps_v2' ORDER BY ordinal_position LIMIT 12")

# session_runs 列
q("session_runs cols", "SELECT column_name FROM information_schema.columns WHERE table_name='session_runs' ORDER BY ordinal_position LIMIT 8")

# orchestrations 是否关联 session
q("orchestrations cols", "SELECT column_name FROM information_schema.columns WHERE table_name='orchestrations' ORDER BY ordinal_position LIMIT 10")

# dept_lead_messages memtest?
q("dept_lead_messages cols", "SELECT column_name FROM information_schema.columns WHERE table_name='dept_lead_messages' ORDER BY ordinal_position LIMIT 8")

conn.close()
