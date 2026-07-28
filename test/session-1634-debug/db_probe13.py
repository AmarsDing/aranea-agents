"""Probe 13: graph stage/node actual status + team definition_json."""
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
            print({k: (str(v)[:2000] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("graph_stages_v2 rows", """
SELECT id, task_id, status, started_at, completed_at, version
FROM graph_stages_v2 WHERE session_id = %s ORDER BY started_at
""", (SID,))

q("graph_nodes_v2 rows", """
SELECT gn.id, gn.graph_stage_id, gn.dag_node_id, gn.team_stage_id, gn.label, gn.status
FROM graph_nodes_v2 gn
JOIN graph_stages_v2 gs ON gs.id = gn.graph_stage_id
WHERE gs.session_id = %s ORDER BY gn.label
""", (SID,))

q("teams definition_json", """
SELECT id, display_name, status, dag_node_id, topology, deliverables, input_contract,
       definition_json
FROM teams WHERE spirit_session_id = %s ORDER BY created_at
""", (SID,))

q("tasks_v2 rows", """
SELECT id, status, user_message, created_at, completed_at
FROM tasks_v2 WHERE session_id = %s ORDER BY created_at
""", (SID,))

q("steps_v2 detail (team sessions)", """
SELECT id, session_id, kind, status, agent_key, content, started_at, completed_at
FROM steps_v2 WHERE session_id IN ('3c730e21-c061-44d6-a56f-99fc8c48f8c9','bdeda93d-f5e4-441c-8dd1-c747cd702cca')
ORDER BY session_id, started_at
""")

conn.close()
