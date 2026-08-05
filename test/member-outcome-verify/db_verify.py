"""Runtime verification for member-outcome single-writer redesign (2026-08-05).

Checks:
1. Recent member_sessions_v2 terminal records carry the outcome sentinel band (1<<40).
2. Any team execution after the new admin.exe started (2026-08-05 19:16) reached terminal.
3. Orphan running members (running but team already terminal) — the original bug.
"""
import psycopg2

SENTINEL = 1 << 40

conn = psycopg2.connect(
    host="127.0.0.1", port=5432, dbname="aranea",
    user="postgres", password="Hangshan@123",
)
cur = conn.cursor()

print("=== 1. latest 15 member_sessions_v2 (status/version band) ===")
cur.execute("""
    SELECT agent_key, status, version,
           CASE WHEN version = %s THEN 'OUTCOME' WHEN version <= 2 THEN 'lifecycle' ELSE 'other' END AS band,
           finished_at IS NOT NULL AS has_finished, started_at
    FROM member_sessions_v2 ORDER BY started_at DESC LIMIT 15
""", (SENTINEL,))
for r in cur.fetchall():
    print(r)

print("\n=== 2. member sessions created after new binary start (2026-08-05 19:16) ===")
cur.execute("""
    SELECT agent_key, status, version, finished_at IS NOT NULL, started_at
    FROM member_sessions_v2
    WHERE started_at >= '2026-08-05 19:16:00'
    ORDER BY started_at DESC LIMIT 20
""")
rows = cur.fetchall()
print(f"count={len(rows)}")
for r in rows:
    print(r)

print("\n=== 3. suspect: running/pending members whose team is already terminal ===")
cur.execute("""
    SELECT ms.agent_key, ms.status AS ms_status, t.status AS team_status, ms.version, ms.started_at
    FROM member_sessions_v2 ms
    JOIN team_stages_v2 ts ON ts.id = ms.team_stage_id
    JOIN teams t ON t.id = ts.team_id
    WHERE ms.status IN ('running','pending','paused')
      AND t.status IN ('completed','failed','cancelled','archived')
    ORDER BY ms.started_at DESC LIMIT 10
""")
rows = cur.fetchall()
print(f"suspect_count={len(rows)}")
for r in rows:
    print(r)

print("\n=== 4. terminal members WITHOUT outcome band (legacy or wrong writer) ===")
cur.execute("""
    SELECT status, version, count(*)
    FROM member_sessions_v2
    WHERE status IN ('completed','failed','skipped')
    GROUP BY status, version
    ORDER BY version DESC LIMIT 10
""")
for r in cur.fetchall():
    print(r)

cur.close()
conn.close()
