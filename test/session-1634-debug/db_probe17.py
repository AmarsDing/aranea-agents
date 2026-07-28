"""Probe 17: consolidated re-verification of all 4 issues for the 16:34 session."""
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
            print({k: (str(v)[:500] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

# Issue 2: graph stage / node status
q("1. graph_stages_v2 for session", """
SELECT id, spirit_session_id, status, title, seq, started_at, finished_at, created_at
FROM graph_stages_v2 WHERE spirit_session_id = %s ORDER BY created_at
""", (SID,))

q("2. graph_nodes_v2 for session", """
SELECT id, graph_stage_id, team_stage_id, title, status, error, depends_on, started_at, finished_at
FROM graph_nodes_v2 WHERE graph_stage_id IN
  (SELECT id FROM graph_stages_v2 WHERE spirit_session_id = %s)
ORDER BY created_at
""", (SID,))

# Issue 4: team stages / runs / tasks failure
q("3. team_stages_v2 for session", """
SELECT id, spirit_session_id, dag_node_id, name, status, error, started_at, finished_at, version
FROM team_stages_v2 WHERE spirit_session_id = %s ORDER BY created_at
""", (SID,))

q("4. team_runs_v2 for session", """
SELECT id, team_stage_id, spirit_session_id, status, error, started_at, finished_at
FROM team_runs_v2 WHERE spirit_session_id = %s ORDER BY created_at
""", (SID,))

q("5. tasks_v2 for session", """
SELECT id, spirit_session_id, parent_task_id, title, status, error, seq, created_at
FROM tasks_v2 WHERE spirit_session_id = %s ORDER BY seq
""", (SID,))

# Issue 1: member sessions + steps
q("6. member_sessions_v2 for session", """
SELECT id, team_run_id, team_stage_id, session_id, spirit_session_id, agent_key,
       status, task_id, started_at, finished_at, version
FROM member_sessions_v2 WHERE spirit_session_id = %s ORDER BY created_at
""", (SID,))

q("7. steps_v2 counts by session for member sessions", """
SELECT session_id, count(*) AS n, min(seq) AS min_seq, max(seq) AS max_seq
FROM steps_v2
WHERE session_id IN (SELECT session_id FROM member_sessions_v2 WHERE spirit_session_id = %s)
GROUP BY session_id ORDER BY session_id
""", (SID,))

q("8. steps_v2 sample for member sessions (latest 10)", """
SELECT id, session_id, task_id, kind, status, title, seq, created_at
FROM steps_v2
WHERE session_id IN (SELECT session_id FROM member_sessions_v2 WHERE spirit_session_id = %s)
ORDER BY created_at DESC LIMIT 10
""", (SID,))

# Outbox: published_at state + kinds
q("9. outbox rows for session (kind counts)", """
SELECT kind, count(*) AS n,
       count(*) FILTER (WHERE published_at IS NULL) AS unpublished
FROM event_delivery_outbox WHERE session_id = %s
GROUP BY kind ORDER BY kind
""", (SID,))

q("10. outbox terminal graph/team rows", """
SELECT seq, kind, entity_id, published_at, created_at
FROM event_delivery_outbox
WHERE session_id = %s AND (kind LIKE 'graph%%' OR kind LIKE 'team%%' OR kind LIKE 'member%%')
ORDER BY seq
""", (SID,))

# Issue 3: sessions around 16:34 — which leak into sidebar
q("11. distinct owner_type/session_type in sessions", """
SELECT owner_type, session_type, count(*) FROM sessions
WHERE deleted_at = '' GROUP BY owner_type, session_type ORDER BY 1, 2
""")

q("12. recent sessions that would show in sidebar (last 20 by updated_at)", """
SELECT id, title, owner_type, session_type, agent_id, team_id, parent_session_id, status, updated_at
FROM sessions
WHERE deleted_at = '' AND archived_at = ''
ORDER BY updated_at DESC LIMIT 20
""")

conn.close()
