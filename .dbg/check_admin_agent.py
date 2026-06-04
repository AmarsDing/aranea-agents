import sqlite3, json

DB = r'f:\project\aranea-agents\data\arenea.sqlite'
c = sqlite3.connect(DB)
c.row_factory = sqlite3.Row

print('=== __system_admin__ agent ===')
rows = c.execute(
    "SELECT id, agent_key, display_name, provider, model, kind, status, system_prompt_mode, context_window, config_json FROM agents WHERE agent_key = '__system_admin__'"
).fetchall()
for r in rows:
    d = dict(r)
    print(f"id={d['id']}\nkey={d['agent_key']}\nname={d['display_name']}\nprovider={d['provider']}\nmodel={d['model']}\nkind={d['kind']}\nstatus={d['status']}\nsys_prompt={d['system_prompt_mode']}\nctx={d['context_window']}\n")
    print('config_json (first 2000 chars):')
    print(d['config_json'][:2000] if d['config_json'] else '(empty)')
    print()

print('=== All agents (summary) ===')
rows = c.execute(
    "SELECT agent_key, display_name, provider, model, status, kind, agent_variant FROM agents WHERE deleted_at = '' ORDER BY agent_key"
).fetchall()
for r in rows:
    print(dict(r))
