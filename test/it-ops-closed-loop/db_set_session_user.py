"""One-off: set user_id on the TS9 spirit session so plan APIs accept the admin token."""
import psycopg2

SID = "ec86e351-88fc-4ffd-88d8-0ffce1e8af53"
conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor()
cur.execute("UPDATE sessions SET user_id='1' WHERE id=%s", (SID,))
print("updated rows:", cur.rowcount)
cur.execute("SELECT id, user_id, status FROM sessions WHERE id=%s", (SID,))
print(cur.fetchone())
conn.commit()
conn.close()
