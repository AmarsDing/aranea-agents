"""Check the plan created during TS9-GAP1 verification run."""
import json

import psycopg2

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor()
cur.execute("""
    SELECT id, spirit_session_id, status, strategy, sub_tasks_json
    FROM task_plans WHERE id='tp_622f0975-e4f9-4868-b7df-85e8cb6f8c59'
""")
row = cur.fetchone()
if not row:
    print("plan NOT found in DB")
else:
    print("id:", row[0])
    print("spirit_session_id:", row[1])
    print("status:", row[2], "strategy:", row[3])
    subs = json.loads(row[4] or "[]")
    print("subtask count:", len(subs))
    for s in subs:
        print(" -", s.get("id"), "|", s.get("name"), "| depends_on:", s.get("depends_on") or s.get("dependsOn"))
conn.close()
