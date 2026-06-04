import sqlite3, json

DB = r'f:\project\aranea-agents\data\arenea.sqlite'
c = sqlite3.connect(DB)
c.row_factory = sqlite3.Row

print('=== Tables ===')
tables = c.execute("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name").fetchall()
for t in tables:
    print(' -', t['name'])

print()
print('=== LLM provider / key tables (look for "llm", "provider", "api_key", "key") ===')
for t in tables:
    name = t['name']
    if any(k in name.lower() for k in ['llm', 'provider', 'key', 'model', 'openrouter', 'api']):
        print(f"\n--- {name} ---")
        try:
            cols = c.execute(f"PRAGMA table_info({name})").fetchall()
            for c2 in cols:
                print('  col:', c2['name'], c2['type'])
        except Exception as e:
            print('  err:', e)

print()
print('=== Schema migrations (look for "provider" or "openrouter") ===')
try:
    rows = c.execute("SELECT name FROM sqlite_master WHERE type='table' AND name LIKE '%schema%'").fetchall()
    print('schema tables:', [r['name'] for r in rows])
except Exception as e:
    print('err:', e)

print()
print('=== Look for openrouter config / api keys ===')
for t in tables:
    name = t['name']
    try:
        rows = c.execute(f"SELECT * FROM {name} WHERE 1=0").fetchall()
        # Skip empty
        sample = c.execute(f"SELECT * FROM {name} LIMIT 1").fetchone()
        if sample is None:
            continue
        # Convert to dict
        d = dict(sample)
        d_str = json.dumps(d, ensure_ascii=False, default=str).lower()
        if 'openrouter' in d_str or 'api_key' in d_str or 'provider' in d_str:
            print(f"\n--- match in {name} ---")
            print(json.dumps(d, ensure_ascii=False, default=str, indent=2)[:2000])
    except Exception as e:
        pass
