import json, urllib.request, urllib.error, sys

SID = 'f5c0d524-e719-478a-adb2-2b366acf4741'
BASE = 'http://localhost:8000'

def req(method, path, body=None, timeout=300):
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(BASE + path, data=data, method=method,
                               headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(r, timeout=timeout) as resp:
            return resp.status, json.loads(resp.read().decode() or "{}")
    except urllib.error.HTTPError as e:
        return e.code, {"raw": e.read().decode()[:500]}
    except Exception as e:
        return -1, {"error": str(e)}

content = sys.argv[1] if len(sys.argv) > 1 else '我叫什么名字？我喜欢喝什么？'
s, d = req("POST", "/v1/chat/messages", {"session_id": SID, "content": content})
am = d.get("agent_message") or {}
txt = am.get("content") or d.get("content") or ""
print(f"status={s}")
print(f"reply[:300]={txt[:300]}")
