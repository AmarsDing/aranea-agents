import psycopg2

conn = psycopg2.connect(
    host="127.0.0.1", port=5432, dbname="aranea",
    user="postgres", password="Hangshan@123",
)
cur = conn.cursor()
cur.execute("SELECT column_name FROM information_schema.columns WHERE table_name='admins' ORDER BY ordinal_position")
print("cols:", [r[0] for r in cur.fetchall()])
cur.execute("SELECT id, name, email, left(password, 20), access FROM admins LIMIT 5")
for r in cur.fetchall():
    print(r)
cur.close()
conn.close()
