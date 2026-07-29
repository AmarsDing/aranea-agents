"""Verify outbox + session root_only state after 2026-07-29 03:18 restart."""
import psycopg2

conn = psycopg2.connect(
    host="127.0.0.1", port=5432, dbname="aranea",
    user="postgres", password="Hangshan@123",
)
cur = conn.cursor()

print("=== 1. event_delivery_outbox: total / unpublished / stuck (>10min old) ===")
cur.execute("""
    SELECT count(*),
           count(*) FILTER (WHERE published_at IS NULL),
           count(*) FILTER (WHERE published_at IS NULL AND created_at < now() - interval '10 minutes')
    FROM event_delivery_outbox
""")
total, unpub, stuck = cur.fetchone()
print(f"total={total} unpublished={unpub} stuck={stuck}")

print("\n=== 2. recent 10 outbox rows (newest first) ===")
cur.execute("""
    SELECT id, event_type, published_at IS NOT NULL AS published, created_at
    FROM event_delivery_outbox ORDER BY created_at DESC LIMIT 10
""")
for r in cur.fetchall():
    print(r)

print("\n=== 3. sessions: root vs child counts ===")
cur.execute("""
    SELECT count(*) FILTER (WHERE parent_session_id = ''),
           count(*) FILTER (WHERE parent_session_id <> '')
    FROM sessions WHERE deleted_at = ''
""")
root, child = cur.fetchone()
print(f"root={root} child={child}")

print("\n=== 4. member_sessions_v2: task_id fill state (last 20) ===")
cur.execute("""
    SELECT id, agent_key, task_id <> '' AS has_task, status, created_at
    FROM member_sessions_v2 ORDER BY created_at DESC LIMIT 20
""")
for r in cur.fetchall():
    print(r)

cur.close()
conn.close()
