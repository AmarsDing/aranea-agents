"""Check admins table state + possible lockout columns."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)
cur.execute("""SELECT column_name FROM information_schema.columns
               WHERE table_name='admins' ORDER BY ordinal_position""")
print("admins cols:", [r["column_name"] for r in cur.fetchall()])
cur.execute("SELECT * FROM admins LIMIT 5")
for r in cur.fetchall():
    print({k: (str(v)[:120] if v is not None else None) for k, v in r.items()})
