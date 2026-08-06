import json

with open(r"f:\aranea-agents\test\it-ops-closed-loop\ts9v2-gap1-plans.json", encoding="utf-8-sig") as f:
    data = json.load(f)
items = data.get("items") or []
print("plans:", len(items))
p = items[0]
subs = p.get("sub_tasks") or p.get("subTasks") or []
print("plan:", p.get("id"), "status:", p.get("status"), "strategy:", p.get("strategy"), "subtasks:", len(subs))
for s in subs:
    print(" -", s.get("name"), "| depends_on:", s.get("depends_on") or s.get("dependsOn"))
pm = [s for s in subs if "复盘" in (s.get("name") or "")]
print("POSTMORTEM NODE PRESENT:", bool(pm))
