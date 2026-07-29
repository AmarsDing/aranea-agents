"""Get the 16:34 session's agent_id + count per-agent root/child split."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

def q(label, sql, args=()):
    print(f"\n===== {label} =====")
    cur.execute(sql, args)
    for r in cur.fetchall():
        print({k: (str(v)[:200] if v is not None else None) for k, v in r.items()})

q("16:34 session row", """
SELECT id, owner_type, agent_id, title, session_type, parent_session_id, archived_at
FROM sessions WHERE id = %s
""", ("f3511b7a-345c-410e-8596-8ab2b0913fcb",))

q("per-agent root/child split (alive)", """
SELECT agent_id,
  COUNT(*) AS total,
  COUNT(*) FILTER (WHERE parent_session_id = '') AS roots,
  COUNT(*) FILTER (WHERE parent_session_id <> '') AS children,
  COUNT(*) FILTER (WHERE archived_at <> '') AS archived
FROM sessions WHERE deleted_at = ''
GROUP BY agent_id ORDER BY total DESC LIMIT 8
""")
