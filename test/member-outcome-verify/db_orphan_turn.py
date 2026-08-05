import psycopg2

conn = psycopg2.connect(
    host="127.0.0.1", port=5432, dbname="aranea",
    user="postgres", password="Hangshan@123",
)
cur = conn.cursor()

for tid in ("91ac4fe8-28b5-4ea5-9bdd-2ce713b33863", "70ea51e4-8f2a-476e-bce9-309b2b0e0c79"):
    print(f"=== task {tid[:8]} in tasks_v2? ===")
    cur.execute("SELECT id, session_id, status, version, created_at FROM tasks_v2 WHERE id = %s", (tid,))
    rows = cur.fetchall()
    print("tasks_v2:", rows if rows else "MISSING")
    cur.execute("SELECT id, task_id, session_id, agent_key, team_id, status, version FROM turns_v2 WHERE id = %s", (tid,))
    print("turns_v2:", cur.fetchall())

cur.close()
conn.close()
