import sqlite3, json

DB = r'f:\project\aranea-agents\data\arenea.sqlite'
c = sqlite3.connect(DB)
c.row_factory = sqlite3.Row

print('=== All agents (active) ===')
rows = c.execute(
    "SELECT agent_key, display_name, provider, model, kind, status, agent_variant, readonly FROM agents WHERE deleted_at = '' ORDER BY agent_key"
).fetchall()
for r in rows:
    print(dict(r))
print()
print('=== All agents latest usage daily (any status) ===')
rows = c.execute(
    "SELECT agent_key, provider_code, model_api_id, request_count, success_count, failed_count, avg_latency_ms, date_key FROM model_token_usage_daily ORDER BY date_key DESC LIMIT 30"
).fetchall()
for r in rows:
    d = dict(r)
    if d['request_count'] > 0 or d['failed_count'] > 0:
        print(d)
print()
print('=== Recent events with errors or long latency ===')
rows = c.execute(
    "SELECT agent_key, provider_code, model_api_id, status, error_code, latency_ms, occurred_at, session_id FROM model_token_usage_events WHERE status != 'success' OR latency_ms > 30000 ORDER BY occurred_at DESC LIMIT 30"
).fetchall()
for r in rows:
    d = dict(r)
    print(d)
