import sqlite3, json

DB = r'f:\project\aranea-agents\data\arenea.sqlite'
c = sqlite3.connect(DB)
c.row_factory = sqlite3.Row

print('=== agents (first 30) ===')
rows = c.execute("SELECT agent_key, name, provider, model FROM agents ORDER BY agent_key LIMIT 30").fetchall()
for r in rows:
    d = dict(r)
    print(f"  {d['agent_key']} | {d['name']} | {d['provider']}/{d['model']}")

print(f'\nTotal agents: {c.execute("SELECT COUNT(*) FROM agents").fetchone()[0]}')

# Check if the referenced agents exist
for key in ['go-senior-general', 'ue-client-general', 'vue3-senior-general']:
    row = c.execute("SELECT agent_key, name FROM agents WHERE agent_key = ?", (key,)).fetchone()
    if row:
        print(f"  FOUND: {key} = {dict(row)}")
    else:
        print(f"  MISSING: {key}")

c.close()
