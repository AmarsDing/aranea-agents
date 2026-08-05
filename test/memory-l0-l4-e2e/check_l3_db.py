# L3 facts 排查：consolidation 声称写入但 API 查不到 —— 直查 DB 定位
import psycopg2

AID = 'f2e5a24ab0756d6413d6a1a3'
SID = 'e16fe63c-26b3-4140-b6e8-a48f65d01924'
conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

print("=== 1. facts for this agent (all scopes) ===")
cur.execute("SELECT count(*) FROM memory_facts WHERE agent_id=%s", (AID,))
print("count by agent_id:", cur.fetchone())

cur.execute("SELECT scope_type, scope_id, workspace_id, count(*) FROM memory_facts WHERE agent_id=%s GROUP BY scope_type, scope_id, workspace_id", (AID,))
for r in cur.fetchall():
    print("  group:", r)

print("\n=== 2. latest 10 facts globally ===")
cur.execute("SELECT scope_type, scope_id, agent_id, workspace_id, substr(statement,1,50), created_at FROM memory_facts ORDER BY created_at DESC LIMIT 10")
for r in cur.fetchall():
    print(" ", r)

print("\n=== 3. episodes for this session ===")
cur.execute("SELECT id, consolidation_status, consolidated_l3_count, episode_kind FROM memory_episodes WHERE session_id=%s", (SID,))
for r in cur.fetchall():
    print(" ", r)

print("\n=== 4. episode->fact link (source_episode_id) ===")
cur.execute("SELECT column_name FROM information_schema.columns WHERE table_name='memory_facts' ORDER BY ordinal_position")
cols = [r[0] for r in cur.fetchall()]
print("memory_facts columns:", cols)

conn.close()
