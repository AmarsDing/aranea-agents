"""Verify TS9-BUG-3 fix: it-ops agents runtime settings after re-seed."""
import psycopg2

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor()

print("--- seed version gate ---")
cur.execute("SELECT version, name FROM schema_migrations WHERE version IN (20261116, 20261117) ORDER BY version")
for row in cur.fetchall():
    print(row)

print("--- it-ops 12 agents settings ---")
cur.execute("""
SELECT a.agent_key, s.tools_enabled, s.tools_profile, s.tools_allow_json, s.tools_deny_json, s.memory_enabled
FROM agents a
JOIN agent_runtime_settings s ON s.agent_id = a.id
WHERE a.position_key LIKE 'it_ops/%'
ORDER BY a.agent_key
""")
rows = cur.fetchall()
for row in rows:
    print(row)
print(f"total: {len(rows)}")

cur.close()
conn.close()
