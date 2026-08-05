# 验证 L0 快照中的 L3/L4 注入计数 + MSG3 回忆答案
import json, urllib.request, urllib.error

BASE = "http://localhost:8000"
SID = "e16fe63c-26b3-4140-b6e8-a48f65d01924"

def req(method, path, body=None, timeout=60):
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(BASE + path, data=data, method=method,
                               headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(r, timeout=timeout) as resp:
            return resp.status, json.loads(resp.read().decode() or "{}")
    except urllib.error.HTTPError as e:
        return e.code, {"raw": e.read().decode()[:300]}
    except Exception as e:
        return -1, {"error": str(e)}

s, d = req("GET", f"/v1/sessions/{SID}/l0/snapshots")
snaps = d.get("items", [])
print(f"snapshots: {len(snaps)}")
for i, sn in enumerate(snaps[:5]):
    print(f"  snap{i}: l1Fields={sn.get('l1FieldCount')} l1Tokens={sn.get('l1TokenEstimate')} "
          f"l3Chunks={sn.get('l3ChunkCount')} l3Tokens={sn.get('l3TokenEstimate')} "
          f"l4Paths={sn.get('l4PathCount')} l4Tokens={sn.get('l4TokenEstimate')} "
          f"promptActual={sn.get('promptTokenActual')} run={str(sn.get('runId'))[:8]}")

# MSG3 的 agent 回复（从 messages 里看）
s, d = req("GET", f"/v1/sessions/{SID}/messages?limit=50")
items = d.get("items", d.get("messages", []))
asst = [m for m in items if m.get("role") == "assistant"]
print(f"\nassistant messages: {len(asst)}")
for m in asst:
    c = str(m.get("content") or m.get("contentMarkdown") or m.get("content_markdown") or "")
    print(f"  - {c[:200]}")
