import psycopg2

conn = psycopg2.connect(
    host="127.0.0.1", port=5432, dbname="aranea",
    user="postgres", password="Hangshan@123",
)
cur = conn.cursor()
cur.execute("""
    SELECT table_name FROM information_schema.tables
    WHERE table_schema='public' AND (table_name LIKE '%message%' OR table_name LIKE '%pending%')
    ORDER BY table_name
""")
for r in cur.fetchall():
    print(r[0])
cur.close()
conn.close()
