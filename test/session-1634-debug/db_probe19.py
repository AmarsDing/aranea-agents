"""Probe 19: corrected column names — full state of the 16:34 session."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

SID = "f3511b7a-345c-410e-8596-8ab2b0913fcb"

def q(label, sql, args=()):
    print(f"\n===== {label} =====")
    try:
        cur.execute(sql, args)
        rows = cur.fetchall()
        for r in rows:
            print({k: (str(v)[:700] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("1. graph_stages_v2", """
SELECT id, task_id, turn_id, session_id, plan_board_id, status, seq, started_at, completed_at
FROM graph_stages_v2 WHERE session_id = %s ORDER BY seq
""", (SID,))

q("2. graph_nodes_v2", """
SELECT n.id, n.graph_stage_id, n.dag_node_id, n.team_stage_id, n.label, n.status
FROM graph_nodes_v2 n
JOIN graph_stages_v2 g ON g.id = n.graph_stage_id
WHERE g.session_id = %s ORDER BY n.label
""", (SID,))

q("3. team_stages_v2", """
SELECT id, task_id, session_id, team_id, dag_node_id, team_name, status, stage,
       members, strategy, seq, version, started_at, completed_at
FROM team_stages_v2 WHERE session_id = %s ORDER BY seq
""", (SID,))

q("4. team_runs_v2", """
SELECT id, team_stage_id, task_id, session_id, dag_node_id, status, error, seq, version,
       started_at, completed_at
FROM team_runs_v2 WHERE spirit_session_id = %s ORDER BY seq
""", (SID,))

q("5. member_sessions_v2", """
SELECT id, team_run_id, team_stage_id, task_id, session_id, agent_key, agent_name,
       status, error, seq, version, started_at, finished_at
FROM member_sessions_v2 WHERE spirit_session_id = %s ORDER BY seq
""", (SID,))

q("6. tasks_v2", """
SELECT id, session_id, status, seq, version, user_message, created_at, completed_at
FROM tasks_v2 WHERE session_id = %s ORDER BY seq
""", (SID,))

q("7. steps_v2 all for session tree", """
SELECT id, turn_id, task_id, session_id, spirit_session_id, kind, author_agent_key,
       status, seq, notice_type, left(content, 120) AS content_head, tool_name,
       started_at, completed_at
FROM steps_v2 WHERE spirit_session_id = %s ORDER BY seq
""", (SID,))

conn.close()
