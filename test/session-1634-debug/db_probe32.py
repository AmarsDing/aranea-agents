"""Probe 32: graph_stages_v2 / graph_nodes_v2 status for 16:34 session."""
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
            print({k: (str(v)[:280] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

for t in ["graph_stages_v2", "graph_nodes_v2"]:
    cur.execute("""
    SELECT column_name FROM information_schema.columns
    WHERE table_name=%s ORDER BY ordinal_position
    """, (t,))
    print(f"--- {t} cols: " + str([r["column_name"] for r in cur.fetchall()]))

q("graph_stages_v2 (16:34)", """
SELECT * FROM graph_stages_v2 WHERE session_id = %s ORDER BY started_at
""", (SID,))

q("graph_nodes_v2 (16:34)", """
SELECT * FROM graph_nodes_v2 WHERE session_id = %s ORDER BY started_at
""", (SID,))
