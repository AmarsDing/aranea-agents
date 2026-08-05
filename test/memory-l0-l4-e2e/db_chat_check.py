import psycopg2

AID = 'f2e5a24ab0756d6413d6a1a3'
SID = '48f311e3-ab69-4823-ba35-fd63767471bd'
conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

cur.execute("SELECT id, session_id, title, episode_kind, consolidation_status, consolidated_l3_count, importance, created_at FROM memory_episodes WHERE agent_id=%s ORDER BY created_at DESC LIMIT 5", (AID,))
print('episodes:')
for r in cur.fetchall():
    print(' ', r)

cur.execute("SELECT count(*) FROM memory_facts WHERE agent_id=%s", (AID,))
print('facts:', cur.fetchone())

cur.execute("SELECT count(*) FROM memory_entities WHERE scope_id=%s", (AID,))
print('entities:', cur.fetchone())

cur.execute("SELECT id, agent_id, context_window_tokens, budget_tokens, prompt_token_estimate, l1_field_count, l3_chunk_count, l4_path_count, used_ratio, warning_codes_json, created_at FROM memory_l0_assembly_snapshots WHERE session_id=%s ORDER BY created_at DESC LIMIT 3", (SID,))
print('l0 snapshots:')
for r in cur.fetchall():
    print(' ', r)

cur.execute("SELECT id, task_key, status FROM memory_l1_tasks WHERE session_id=%s", (SID,))
print('l1 tasks:')
for r in cur.fetchall():
    print(' ', r)
cur.execute("SELECT count(*) FROM memory_l1_fields WHERE session_id=%s", (SID,))
print('l1 fields:', cur.fetchone())

conn.close()
