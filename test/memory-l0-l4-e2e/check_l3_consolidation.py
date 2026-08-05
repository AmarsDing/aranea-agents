# L3 consolidation 排查：episode consolidationStatus + facts 表直查
import json, urllib.request, urllib.error

BASE = "http://localhost:8000"
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

s, d = req("GET", f"/v1/memory/episodes?agent_id={AID}&limit=10")
for e in d.get("items", []):
    print(f"ep {str(e.get('id'))[:8]}: consolidationStatus={e.get('consolidationStatus')!r} consolidatedL3Count={e.get('consolidatedL3Count')} kind={e.get('episodeKind')} title={str(e.get('title'))[:40]}")

# 全量 facts（不过滤 agent）确认 consolidation 是否全局停滞
s, d = req("GET", f"/v1/memory/l3/facts?scope_type=agent&scope_id={AID}")
print(f"\nfacts(agent={AID[:8]}): {len(d.get('items', []))}")

# 直接查 DB
import subprocess
sql = """
SELECT consolidation_status, count(*) FROM memory_episodes GROUP BY consolidation_status;
"""
r = subprocess.run(["psql", "-h", "localhost", "-p", "5432", "-U", "postgres", "-d", "aranea_dev", "-t", "-A", "-c", sql], capture_output=True, text=True, env={"PGPASSWORD": "postgres"})
print("\n[DB] memory_episodes.consolidation_status distribution:")
print(r.stdout or r.stderr)

sql2 = """
SELECT scope_type, count(*) FROM memory_facts GROUP BY scope_type;
"""
r2 = subprocess.run(["psql", "-h", "localhost", "-p", "5432", "-U", "postgres", "-d", "aranea_dev", "-t", "-A", "-c", sql2], capture_output=True, text=True, env={"PGPASSWORD": "postgres"})
print("[DB] memory_facts by scope_type:")
print(r2.stdout or r2.stderr)

sql3 = """
SELECT id, scope_type, scope_id, substr(statement,1,60), created_at FROM memory_facts ORDER BY created_at DESC LIMIT 8;
"""
r3 = subprocess.run(["psql", "-h", "localhost", "-p", "5432", "-U", "postgres", "-d", "aranea_dev", "-t", "-A", "-c", sql3], capture_output=True, text=True, env={"PGPASSWORD": "postgres"})
print("[DB] latest memory_facts:")
print(r3.stdout or r3.stderr)
