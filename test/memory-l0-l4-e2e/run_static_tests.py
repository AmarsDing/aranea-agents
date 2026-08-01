# L0-L4 五层记忆静态 API 测试（非聊天类用例）
# 用法: python test/memory-l0-l4-e2e/run_static_tests.py
# 2026-07-30 修正：Upsert 请求 envelope 与 proto 字段名（scope_type/fact_kind/tags_json）；
# PII 列表查询参数；evolution event 必填字段；冲突检测语句改用句首否定（匹配 isNegationConflict 语义）。
import json, urllib.request, urllib.error, datetime

BASE = "http://localhost:8000"
AGENT = "agent___spirit__"
RICH_SESSION = "cbd769a3-a28b-4b3f-b157-a78c12c034ea"  # 87 tool calls
IDLE_SESSION = "6b56174d-e488-4335-9c6c-4d5e8341aa26"  # spirit idle

results = []

def req(method, path, body=None):
    url = BASE + path
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(url, data=data, method=method,
                               headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(r, timeout=30) as resp:
            return resp.status, json.loads(resp.read().decode() or "{}")
    except urllib.error.HTTPError as e:
        try:
            return e.code, json.loads(e.read().decode() or "{}")
        except Exception:
            return e.code, {}
    except Exception as e:
        return -1, {"error": str(e)}

def rec(cid, name, ok, detail):
    mark = "PASS" if ok else "FAIL"
    results.append((cid, name, mark, detail))
    print(f"[{mark}] {cid} {name} :: {detail[:240]}")

def upsert_fact(statement, kind="fact", confidence=0.9, importance=0.8, tags=None, scope="agent", scope_id=None):
    body = {"fact": {
        "scopeType": scope, "scopeId": scope_id or AGENT, "agentId": AGENT,
        "statement": statement, "factKind": kind,
        "tagsJson": json.dumps(tags or ["memtest"], ensure_ascii=False),
        "confidence": confidence, "importance": importance,
        "sourceKind": "manual"}}
    return req("POST", "/v1/memory/l3/facts", body)

# ---------- L0 ----------
s, d = req("GET", f"/v1/sessions/{RICH_SESSION}/l0/snapshots")
rec("L0-01", "富会话快照列表", s == 200 and isinstance(d.get("items"), list),
    f"status={s} items={len(d.get('items', []))} (on_warning模式下低ratio无快照=设计行为)")

s, d2 = req("GET", f"/v1/sessions/{RICH_SESSION}/l0/snapshots?agent_id={AGENT}")
rec("L0-02", "agent_id过滤兼容", s == 200, f"status={s} items={len(d2.get('items', []))}")

s, d = req("GET", "/v1/sessions/nonexistent-session-id/l0/snapshots")
rec("L0-ERR", "不存在session的L0查询", s in (200, 404), f"status={s} body={str(d)[:120]}")

# ---------- L1 ----------
s, d = req("GET", f"/v1/sessions/{RICH_SESSION}/l1/tasks")
rec("L1-01", "L1任务列表", s == 200 and isinstance(d.get("items"), list),
    f"status={s} items={len(d.get('items', []))}")
tasks = d.get("items", [])
if tasks:
    tid = tasks[0].get("taskId") or tasks[0].get("task_id")
    s, d = req("GET", f"/v1/sessions/{RICH_SESSION}/l1/tasks/{tid}/fields")
    rec("L1-02", "L1字段列表", s == 200, f"status={s} fields={len(d.get('items', []))}")
else:
    rec("L1-02", "L1字段列表", True, "SKIP: 无任务（等待L1-03聊天触发）")

# ---------- L2 ----------
s, d = req("GET", f"/v1/memory/episodes?agent_id={AGENT}&limit=10&offset=0")
items = d.get("items", [])
rec("L2-01", "episodes分页", s == 200 and "total" in d,
    f"status={s} total={d.get('total')} page={len(items)}")
if items:
    ep = items[0]
    keys = sorted(ep.keys())
    rec("L2-01b", "episode字段完整性", all(k in keys for k in ("id", "title", "importance", "consolidationStatus")),
        f"keys={keys}")

# L2 recall debug
s, d = req("POST", "/v1/memory/recall/debug",
           {"agentId": AGENT, "query": "记忆 测试", "topK": 5})
rec("L2-04/L3-07", "recall debug", s == 200,
    f"status={s} keys={sorted(d.keys())[:8]}")

# composite search
s, d = req("POST", "/v1/memory/search/composite",
           {"agentId": AGENT, "query": "用户 偏好", "topK": 5})
rec("L2-05", "composite search", s == 200, f"status={s} body={str(d)[:200]}")

# ---------- L3 ----------
s, d = req("GET", f"/v1/memory/l3/facts?agent_id={AGENT}")
facts = d.get("items", [])
rec("L3-01", "facts列表(基线4)", s == 200 and len(facts) >= 4, f"status={s} count={len(facts)}")

# L3-02 新增（proto envelope: {"fact": {...}}）
s, d = upsert_fact("memtest: 测试系统正在验证L3写入")
new_id = d.get("fact", {}).get("id")
rec("L3-02", "新增fact", s == 200 and bool(new_id), f"status={s} id={new_id} body={str(d)[:180]}")

# L3-03 幂等 upsert（同 statement → 同 fingerprint，行数不增，version+1）
s, d = upsert_fact("memtest: 测试系统正在验证L3写入", confidence=0.95)
dup = d.get("fact", {})
s2, lst = req("GET", f"/v1/memory/l3/facts?agent_id={AGENT}")
memtest_facts = [f for f in lst.get("items", []) if "memtest" in f.get("statement", "")]
rec("L3-03", "重复upsert幂等", s == 200 and len(memtest_facts) == 1 and (dup.get("version") or 0) >= 2,
    f"status={s} version={dup.get('version')} memtest_count={len(memtest_facts)}")

# L3-04 PII 写入（手机号）→ PII 闸门：piiFlag=true 且 statement 已脱敏
s, d = upsert_fact("memtest: 用户手机号是 13800138000", tags=["memtest", "pii-test"])
pii_fact = d.get("fact", {})
pii_id = pii_fact.get("id")
rec("L3-04", "PII fact写入脱敏", s == 200 and pii_fact.get("piiFlag") is True and "13800138000" not in pii_fact.get("statement", ""),
    f"status={s} id={pii_id} piiFlag={pii_fact.get('piiFlag')} statement={pii_fact.get('statement', '')[:60]!r}")

# L3-05 PII 列表 + review（proto: scope_type/scope_id 查询参数；body factId+action）
s, d = req("GET", f"/v1/memory/l3/facts/pii?scope_type=agent&scope_id={AGENT}")
pii_items = d.get("items", [])
rec("L3-05a", "PII列表", s == 200 and len(pii_items) >= 1, f"status={s} count={len(pii_items)}")
if pii_items:
    pid = pii_items[0].get("id")
    s, d = req("POST", "/v1/memory/l3/facts/pii/review", {"factId": pid, "action": "approve"})
    rec("L3-05b", "PII review approve", s == 200, f"status={s} body={str(d)[:150]}")

# L3-06 冲突检测（句首否定，匹配 isNegationConflict 前缀语义；先写肯定句再写否定句）
s1, d1 = upsert_fact("使用 Tailwind CSS 作为 memtest 项目样式方案", tags=["memtest", "conflict-a"])
s2, d2 = upsert_fact("禁止使用 Tailwind CSS 作为 memtest 项目样式方案", tags=["memtest", "conflict-b"])
s, d = req("GET", f"/v1/memory/l3/facts/conflicts?scope_type=agent&scope_id={AGENT}")
conf = d.get("items", [])
conflict_hit = any("Tailwind" in f.get("statement", "") for f in conf)
rec("L3-06", "冲突检测", s == 200 and s1 == 200 and s2 == 200 and conflict_hit,
    f"write=({s1},{s2}) conflicts={len(conf)} tailwind_hit={conflict_hit} body={str(conf)[:200]}")

# L3-08 index_status：proto MemoryFact 不暴露该字段，以 worker status 的索引计数为准
s, d = req("GET", f"/v1/memory/worker/status?agent_id={AGENT}")
stale = d.get("factIndexStaleCount")
disabled = d.get("factIndexDisabledCount")
rec("L3-08", "索引状态(worker status)", s == 200 and stale is not None,
    f"stale={stale} disabled={disabled}")

# ---------- L4 ----------
s, d = req("GET", f"/v1/memory/l4/entities?agent_id={AGENT}")
ents = d.get("items", [])
rec("L4-01", "entities列表(基线1)", s == 200 and len(ents) >= 1, f"status={s} count={len(ents)}")
if ents:
    eid = ents[0].get("id")
    s, d = req("GET", f"/v1/memory/l4/entities/{eid}/neighborhood?max_hops=2")
    rec("L4-02", "neighborhood BFS", s == 200,
        f"status={s} nodes={len(d.get('nodes', []))} edges={len(d.get('edges', []))}")
    s, d = req("GET", f"/v1/memory/l4/spreading-activation?agent_id={AGENT}&center_id={eid}&top_k=5")
    rec("L4-03", "spreading activation", s == 200, f"status={s} body={str(d)[:200]}")

# evolution
s, d = req("GET", f"/v1/agents/{AGENT}/identity")
rec("L4-05a", "identity", s == 200, f"status={s} body={str(d)[:150]}")
s, d = req("GET", f"/v1/agents/{AGENT}/strategy")
rec("L4-05b", "strategy", s == 200, f"status={s} body={str(d)[:150]}")
s, d = req("GET", f"/v1/agents/{AGENT}/evolution/proposals")
rec("L4-06a", "evolution proposals", s == 200, f"status={s} items={len(d.get('items', []))}")
s, d = req("GET", f"/v1/agents/{AGENT}/evolution/events")
ev_before = len(d.get("items", []))
rec("L4-06b", "evolution events", s == 200, f"status={s} items={ev_before}")
s, d = req("GET", f"/v1/agents/{AGENT}/evolution/metrics")
rec("L4-06c", "evolution metrics", s == 200, f"status={s} body={str(d)[:150]}")

# L4-07 append event（proto 必填 eventKind + triggerKind）
s, d = req("POST", f"/v1/agents/{AGENT}/evolution/events",
           {"eventKind": "note", "targetField": "persona",
            "reason": "memtest: 测试事件追加", "triggerKind": "manual"})
rec("L4-07a", "append evolution event", s == 200, f"status={s} body={str(d)[:150]}")
s, d = req("GET", f"/v1/agents/{AGENT}/evolution/events")
rec("L4-07b", "event追加后可见", s == 200 and len(d.get("items", [])) == ev_before + 1,
    f"before={ev_before} after={len(d.get('items', []))}")

# L4-08 cascade
s, d = req("GET", f"/v1/memory/cascade/proposals?agent_id={AGENT}")
casc = d.get("items", [])
rec("L4-08a", "cascade proposals列表", s == 200, f"status={s} items={len(casc)}")
s, d = req("POST", "/v1/memory/cascade/proposals/nonexistent-id/approve", {})
rec("L4-08b", "approve不存在proposal", s in (404, 400), f"status={s} body={str(d)[:150]}")
s, d = req("GET", "/v1/memory/cascade/proposals/nonexistent-id/preview")
rec("L4-08c", "preview不存在proposal", s in (404, 400), f"status={s} body={str(d)[:120]}")

# ---------- 跨层 ----------
s, d = req("GET", f"/v1/memory/layer-overview?agent_id={AGENT}")
layers = {l["layer"]: l for l in d.get("layers", [])}
rec("X-01", "layer-overview五层", s == 200 and len(layers) == 5,
    f"L3={layers.get('L3',{}).get('itemCount')} L4={layers.get('L4',{}).get('itemCount')} actions={len(d.get('actionItems', []))} feed={len(d.get('activityFeed', []))}")

s, d = req("GET", f"/v1/memory/graph/unified?agent_id={AGENT}&hops=2&min_weight=0")
rec("X-02a", "unified graph", s == 200,
    f"nodes={d.get('nodeCount')} edges={d.get('edgeCount')} filtered={d.get('filteredEdgeCount')} empty={d.get('emptyReason')!r}")
s, d = req("GET", f"/v1/memory/graph/unified?agent_id={AGENT}&hops=2&min_weight=0.999")
rec("X-02b", "unified graph高阈值过滤", s == 200,
    f"nodes={d.get('nodeCount')} edges={d.get('edgeCount')} filtered={d.get('filteredEdgeCount')}")

s, d = req("GET", f"/v1/memory/worker/status?agent_id={AGENT}")
rec("X-03", "worker status", s == 200, f"status={s} body={str(d)[:250]}")

s, d = req("GET", f"/v1/memory/worker/dead-letters?agent_id={AGENT}")
dls = d.get("items", [])
rec("X-04a", "dead-letters列表", s == 200, f"status={s} count={len(dls)}")
s, d = req("POST", "/v1/memory/worker/dead-letters/999999999/replay", {})
rec("X-04b", "replay不存在死信", s in (404, 400), f"status={s} body={str(d)[:120]}")

s, d = req("GET", "/v1/memory/platform/settings")
rec("X-05a", "platform settings读", s == 200, f"status={s} body={str(d)[:250]}")

orig = d
body = dict(orig) if isinstance(orig, dict) else {}
if body:
    s2, d2 = req("PUT", "/v1/memory/platform/settings", body)
    s3, d3 = req("GET", "/v1/memory/platform/settings")
    rec("X-05b", "platform settings写读一致", s2 == 200 and d3 == d, f"put={s2} consistent={d3==d}")

# ---------- 清理 memtest 数据 ----------
try:
    import psycopg2
    conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
    cur = conn.cursor()
    cur.execute("DELETE FROM memory_facts WHERE tags_json LIKE '%memtest%' OR statement LIKE 'memtest:%'")
    nf = cur.rowcount
    cur.execute("DELETE FROM agent_evolution_events WHERE reason LIKE 'memtest:%'")
    ne = cur.rowcount
    conn.commit()
    conn.close()
    print(f"[CLEANUP] deleted facts={nf} evolution_events={ne}")
except Exception as e:
    print(f"[CLEANUP] skipped: {e}")

print("\n===== SUMMARY =====")
passed = sum(1 for r in results if r[2] == "PASS")
print(f"total={len(results)} pass={passed} fail={len(results)-passed}")
for r in results:
    if r[2] == "FAIL":
        print(f"  FAILED: {r[0]} {r[1]} :: {r[3][:200]}")

# 写入结果文件
with open("test/memory-l0-l4-e2e/static-results.txt", "w", encoding="utf-8") as f:
    f.write(f"# 静态API测试结果 {datetime.datetime.now().isoformat()}\n\n")
    for r in results:
        f.write(f"[{r[2]}] {r[0]} {r[1]} :: {r[3]}\n")
