"""Probe 30: column discovery for v2 tables + outbox."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

for t in ["team_stages_v2", "team_runs_v2", "tasks_v2", "event_delivery_outbox", "member_sessions_v2"]:
    cur.execute("""
    SELECT column_name FROM information_schema.columns
    WHERE table_name=%s ORDER BY ordinal_position
    """, (t,))
    print(f"\n===== {t} =====")
    print([r["column_name"] for r in cur.fetchall()])
