"""Schema probe for verification script fix."""
import psycopg2

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor()
for t in ("steps_v2",):
    cur.execute("""SELECT column_name FROM information_schema.columns
                   WHERE table_name = %s ORDER BY ordinal_position""", (t,))
    print(t, ":", [r[0] for r in cur.fetchall()])
