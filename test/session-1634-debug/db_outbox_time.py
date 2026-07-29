"""Probe event_delivery_outbox schema + publish state by time bucket."""
import psycopg2

conn = psycopg2.connect(
    host="127.0.0.1", port=5432, dbname="aranea",
    user="postgres", password="Hangshan@123",
)
cur = conn.cursor()

print("=== columns ===")
cur.execute("""
    SELECT column_name, data_type FROM information_schema.columns
    WHERE table_name = 'event_delivery_outbox' ORDER BY ordinal_position
""")
for r in cur.fetchall():
    print(r)

print("\n=== publish state by hour (last 24h + older) ===")
cur.execute("""
    SELECT date_trunc('hour', created_at) AS hr,
           count(*),
           count(*) FILTER (WHERE published_at IS NULL) AS unpub
    FROM event_delivery_outbox
    GROUP BY 1 ORDER BY 1 DESC LIMIT 12
""")
for r in cur.fetchall():
    print(r)

print("\n=== rows created AFTER 2026-07-29 03:18 (new binary) ===")
cur.execute("""
    SELECT count(*),
           count(*) FILTER (WHERE published_at IS NULL) AS unpub
    FROM event_delivery_outbox
    WHERE created_at > '2026-07-29 03:18:00+08'
""")
print("total / unpub after restart:", cur.fetchone())

print("\n=== newest 5 rows ===")
cur.execute("""
    SELECT id, published_at IS NOT NULL AS published, created_at
    FROM event_delivery_outbox ORDER BY created_at DESC LIMIT 5
""")
for r in cur.fetchall():
    print(r)

cur.close()
conn.close()
