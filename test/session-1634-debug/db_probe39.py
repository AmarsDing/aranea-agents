"""Probe 39: what happened inside the failed team session at 16:33."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

def show(label, q, args=()):
    print(f"=== {label} ===")
    cur.execute(q, args)
    rows = cur.fetchall()
    if not rows:
        print("(empty)")
    for r in rows:
        print({k: (str(v)[:260] if v is not None else None) for k, v in r.items()})
    print()

# team run row
show("team_runs_v2 row",
     "SELECT id, team_stage_id, task_id, session_id, spirit_session_id, status, started_at, completed_at FROM team_runs_v2 WHERE id = %s",
     ('0a49efd3-eb2c-5602-9b84-e54e72b7db69',))

# steps in team session 3c730e21
show("steps_v2 in team session 3c730e21",
     """SELECT id, kind, status, author_agent_key, tool_name, seq,
        left(coalesce(content,''),150) AS content
        FROM steps_v2 WHERE session_id = '3c730e21-c061-44d6-a56f-99fc8c48f8c9'
        ORDER BY seq LIMIT 30""")

# sessions created around 16:33 with parent = team session
show("child agent sessions of team session",
     """SELECT id, session_type, agent_id, member_agent_key, left(title,60) AS title, created_at
        FROM sessions WHERE parent_session_id = '3c730e21-c061-44d6-a56f-99fc8c48f8c9'""")

# member_sessions_v2 for the team run
cur.execute("""SELECT column_name FROM information_schema.columns
               WHERE table_name='member_sessions_v2' ORDER BY ordinal_position""")
print("member_sessions_v2 cols:", [r['column_name'] for r in cur.fetchall()])
show("member_sessions_v2 for team run",
     """SELECT id, team_run_id, agent_key, status, session_id, task_id
        FROM member_sessions_v2 WHERE team_run_id = '0a49efd3-eb2c-5602-9b84-e54e72b7db69'""")
