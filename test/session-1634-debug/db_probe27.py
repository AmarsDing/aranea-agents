"""Probe 27: steps_v2 of the two failed team runs — what did the member actually do."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

def q(label, sql, args=()):
    print(f"\n===== {label} =====")
    try:
        cur.execute(sql, args)
        rows = cur.fetchall()
        for r in rows:
            print({k: (str(v)[:800] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("0. steps_v2 columns", """
SELECT column_name FROM information_schema.columns
WHERE table_name='steps_v2' ORDER BY ordinal_position
""")

for sid, name in [("3c730e21-c061-44d6-a56f-99fc8c48f8c9", "pdf team"),
                  ("bdeda93d-f5e4-441c-8dd1-c747cd702cca", "docx team")]:
    q(f"steps_v2 of {name} session", """
    SELECT id, kind, status, author_agent_key, tool_name, left(COALESCE(content,''), 600) AS content,
           left(COALESCE(tool_args,''), 300) AS tool_args, left(COALESCE(tool_result,''), 300) AS tool_result,
           started_at
    FROM steps_v2 WHERE session_id = %s ORDER BY started_at LIMIT 25
    """, (sid,))
