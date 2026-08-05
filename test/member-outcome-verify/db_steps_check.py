import psycopg2

conn = psycopg2.connect(
    host="127.0.0.1", port=5432, dbname="aranea",
    user="postgres", password="Hangshan@123",
)
cur = conn.cursor()

print("=== steps_v2 created 19:40-20:00 today ===")
cur.execute("""
    SELECT id, kind, status, task_id, turn_id, left(coalesce(content,''), 40), started_at
    FROM steps_v2
    WHERE started_at >= '2026-08-05 19:40:00' AND started_at <= '2026-08-05 20:00:00'
    ORDER BY started_at DESC LIMIT 10
""")
rows = cur.fetchall()
print(f"count={len(rows)}")
for r in rows:
    print(r)

print("\n=== turns_v2 columns ===")
cur.execute("""
    SELECT column_name FROM information_schema.columns
    WHERE table_name='turns_v2' ORDER BY ordinal_position
""")
print([r[0] for r in cur.fetchall()])

print("\n=== turns_v2 recent (by started_at) ===")
cur.execute("""
    SELECT id, task_id, status, started_at FROM turns_v2
    ORDER BY started_at DESC LIMIT 3
""")
for r in cur.fetchall():
    print(r)

cur.close()
conn.close()
