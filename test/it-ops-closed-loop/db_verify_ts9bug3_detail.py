"""Inspect actual position_key values of it-ops agents."""
import psycopg2

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor()
cur.execute("""
SELECT a.agent_key, a.position_key, a.position_id, s.tools_enabled, s.tools_profile,
       s.tools_allow_json, s.tools_deny_json, s.memory_enabled, s.updated_at
FROM agents a
JOIN agent_runtime_settings s ON s.agent_id = a.id
WHERE a.agent_key IN ('alert_handler__general','change_executor__general','compliance_checker__general',
                      'db_operator__general','fault_diagnostician__general','incident_commander__general',
                      'log_analyst__general','metric_analyst__general','network_inspector__general',
                      'postmortem_writer__general','runbook_engineer__general','system_inspector__general')
ORDER BY a.agent_key
""")
for row in cur.fetchall():
    print(row)
cur.close()
conn.close()
