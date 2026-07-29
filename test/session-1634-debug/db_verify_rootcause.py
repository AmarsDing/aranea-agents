"""Root-cause probe: why did the 2 system-admin member sessions fail (16:34 task)."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

SID = "f3511b7a-345c-410e-8596-8ab2b0913fcb"
TS1 = "1bfb37f9-0bb9-5264-a01e-adcda6c67d95"  # pdf install team stage
TS2 = "60dc773c-c2e5-5d3d-b596-1bfe6748b708"  # docx install team stage
MS1 = "3c730e21-c061-44d6-a56f-99fc8c48f8c9"  # member session 1
MS2 = "bdeda93d-f5e4-441c-8dd1-c747cd702cca"  # member session 2

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

q("team_runs_v2 for the 2 failed team stages", """
SELECT id, team_stage_id, status, started_at, completed_at
FROM team_runs_v2 WHERE team_stage_id IN (%s, %s) ORDER BY started_at
""", (TS1, TS2))

q("steps_v2 in member session 1 (pdf)", """
SELECT kind, status, tool_name, tool_error_code, content, tool_result
FROM steps_v2 WHERE session_id = %s ORDER BY seq LIMIT 30
""", (MS1,))

q("steps_v2 in member session 2 (docx)", """
SELECT kind, status, tool_name, tool_error_code, content, tool_result
FROM steps_v2 WHERE session_id = %s ORDER BY seq LIMIT 30
""", (MS2,))
