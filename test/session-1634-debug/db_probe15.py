"""Probe 15: steps content for the two team sessions."""
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
            print({k: (str(v)[:2500] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("steps in team sessions (kind/tool/content)", """
SELECT id, session_id, kind, author_agent_key, tool_name, status,
       left(coalesce(content,''), 600) AS content_head,
       left(coalesce(tool_args,''), 400) AS args_head,
       left(coalesce(tool_result,''), 800) AS result_head,
       tool_error_code, started_at
FROM steps_v2
WHERE session_id IN ('3c730e21-c061-44d6-a56f-99fc8c48f8c9','bdeda93d-f5e4-441c-8dd1-c747cd702cca')
ORDER BY session_id, started_at
""")

conn.close()
