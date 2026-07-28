"""Probe 38: team runs + failure details for the two failed team stages."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

def cols(table):
    cur.execute("""SELECT column_name FROM information_schema.columns
                   WHERE table_name=%s ORDER BY ordinal_position""", (table,))
    return [r['column_name'] for r in cur.fetchall()]

def show(label, q, args=()):
    print(f"=== {label} ===")
    cur.execute(q, args)
    for r in cur.fetchall():
        print({k: (str(v)[:300] if v is not None else None) for k, v in r.items()})
    print()

print("team_runs_v2 cols:", cols('team_runs_v2'))
print("steps_v2 cols:", cols('steps_v2'))
print()

show("team_runs_v2 for failed stages",
     """SELECT id, team_stage_id, status, left(coalesce(error,''),400) AS error, completed_at
        FROM team_runs_v2 WHERE team_stage_id IN ('1bfb37f9-0bb9-5264-a01e-adcda6c67d95','60dc773c-c2e5-5d3d-b596-1bfe6748b708')""")

show("steps_v2 (final/error) for failed stage team runs' sessions",
     """SELECT s.id, s.kind, s.status, s.author_agent_key, s.tool_name,
        left(coalesce(s.content,''),200) AS content,
        left(coalesce(s.tool_error_code,''),80) AS tool_err
        FROM steps_v2 s
        WHERE s.session_id IN (
          SELECT session_id FROM team_runs_v2
          WHERE team_stage_id IN ('1bfb37f9-0bb9-5264-a01e-adcda6c67d95','60dc773c-c2e5-5d3d-b596-1bfe6748b708'))
        ORDER BY s.started_at DESC LIMIT 20""")
