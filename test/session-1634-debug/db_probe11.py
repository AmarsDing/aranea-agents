"""Final comprehensive probe for the 16:34 session — all 4 issues."""
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
            print({k: (str(v)[:1200] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("0. outbox published_at type on PG", """
SELECT column_name, data_type FROM information_schema.columns
WHERE table_name = 'event_delivery_outbox' ORDER BY ordinal_position
""")

q("1. spirit session row", """
SELECT id, title, status, owner_type, session_type, agent_id, team_id,
       parent_session_id, created_at, updated_at
FROM sessions WHERE id = %s
""", (SID,))

q("2. tasks_v2 for session", """
SELECT id, status, user_message, error_message, created_at, completed_at
FROM tasks_v2 WHERE session_id = %s ORDER BY created_at
""", (SID,))

q("3. team_stages_v2", """
SELECT id, team_id, team_name, status, stage, version, started_at, completed_at
FROM team_stages_v2 WHERE session_id = %s ORDER BY started_at
""", (SID,))

q("4. team_runs_v2", """
SELECT id, team_id, status, error, started_at, completed_at
FROM team_runs_v2 WHERE spirit_session_id = %s ORDER BY started_at
""", (SID,))

q("5. member_sessions_v2", """
SELECT id, team_run_id, team_stage_id, session_id, agent_key, agent_name,
       status, task_id, error, started_at, finished_at
FROM member_sessions_v2 WHERE spirit_session_id = %s ORDER BY started_at
""", (SID,))

q("6. graph_stages_v2", """
SELECT id, session_id, status, created_at, completed_at
FROM graph_stages_v2 WHERE session_id = %s ORDER BY created_at
""", (SID,))

q("7. graph_nodes_v2", """
SELECT id, graph_stage_id, name, status, team_id, team_stage_id, depends_on,
       error, started_at, completed_at
FROM graph_nodes_v2 WHERE session_id = %s ORDER BY created_at
""", (SID,))

q("8. child sessions of spirit (team/agent sessions)", """
SELECT id, title, status, owner_type, session_type, agent_id, team_id,
       member_agent_key, parent_session_id, created_at
FROM sessions WHERE root_session_id = %s OR parent_session_id = %s
ORDER BY created_at
""", (SID, SID))

q("9. steps_v2 count per session", """
SELECT session_id, status, count(*), min(started_at) AS first_at, max(started_at) AS last_at
FROM steps_v2 WHERE session_id IN (
  SELECT id FROM sessions WHERE id = %s OR root_session_id = %s OR parent_session_id = %s
) GROUP BY session_id, status ORDER BY session_id, status
""", (SID, SID, SID))

q("10. teams entities + definition_json", """
SELECT id, display_name, status, dag_node_id, depends_on, topology,
       definition_json, deliverables, input_contract, created_at
FROM teams WHERE spirit_session_id = %s ORDER BY created_at
""", (SID,))

q("11. outbox rows for session (kinds)", """
SELECT kind, entity_id, published_at IS NOT NULL AS published, count(*)
FROM event_delivery_outbox WHERE session_id = %s GROUP BY kind, entity_id, published ORDER BY kind
""", (SID,))

conn.close()
