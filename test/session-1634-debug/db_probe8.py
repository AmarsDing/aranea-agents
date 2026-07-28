"""Probe failure reason + graph nodes + sessions for the 16:34 session."""
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
            print({k: (str(v)[:400] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("graph_nodes rows", """
SELECT * FROM graph_nodes_v2 WHERE graph_stage_id = '86677865-60ce-5ed6-b9e0-40c281f7a160'
""")

q("team_runs rows (error)", """
SELECT id, team_stage_id, status, error, version, started_at, completed_at
FROM team_runs_v2 WHERE team_stage_id IN
  ('1bfb37f9-0bb9-5264-a01e-adcda6c67d95','60dc773c-c2e5-5d3d-b596-1bfe6748b708')
""")

q("plan_steps rows", """
SELECT id, label, status, agent_keys, error, result, mapped_team_stage_id
FROM plan_steps_v2 WHERE plan_id = 'pb_tp_63f7f8ac-651b-4eb2-ad46-a7d866bffaf8'
""")

q("sessions cols", """
SELECT column_name FROM information_schema.columns
WHERE table_name = 'sessions' ORDER BY ordinal_position
""")

q("member session rows", """
SELECT * FROM sessions WHERE id IN
  ('3c730e21-c061-44d6-a56f-99fc8c48f8c9','bdeda93d-f5e4-441c-8dd1-c747cd702cca')
""")

q("all sessions created 16:30-17:10", """
SELECT id, title, session_type, owner_type, agent_id, created_at, updated_at
FROM sessions
WHERE created_at BETWEEN '2026-07-28 16:30:00+08' AND '2026-07-28 17:10:00+08'
ORDER BY created_at
""")
