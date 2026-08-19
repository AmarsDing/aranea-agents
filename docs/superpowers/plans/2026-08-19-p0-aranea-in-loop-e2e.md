# P0 aranea 在环端到端最小闭环实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 打通「13 创建 Run → aranea 执行 → Webhook 回写 → 状态推进」最小闭环，解锁当前第一阻塞。覆盖：aranea 服务部署、13 连通性探测、种子同步、最小 E2E、HITL interrupt 闭环、RCA 端到端共 6 个验收项（总纲 §6 阶段 P0）。

**Architecture:** aranea 侧提供 REST 兼容门面（`twin_openapi_compat.go`）+ Webhook 事件投递（`outboundwebhook` HMAC-SHA256）；13 侧通过 `AraneaClient` 调用、`WebhookReceiver` 消费事件、`TaskUsecase` 推进镜像状态机。种子同步由 `SeedSynchronizer` 将 12 预设 Agent + 2 系统场景注册到 aranea。

**Tech Stack:** Go + aranea-agents（twin-openapi 门面 + graph 执行 + webhook 出站）+ twinmonitor 13-aiops（AraneaClient + WebhookReceiver + Task 镜像状态机）+ Docker Compose / PowerShell 本地启动 + NATS + PostgreSQL + psql。

**前置依赖：**
- aranea-agents 仓库可构建：`go build ./cmd/... ./internal/...` 通过（排除 `test/b1t3-gate`）。
- twinmonitor 13-aiops 可构建：`go build ./app/aiops/...` 通过。
- PostgreSQL 本机实例（5432）密码 `123456`；aranea 侧需库 `aranea`、13 侧需库 `twinmonitor`/`twinmonitor_log`。
- NATS（4222）与 Redis（6379）本地运行；可用 `f:/myproject/twinmonitor/TwinServer/scripts/deploy/start-env.ps1` 一键启动。

---

## 全局约定

- **TDD 铁律**：每个 Task 先写失败测试/验证脚本，再补实现。
- **验证命令**（每个 Task 收尾必跑）：
  - aranea: `cd f:/myproject/aranea-agents && go build ./cmd/... ./internal/...`
  - twinmonitor: `cd f:/myproject/twinmonitor/TwinServer && go build ./app/aiops/...`
- **SQL 执行铁律**：禁止 PowerShell 内联复杂引号串执行 SQL，一律用 `psql -f file.sql`。
- **commit 风格**：参照 `git log --oneline` 既有前缀。aranea 用 `feat(twinopenapi): ...` / `fix(webhook): ...`；twinmonitor 用 `feat(aiops): ...`。

---

## Task 1：T1 aranea 服务以 twin-openapi token 启动

**目标**：aranea 侧服务启动并暴露 `/api/v1/health`，返回 200 + 版本/计数。

**Files:**
- None（仅配置与命令）

- [ ] **Step 1.1 确认环境变量与端口**

```bash
cd f:/myproject/aranea-agents
# 确认 docker-compose.yaml 中 token 与端口
grep -n "ARANEA_TWINOPENAPI_TOKEN\|8810\|9910" docker-compose.yaml
# 预期命中：
#   70: ARANEA_TWINOPENAPI_TOKEN: "twin_fdfee1b8c9443c5338a781bd9a4e73074916333b631dad51"
#   94: - "8810:8810"
#   95: - "9910:9910"
```

- [ ] **Step 1.2 启动依赖（PG/Redis）**

```powershell
cd f:/myproject/twinmonitor/TwinServer/scripts/deploy
./start-env.ps1
# 预期：PG 5432、Redis 6379、NATS 4222 均启动成功
```

- [ ] **Step 1.3 创建 aranea 数据库**

```sql
-- file: f:/myproject/aranea-agents/docs/superpowers/plans/tmp_create_aranea_db.sql
CREATE DATABASE aranea WITH ENCODING = 'UTF8';
```

```bash
psql -h 127.0.0.1 -p 5432 -U postgres -f f:/myproject/aranea-agents/docs/superpowers/plans/tmp_create_aranea_db.sql
# 预期：CREATE DATABASE
```

- [ ] **Step 1.4 本地 go run 启动 aranea（快速验证）**

```powershell
$env:ARANEA_TWINOPENAPI_TOKEN="twin_fdfee1b8c9443c5338a781bd9a4e73074916333b631dad51"
$env:DATABASE_URL="postgres://postgres:123456@127.0.0.1:5432/aranea?sslmode=disable"
cd f:/myproject/aranea-agents
go run ./cmd/admin
# 预期：服务监听 :8810，控制台无 panic，migration 自动执行
```

> 备选 Docker 启动：`docker compose up -d admin`（需确保 `DATABASE_URL` 指向宿主 PG）。

- [ ] **Step 1.5 验证健康探测**

```bash
curl -s http://127.0.0.1:8810/api/v1/health | jq .
# 预期：
# {
#   "status": "healthy",
#   "version": "dev",
#   "uptime_seconds": ...,
#   "agent_count": 0,
#   "graph_count": 0,
#   "model_count": 0
# }
```

- [ ] **Step 1.6 git commit（仅当修改了配置/脚本时）**

本 Task 无代码修改，无需 commit。

---

## Task 2：T2 13 侧 ai_aranea_instances 配置启用行 + 探测 green

**目标**：13 侧登记 aranea 实例，AraneaPage 探测按钮 green，健康横幅消失。

**Files:**
- SQL: `f:/myproject/twinmonitor/TwinServer/scripts/deploy/seed_aranea_instance.sql`（新建，执行后删除）

- [ ] **Step 2.1 编写实例插入 SQL**

```sql
-- file: f:/myproject/twinmonitor/TwinServer/scripts/deploy/seed_aranea_instance.sql
-- 注意：api_token_encrypted / webhook_secret_encrypted 由应用层 AES-256-GCM 加密，
-- 这里仅插入占位，真实凭据由 13 AraneaPage「编辑并启用」时重新加密写入。
INSERT INTO ai_aranea_instances
    (name, base_url, api_token_encrypted, webhook_secret_encrypted, status, is_enabled, created_by, updated_by, created_at, updated_at)
VALUES
    ('aranea-local-dev', 'http://127.0.0.1:8810',
     '', '', 'disabled', 0, 1, 1, NOW(), NOW())
ON CONFLICT (name) DO NOTHING;
```

- [ ] **Step 2.2 执行 SQL**

```bash
psql -h 127.0.0.1 -p 5432 -U postgres -d twinmonitor -f f:/myproject/twinmonitor/TwinServer/scripts/deploy/seed_aranea_instance.sql
# 预期：INSERT 0 1（或 ON CONFLICT 无冲突）
```

- [ ] **Step 2.3 启动 13-aiops 服务**

```powershell
cd f:/myproject/twinmonitor/TwinServer/app/aiops
# 确保 config.yaml 中 aranea.callback_base_url 指向可到达地址
go run ./cmd/
# 预期：服务启动于 :8100（REST）/:9100（gRPC）
```

- [ ] **Step 2.4 调用 API 更新实例为启用并配置凭据**

```bash
# 先查询实例 ID
curl -s "http://127.0.0.1:8100/api/v1/monitor/aiops/aranea/instances" | jq '.items[0].id'
# 假设返回 1

# 更新为启用并写入 token / webhook_secret（与 aranea 侧 docker-compose 一致）
curl -s -X PUT "http://127.0.0.1:8100/api/v1/monitor/aiops/aranea/instances/1" \
  -H "Content-Type: application/json" \
  -d '{
    "baseUrl": "http://127.0.0.1:8810",
    "apiToken": "twin_fdfee1b8c9443c5338a781bd9a4e73074916333b631dad51",
    "webhookSecret": "whsec_test_p0_e2e_20260819",
    "isEnabled": 1
  }' | jq .
# 预期：isEnabled=1，status 由探测自动更新
```

- [ ] **Step 2.5 触发探测并验收 green**

```bash
curl -s -X POST "http://127.0.0.1:8100/api/v1/monitor/aiops/aranea/instances/1/test" | jq .
# 预期：
# {
#   "status": "healthy",
#   "version": "dev",
#   "agentCount": 0,
#   "graphCount": 0,
#   "probeDurationMs": ...
# }
```

- [ ] **Step 2.6 git commit**

```bash
cd f:/myproject/twinmonitor/TwinServer
git add scripts/deploy/seed_aranea_instance.sql
git commit -m "$(cat <<'EOF'
feat(aiops): aranea 实例初始化种子 SQL（P0 E2E）

- 插入 aranea-local-dev 占位行，供 P0 探测与种子同步使用
EOF
)"
```

---

## Task 3：T3 种子同步 12 预设 Agent + 2 场景

**目标**：`SeedSynchronizer.SyncAll` 将 12 预设 Agent + 2 系统场景注册到 aranea；验收 aranea GET /api/v1/agents 返回 12 个。

**Files:**
- None（调用既有 API，无代码修改）

- [ ] **Step 3.1 触发种子同步**

```bash
curl -s -X POST "http://127.0.0.1:8100/api/v1/monitor/aiops/agents/seed-sync" | jq .
# 预期：total=12，failed=0，items 中每个 sync_status 为 "created" 或 "updated"
```

- [ ] **Step 3.2 验收 aranea 侧 Agent 清单**

```bash
curl -s "http://127.0.0.1:8810/api/v1/agents" \
  -H "Authorization: Bearer twin_fdfee1b8c9443c5338a781bd9a4e73074916333b631dad51" | jq '.items | length'
# 预期：12
```

- [ ] **Step 3.3 验收 aranea 侧 Graph 清单（2 系统场景）**

```bash
curl -s "http://127.0.0.1:8810/api/v1/graphs" \
  -H "Authorization: Bearer twin_fdfee1b8c9443c5338a781bd9a4e73074916333b631dad51" | jq '.items[].name'
# 预期包含：
# "AI根因分析"
# "告警自动诊断"
```

- [ ] **Step 3.4 编译验证（无代码改动，确认构建干净）**

```bash
cd f:/myproject/twinmonitor/TwinServer
go build ./app/aiops/...
# 预期：0 错误
```

- [ ] **Step 3.5 git commit**

本 Task 无代码修改，无需 commit。

---

## Task 4：T4 最小 E2E：13 创建场景 Run → Webhook 回写 → 任务镜像 success

**目标**：13 场景模板页「立即执行」→ aranea Run 创建 → Webhook `run.created/started/completed` 依次到达 → `ai_tasks` 镜像 success。

**Files:**
- None（调用既有 API 验证闭环）

- [ ] **Step 4.1 确认 Webhook 路由可达**

13 侧 Webhook 接收端点：
```
POST http://127.0.0.1:8100/api/v1/monitor/aiops/webhooks/aranea
```

aranea 侧创建 Run 时会向 `callback_base_url + /api/v1/monitor/aiops/webhooks/aranea` 投递事件。
确保 13 侧 `config.yaml` 中 `aranea.callback_base_url` 为 `http://host.docker.internal:8100`（Docker）或 `http://127.0.0.1:8100`（本机双服务）。

- [ ] **Step 4.2 查询场景模板 ID**

```bash
curl -s "http://127.0.0.1:8100/api/v1/monitor/aiops/scenarios" | jq '.items[] | {id,name,araneaGraphId}'
# 预期：
# { "id": 1, "name": "AI根因分析", "araneaGraphId": "..." }
# { "id": 2, "name": "告警自动诊断", "araneaGraphId": "..." }
```

- [ ] **Step 4.3 创建测试 Run（手动触发场景执行）**

```bash
curl -s -X POST "http://127.0.0.1:8100/api/v1/monitor/aiops/scenarios/1/execute" \
  -H "Content-Type: application/json" \
  -d '{"params":{"input":"最小E2E测试：请回答 hello"}}' | jq .
# 预期：
# {
#   "taskId": 1,
#   "taskNo": "AIT20260819000001",
#   "status": "pending",
#   "araneaRunId": "..."
# }
```

- [ ] **Step 4.4 轮询验证任务状态推进**

```bash
# 等待 10~30 秒，让 webhook 事件到达并处理
sleep 15
curl -s "http://127.0.0.1:8100/api/v1/monitor/aiops/tasks/1" | jq '{status,araneaRunId,resultSummary}'
# 预期状态流转：pending → running → completed
# 终态：status="completed"，resultSummary 非空
```

- [ ] **Step 4.5 数据库验证 ai_tasks 镜像**

```sql
-- file: f:/myproject/aranea-agents/docs/superpowers/plans/tmp_verify_task.sql
SELECT id, task_no, status, aranea_run_id, result_summary IS NOT NULL AS has_output
FROM ai_tasks WHERE id = 1;
```

```bash
psql -h 127.0.0.1 -p 5432 -U postgres -d twinmonitor_log -f f:/myproject/aranea-agents/docs/superpowers/plans/tmp_verify_task.sql
# 预期：status=completed，has_output=t
```

- [ ] **Step 4.6 检查 Webhook 事件去重与 HMAC 验证日志**

查看 13-aiops 服务控制台，应无 `webhook signature mismatch` 错误；应有：
```
webhook event ... processed
poll apply snapshot ... (可选，若 webhook 先到达则轮询跳过)
```

- [ ] **Step 4.7 git commit**

本 Task 无代码修改，无需 commit。

---

## Task 5：T5 HITL interrupt 闭环

**目标**：构建节点带 `interrupt_before` 的测试 Graph → 触发 `run.waiting_approval` Webhook 事件 → 13 审批中心通过 → Resume → 执行 success。

> 契约要点（已核对 aranea 源码）：
> - 中断由节点 `interrupt_before: true` 触发（`internal/graph/trpc/node_wiring.go` → `trpcgraph.WithInterruptBefore()`），与工具 risk_level 无关；
> - aranea 发出的 Webhook 事件名为 `run.waiting_approval`（非 run.interrupted），载荷含 `interrupt_id`（= 中断节点 ID，见 `twin_openapi_compat.go` OnRunWaitingApproval）；
> - 13 侧 `biz/webhook.go` 以 `AraneaEventRunWaitingApproval = "run.waiting_approval"` 接收，任务 → `waiting_approval` 并生成 `ai_approvals(pending)`；
> - Resume 端点 `POST /api/v1/runs/{id}/interrupts/{interrupt_id}/resume`，body `{"approved":true,"comment":"...","approver_id":N}`，由 13 审批通过动作内部调用，E2E 无需手调；
> - 恢复要求 Graph 开 `enable_checkpoint: true`（中断/恢复走 checkpoint）。
> - 节点类型：`type: "agent"` + `agent_name` 引用已同步的预设 Agent 名（`biz.NodeTypeAgent = "agent"`，node_wiring 按 AgentName 解析）。
> - 普通边不允许指向 `__end__`（validator 报 edge_target_missing）；单节点图用 `finish_point` = 本节点 ID，不写 edges。

**Files:**
- 测试数据：通过 API 动态创建 Graph（无需改代码）

- [ ] **Step 5.1 在 aranea 侧创建含 interrupt 的测试 Graph**

```bash
# 创建测试 Graph：单 agent 节点 + interrupt_before（变更执行 Agent 为 T3 已同步的 12 预设之一）
curl -s -X POST "http://127.0.0.1:8810/api/v1/graphs" \
  -H "Authorization: Bearer twin_fdfee1b8c9443c5338a781bd9a4e73074916333b631dad51" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "hitl-test-interrupt",
    "entry_point": "node_1",
    "finish_point": "node_1",
    "enable_checkpoint": true,
    "nodes": [
      {
        "id": "node_1",
        "type": "agent",
        "agent_name": "变更执行 Agent",
        "interrupt_before": true
      }
    ]
  }' | jq .
# 预期：{ "id": "<graph_id>" }
```

- [ ] **Step 5.2 创建绑定该 Graph 的场景并执行，触发 interrupt**

```bash
# 13 侧创建场景模板，绑定 Step 5.1 的 graph_id（契约：scenario.proto CreateScenarioRequest）
curl -s -X POST "http://127.0.0.1:8100/api/v1/monitor/aiops/scenarios" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "HITL闭环测试场景",
    "description": "P0 E2E：interrupt_before → 审批 → resume",
    "category": "system",
    "araneaGraphId": "<graph_id>",
    "riskLevel": "high",
    "paramsSchema": [
      {"name":"fault_description","type":"string","required":true,"description":"故障描述"}
    ]
  }' | jq '{id,name,araneaGraphId}'
# 预期：返回新场景 id（下文记为 <sid>）

# 执行场景（契约：ExecuteScenarioRequest{id, params, targetAssetIds}）
curl -s -X POST "http://127.0.0.1:8100/api/v1/monitor/aiops/scenarios/<sid>/execute" \
  -H "Content-Type: application/json" \
  -d '{"params":{"fault_description":"E2E-HITL测试"}}' | jq .
# 预期：返回 taskId，任务状态 pending → running
```

- [ ] **Step 5.3 验证任务进入 waiting_approval**

```bash
sleep 10
curl -s "http://127.0.0.1:8100/api/v1/monitor/aiops/tasks/<task_id>" | jq '{status,currentNodeId}'
# 预期：status="waiting_approval"
```

- [ ] **Step 5.4 查询审批待办并通过**

```bash
# 查询审批列表
curl -s "http://127.0.0.1:8100/api/v1/monitor/aiops/approvals" | jq '.items[0] | {id,status,araneaRunId,interruptId}'
# 假设 approval id = 1，interruptId = "node_xxx"

# 通过审批（ Resume aranea run ）
curl -s -X POST "http://127.0.0.1:8100/api/v1/monitor/aiops/approvals/1/approve" \
  -H "Content-Type: application/json" \
  -d '{"comment":"E2E测试通过","approverId":1}' | jq '{id,status}'
# 预期：approval status="approved"
```

- [ ] **Step 5.5 验证任务终态 success**

```bash
sleep 15
curl -s "http://127.0.0.1:8100/api/v1/monitor/aiops/tasks/<task_id>" | jq '{status,resultSummary}'
# 预期：status="completed" 或 "failed"（若工具实际执行失败，只要 interrupt 闭环通即算成功）
```

- [ ] **Step 5.6 git commit**

本 Task 无代码修改，无需 commit。

---

## Task 6：T6 RCA 端到端

**目标**：用 `nats-pub-alarm` 发模拟告警 → 13 订阅触发 → aranea RCA Run → Webhook 回写 → RCA 记录落库 `ai_rca_records`。

**Files:**
- None（调用既有工具与 API）

- [ ] **Step 6.1 确认 NATS 已启动且 alarm.events 主题可发布**

```bash
nats stream info alarm.events 2>/dev/null || echo "stream 可能未显式创建，JetStream 默认接收"
# 只要 NATS 4222 可达即可
```

- [ ] **Step 6.2 编译并运行 nats-pub-alarm**

```powershell
cd f:/myproject/twinmonitor/TwinServer/cmd/tools/nats-pub-alarm
go run .
# 预期输出：published: stream=alarm.events seq=...
```

- [ ] **Step 6.3 验证 RCA 记录自动创建并推进到 analyzing/completed**

```bash
# 等待 10~30 秒
sleep 20
# 查询 RCA 记录列表
curl -s "http://127.0.0.1:8100/api/v1/monitor/aiops/rca-records" | jq '.items[0] | {id,alarmId,status,title}'
# 预期：alarmId="ALM20260814E2E01"，status 从 analyzing → completed
```

- [ ] **Step 6.4 数据库验证 ai_rca_records 落库**

```sql
-- file: f:/myproject/aranea-agents/docs/superpowers/plans/tmp_verify_rca.sql
SELECT id, alarm_id, status, title, root_cause IS NOT NULL AS has_root_cause
FROM ai_rca_records WHERE alarm_id = 'ALM20260814E2E01' ORDER BY id DESC LIMIT 1;
```

```bash
psql -h 127.0.0.1 -p 5432 -U postgres -d twinmonitor_log -f f:/myproject/aranea-agents/docs/superpowers/plans/tmp_verify_rca.sql
# 预期：status=completed，has_root_cause=t
```

- [ ] **Step 6.5 验证 aranea 侧 Run 与 Webhook 事件链路**

```bash
# 通过 task 反查 aranea_run_id
curl -s "http://127.0.0.1:8100/api/v1/monitor/aiops/tasks" | jq '.items[] | select(.scenarioName=="AI根因分析") | {id,status,araneaRunId}'
# 确认 status=completed
```

- [ ] **Step 6.6 git commit**

本 Task 无代码修改，无需 commit。

---

## 全局回归

- [ ] **回归 1：aranea 全量构建**

```bash
cd f:/myproject/aranea-agents
go build ./cmd/... ./internal/...
# 预期：0 错误
```

- [ ] **回归 2：twinmonitor 13-aiops 全量构建**

```bash
cd f:/myproject/twinmonitor/TwinServer
go build ./app/aiops/...
# 预期：0 错误
```

- [ ] **回归 3：清理临时 SQL 文件**

```bash
rm -f f:/myproject/aranea-agents/docs/superpowers/plans/tmp_*.sql
```

---

## 验收清单（对照总纲 §6 P0）

| 验收项 | 标准 | 验证方式 |
|--------|------|----------|
| aranea 服务部署 | `GET /api/v1/health` 200 | curl |
| 13 连通性探测 | AraneaPage 探测 green | POST /instances/{id}/test |
| 种子同步验证 | aranea GET /api/v1/agents 返回 12 个 | curl + jq |
| 最小 E2E | 任务状态 pending→running→completed | GET /tasks/{id} |
| 审批 interrupt 闭环 | waiting_approval → approve → completed | 审批 API + 任务查询 |
| RCA 端到端 | alarm.events → ai_rca_records completed | nats-pub-alarm + RCA API |
