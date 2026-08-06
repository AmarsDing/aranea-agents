"""P2-1 L2 recall smoke: send msg to the fts-smoke session asking about
FALCON again — the L2 episode archived from the earlier fts-smoke session
(d7680bde) should surface as an L2-layer hit in the memory_recalled notice."""
import json, time, urllib.request, urllib.error

BASE = 'http://localhost:8000'
SID = open('fts_smoke_session.txt').read().strip()

def req(method, path, body=None, timeout=300):
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(BASE + path, data=data, method=method,
                               headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(r, timeout=timeout) as resp:
            return resp.status, json.loads(resp.read().decode() or "{}")
    except urllib.error.HTTPError as e:
        return e.code, {"raw": e.read().decode()[:500]}

s, d = req("POST", "/v1/chat/messages",
           {"session_id": SID, "content": "再确认一次：FALCON 控制台那个账号，我上次是不是也问过你？"})
print("send status:", s)
print("session:", SID)
