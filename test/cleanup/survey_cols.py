# 测试中间数据普查 v2（只读）
import psycopg2

conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

def q(label, sql, params=None, limit=40):
    try:
        cur.execute(sql, params or ())
        rows = cur.fetchall()
        print(f"\n=== {label} ===")
        for r in rows[:limit]:
            print(" ", r)
        if len(rows) > limit:
            print(f"  ...({len(rows)} rows total)")
    except Exception as e:
        conn.rollback()
        print(f"\n=== {label} === ERROR: {e}")

def cols(table):
    cur.execute("SELECT column_name FROM information_schema.columns WHERE table_name=%s ORDER BY ordinal_position", (table,))
    return [r[0] for r in cur.fetchall()]

for t in ['agents', 'memory_entities', 'memory_facts', 'memory_episodes', 'vector_embeddings', 'knowledge_documents', 'sessions']:
    try:
        print(f"\n--- columns: {t} ---")
        print(", ".join(cols(t)))
    except Exception as e:
        conn.rollback()
        print(f"{t}: ERROR {e}")
conn.close()
