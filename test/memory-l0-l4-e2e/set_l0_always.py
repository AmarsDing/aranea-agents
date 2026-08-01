# 将 spirit agent 的 l0SnapshotMode 临时切为 always
import json, urllib.request

BASE = "http://localhost:8000"
AID = "agent___spirit__"

def req(method, path, body=None):
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(BASE + path, data=data, method=method,
                               headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(r, timeout=30) as resp:
            return resp.status, json.loads(resp.read().decode() or "{}")
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read().decode() or "{}")

s, agent = req("GET", f"/v1/agents/{AID}")
settings = agent.get("settings") or {}
print("before l0SnapshotMode =", settings.get("l0SnapshotMode"))
settings["l0SnapshotMode"] = "always"
agent["settings"] = settings

s, resp = req("PATCH", f"/v1/agents/{AID}", {"id": AID, "agent": agent})
print("PATCH status =", s)
if s == 200:
    print("after l0SnapshotMode =", ((resp.get("settings") or {}).get("l0SnapshotMode")))
else:
    print("resp:", str(resp)[:300])
