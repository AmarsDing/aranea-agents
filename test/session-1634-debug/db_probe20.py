"""Probe 20: plan board + steps for the 16:34 session."""
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

q("plan_boards_v2 cols", """
SELECT column_name FROM information_schema.columns
WHERE table_name='plan_boards_v2' ORDER BY ordinal_position""")

q("plan_boards_v2 for session", """
SELECT * FROM plan_boards_v2 WHERE session_id = %s
""", (SID,))

q("plan_steps_v2 cols", """
SELECT column_name FROM information_schema.columns
WHERE table_name='plan_steps_v2' ORDER BY ordinal_position""")

q("plan_steps for board", """
SELECT * FROM plan_steps_v2 WHERE plan_board_id = 'pb_tp_63f7f8ac-651b-4eb2-ad46-a7d866bffaf8'
ORDER BY seq
""")

conn.close()
