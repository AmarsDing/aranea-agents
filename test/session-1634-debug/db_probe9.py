"""Steps for the two failed system-admin member sessions."""
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
            print({k: (str(v)[:600] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("steps_v2 cols", """
SELECT column_name FROM information_schema.columns
WHERE table_name = 'steps_v2' ORDER BY ordinal_position
""")

for sid in ("3c730e21-c061-44d6-a56f-99fc8c48f8c9", "bdeda93d-f5e4-441c-8dd1-c747cd702cca"):
    q(f"steps for {sid[:8]}", """
    SELECT * FROM steps_v2 WHERE session_id = %s ORDER BY seq
    """, (sid,))

q("messages cols", """
SELECT column_name FROM information_schema.columns
WHERE table_name = 'messages' ORDER BY ordinal_position
""")
