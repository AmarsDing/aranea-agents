"""Inspect the 16:34 session: member_sessions task_id, graph stages/nodes status."""
import psycopg2

conn = psycopg2.connect(
    host="127.0.0.1", port=5432, dbname="aranea",
    user="postgres", password="Hangshan@123",
)
cur = conn.cursor()

# The 16:34 session was investigated previously; find root sessions created around 16:34 on 7/28.
print("=== root sessions created 7/28 16:00-17:30 ===")
cur.execute("""
    SELECT id, title, owner_type, agent_id, created_at
    FROM sessions
    WHERE deleted_at = '' AND parent_session_id = ''
      AND created_at BETWEEN '2026-07-28 16:00:00+08' AND '2026-07-28 17:30:00+08'
    ORDER BY created_at
""")
rows = cur.fetchall()
for r in rows:
    print(r)

if rows:
    sid = rows[-1][0]
    print(f"\n=== member_sessions for spirit root {sid} ===")
    cur.execute("""
        SELECT id, agent_key, task_id <> '' AS has_task, session_id, status
        FROM member_sessions_v2 WHERE spirit_session_id = %s
    """, (sid,))
    for r in cur.fetchall():
        print(r)

    print(f"\n=== graph_stages for session tasks ===")
    cur.execute("""
        SELECT gs.id, gs.status, gs.task_id, gs.started_at
        FROM graph_stages_v2 gs
        JOIN tasks_v2 t ON t.id = gs.task_id
        WHERE t.session_id = %s
        ORDER BY gs.started_at
    """, (sid,))
    for r in cur.fetchall():
        print(r)

    print(f"\n=== graph_nodes status ===")
    cur.execute("""
        SELECT gn.id, gn.status, gn.dag_node_id
        FROM graph_nodes_v2 gn
        JOIN graph_stages_v2 gs ON gs.id = gn.graph_stage_id
        JOIN tasks_v2 t ON t.id = gs.task_id
        WHERE t.session_id = %s
    """, (sid,))
    for r in cur.fetchall():
        print(r)

cur.close()
conn.close()
