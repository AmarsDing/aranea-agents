"""State of the 16:34 session f3511b7a: member_sessions / graph stages / nodes."""
import psycopg2

SID = "f3511b7a-345c-410e-8596-8ab2b0913fcb"

conn = psycopg2.connect(
    host="127.0.0.1", port=5432, dbname="aranea",
    user="postgres", password="Hangshan@123",
)
cur = conn.cursor()

print("=== member_sessions (spirit root) ===")
cur.execute("""
    SELECT agent_key, task_id <> '' AS has_task, left(session_id, 8) AS sess, status
    FROM member_sessions_v2 WHERE spirit_session_id = %s
""", (SID,))
for r in cur.fetchall():
    print(r)

print("\n=== graph_stages ===")
cur.execute("""
    SELECT gs.id, gs.status, left(gs.task_id, 8)
    FROM graph_stages_v2 gs
    JOIN tasks_v2 t ON t.id = gs.task_id
    WHERE t.session_id = %s
""", (SID,))
for r in cur.fetchall():
    print(r)

print("\n=== graph_nodes ===")
cur.execute("""
    SELECT gn.dag_node_id, gn.status
    FROM graph_nodes_v2 gn
    JOIN graph_stages_v2 gs ON gs.id = gn.graph_stage_id
    JOIN tasks_v2 t ON t.id = gs.task_id
    WHERE t.session_id = %s
""", (SID,))
for r in cur.fetchall():
    print(r)

print("\n=== steps count for member sessions of __system_admin__ ===")
cur.execute("""
    SELECT ms.agent_key, ms.session_id, count(s.id) AS steps
    FROM member_sessions_v2 ms
    LEFT JOIN steps_v2 s ON s.session_id = ms.session_id
    WHERE ms.spirit_session_id = %s
    GROUP BY ms.agent_key, ms.session_id
""", (SID,))
for r in cur.fetchall():
    print(r)

cur.close()
conn.close()
