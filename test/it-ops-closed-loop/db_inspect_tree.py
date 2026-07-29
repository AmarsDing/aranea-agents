"""Inspect spirit session row fields relevant to GetSessionTree."""
import psycopg2

SID = "ec86e351-88fc-4ffd-88d8-0ffce1e8af53"
conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor()
cur.execute("""SELECT id, session_type, parent_session_id, root_session_id, agent_depth, agent_id, status
               FROM sessions WHERE id=%s OR root_session_id=%s ORDER BY agent_depth, created_at""", (SID, SID))
for r in cur.fetchall():
    print(r)
print("---- sessions with empty root_session_id among them:")
cur.execute("""SELECT count(*) FROM sessions WHERE (id=%s OR root_session_id=%s) AND root_session_id=''""", (SID, SID))
print(cur.fetchone())
conn.close()
