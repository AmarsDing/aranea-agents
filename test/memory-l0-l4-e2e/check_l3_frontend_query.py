# 模拟前端记忆中心 L3 Tab 查询（不带 scope_type）
import json, urllib.request, urllib.error

BASE = "http://localhost:8000"

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

# 前端 loadFacts: keyword/scope_type/status 均可选，limit=50
s, d = req("GET", "/v1/memory/l3/facts?limit=50")
items = d.get("items", [])
print(f"[L3 facts no-scope-filter] http={s} total={d.get('total')} items={len(items)}")
for f in items[:8]:
    print(f"  [{f.get('scopeType') or f.get('scope_type')}] {str(f.get('statement'))[:60]} status={f.get('status')} pii={f.get('piiFlag') or f.get('pii_flag')}")

# scope_type=user 过滤
s, d = req("GET", "/v1/memory/l3/facts?scope_type=user&limit=50")
print(f"\n[L3 facts scope=user] total={d.get('total')} items={len(d.get('items', []))}")

# layer-overview 全 agent
s, d = req("GET", "/v1/memory/layer-overview")
print(f"\n[layer-overview all] http={s}")
for l in d.get("layers", []):
    print(f"  {l['layer']}: count={l.get('itemCount')} today={l.get('todayAdded')} health={l.get('health')}")
