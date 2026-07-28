"""Outbox event stream for the 16:34 session."""
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
            print({k: (str(v)[:220] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("outbox cols", """
SELECT column_name FROM information_schema.columns
WHERE table_name = 'event_delivery_outbox' ORDER BY ordinal_position
""")

q("outbox events for spirit session", """
SELECT seq, kind, entity_id, event_id, published_at, created_at
FROM event_delivery_outbox
WHERE session_id = %s
ORDER BY seq
""", (SID,))
