"""Probe v2 with correct columns."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

SID = "f3511b7a-345c-410e-8596-8ab2b0913fcb"
TASK = "baba1cba-4c2e-4621-be39-d2fd18f651bc"

def q(label, sql, args=()):
    print(f"\n===== {label} =====")
    try:
        cur.execute(sql, args)
        rows = cur.fetchall()
        for r in rows:
            print({k: (str(v)[:300] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

for t in ["tasks_v2", "turns_v2", "steps_v2", "plan_boards_v2", "plan_steps_v2",
          "graph_stages_v2", "graph_nodes_v2", "team_stages_v2", "team_runs_v2"]:
    q(f"cols {t}", """
    SELECT column_name FROM information_schema.columns
    WHERE table_name = %s ORDER BY ordinal_position
    """, (t,))

q("tasks_v2 row", "SELECT * FROM tasks_v2 WHERE id = %s", (TASK,))
q("tasks_v2 for session", "SELECT * FROM tasks_v2 WHERE session_id = %s ORDER BY created_at", (SID,))
