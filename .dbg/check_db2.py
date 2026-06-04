import sqlite3

DB = r'f:\project\aranea-agents\data\arenea.sqlite'
c = sqlite3.connect(DB)
c.row_factory = sqlite3.Row

print('=== Sessions updated today (>= 2026-06-04) ===')
rows = c.execute(
    "SELECT id, status, status_reason, run_count, message_count, created_at, updated_at FROM sessions WHERE updated_at >= '2026-06-04' ORDER BY updated_at DESC LIMIT 20"
).fetchall()
for r in rows:
    print(dict(r))
if not rows:
    print('(none)')

print()
print('=== Sessions created today (>= 2026-06-04) ===')
rows = c.execute(
    "SELECT id, status, status_reason, run_count, message_count, created_at FROM sessions WHERE created_at >= '2026-06-04' ORDER BY created_at DESC LIMIT 20"
).fetchall()
for r in rows:
    print(dict(r))
if not rows:
    print('(none)')

print()
print('=== Messages created today (>= 2026-06-04) ===')
rows = c.execute(
    "SELECT id, session_id, role, status, created_at FROM messages WHERE created_at >= '2026-06-04' ORDER BY created_at DESC LIMIT 20"
).fetchall()
for r in rows:
    print(dict(r))
if not rows:
    print('(none)')

print()
print('=== Latest 5 messages overall ===')
rows = c.execute(
    'SELECT id, session_id, role, status, created_at FROM messages ORDER BY created_at DESC LIMIT 5'
).fetchall()
for r in rows:
    print(dict(r))
