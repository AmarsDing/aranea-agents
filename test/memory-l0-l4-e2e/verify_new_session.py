# 验证新 session 的 L0 快照注入计数 + agent 回复 + L3 facts DB 直查
import json, urllib.request, urllib.error
import psycopg2

BASE = "http://localhost:8000"
SID = "685dfbcb-f7e0-40f7-9792-003bfe2405ca"
AID = "f2e5a24ab0756d6413d6a1a3"

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
    print(f"  snap{i}: l1Fields={sn.get('l1FieldCount')} l3Chunks={sn.get('l3ChunkCount')} "
          f"l4Paths={sn.get('l4PathCount')} l4Tokens={sn.get('l4TokenEstimate')} "
          f"promptActual={sn.get('promptTokenActual')} turn={sn.get('turnId','')[:8]}")
    seg = str(sn.get("segmentsJson") or "")
    import re
    segnames = re.findall(r'"([\w.]+)":\s*\{', seg)
    print(f"    segments: {segnames}")

s, d = req("GET", f"/v1/sessions/{SID}/messages?limit=50")
items = d.get("items", d.get("messages", []))
print(f"\nmessages: {len(items)}")
for m in items:
    role = m.get("role")
    c = str(m.get("content") or m.get("contentMarkdown") or "")[:150]
    print(f"  [{role}] {c}")

conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()
cur.execute("SELECT count(*) FROM memory_facts WHERE agent_id=%s", (AID,))
print('\nfacts(agent_id) total:', cur.fetchone())
cur.execute("SELECT scope_type, scope_id, status, count(*) FROM memory_facts WHERE agent_id=%s GROUP BY 1,2,3", (AID,))
for r in cur.fetchall():
    print('  ', r)
cur.execute("SELECT count(*) FROM steps_v2 WHERE session_id=%s", (SID,))
print('steps_v2 for session:', cur.fetchone())
cur.execute("SELECT count(*) FROM tasks_v2 WHERE session_id=%s", (SID,))
print('tasks_v2 for session:', cur.fetchone())
conn.close()
