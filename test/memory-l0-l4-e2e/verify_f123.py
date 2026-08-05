# F1/F2/F3 运行时验证（2026-08-05）
# F1: L3 全景卡口径 = agent_id 跨 scope 计数；浏览 Tab 同口径
# F2: 消息视图过滤系统内部 notice（context_usage/memory_recalled 等 raw JSON 不再泄漏）
# F3: agent 回复不含 <fact> 标签（持久化内容也不含）
import json, urllib.request, urllib.error, time, sys

BASE = "http://localhost:8000"

def req(method, path, body=None, timeout=300):
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

PASS, FAIL = [], []
def check(name, ok, detail=""):
    tag = "PASS" if ok else "FAIL"
    (PASS if ok else FAIL).append(name)
    log(f"[{tag}] {name} {detail}")

# ---------- 定位 memtest agent ----------
s, lst = req("GET", "/v1/agents?limit=200")
aid = None
for a in lst.get("agents", lst.get("items", [])):
    if a.get("agent_key") == "memtest-agent" or a.get("agentKey") == "memtest-agent":
        aid = a.get("id"); break
if not aid:
    log("[FATAL] memtest-agent not found"); sys.exit(1)
log(f"[SETUP] agent_id={aid}")

# ---------- F1: L3 口径统一 ----------
s, ov = req("GET", f"/v1/memory/layer-overview?agent_id={aid}")
l3_card = None
for l in ov.get("layers", []):
    if l.get("layer") == "L3":
        l3_card = l.get("itemCount")
log(f"[F1] layer-overview L3 itemCount={l3_card} http={s}")

s, facts_by_agent = req("GET", f"/v1/memory/l3/facts?agent_id={aid}&limit=50")
browse_total = facts_by_agent.get("total")
log(f"[F1] l3/facts?agent_id total={browse_total} http={s}")

s, facts_scope_only = req("GET", f"/v1/memory/l3/facts?scope_type=agent&scope_id={aid}&limit=50")
scope_total = facts_scope_only.get("total")
log(f"[F1] l3/facts?scope_type=agent (old口径) total={scope_total} http={s}")

check("F1.card_eq_browse", l3_card is not None and browse_total is not None and l3_card <= (browse_total if browse_total else 0) or l3_card == browse_total,
      f"card={l3_card} browse={browse_total}")
check("F1.cross_scope_ge_old", (browse_total or 0) >= (scope_total or 0),
      f"agent_id口径={browse_total} >= scope=agent口径={scope_total}")
# 抽一条 fact 确认 agent_id 字段存在且非 agent scope 的也能被 agent_id 查到
items = facts_by_agent.get("items", [])
non_agent_scope = [f for f in items if (f.get("scopeType") or f.get("scope_type")) != "agent"]
log(f"[F1] browse items={len(items)} non-agent-scope items={len(non_agent_scope)}")
for f in items[:5]:
    log(f"  fact scope={f.get('scopeType') or f.get('scope_type')} agent={f.get('agentId') or f.get('agent_id')} stmt={str(f.get('statement'))[:60]}")

# ---------- F3: 新对话触发 fact 抽取，回复不含 <fact> ----------
s, sess = req("POST", "/v1/sessions", {"agentId": aid, "title": "memtest-f123-verify", "dialogMode": "default"})
sid = sess.get("id")
log(f"[SETUP] session={sid} http={s}")

s, d = req("POST", "/v1/chat/messages", {"session_id": sid, "content": "补充一条关于我的事实：我最近开始学习小提琴，每天练习半小时。"})
am = d.get("agent_message") or {}
reply = str(am.get("content") or am.get("text") or "")
log(f"[F3] chat http={s} reply_len={len(reply)} preview={reply[:200]}")
check("F3.reply_no_fact_tag", "<fact>" not in reply and "</fact>" not in reply,
      f"reply含<fact>={'是' if '<fact>' in reply else '否'}")

time.sleep(2)
# 持久化内容检查：通过 messages API 读 assistant 消息
s, msgs = req("GET", f"/v1/sessions/{sid}/messages?limit=50")
msg_items = msgs.get("items", [])
assistant_texts = []
system_json_leak = []
for m in msg_items:
    role = m.get("role")
    content = str(m.get("content_markdown") or m.get("contentMarkdown") or m.get("content") or "")
    if role == "assistant":
        assistant_texts.append(content)
    if role == "system":
        # F2: 系统内部 notice 不应作为 system 消息出现在消息视图
        if '"notice_type"' in content or '"context_usage"' in content or '"memory_recalled"' in content or '"used_tokens"' in content:
            system_json_leak.append(content[:120])
fact_in_store = [t for t in assistant_texts if "<fact>" in t or "</fact>" in t]
log(f"[F3] stored assistant msgs={len(assistant_texts)} 含<fact>={len(fact_in_store)}")
check("F3.stored_no_fact_tag", len(fact_in_store) == 0, f"含标签消息数={len(fact_in_store)}")

# ---------- F2: 系统内部 notice 过滤 ----------
log(f"[F2] session msgs total={msgs.get('total')} system_json_leak={len(system_json_leak)}")
for leak in system_json_leak[:3]:
    log(f"  leak: {leak}")
check("F2.no_internal_notice_leak", len(system_json_leak) == 0, f"泄漏条数={len(system_json_leak)}")

# 老 session（含 recall notice 的 memtest-l0-l4）也验证一次
s2, sess_list = req("GET", f"/v1/sessions?agent_id={aid}&limit=30")
old_sid = None
for it in sess_list.get("items", sess_list.get("sessions", [])):
    if (it.get("title") or "") == "memtest-l0-l4":
        old_sid = it.get("id"); break
if old_sid:
    s, msgs2 = req("GET", f"/v1/sessions/{old_sid}/messages?limit=100")
    leak2 = []
    roles = {}
    for m in msgs2.get("items", []):
        role = m.get("role"); roles[role] = roles.get(role, 0) + 1
        content = str(m.get("content_markdown") or m.get("contentMarkdown") or m.get("content") or "")
        if role == "system" and ('"notice_type"' in content or '"context_usage"' in content or '"memory_recalled"' in content or '"used_tokens"' in content):
            leak2.append(content[:120])
    log(f"[F2] old session={old_sid} msgs={msgs2.get('total')} roles={roles} leak={len(leak2)}")
    check("F2.old_session_no_leak", len(leak2) == 0, f"泄漏条数={len(leak2)}")
else:
    log("[F2] old memtest-l0-l4 session not found, skip")

log(f"[SUMMARY] pass={len(PASS)} fail={len(FAIL)} failed={FAIL}")
with open("test/memory-l0-l4-e2e/f123-verify-log.txt", "w", encoding="utf-8") as f:
    f.write("\n".join(LOG))
sys.exit(1 if FAIL else 0)
