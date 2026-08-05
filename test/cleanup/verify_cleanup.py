# 清理验证（只读）
import psycopg2
conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

checks = [
    ("memtest agent", "SELECT count(*) FROM agents WHERE id='f2e5a24ab0756d6413d6a1a3'"),
    ("memtest sessions", "SELECT count(*) FROM sessions WHERE agent_id='f2e5a24ab0756d6413d6a1a3'"),
    ("canary facts", "SELECT count(*) FROM memory_facts WHERE source_kind='memory_canary'"),
    ("memtest facts", "SELECT count(*) FROM memory_facts WHERE agent_id='f2e5a24ab0756d6413d6a1a3'"),
    ("spirit test facts", "SELECT count(*) FROM memory_facts WHERE id IN ('47286a72-3e3b-4c04-ab05-1cc40ae1c14b','cd32091b-7c19-46a8-8978-32ed96fb4de9')"),
    ("memtest entities", "SELECT count(*) FROM memory_entities WHERE scope_id='f2e5a24ab0756d6413d6a1a3'"),
    ("spirit test entity", "SELECT count(*) FROM memory_entities WHERE id='l4-pref-agent___spirit__-_5403_65e5_6599'"),
    ("test collections", "SELECT count(*) FROM knowledge_collections WHERE id IN ('1612fe0708099f10331c','997a33fffc59f8894eac','d015dea03d40f77084a0')"),
    ("action_log", "SELECT count(*) FROM memory_action_log"),
    ("L0 snapshots", "SELECT count(*) FROM memory_l0_assembly_snapshots"),
    ("--- 保留项核对 ---", None),
    ("spirit baseline facts (应≥4)", "SELECT count(*) FROM memory_facts WHERE agent_id='agent___spirit__'"),
    ("spirit baseline entity (应=1)", "SELECT count(*) FROM memory_entities WHERE scope_id='agent___spirit__'"),
    ("skills entities (应=1)", "SELECT count(*) FROM memory_entities WHERE scope_id='agent___skills__'"),
    ("system_admin entities (应=1)", "SELECT count(*) FROM memory_entities WHERE scope_id='agent___system_admin__'"),
    ("spirit sessions (应=73)", "SELECT count(*) FROM sessions s JOIN agents a ON a.id=s.agent_id WHERE a.agent_key='__spirit__'"),
    ("facts total", "SELECT count(*) FROM memory_facts"),
    ("episodes total", "SELECT count(*) FROM memory_episodes"),
    ("entities total", "SELECT count(*) FROM memory_entities"),
]
for label, sql in checks:
    if sql is None:
        print(f"\n{label}")
        continue
    cur.execute(sql)
    print(f"{label}: {cur.fetchone()[0]}")
conn.close()
