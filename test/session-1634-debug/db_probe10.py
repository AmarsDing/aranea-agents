"""Team entities + user task full message."""
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
            print({k: (str(v)[:1500] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("teams cols", """
SELECT column_name FROM information_schema.columns
WHERE table_name = 'teams' ORDER BY ordinal_position
""")

q("team entities", """
SELECT * FROM teams WHERE id IN ('8455b32294f5a840220c0fdf','3c1676eee233b3f049ce673a')
""")

q("full user task message", """
SELECT user_message FROM tasks_v2 WHERE id = 'baba1cba-4c2e-4621-be39-d2fd18f651bc'
""")
