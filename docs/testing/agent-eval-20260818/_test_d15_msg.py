import requests, json, time, sys

BASE = "http://127.0.0.1:8810"
TOKEN = open(r"f:\myproject\aranea-agents\docker\.test-token.txt").read().strip()
SID = "3c596474-a471-4fcf-ac49-ebdd8426f237"
CONTENT = '请立即调用 knowledge_write 工具写入一条事实，参数：statement="评测-核心交换机SW-Eval-01的管理IP为10.20.99.1"，tags=["评测-核心交换机","SW-Eval-01"]，fact_id="eval-sw-ip"，confidence=0.95。只调用这一个工具。'

t0 = time.time()
resp = requests.post(
    f"{BASE}/v1/chat/messages",
    headers={"Authorization": f"Bearer {TOKEN}", "Content-Type": "application/json"},
    json={"session_id": SID, "agent_key": "eval_memory_probe", "content": CONTENT},
    timeout=180,
)
elapsed = int((time.time() - t0) * 1000)
out = resp.text
with open(r"f:\myproject\aranea-agents\docs\testing\agent-eval-20260818\_test-d15-response.json", "w", encoding="utf-8") as f:
    f.write(out)
print(f"elapsed={elapsed}ms code={resp.status_code}")
print(f"response_body_length={len(out)}")
try:
    print(json.dumps(json.loads(out), indent=2, ensure_ascii=False))
except Exception:
    print(out)
