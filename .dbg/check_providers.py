import sqlite3, json

DB = r'f:\project\aranea-agents\data\arenea.sqlite'
c = sqlite3.connect(DB)
c.row_factory = sqlite3.Row

print('=== llm_provider_models: openrouter ===')
rows = c.execute(
    "SELECT model_key, provider, model, status, enabled, config_json FROM llm_provider_models WHERE provider IN ('openrouter', 'deepseek') ORDER BY provider, model_key"
).fetchall()
for r in rows:
    d = dict(r)
    print(f"\n--- {d['model_key']} ({d['provider']}/{d['model']}) status={d['status']} enabled={d['enabled']} ---")
    try:
        cfg = json.loads(d['config_json'])
        for k in ['api_base_url', 'api_key_set', 'api_key_enc', 'provider_type', 'auth_header']:
            if k in cfg:
                v = cfg[k]
                if 'key' in k.lower() and v:
                    v = v[:30] + '...' + v[-8:] + f' (len={len(v)})'
                print(f"  {k}: {v}")
    except Exception as e:
        print(f'  cfg parse err: {e}')
