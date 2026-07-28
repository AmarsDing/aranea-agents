"""Probe 28: spirit session's own steps — the user's original message & planner output."""
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
            print({k: (str(v)[:900] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("steps_v2 of spirit session (user msg + planner + replies)", """
SELECT id, kind, status, author_agent_key, tool_name,
       left(COALESCE(content,''), 800) AS content,
       left(COALESCE(tool_args,''), 500) AS tool_args,
       started_at
FROM steps_v2 WHERE session_id = %s
ORDER BY started_at LIMIT 40
""", (SID,))
