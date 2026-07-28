"""Probe 37: graph stage/team stage/task status for 16:34 session."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

def show(label, q, args=()):
    print(f"=== {label} ===")
    cur.execute(q, args)
    for r in cur.fetchall():
        print({k: (str(v)[:220] if v is not None else None) for k, v in r.items()})
    print()

show("graph_stages_v2",
     "SELECT id, task_id, status, started_at, completed_at, seq, version FROM graph_stages_v2 WHERE id = %s",
     ('86677865-60ce-5ed6-b9e0-40c281f7a160',))

show("team_stages_v2 (two failed nodes)",
     """SELECT id, dag_node_id, status, team_name, members, started_at, completed_at, version
        FROM team_stages_v2 WHERE id IN ('1bfb37f9-0bb9-5264-a01e-adcda6c67d95','60dc773c-c2e5-5d3d-b596-1bfe6748b708')""")

show("tasks_v2 for 16:34 root session",
     """SELECT id, left(coalesce(user_message,''),60) AS msg, status, created_at, completed_at, version
        FROM tasks_v2 WHERE session_id = 'f3511b7a-345c-410e-8596-8ab2b0913fcb'
        ORDER BY created_at DESC LIMIT 5""")
