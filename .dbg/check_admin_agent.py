import sqlite3, json

DB = r'f:\project\aranea-agents\data\arenea.sqlite'
c = sqlite3.connect(DB)

row = c.execute("SELECT config_json FROM agents WHERE agent_key='__system_admin__'").fetchone()
if row and row[0]:
    parsed = json.loads(row[0])
    print(json.dumps(parsed, indent=2, ensure_ascii=False))
else:
    print("NULL")

c.close()
