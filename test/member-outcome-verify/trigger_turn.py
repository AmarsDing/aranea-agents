"""Trigger a team turn on standalone team 9755a723338ec796b02c36c9 via chat API."""
import json
import urllib.request

TOKEN = open("test/member-outcome-verify/token.txt", encoding="utf-8").read().strip()
SESSION_ID = "02e2a664-bb47-4540-9e07-6b8c8b71cec0"  # active session of the team
TEAM_ID = "9755a723338ec796b02c36c9"

req = urllib.request.Request(
    "http://127.0.0.1:8000/v1/chat/messages",
    data=json.dumps({
        "session_id": SESSION_ID,
        "team_id": TEAM_ID,
        "content": "请用一句话回复：收到验证消息。",
    }).encode("utf-8"),
    headers={
        "Content-Type": "application/json",
        "Authorization": f"Bearer {TOKEN}",
    },
    method="POST",
)
try:
    with urllib.request.urlopen(req, timeout=300) as resp:
        raw = resp.read().decode("utf-8")
        print("HTTP", resp.status)
        print("raw (first 600):", raw[:600])
except urllib.error.HTTPError as e:
    print("HTTPError", e.code, e.read().decode("utf-8")[:500])
