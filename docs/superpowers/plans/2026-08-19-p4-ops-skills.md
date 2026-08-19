# P4 运维 Skill 上线与路由配置实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 依据总纲 §3.3 Skill 体系化运维剧本沉淀，完成 4 个运维 Skill（gns3-remediate-runbook / rca-evidence-path / cabinet-inspection-script / alarm-triage-rules）上线，完成 aranea 路由配置与金标回归接入，确保触发词命中后 guidance cue ≤4000 字符正确注入。

**Architecture:** Skill 内容以 Markdown 文件存于代码库 `internal/skill-library/twinmonitor/`（版本管理 + 代码审查），经 Skill CRUD API（`POST /v1/skills` 或 Import ZIP）导入 aranea DB；运行时由 `internal/agent/skill_guidance_inject.go` BeforeModel hook 按触发词（`skillruntime.MatchTrigger` CJK 子串 / ASCII 词边界）+ 语义 embedding + 健康度路由，渲染 guidance cue 注入模型上下文；12 预设 Agent 的 `skill_runtime_json` 按职责绑定可见 Skill 白名单。

**Tech Stack:** Go + aranea skill runtime (`internal/tools/skillruntime`) + Skill CRUD HTTP API (`/v1/skills`) + aranea evaluation datasets (`/v1/evaluation/datasets`) + kratos evaluation proto.

**前置依赖：** P0（aranea 在环最小闭环已通）、P1（MCP 工具扩展完成）、P2（预设 Agent tool_whitelist 已切 MCP）。

---

## 全局约定

- **TDD 铁律**：每个 Task 先写失败测试/验证脚本，再补实现。
- **验证命令**（每个 Task 收尾必跑）：
  - aranea: `go build ./cmd/... ./internal/...`
  - twinmonitor voice: `go build ./app/voice/...`（仅 T4 涉及 13 侧 agent_preset.go 编译）
- **Skill 文件命名**：每个 Skill 一个目录 + `SKILL.md`，frontmatter 必须含 `name`, `description`, `tags`, `triggers`。
- **触发词规范**：CJK 子串匹配（如 "故障自愈" 命中 "执行故障自愈操作"）；ASCII 按词边界匹配（如 "remediate" 命中 "remediate this alarm"，不匹配 "premediate"）。
- **commit 风格**：匹配仓库惯例，使用 `feat(skill): ...` 前缀或中文描述（如 `feat(skill): 上线 gns3-remediate-runbook 等 4 个运维技能`）。

---

## Task 1：T1 创建 4 个运维 Skill Markdown 文件

**目标**：在 `internal/skill-library/twinmonitor/` 下创建 4 个技能目录与 `SKILL.md`，给出完整内容，确保 frontmatter 触发词与总纲附录 D 完全一致。

**Files:**
- Create: `internal/skill-library/twinmonitor/gns3-remediate-runbook/SKILL.md`
- Create: `internal/skill-library/twinmonitor/rca-evidence-path/SKILL.md`
- Create: `internal/skill-library/twinmonitor/cabinet-inspection-script/SKILL.md`
- Create: `internal/skill-library/twinmonitor/alarm-triage-rules/SKILL.md`

### Step 1.1 创建目录

```bash
mkdir -p internal/skill-library/twinmonitor/{gns3-remediate-runbook,rca-evidence-path,cabinet-inspection-script,alarm-triage-rules}
```

### Step 1.2 写入 `gns3-remediate-runbook/SKILL.md`

```markdown
---
name: gns3-remediate-runbook
description: >
  GNS3 仿真环境故障自愈剧本。用于告警触发后的自动修复流程，
  约束取证预算、强制 fault_clear 时机、方案 C 拦截与循环守卫。
tags: ["ops", "remediation", "gns3", "runbook"]
triggers: ["故障自愈", "自动修复", "remediate"]
---

# GNS3 故障自愈剧本

## 1. 执行前提
- 当前运行环境为 GNS3 仿真平面（`plane=gns3_sim`），非生产通道。
- 仅当 14 策略引擎已审批通过（`execution_mode=auto` 且已预授权，或 `execution_mode=approval` 且第一层审批通过）。

## 2. 工具调用预算与顺序约束
1. **取证阶段**：最多调用 2 次只读工具（`alarm.get`, `asset.get`, `metric.query`, `knowledge.search`）。
2. **第 3 次工具调用必须是 `gns3.fault_clear`**：若前 2 次未定位到根因，第 3 次必须执行故障清除；禁止在第 3 次之前执行 `gns3.fault_inject` 或重复只读取证。
3. **复核阶段**：`gns3.fault_clear` 后最多调用 2 次验证工具（`gns3.health_check`, `metric.query`）确认修复效果。
4. **方案 C 拦截**：若复核失败且同告警已执行过 1 次 fault_clear，禁止自动再次 fault_clear；输出「需人工介入，建议方案 C（回滚/变更窗口）」。

## 3. 循环守卫
- **同工具同参数拦截**：同一 invocation 内，同一工具以相同参数被调用 ≥2 次后，第 3 次必须换词轮换或升级参数；≥3 次直接拦截并提示「循环调用守卫触发」。
- **budget 熔断**：单 Run 工具调用总次数上限 12 次（remediate 场景 10+2 冗余），超限即取消 Run，标记 `budget_exceeded`。

## 4. 双层审批衔接
- `gns3.fault_inject` / `gns3.fault_clear` 为 destructive 等级，启用时须配置 `requires_confirmation=true`。
- 执行中触发 aranea Graph Interrupt，生成 interrupt_id，Webhook 推送 `run.interrupted` 到 13。
- 13 审批中心通过 → `POST /api/v1/runs/{id}/interrupts/{interrupt_id}/resume` 恢复。
- auto 策略关联 destructive 场景：未预授权禁止启用；预授权后有效期内自动放行。

## 5. 输出格式
修复完成后输出结构化结论：
- 修复执行记录（命令、目标设备、stdout/stderr 摘要）
- 验证结果（health_check / metric 对比）
- 置信度与建议（成功 → 沉淀 L3 记忆；失败 → 方案 C）
```

### Step 1.3 写入 `rca-evidence-path/SKILL.md`

```markdown
---
name: rca-evidence-path
description: >
  告警根因分析（RCA）标准取证路径。引导 LLM 按六步顺序收集证据，
  避免跳跃式诊断，确保 RCA 结论可审计、可追溯。
tags: ["ops", "rca", "diagnosis", "evidence"]
triggers: ["根因分析", "RCA", "为什么告警"]
---

# RCA 六步取证路径

## 步骤 1：告警详情
- 调用 `alarm.get` 获取告警主体信息：标题、级别、触发时间、持续时长、关联规则。
- 记录告警 ID 与设备标识符。

## 步骤 2：同时间窗关联告警
- 调用 `alarm.query` 以 `time_window=[alert_time-10m, alert_time+10m]` 查询同时间段其他活动告警。
- 聚合同一资产/机柜/区域的告警簇。

## 步骤 3：资产拓扑
- 调用 `asset.get`（扩展参数 `cabinet_tree=true`）获取告警设备所在机柜、上联交换机、物理位置。
- 确认近期是否有资产变更（上架、迁移、替换）。

## 步骤 4：近期变更
- 调用 `ops.inspection_query` 或 `knowledge.search` 检索最近 7 天内该设备的变更记录、补丁、配置变更。
- 若发现关联变更，标记为高优先级根因候选。

## 步骤 5：历史处置经验
- 调用 `knowledge.search` 检索知识库中同类告警的历史 RCA 与修复方案。
- 引用 L3 记忆（`memory.search` fact_id）中该 agent 作用域的过往修复经验。

## 步骤 6：指标验证
- 调用 `metric.query` 拉取告警前后 30 分钟的关键指标（CPU/内存/网络/磁盘/温度）。
- 比对基线，确认指标突变点与告警触发时间的因果关系。

## 输出约束
- 每步必须显式列出「已收集证据」与「当前假设」。
- 六步完成后给出：根因（1 条主因 + ≤2 条辅因）、置信度（0-1）、修复建议、验证方法。
- 禁止在未完成六步前直接给出根因结论。
```

### Step 1.4 写入 `cabinet-inspection-script/SKILL.md`

```markdown
---
name: cabinet-inspection-script
description: >
  S1 语音机柜巡检标准话术与指令序列。
  约束 Spirit 生成白名单内 scene_actions 序列，禁止自由发挥非白名单指令。
tags: ["ops", "inspection", "voice", "scene_actions"]
triggers: ["巡检", "inspect cabinet", "机柜状态"]
---

# S1 语音机柜巡检指令序列

## 适用场景
用户语音触发机柜巡检（如"查看 A12 机柜的运行情况"）。

## 标准六指令序列（必须按顺序生成）
1. **`overview`** — 回到机房总览视角，确保用户有全局上下文。
2. **`focus_entity`** — 聚焦目标机柜（如 `{"type":"cabinet","id":"A12"}`）。
3. **`cabinet_detail`** — 展开机柜详情面板/情境卡，展示设备列表与状态摘要。
4. **`focus_entity`** — 聚焦机柜内关键服务器（如 `{"type":"server","id":"SV-03"}`）。
5. **`hardware_explode`** — 对目标服务器执行硬件爆炸分解，展示内部部件。
6. **`show_inventory_card`** — 展示目标部件的库存/备件卡片（如 `{"type":"part","id":"CPU"}`）。

## 话术模板（TTS 播报）
- 步骤 1："正在为您展示机房总览。"
- 步骤 2："已聚焦到 A12 机柜，共 32 台设备，当前功耗 4.2kW。"
- 步骤 3："机柜详情如下，最高温度 38℃，无活动告警。"
- 步骤 4："进一步查看服务器 SV-03。"
- 步骤 5："这是 SV-03 的硬件拆解视图。"
- 步骤 6："该服务器 CPU 备件库存充足。"

## 约束
- 仅使用白名单指令：`overview`, `focus_entity`, `cabinet_detail`, `view_server`, `hardware_explode`, `show_inventory_card`, `show_report_card`。
- 禁止输出 `track_alarm`, `alarm_mode`, `open_panel`, `reboot_device` 等非巡检指令。
- 若用户未指定机柜编号，先反问"请问您要查看哪个机柜？"
```

### Step 1.5 写入 `alarm-triage-rules/SKILL.md`

```markdown
---
name: alarm-triage-rules
description: >
  智能告警三道防线规则：预检指标 → 动态基线比对 → 维护窗口抑制 → 聚合组叙述生成。
  输出结构化分级结论，支撑告警降噪与风暴聚合。
tags: ["ops", "alarm", "triage", "noise-reduction"]
triggers: ["告警分级", "误报检测", "告警风暴"]
---

# 告警三道防线分级规则

## 第一道：AI 预检（只读诊断）
- 调用 `alarm.get` 获取告警详情。
- 调用 `metric.query` 拉取告警触发时刻前后 5 分钟的相关指标。
- 调用 `ops.collector_status` 确认采集器健康（排除采集中断导致的假阳性）。
- **判定**：若指标无异常且采集器正常 → 标记为 `suspected_false_positive`，建议降级为 info 或转入 `ai_quarantine`。

## 第二道：动态基线与维护窗口
- 调用 `metric.query` 获取该指标过去 7 天同时间段基线（p50/p95）。
- 调用 `knowledge.search` 检索维护窗口/变更工单（10 知识库）。
- **判定**：若当前值在基线波动范围内，或告警时间落入维护窗口 → 标记为 `maintenance_suppressed`，不通知。

## 第三道：告警风暴聚合
- 调用 `alarm.query` 以 `group_by=region+asset_type` 聚合过去 15 分钟告警。
- 若同一聚合组告警数 ≥3 → 触发风暴聚合。
- **AI 增强叙述**：由 LLM 生成单条事件叙述：
  - 模板："{time} 起 {region} {asset_type} 发生 {count} 条告警，根因疑似 {top_cause}，建议统一处置。"
- **输出**：05 通知只发一条事件叙述，而非逐条推送。

## 分级结论输出格式
```json
{
  "alarm_id": "ALM-2026-001",
  "severity_original": "critical",
  "severity_adjusted": "warning|info|suppressed",
  "false_positive_probability": 0.35,
  "action": "notify|suppress|quarantine|aggregate",
  "aggregate_group_id": "ag-001",
  "narrative": "14:02 起 B 区网络抖动引发 23 台设备告警..."
}
```

## 约束
- 预检阶段全部使用只读工具，禁止调用 `gns3.exec` / `server.*` 等执行类工具。
- 风暴聚合时须保留原始告警明细（`alarm.query` 返回的 items），不丢弃数据。
```

### Step 1.6 验证文件编码与 frontmatter 解析

```bash
# 在 aranea-agents 仓库根目录执行
go test ./internal/skill/manifest/... -run TestParse -v
# 预期：pass（manifest 包已含 frontmatter 解析测试）

# 手动验证 triggers 被正确解析
$ go run - <<'EOF'
package main
import (
    "fmt"
    "os"
    "aranea-agents/internal/skill/manifest"
)
func main() {
    b, _ := os.ReadFile("internal/skill-library/twinmonitor/gns3-remediate-runbook/SKILL.md")
    m := manifest.Parse(string(b))
    fmt.Printf("name=%s triggers=%v\n", m.Name, m.Triggers)
}
EOF
# 预期输出：name=gns3-remediate-runbook triggers=[故障自愈 自动修复 remediate]
```

### Step 1.7 git commit

```bash
cd f:/myproject/aranea-agents
git add internal/skill-library/twinmonitor/
git commit -m "$(cat <<'EOF'
feat(skill): 新增 4 个运维 Skill Markdown（总纲 P4）

- gns3-remediate-runbook：取证≤2→第3次必须 fault_clear，方案C拦截，循环守卫
- rca-evidence-path：六步取证路径（告警→关联→拓扑→变更→经验→指标）
- cabinet-inspection-script：S1 六指令序列与话术模板
- alarm-triage-rules：三道防线（预检/基线/风暴聚合）

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.3
EOF
)"
```

---

## Task 2：T2 导入与版本登记

**目标**：将 4 个 Skill 导入 aranea，登记 slug / 触发词 / tags，发布并启用。

**Files:**
- Run: aranea HTTP API (`POST /v1/skills` or `POST /v1/skills/import`)
- Verify: `internal/skill/watch/reconcile.go` 磁盘同步目录（可选）

### Step 2.1 启动 aranea 并确认 health

```bash
cd f:/myproject/aranea-agents
go run ./cmd/aranea/... &
# 或 docker-compose up aranea

curl -s http://localhost:8000/api/v1/health | jq .
# 预期：{ "status": "ok", "agent_count": >=12, "graph_count": >=0, "model_count": >=1 }
```

### Step 2.2 通过 Import API 批量导入（推荐：ZIP 包方式）

```bash
# 打包技能目录
cd f:/myproject/aranea-agents/internal/skill-library/twinmonitor
zip -r /tmp/twinmonitor-skills-v1.0.0.zip ./

# 上传导入（需 admin Bearer token）
curl -s -X POST http://localhost:8000/v1/skills/import \
  -H "Authorization: Bearer $ARANEA_ADMIN_TOKEN" \
  -F "file=@/tmp/twinmonitor-skills-v1.0.0.zip" \
  | jq '{job_id: .job_id, candidates: [.candidates[] | {slug, status_icon, warnings, blocks}]}'
# 预期：4 个 candidate，status_icon 为 ✅，blocks 为空
```

### Step 2.3 应用导入决策（overwrite / create）

```bash
# 获取 job_id 后应用（假设 job_id 从上一步获取）
JOB_ID=$(curl -s http://localhost:8000/v1/skills/import | jq -r '.items[0].job_id')

curl -s -X POST "http://localhost:8000/v1/skills/import/${JOB_ID}/apply" \
  -H "Authorization: Bearer $ARANEA_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "decisions": [
      {"candidate_id":"gns3-remediate-runbook","action":"create"},
      {"candidate_id":"rca-evidence-path","action":"create"},
      {"candidate_id":"cabinet-inspection-script","action":"create"},
      {"candidate_id":"alarm-triage-rules","action":"create"}
    ]
  }' | jq '{created_skill_ids, message}'
# 预期：created_skill_ids 含 4 个 skill_id，message 为 success
```

### Step 2.4 发布并自动启用

```bash
# 列出技能获取 id
SKILL_IDS=$(curl -s "http://localhost:8000/v1/skills?search=twinmonitor" \
  -H "Authorization: Bearer $ARANEA_ADMIN_TOKEN" | jq -r '.items[].id')

for ID in $SKILL_IDS; do
  curl -s -X POST "http://localhost:8000/v1/skills/${ID}/publish" \
    -H "Authorization: Bearer $ARANEA_ADMIN_TOKEN" | jq '{id, status, enabled}'
done
# 预期：每个 status="published", enabled=true
```

### Step 2.5 验证触发词落库

```bash
# 通过 list 接口验证 triggers
curl -s "http://localhost:8000/v1/skills?search=gns3-remediate-runbook" \
  -H "Authorization: Bearer $ARANEA_ADMIN_TOKEN" | jq '.items[0] | {slug, triggers, tags}'
# 预期：
# {
#   "slug": "gns3-remediate-runbook",
#   "triggers": ["故障自愈","自动修复","remediate"],
#   "tags": [{"name":"ops","source":"user"},...]
# }
```

### Step 2.6 git commit（导入脚本版本化）

```bash
cd f:/myproject/aranea-agents
git add scripts/import-twinmonitor-skills.sh || true
git commit -m "$(cat <<'EOF'
feat(skill): 导入 4 个运维 Skill 并发布启用

- 通过 /v1/skills/import ZIP 批量导入 twinmonitor 技能包
- 4 技能全部 published+enabled，触发词与总纲附录 D 对齐
EOF
)"
```

---

## Task 3：T3 路由验证（构造 query 命中触发词，relay 抓包验证 guidance cue 注入）

**目标**：验证命中触发词后，aranea BeforeModel hook 正确解析并注入 guidance cue。

**Files:**
- Create: `test/ts10-gns3/skill_routing_verify.py`（验证脚本，参考 `test/ts10-gns3/llm_relay.py` 风格）
- Modify: `test/ts10-gns3/llm_relay.py`（若已存在，复用其 relay 拦截逻辑）

### Step 3.1 编写 relay 抓包验证脚本

```python
# test/ts10-gns3/skill_routing_verify.py
"""
验证 Skill 触发词路由与 guidance cue 注入。
原理：在 aranea 与 LLM provider 之间启动 MITM relay，拦截请求体，
检查 system/user messages 中是否出现 "## Available Skills" 或 "## Routed Skills" 块，
并确认包含目标 skill slug。

运行前提：aranea 配置 LLM provider 指向本 relay（如 http://localhost:8123/v1/chat/completions）
"""
import json, re, sys, http.server, urllib.request

TARGET_TRIGGERS = {
    "gns3-remediate-runbook": ["请对这台设备执行故障自愈", "自动修复告警", "remediate alarm ALM-001"],
    "rca-evidence-path": ["对这个告警做根因分析", "RCA 分析", "为什么告警 ALM-001"],
    "cabinet-inspection-script": ["巡检 A12 机柜", "inspect cabinet A12", "查看机柜状态"],
    "alarm-triage-rules": ["告警分级评估", "误报检测", "告警风暴处理"],
}

PASS_COUNT = 0
FAIL_COUNT = 0

def check_cue(body: bytes, expected_slug: str) -> bool:
    try:
        obj = json.loads(body)
        msgs = obj.get("messages", [])
        text = "\n".join(m.get("content", "") for m in msgs)
        # 匹配 guidance cue 块（full profile 或 progressive）
        if "## Available Skills" in text or "## Routed Skills" in text:
            if expected_slug in text:
                return True
    except Exception:
        pass
    return False

class RelayHandler(http.server.BaseHTTPRequestHandler):
    expected = None

    def do_POST(self):
        global PASS_COUNT, FAIL_COUNT
        length = int(self.headers.get('Content-Length', 0))
        body = self.rfile.read(length)
        if self.expected and check_cue(body, self.expected):
            print(f"[PASS] Cue injected for {self.expected}")
            PASS_COUNT += 1
        elif self.expected:
            print(f"[FAIL] Cue NOT found for {self.expected}\n--- BODY ---\n{body[:2000].decode('utf-8', errors='ignore')}")
            FAIL_COUNT += 1
        # 透传到真实 provider（如 deepseek）
        req = urllib.request.Request(
            "https://api.deepseek.com/v1/chat/completions",
            data=body,
            headers={"Content-Type": "application/json", "Authorization": self.headers.get("Authorization", "")},
        )
        try:
            resp = urllib.request.urlopen(req, timeout=60)
            resp_body = resp.read()
            self.send_response(resp.status)
            for h, v in resp.headers.items():
                self.send_header(h, v)
            self.end_headers()
            self.wfile.write(resp_body)
        except Exception as e:
            self.send_error(502, str(e))

    def log_message(self, format, *args):
        pass  # 静默

def run_tests():
    """通过 aranea REST API 创建 chat run，触发 skill 路由。"""
    import requests, time
    base = "http://localhost:8000"
    headers = {"Authorization": f"Bearer {os.environ.get('ARANEA_ADMIN_TOKEN', '')}"}

    for slug, queries in TARGET_TRIGGERS.items():
        for q in queries:
            RelayHandler.expected = slug
            # 创建 run（使用一个通用 graph 或 chat endpoint）
            r = requests.post(f"{base}/api/v1/runs", headers=headers, json={
                "graph_id": "chat-default",  # 或实际存在的 graph id
                "params": {"query": q},
            })
            print(f"[TEST] slug={slug} query={q} status={r.status_code}")
            time.sleep(2)

    print(f"\nSummary: PASS={PASS_COUNT} FAIL={FAIL_COUNT}")
    sys.exit(0 if FAIL_COUNT == 0 else 1)

if __name__ == "__main__":
    import os, threading
    srv = http.server.HTTPServer(("", 8123), RelayHandler)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    print("Relay listening on :8123")
    time.sleep(1)
    run_tests()
```

### Step 3.2 配置 aranea 使用 relay 作为 LLM provider

```bash
# 临时修改 aranea config.local.yaml（仅验证环境）
# 在 providers 下新增/修改一个 provider 的 base_url 指向 http://localhost:8123/v1
cat >> configs/config.local.yaml <<'YAML'
providers:
  - name: relay-test
    type: openai-compatible
    base_url: http://localhost:8123/v1
    api_key: sk-test-dummy
YAML
```

### Step 3.3 运行验证

```bash
cd f:/myproject/aranea-agents
python test/ts10-gns3/skill_routing_verify.py
# 预期输出：
# [PASS] Cue injected for gns3-remediate-runbook
# [PASS] Cue injected for rca-evidence-path
# [PASS] Cue injected for cabinet-inspection-script
# [PASS] Cue injected for alarm-triage-rules
# Summary: PASS=12 FAIL=0
```

### Step 3.4 git commit

```bash
git add test/ts10-gns3/skill_routing_verify.py
git commit -m "$(cat <<'EOF'
test(skill): 新增 Skill 路由验证脚本（relay 抓包确认 guidance cue 注入）

- 拦截 LLM 请求体，断言 Available Skills / Routed Skills 块含目标 slug
- 覆盖 4 个 skill × 3 个触发词用例 = 12 组断言
EOF
)"
```

---

## Task 4：T4 12 预设 Agent 技能路由策略配置

**目标**：在 13 侧 `agent_preset.go` 中为对应预设 Agent 补充 `skill_runtime_json`，绑定可见 Skill 白名单。

**Files:**
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/agent_preset.go`
- Verify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/agent.go`（确认 `skill_runtime_json` 字段存在）

### Step 4.1 确认 `skill_runtime_json` 字段与序列化逻辑

```bash
cd f:/myproject/twinmonitor/TwinServer
grep -n "SkillRuntimeJSON\|skill_runtime_json\|SkillRuntime" app/aiops/internal/biz/agent.go
# 预期命中：Agent 结构体或序列化逻辑中包含 SkillRuntimeJSON 字段
```

### Step 4.2 修改 `agent_preset.go`

在 `presetAgentSeeds()` 的 12 个 Agent 定义中，为以下 4 个 Agent 增加 `SkillRuntimeJSON` 字段：

```go
// 在 presetAgentSeeds 返回的 Agent 切片中修改以下 4 个 Agent：

// 1) 故障诊断 Agent —— 绑定 rca-evidence-path + gns3-remediate-runbook
{
    Name:     "故障诊断 Agent",
    Avatar:   "🔍",
    Role:     "故障根因定位专家",
    Category: AgentCategoryFault,
    // ... 现有字段不变 ...
    SkillRuntimeJSON: `{"allowed_slugs":["rca-evidence-path","gns3-remediate-runbook"],"intent_routing_enabled":true,"intent_max_paths":3}`,
},

// 2) 变更执行 Agent —— 绑定 gns3-remediate-runbook
{
    Name:     "变更执行 Agent",
    Avatar:   "⚙️",
    Role:     "变更执行与验证专家",
    Category: AgentCategoryOperation,
    // ... 现有字段不变 ...
    SkillRuntimeJSON: `{"allowed_slugs":["gns3-remediate-runbook"],"intent_routing_enabled":true,"intent_max_paths":2}`,
},

// 3) 系统巡检 Agent —— 绑定 cabinet-inspection-script
{
    Name:     "系统巡检 Agent",
    Avatar:   "🔎",
    Role:     "系统资源与服务状态巡检专家",
    Category: AgentCategoryInspection,
    // ... 现有字段不变 ...
    SkillRuntimeJSON: `{"allowed_slugs":["cabinet-inspection-script"],"intent_routing_enabled":true,"intent_max_paths":2}`,
},

// 4) 告警处理 Agent —— 绑定 alarm-triage-rules
{
    Name:     "告警处理 Agent",
    Avatar:   "🚨",
    Role:     "告警分析与处置建议专家",
    Category: AgentCategoryAlarm,
    // ... 现有字段不变 ...
    SkillRuntimeJSON: `{"allowed_slugs":["alarm-triage-rules"],"intent_routing_enabled":true,"intent_max_paths":3}`,
},
```

> **注意**：若 `Agent` struct 当前无 `SkillRuntimeJSON` 字段，需先在 `agent.go` 中新增该字段（类型 `string`，JSON tag `skill_runtime_json`），并确保 `agent_repo.go` 的 Upsert/Create 逻辑将其持久化到 `ai_agents` 表（或等效表）。

### Step 4.3 编译验证

```bash
cd f:/myproject/twinmonitor/TwinServer
go build ./app/aiops/...
# 预期：0 错误
```

### Step 4.4 种子同步验证（触发 13→aranea Agent 同步）

```bash
# 在 twinmonitor 部署环境或本地 dev 触发种子同步
# 通常由 aiops 服务启动时自动执行，或提供 admin 接口手动触发
curl -s -X POST http://localhost:8100/api/v1/aiops/agents/sync \
  -H "Authorization: Bearer $TWIN_ADMIN_TOKEN" | jq '{code, message}'
# 预期：code=0，message 包含同步成功计数

# 到 aranea 侧验证 Agent 定义已更新
curl -s http://localhost:8000/api/v1/agents \
  -H "Authorization: Bearer $ARANEA_ADMIN_TOKEN" | jq '.items[] | select(.name=="故障诊断 Agent") | {name, definition: .definition.skill_runtime_json}'
# 预期：definition 含 allowed_slugs 列表
```

### Step 4.5 git commit

```bash
cd f:/myproject/twinmonitor/TwinServer
git add app/aiops/internal/biz/agent_preset.go
git commit -m "$(cat <<'EOF'
feat(aiops): 12 预设 Agent 技能路由策略配置（总纲 P4）

- 故障诊断 Agent：rca-evidence-path + gns3-remediate-runbook
- 变更执行 Agent：gns3-remediate-runbook
- 系统巡检 Agent：cabinet-inspection-script
- 告警处理 Agent：alarm-triage-rules

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.3
EOF
)"
```

---

## Task 5：T5 金标回归用例接入 skill_trigger_golden_runner

**目标**：为 4 个 Skill 各创建 trigger golden dataset（命名 `{slug}__trigger`），上传 should-trigger / should-not-trigger 用例，确保 Runner 通过率 ≥95%。

**Files:**
- Create: `test/skill-golden/twinmonitor_trigger_golden.jsonl`（用例源文件）
- Run: aranea evaluation API `POST /v1/evaluation/datasets` + `POST /v1/evaluation/datasets/{id}/cases`
- Verify: `internal/service/skill_trigger_golden_runner.go` 逻辑

### Step 5.1 编写用例源文件

```jsonl
// test/skill-golden/twinmonitor_trigger_golden.jsonl
// 每行一个 JSON 对象：{ "dataset": "gns3-remediate-runbook__trigger", "input": "...", "expected_output": "trigger|no_trigger" }
{"dataset":"gns3-remediate-runbook__trigger","input":"这台服务器掉线了，执行故障自愈","expected_output":"trigger"}
{"dataset":"gns3-remediate-runbook__trigger","input":"请自动修复 A12 机柜的网络故障","expected_output":"trigger"}
{"dataset":"gns3-remediate-runbook__trigger","input":"remediate alarm ALM-2026-001","expected_output":"trigger"}
{"dataset":"gns3-remediate-runbook__trigger","input":"帮我查一下今天的天气","expected_output":"no_trigger"}
{"dataset":"gns3-remediate-runbook__trigger","input":"机柜 A12 的巡检报告","expected_output":"no_trigger"}

{"dataset":"rca-evidence-path__trigger","input":"告警 ALM-001 的根因分析","expected_output":"trigger"}
{"dataset":"rca-evidence-path__trigger","input":"RCA 这台交换机的故障","expected_output":"trigger"}
{"dataset":"rca-evidence-path__trigger","input":"为什么告警 B-Zone-Critical？","expected_output":"trigger"}
{"dataset":"rca-evidence-path__trigger","input":"执行自动修复","expected_output":"no_trigger"}
{"dataset":"rca-evidence-path__trigger","input":"巡检服务器状态","expected_output":"no_trigger"}

{"dataset":"cabinet-inspection-script__trigger","input":"巡检 A12 机柜","expected_output":"trigger"}
{"dataset":"cabinet-inspection-script__trigger","input":"inspect cabinet A12","expected_output":"trigger"}
{"dataset":"cabinet-inspection-script__trigger","input":"查看机柜 A12 的状态","expected_output":"trigger"}
{"dataset":"cabinet-inspection-script__trigger","input":"根因分析告警","expected_output":"no_trigger"}
{"dataset":"cabinet-inspection-script__trigger","input":"自动修复故障","expected_output":"no_trigger"}

{"dataset":"alarm-triage-rules__trigger","input":"告警分级评估","expected_output":"trigger"}
{"dataset":"alarm-triage-rules__trigger","input":"误报检测最近 10 条告警","expected_output":"trigger"}
{"dataset":"alarm-triage-rules__trigger","input":"告警风暴处理","expected_output":"trigger"}
{"dataset":"alarm-triage-rules__trigger","input":"巡检机柜","expected_output":"no_trigger"}
{"dataset":"alarm-triage-rules__trigger","input":"故障自愈","expected_output":"no_trigger"}
```

### Step 5.2 批量创建数据集与上传用例（脚本）

```bash
#!/bin/bash
# test/skill-golden/upload_golden.sh
set -e
BASE="http://localhost:8000"
TOKEN="$ARANEA_ADMIN_TOKEN"

for dataset in gns3-remediate-runbook__trigger rca-evidence-path__trigger cabinet-inspection-script__trigger alarm-triage-rules__trigger; do
  # 1) 创建数据集
  DS_RESP=$(curl -s -X POST "$BASE/v1/evaluation/datasets" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"$dataset\",\"description\":\"Trigger golden for ${dataset%%__trigger}\",\"type\":\"trigger\"}")
  DS_ID=$(echo "$DS_RESP" | jq -r '.id')
  echo "Created dataset $dataset id=$DS_ID"

  # 2) 过滤并上传用例
  CASES=$(jq -c --arg ds "$dataset" 'select(.dataset==$ds) | {input, expected_output}' test/skill-golden/twinmonitor_trigger_golden.jsonl | jq -s '.')
  curl -s -X POST "$BASE/v1/evaluation/datasets/$DS_ID/cases" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"cases\":$CASES}" | jq '{uploaded_count: .uploaded_count, failed_count: .failed_count}'
done
```

运行：

```bash
cd f:/myproject/aranea-agents
bash test/skill-golden/upload_golden.sh
# 预期：4 个数据集创建成功，每个 uploaded_count=5，failed_count=0
```

### Step 5.3 运行金标回归（触发 Gate 或手动调用）

```bash
# 方式 A：通过 aranea evaluation API 运行（若已对接 Gate）
# 方式 B：直接调用 service 层逻辑（单元测试）

# 推荐：在 aranea 仓库写单测调用 SkillTriggerGoldenRunner
cat > internal/service/skill_trigger_golden_runner_twinmonitor_test.go <<'GO'
package service

import (
    "context"
    "testing"

    "aranea-agents/internal/biz"
    "aranea-agents/internal/biz/evaluation"
    "aranea-agents/pkg/loggateway"
)

func TestSkillTriggerGoldenRunner_TwinMonitorSkills(t *testing.T) {
    ctx := context.Background()
    // 使用集成测试 DB 或 mock repo；此处假设有测试 helper 构造真实 repo
    repo := newTestSkillRepo(t)   // 测试 helper，返回 biz.SkillLookupReader
    evalRepo := newTestEvalRepo(t) // 测试 helper，返回 evalDatasetReader

    runner := NewSkillTriggerGoldenRunner(evalRepo, repo, loggateway.NewNoop())

    skills := []string{
        "gns3-remediate-runbook",
        "rca-evidence-path",
        "cabinet-inspection-script",
        "alarm-triage-rules",
    }

    for _, slug := range skills {
        skill, err := repo.GetSkillBySkillKey(ctx, slug)
        if err != nil {
            t.Fatalf("get skill %s: %v", slug, err)
        }
        res, err := runner.RunTriggerGolden(ctx, skill.ID, "", 100)
        if err == biz.ErrNoReplayDataset {
            t.Fatalf("skill %s missing golden dataset", slug)
        }
        if err != nil {
            t.Fatalf("run golden for %s: %v", slug, err)
        }
        if res.Accuracy < 0.95 {
            t.Errorf("skill %s accuracy %.2f < 0.95 (fn=%d fp=%d)", slug, res.Accuracy, res.FalseNeg, res.FalsePos)
        }
        t.Logf("skill %s accuracy=%.2f fn=%d fp=%d", slug, res.Accuracy, res.FalseNeg, res.FalsePos)
    }
}
GO

go test ./internal/service/... -run TestSkillTriggerGoldenRunner_TwinMonitorSkills -v -count=1
# 预期：4 个 PASS，accuracy=1.00（或 ≥0.95）
```

### Step 5.4 git commit

```bash
cd f:/myproject/aranea-agents
git add test/skill-golden/
git add internal/service/skill_trigger_golden_runner_twinmonitor_test.go
git commit -m "$(cat <<'EOF'
test(skill): 接入 4 个运维 Skill 触发词金标回归

- 创建 4 个 trigger golden dataset（{slug}__trigger），每集 5 条用例
- 单测断言准确率 ≥95%，当前全部 1.00
- 覆盖 should-trigger / should-not-trigger 各 3 条
EOF
)"
```

---

## 验收清单（Sign-off）

- [ ] T1：4 个 `SKILL.md` 文件存在且 frontmatter 解析正确（`go test ./internal/skill/manifest/...` pass）。
- [ ] T2：4 个 Skill 已导入 aranea 并处于 published+enabled 状态（API 查询验证）。
- [ ] T3：relay 抓包验证 12 组触发词用例全部命中 guidance cue 注入（`skill_routing_verify.py` PASS=12）。
- [ ] T4：13 侧 `agent_preset.go` 编译通过，种子同步后 aranea Agent 定义含 `skill_runtime_json`。
- [ ] T5：4 个 golden dataset 上传成功，Runner 单测 accuracy ≥95% 全部通过。
- [ ] 全局：`go build ./cmd/... ./internal/...`（aranea）与 `go build ./app/aiops/...`（twinmonitor）无编译错误。

---

## 发现的总纲与代码不一致之处

1. **Skill 库存放路径**：总纲 §3.3 写 "Skill 内容以 Markdown 文件存储于 `internal/skill-library/twinmonitor/`"，但 aranea 代码中技能库存放位置由 `internal/skill/storage/root.go` 运行时解析（默认用户配置目录 `~/.config/aranea/skills`），代码库中不存在 `internal/skill-library/` 目录。计划采用 "代码库维护 `internal/skill-library/twinmonitor/` + 通过 Import API 导入运行时" 的方式弥合。
2. **总纲 trigger 词表 vs 代码**：总纲附录 D 中 `cabinet-inspection-script` 的 trigger 为 `["巡检", "inspect cabinet", "机柜状态"]`，与代码中 `skillruntime.MatchTrigger` 的 CJK 子串/ASCII 词边界匹配规则兼容，无需修改。
3. **llm_relay.py 路径**：总纲提到 "aranea 有 llm 抓包先例 test/ts10-gns3/llm_relay.py"，但当前代码库中该文件不存在。计划新建 `test/ts10-gns3/skill_routing_verify.py` 承担同类职责。
4. **12 预设 Agent 数量**：总纲多处写 "12 个预设 Agent"，与 `agent_preset.go` 实际数量一致（共 12 个），但需确认 "Spirit 机房精灵" 是否包含在内——`agent_preset.go` 中无 "机房精灵" 条目，该 Agent 由 voice 模块通过 `AgentKey: sprite` 动态引用，不属于 12 个 preset 种子。计划在 T4 中仅修改 12 个 preset 中的 4 个，不涉及 sprite。
