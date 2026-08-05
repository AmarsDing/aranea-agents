import psycopg2

conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

# 1. Agents with memory enabled
cur.execute("""SELECT s.agent_id, a.display_name, s.memory_enabled, s.l2_recall_enabled,
                      s.l3_enabled, s.l0_inject_l3, s.l0_inject_l1
               FROM agent_runtime_settings s
               LEFT JOIN agents a ON a.id = s.agent_id
               WHERE s.memory_enabled = true
               ORDER BY a.updated_at DESC NULLS LAST
               LIMIT 10""")
print('--- memory-enabled agents ---')
for r in cur.fetchall():
    print(r)

# 2. Memory data inventory
for tbl in ('memory_facts', 'memory_episodes', 'memory_entities'):
    try:
        cur.execute(f"SELECT count(*) FROM {tbl}")
        print(f'{tbl}: {cur.fetchone()[0]}')
    except Exception as e:
        conn.rollback()
        print(f'{tbl}: ERR {e}')

# 3. Facts per agent (top 5)
try:
    cur.execute("""SELECT agent_id, count(*), max(updated_at) FROM memory_facts
                   GROUP BY agent_id ORDER BY count(*) DESC LIMIT 5""")
    print('--- facts per agent ---')
    for r in cur.fetchall():
        print(r)
except Exception as e:
    conn.rollback()
    print('facts per agent ERR:', e)

# 4. Recent memory_recalled notice steps (backend emission evidence)
try:
    cur.execute("""SELECT id, turn_id, session_id, left(content, 120), started_at
                   FROM steps_v2 WHERE kind='notice' AND notice_type='memory_recalled'
                   ORDER BY started_at DESC LIMIT 5""")
    rows = cur.fetchall()
    print('--- memory_recalled steps (latest 5) ---')
    for r in rows:
        print(r)
    if not rows:
        print('(none)')
except Exception as e:
    conn.rollback()
    print('memory_recalled steps ERR:', e)

conn.close()
