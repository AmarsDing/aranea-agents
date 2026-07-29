"""Reset it-ops seed version gate so the seed re-runs on next startup."""
import psycopg2

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
conn.autocommit = True
cur = conn.cursor()
cur.execute("DELETE FROM schema_migrations WHERE version = 20261117")
print(f"deleted rows: {cur.rowcount}")
cur.execute("SELECT version, name FROM schema_migrations WHERE name LIKE 'pack_it_ops%' ORDER BY version")
for row in cur.fetchall():
    print(row)
cur.close()
conn.close()
