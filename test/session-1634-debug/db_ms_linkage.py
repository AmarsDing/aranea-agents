"""Check member_sessions_v2 linkage fields vs team_runs_v2 / team_stages_v2."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

def q(label, sql, args=()):
    print(f"\n===== {label} =====")
    cur.execute(sql, args)
    for r in cur.fetchall():
        print({k: (str(v)[:220] if v is not None else None) for k, v in r.items()})

SID = "f3511b7a-345c-410e-8596-8ab2b0913fcb"

q("member_sessions_v2 linkage for 16:34 session", """
SELECT id, team_run_id, team_stage_id, task_id, session_id, agent_key, status
FROM member_sessions_v2 WHERE spirit_session_id = %s ORDER BY started_at
""", (SID,))

q("team_runs_v2 ids for comparison", """
SELECT id, team_stage_id, status FROM team_runs_v2 WHERE spirit_session_id = %s
""", (SID,))
