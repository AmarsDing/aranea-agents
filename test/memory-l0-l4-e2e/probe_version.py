import json, urllib.request, urllib.error

BASE = "http://localhost:8000"
AGENT = "agent___spirit__"

def req(method, path, body=None):
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(BASE + path, data=data, method=method,
                               headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(r, timeout=30) as resp:
            return resp.status, json.loads(resp.read().decode() or "{}")
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read().decode() or "{}")

body = {"fact": {
    "scopeType": "agent", "scopeId": AGENT, "agentId": AGENT,
    "statement": "memtest: version probe", "factKind": "fact",
    "tagsJson": '["memtest"]', "confidence": 0.9, "importance": 0.8,
    "sourceKind": "manual"}}

s1, d1 = req("POST", "/v1/memory/l3/facts", body)
print("insert#1:", s1, json.dumps(d1, ensure_ascii=False)[:400])
body["fact"]["confidence"] = 0.77
s2, d2 = req("POST", "/v1/memory/l3/facts", body)
print("insert#2:", s2, json.dumps(d2, ensure_ascii=False)[:400])

# DB 侧确认
import psycopg2
conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()
cur.execute("SELECT id, version, confidence FROM memory_facts WHERE statement LIKE 'memtest: version probe%'")
print("db rows:", cur.fetchall())
cur.execute("DELETE FROM memory_facts WHERE statement LIKE 'memtest: version probe%'")
conn.commit()
conn.close()
print("cleaned")
