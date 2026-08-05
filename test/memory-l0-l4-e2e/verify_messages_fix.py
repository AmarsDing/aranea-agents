# 验证 sessionActivityLister 修复：ListSessionMessages 必须含 role=user 消息
import json, urllib.request, urllib.error, sys

BASE = "http://localhost:8000"
SID = "e16fe63c-26b3-4140-b6e8-a48f65d01924"
AID = "f2e5a24ab0756d6413d6a1a3"

def req(method, path, body=None, timeout=60):
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
            return e.code, {"raw": raw[:500]}
    except Exception as e:
        return -1, {"error": str(e)}

# 1. ListSessionMessages — 前端聊天历史依赖的 RPC
s, d = req("GET", f"/v1/sessions/{SID}/messages?limit=50")
print(f"[ListSessionMessages] http={s}")
items = d.get("items", d.get("messages", []))
print(f"  total items: {len(items)}")
roles = {}
for m in items:
    role = m.get("role") or m.get("side") or "?"
    roles[role] = roles.get(role, 0) + 1
print(f"  role distribution: {roles}")
user_msgs = [m for m in items if (m.get("role") == "user")]
print(f"  user messages: {len(user_msgs)}")
for m in user_msgs[:5]:
    content = str(m.get("content") or m.get("contentMarkdown") or "")[:80]
    print(f"    - {content}")
if not user_msgs:
    print("  [FAIL] no user messages in ListSessionMessages — fix NOT effective")
else:
    print("  [OK] user messages present — sessionActivityLister fix effective")

# 2. 单条 chat 响应形状（排查 agent_reply 为空）
s, d = req("POST", "/v1/chat/messages", {"session_id": SID, "content": "你好，请回复一个字：好"}, timeout=300)
print(f"\n[chat] http={s} top-level keys={sorted(d.keys())}")
print(f"  raw preview: {str(d)[:600]}")

# 3. L3 consolidation 排查：episodes 有 5 条但 facts=0
s, d = req("GET", f"/v1/memory/episodes?agent_id={AID}&limit=10")
eps = d.get("items", [])
print(f"\n[L2 episodes] count={len(eps)}")
for e in eps[:5]:
    print(f"  ep: id={str(e.get('id'))[:16]} imp={e.get('importance')} consolidated={e.get('consolidated') or e.get('consolidatedAt')} status={e.get('status')} keys={sorted(e.keys())[:12]}")
