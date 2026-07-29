"""Check scope of zero-value runtime settings across pack-imported agents."""
import psycopg2

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor()

print("--- it-ops agents: tools_enabled / updated_at ---")
cur.execute("""
SELECT a.agent_key, s.tools_enabled, s.tools_profile, s.tools_allow_json, s.tools_deny_json, s.updated_at
FROM agents a
JOIN agent_runtime_settings s ON s.agent_id = a.id
WHERE a.agent_key LIKE '%\_\_general' AND a.position_key LIKE 'it_ops/%'
ORDER BY a.agent_key
""")
for row in cur.fetchall():
    print(row)

print("--- agency-pack sample (5 rows) ---")
cur.execute("""
SELECT a.agent_key, s.tools_enabled, s.tools_profile, s.memory_enabled
FROM agents a
JOIN agent_runtime_settings s ON s.agent_id = a.id
WHERE a.position_key NOT LIKE 'it_ops/%' AND a.kind = 'ecosystem_preset'
LIMIT 5
""")
for row in cur.fetchall():
    print(row)

print("--- aggregate: tools_enabled distribution over ecosystem_preset ---")
cur.execute("""
SELECT s.tools_enabled, s.memory_enabled, count(*)
FROM agents a
JOIN agent_runtime_settings s ON s.agent_id = a.id
WHERE a.kind = 'ecosystem_preset'
GROUP BY s.tools_enabled, s.memory_enabled
ORDER BY 1, 2
""")
for row in cur.fetchall():
    print(row)

print("--- system agents for contrast ---")
cur.execute("""
SELECT a.agent_key, s.tools_enabled, s.tools_profile
FROM agents a
JOIN agent_runtime_settings s ON s.agent_id = a.id
WHERE a.kind != 'ecosystem_preset'
LIMIT 8
""")
for row in cur.fetchall():
    print(row)

cur.close()
conn.close()
