# 测试中间数据清理（2026-08-05）
# 范围：memtest-agent 全链路 + canary facts + 3 个测试知识库 + 边界项（spirit 吃日料实体/关系/facts/episode + user_id='1' 张三 facts）+ memory_action_log 全清
# 流程：备份 JSON → 按依赖序 DELETE → 打印计数 → 单事务提交
import json, os, sys
import psycopg2

BACKUP_DIR = os.path.join(os.path.dirname(__file__), 'backup-20260805')
os.makedirs(BACKUP_DIR, exist_ok=True)

MEMTEST_AGENT = 'f2e5a24ab0756d6413d6a1a3'
SIDS = [
    '48f311e3-ab69-4823-ba35-fd63767471bd',
    '97b36ce6-a581-42e9-a664-b65e6bc30ccd',
    'f5c0d524-e719-478a-adb2-2b366acf4741',
    'e16fe63c-26b3-4140-b6e8-a48f65d01924',
    '685dfbcb-f7e0-40f7-9792-003bfe2405ca',
    '33461c44-fd97-44c4-b755-a98597876d66',
]
SPIRIT_TEST_FACTS = ['47286a72-3e3b-4c04-ab05-1cc40ae1c14b', 'cd32091b-7c19-46a8-8978-32ed96fb4de9']
SPIRIT_TEST_EPISODE = '12a9ced4-640f-4900-bd1a-2d5a8200ac43'
SPIRIT_TEST_ENTITY = 'l4-pref-agent___spirit__-_5403_65e5_6599'
SPIRIT_TEST_RELATION = '67814e8b-075a-473d-a351-b734bac2d730'
TEST_COLLECTIONS = ['1612fe0708099f10331c', '997a33fffc59f8894eac', 'd015dea03d40f77084a0']

conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
conn.autocommit = False
cur = conn.cursor()

def backup(label, sql, params=None):
    cur.execute(sql, params or ())
    cols = [d[0] for d in cur.description]
    rows = [dict(zip(cols, r)) for r in cur.fetchall()]
    path = os.path.join(BACKUP_DIR, f'{label}.json')
    with open(path, 'w', encoding='utf-8') as f:
        json.dump(rows, f, ensure_ascii=False, default=str, indent=1)
    print(f"[backup] {label}: {len(rows)} rows -> {path}")
    return len(rows)

def delete(label, sql, params=None):
    cur.execute(sql, params or ())
    print(f"[delete] {label}: {cur.rowcount} rows")
    return cur.rowcount

SID_T = tuple(SIDS)
COL_T = tuple(TEST_COLLECTIONS)

try:
    # ---------- 备份 ----------
    backup('sessions', "SELECT * FROM sessions WHERE id = ANY(%s)", (SIDS,))
    backup('agents', "SELECT * FROM agents WHERE id = %s", (MEMTEST_AGENT,))
    backup('memory_facts', "SELECT * FROM memory_facts WHERE agent_id = %s OR source_kind = 'memory_canary' OR id = ANY(%s)", (MEMTEST_AGENT, SPIRIT_TEST_FACTS))
    backup('memory_episodes', "SELECT * FROM memory_episodes WHERE agent_id = %s OR id = %s", (MEMTEST_AGENT, SPIRIT_TEST_EPISODE))
    backup('memory_entities', "SELECT * FROM memory_entities WHERE scope_id = %s OR id = %s", (MEMTEST_AGENT, SPIRIT_TEST_ENTITY))
    backup('memory_relations', "SELECT * FROM memory_relations WHERE scope_id = %s OR id = %s", (MEMTEST_AGENT, SPIRIT_TEST_RELATION))
    backup('knowledge_collections', "SELECT * FROM knowledge_collections WHERE id = ANY(%s)", (TEST_COLLECTIONS,))
    backup('knowledge_documents', "SELECT * FROM knowledge_documents WHERE collection_id = ANY(%s)", (TEST_COLLECTIONS,))
    backup('knowledge_links', "SELECT * FROM knowledge_links WHERE collection_id = ANY(%s)", (TEST_COLLECTIONS,))
    backup('memory_action_log', "SELECT * FROM memory_action_log")

    # ---------- 删除（依赖序） ----------
    # 计划板
    delete('plan_steps_v2', "DELETE FROM plan_steps_v2 WHERE plan_id IN (SELECT id FROM plan_boards_v2 WHERE session_id = ANY(%s))", (SIDS,))
    delete('plan_boards_v2', "DELETE FROM plan_boards_v2 WHERE session_id = ANY(%s)", (SIDS,))
    # L1
    delete('memory_l1_field_history', "DELETE FROM memory_l1_field_history WHERE task_id IN (SELECT id FROM memory_l1_tasks WHERE session_id = ANY(%s))", (SIDS,))
    delete('memory_l1_fields', "DELETE FROM memory_l1_fields WHERE session_id = ANY(%s)", (SIDS,))
    delete('memory_l1_tasks', "DELETE FROM memory_l1_tasks WHERE session_id = ANY(%s)", (SIDS,))
    # L0
    delete('memory_l0_assembly_snapshots', "DELETE FROM memory_l0_assembly_snapshots WHERE session_id = ANY(%s)", (SIDS,))
    # L3 facts（memtest + canary + spirit 测试 2 条）
    delete('memory_facts', "DELETE FROM memory_facts WHERE agent_id = %s OR source_kind = 'memory_canary' OR id = ANY(%s)", (MEMTEST_AGENT, SPIRIT_TEST_FACTS))
    # L2 episodes（memtest + spirit 测试 1 条）
    delete('memory_episodes', "DELETE FROM memory_episodes WHERE agent_id = %s OR id = %s", (MEMTEST_AGENT, SPIRIT_TEST_EPISODE))
    # L4 relations -> entities
    delete('memory_relations', "DELETE FROM memory_relations WHERE scope_id = %s OR id = %s", (MEMTEST_AGENT, SPIRIT_TEST_RELATION))
    delete('memory_entities', "DELETE FROM memory_entities WHERE scope_id = %s OR id = %s", (MEMTEST_AGENT, SPIRIT_TEST_ENTITY))
    # 聊天链路
    delete('steps_v2', "DELETE FROM steps_v2 WHERE session_id = ANY(%s)", (SIDS,))
    delete('tasks_v2', "DELETE FROM tasks_v2 WHERE session_id = ANY(%s)", (SIDS,))
    delete('session_turns', "DELETE FROM session_turns WHERE session_id = ANY(%s)", (SIDS,))
    delete('session_runs', "DELETE FROM session_runs WHERE session_id = ANY(%s)", (SIDS,))
    # 日志/监控
    delete('flow_log_events', "DELETE FROM flow_log_events WHERE session_id = ANY(%s)", (SIDS,))
    delete('monitor_trace_spans', "DELETE FROM monitor_trace_spans WHERE trace_id IN (SELECT id FROM monitor_traces WHERE session_id = ANY(%s))", (SIDS,))
    delete('monitor_traces', "DELETE FROM monitor_traces WHERE session_id = ANY(%s)", (SIDS,))
    # 会话
    delete('sessions', "DELETE FROM sessions WHERE id = ANY(%s)", (SIDS,))
    # agent
    delete('agent_prompt_files', "DELETE FROM agent_prompt_files WHERE agent_id = %s", (MEMTEST_AGENT,))
    delete('agent_runtime_settings', "DELETE FROM agent_runtime_settings WHERE agent_id = %s", (MEMTEST_AGENT,))
    delete('agents', "DELETE FROM agents WHERE id = %s", (MEMTEST_AGENT,))
    # 知识库
    delete('knowledge_chunks', "DELETE FROM knowledge_chunks WHERE collection_id = ANY(%s)", (TEST_COLLECTIONS,))
    delete('knowledge_links', "DELETE FROM knowledge_links WHERE collection_id = ANY(%s)", (TEST_COLLECTIONS,))
    delete('knowledge_documents', "DELETE FROM knowledge_documents WHERE collection_id = ANY(%s)", (TEST_COLLECTIONS,))
    delete('knowledge_collections', "DELETE FROM knowledge_collections WHERE id = ANY(%s)", (TEST_COLLECTIONS,))
    # 操作日志全清
    delete('memory_action_log', "DELETE FROM memory_action_log")

    conn.commit()
    print("\n[commit] OK — 全部删除已提交")
except Exception as e:
    conn.rollback()
    print(f"\n[rollback] ERROR: {e}")
    sys.exit(1)
finally:
    conn.close()
