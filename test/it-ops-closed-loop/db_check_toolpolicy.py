import psycopg2

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor()
cur.execute("""
SELECT a.agent_key, s.tools_enabled, s.tools_profile, s.tools_allow_json, s.tools_deny_json, s.memory_enabled
FROM agents a
LEFT JOIN agent_runtime_settings s ON s.agent_id = a.id
WHERE a.agent_key IN ('change_executor__general','db_operator__general','alert_handler__general')
ORDER BY a.agent_key
""")
for row in cur.fetchall():
    print(row)
cur.close()
conn.close()
