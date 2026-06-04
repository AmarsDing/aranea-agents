import sqlite3

DB = r'f:\project\aranea-agents\data\arenea.sqlite'
SESSION_ID = '63472be0-4ff3-4772-9788-3dbd48a534a7'

c = sqlite3.connect(DB)
c.row_factory = sqlite3.Row

print('=== Session: 63472be0-... (the user-reproduced one) ===')
rows = c.execute(
    "SELECT id, status, status_reason, run_count, model_call_count, tool_call_count, error_count, message_count, created_at, updated_at FROM sessions WHERE id = ?",
    (SESSION_ID,),
).fetchall()
for r in rows:
    print(dict(r))
if not rows:
    print('(not found)')

print()
print('=== Messages for this session ===')
mrows = c.execute(
    "SELECT id, role, status, created_at FROM messages WHERE session_id = ? ORDER BY created_at",
    (SESSION_ID,),
).fetchall()
for r in mrows:
    print(dict(r))
if not mrows:
    print('(none)')

print()
print('=== Runs for this session ===')
rrows = c.execute(
    "SELECT id, status, created_at, error_code, error_message FROM runs WHERE session_id = ?",
    (SESSION_ID,),
).fetchall()
for r in rrows:
    print(dict(r))
if not rrows:
    print('(none)')

print()
print('=== Latest 5 sessions overall ===')
rows2 = c.execute(
    'SELECT id, status, status_reason, run_count, message_count, created_at, updated_at FROM sessions ORDER BY created_at DESC LIMIT 5'
).fetchall()
for r in rows2:
    print(dict(r))

print()
print('=== Tables ===')
tables = c.execute("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name").fetchall()
print([t['name'] for t in tables])
