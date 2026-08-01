# 聊天触发全链路记忆测试（L0快照/L1工具/L4抽取/prompt注入/L2,L3 worker）
import json, urllib.request, urllib.error, time, sys

BASE = "http://localhost:8000"

def req(method, path, body=None, timeout=180):
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(BASE + path, data=data, method=method,
                               headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(r, timeout=timeout) as resp:
            return resp.status, json.loads(resp.read().decode() or "{}")
    except urllib.error.HTTPError as e:
        raw = e.read().decode()
        try:
            return e.code, json.loads(raw or "{}")
        except Exception:
            return e.code, {"raw": raw[:300]}
    except Exception as e:
        return -1, {"error": str(e)}

LOG = []
def log(msg):
    print(msg, flush=True)
    LOG.append(msg)

# ---------- 1. 创建测试 agent（l0SnapshotMode=always，全层注入开） ----------
settings = {
    "memoryEnabled": True,
    "l0RecentWindowTurns": 12,
    "l0SummaryThreshold": 0.6,
    "l0SummaryKeepTurns": 4,
    "l0TruncateStrategy": "summary",
    "l0InjectL1": True, "l0InjectL2": True, "l0InjectL3": True, "l0InjectL4": True,
    "l0L3MaxChunks": 5, "l0L4MaxPaths": 3,
    "l0SnapshotMode": "always", "l0SnapshotEnabled": True,
    "l1Enabled": True, "l1BudgetTokens": 8192, "l1FieldMaxTokens": 2048,
    "l1HistoryKeepRevisions": 10, "l1ArchiveOnIdleMinutes": 60,
    "l2EpisodeEnabled": True, "l2EpisodeMinImportance": 0.3,
    "l2IndexEnabled": True, "l2RecallEnabled": True, "l2RecallMax": 3,
    "l2RetentionDays": 90, "l2ArchiveAfterDays": 30,
    "l3Enabled": True, "l3RecallTopK": 5, "l3RecallMinScore": 0.35,
    "l3RecallScopesJson": '["agent","user","team","workspace"]',
    "l3DecayIntervalHours": 24, "l3ArchiveThreshold": 0.2, "l3MaxPerRecallChars": 1500,
    "l4Enabled": True, "l4GraphInjectNeighbors": True, "l4GraphMaxNeighbors": 6,
    "l4GraphMaxHops": 2, "l4IdentityInject": True, "l4StrategyInject": False,
}
s, agent = req("POST", "/v1/agents", {
    "agent_key": "memtest-agent", "display_name": "记忆测试Agent",
    "provider": "deepseek", "model": "deepseek-v4-flash",
    "agent_description": "你是一个乐于助人的测试助手。当用户告诉你个人信息时记住它们；当被要求记录任务状态时使用 working memory 工具。",
    "settings": settings,
})
if s not in (200, 409):
    log(f"[FATAL] create agent: {s} {str(agent)[:300]}")
    sys.exit(1)
aid = agent.get("id")
log(f"[SETUP] agent created: id={aid} status={s}")

# ---------- 2. 创建 session ----------
s, sess = req("POST", "/v1/sessions", {"agentId": aid, "title": "memtest-l0-l4", "dialogMode": "default"})
sid = sess.get("id")
log(f"[SETUP] session created: id={sid} status={s}")
if not sid:
    log(f"[FATAL] create session: {str(sess)[:300]}")
    sys.exit(1)

def chat(content, tag):
    s, d = req("POST", "/v1/chat/messages", {"session_id": sid, "content": content}, timeout=300)
    am = d.get("agent_message") or {}
    txt = ""
    if isinstance(am, dict):
        txt = str(am.get("content") or am.get("text") or "")[:400]
    log(f"[{tag}] http={s} agent_reply={txt[:300]}")
    return s, d

# ---------- 3. 消息1：个人信息（触发 L4 中文实体抽取） ----------
chat("我叫测试用户张三，我喜欢喝咖啡，我养了一只叫小白的猫。", "MSG1-L4-extract")

# ---------- 4. 消息2：要求使用 working memory（触发 L1 工具） ----------
chat("请使用 working memory 工具记录：当前任务目标是验证L0-L4五层记忆系统的全部功能，关键决策是使用真实对话触发各层写入。", "MSG2-L1-tool")

# ---------- 5. 消息3：记忆回读（验证 prompt 注入 L3/L4 cue） ----------
chat("请直接回答：我叫什么名字？我喜欢喝什么？我的猫叫什么？", "MSG3-recall")

# ---------- 6. 验证各层 ----------
time.sleep(3)

# L0 快照
s, d = req("GET", f"/v1/sessions/{sid}/l0/snapshots")
snaps = d.get("items", [])
log(f"[L0-04] snapshots after chat: count={len(snaps)}")
if snaps:
    latest = snaps[0]
    keys = sorted(latest.keys())
    log(f"[L0-03] snapshot keys={keys}")
    log(f"[L0-03] usage={latest.get('contextUsage') or latest.get('context_usage')} warnings={latest.get('warningCodesJson') or latest.get('warning_codes_json')}")
    seg = latest.get("segmentsJson") or latest.get("segments_json") or ""
    log(f"[L0-03] segments len={len(str(seg))} preview={str(seg)[:400]}")

# L1 tasks
s, d = req("GET", f"/v1/sessions/{sid}/l1/tasks")
tasks = d.get("items", [])
log(f"[L1-03] l1 tasks: count={len(tasks)}")
for t in tasks[:3]:
    log(f"  task: {str(t)[:200]}")
    tid = t.get("taskId") or t.get("task_id")
    if tid:
        s2, d2 = req("GET", f"/v1/sessions/{sid}/l1/tasks/{tid}/fields")
        log(f"  [L1-02] fields: count={len(d2.get('items', []))} sample={str(d2.get('items', []))[:300]}")

# L4 entities
s, d = req("GET", f"/v1/memory/l4/entities?scope_type=agent&scope_id={aid}")
ents = d.get("items", [])
log(f"[L4-04] entities: count={len(ents)}")
for e in ents[:5]:
    log(f"  entity: name={e.get('name')} type={e.get('entityType') or e.get('entity_type')} ws={e.get('workspaceId')!r}")

# L2 episodes（等待 worker）
for attempt in range(3):
    s, d = req("GET", f"/v1/memory/episodes?agent_id={aid}&limit=10")
    eps = d.get("items", [])
    log(f"[L2-02] episodes attempt{attempt}: total={d.get('total')} items={len(eps)}")
    if eps:
        break
    time.sleep(10)

# L3 facts
s, d = req("GET", f"/v1/memory/l3/facts?scope_type=agent&scope_id={aid}")
facts = d.get("items", [])
log(f"[L3] facts for new agent: count={len(facts)}")
for f in facts[:5]:
    log(f"  fact: {str(f.get('statement'))[:80]} pii={f.get('piiFlag')}")

# layer-overview 对比
s, d = req("GET", f"/v1/memory/layer-overview?agent_id={aid}")
for l in d.get("layers", []):
    log(f"[X-01-new] {l['layer']}: count={l.get('itemCount')} today={l.get('todayAdded')} health={l.get('health')}")

with open("test/memory-l0-l4-e2e/chat-test-log.txt", "w", encoding="utf-8") as f:
    f.write("\n".join(LOG))
log(f"[DONE] agent_id={aid} session_id={sid}")
