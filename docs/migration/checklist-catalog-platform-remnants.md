# Catalog / 平台残余 + 可观测性 收口清单（逐个 PR）

> **母文档**：[`AI-full-stack-migration-playbook.md`](./AI-full-stack-migration-playbook.md)、[`pkg-backend-to-kratos.md`](./pkg-backend-to-kratos.md) §6.3。  
> **目的**：在 **`session/v1` 与 Avatar/平台 CRUD 已落地** 的前提下，把仍走 **`pkg/backend`** `registerRoutes`（`/api/v1/...`）的 **通道 / 工具 / 监控 / 用量** 按 **依赖由浅入深、先读后写、先 JSON 后 SSE/multipart** 收口到 **`cmd/admin` + `api/kratos/*/v1`**。

---

## 1. 现状速览（旧路由 → 前端）

| 优先级 | 遗留分组 | 旧前缀（`handler.go`） | 前端落点 | 备注 |
|--------|----------|------------------------|----------|------|
| **①** | **Tools（capability 目录）** | `/api/v1/tools`、`/tools/*/runs` 等 | [`web/src/features/tools/api.ts`](../../web/src/features/tools/api.ts)，`ToolsPage.vue`、`ToolRunsPage.vue` | **目录 CRUD + runs** 已迁 **`tool/v1`**（`createToolService()` → `/v1/tools*`）；**Agent 生效工具矩阵** **`GET /v1/agents/{id}/tools/effective`** + **`PUT .../tools/policy`** 已迁 **`agent/v1`**（`createAgentService()`）；**其它**仍可能走 **`legacyRestApi`** `/api/v1/...` |
| **②** | **Channels 通道** | `/channels/catalog`、`/channels`、`/channels/*` | [`web/src/features/channels/api.ts`](../../web/src/features/channels/api.ts)，`ChannelsPage.vue` | catalog 只读 + 实例 CRUD / test / credentials |
| **③** | **Skill 导入（multipart / 多段 JSON）** | `/skills/import`、`/skills/import/*` | [`web/src/features/skills/api.ts`](../../web/src/features/skills/api.ts)，`SkillsPage.vue` | Playbook **A9**：须写明兼容策略 |
| **④** | **用量 model-usage** | ~~`/model-usage/...`~~ → **`/v1/usage/*`**（Kratos） | [`web/src/features/usage/api.ts`](../../web/src/features/usage/api.ts)：`**createUsageService()`** + snake_case 映射；`OverviewPage`、`ProviderTrendDialog` | ✅ **`usage/v1`**（后端 `UsageService` + `biz/data usage`） |
| **⑤** | **Monitor 监控** | ~~`/monitor/...`~~ → **`/v1/monitor/*`**（Kratos）；SSE：**`/sse/monitor/logs/stream`** → **`server.sse`**（tx7do） | [`web/src/features/monitor/api.ts`](../../web/src/features/monitor/api.ts)：**`createMonitorService()`**；展示 UI：[`web/src/components/monitor/`](../../web/src/components/monitor/)（`MonitorPage.vue` 与各 Tab：`AuditTable`、`TraceList`、`RealtimeEvents`、`LogStream`、Hero/Glass/Error 等）；Trace 列表 → **`usage/v1`**（`listModelUsageEvents`） | ✅ **读路径 + logs SSE 已收口** |
| **⑥（横切）** | **Team 运行 SSE** | ~~`/api/v1/team-run-events`~~ → **`/sse/team-run-events`**（`cmd/admin` SSE，`biz.TeamRunEventBroker`） | [`web/src/features/monitor/api.ts`](../../web/src/features/monitor/api.ts) `subscribeMonitorRuntimeEvents`；[`web/src/features/teams/api.ts`](../../web/src/features/teams/api.ts) `subscribeTeamRunEvents` | ✅ **端点已迁**；**Publish** 接线编排栈待后续 |

---

## 2. 建议「逐个收口」顺序（产品与风险可微调）

自上而下：**工具目录 → 通道 → 用量 → 监控**；技能 ZIP 导入可并行或紧随其后（耦合 multipart）。

1. **`tool/v1`**（✅ 管理 UI 已收口）  
   - 对齐 **`pkg/backend`** capability  **`/tools`** 列表/CRUD/runs。  
   - 前端：`features/tools/api.ts` → **`createToolService()`**（`/v1/tools*`）；**`getAgentEffectiveTools`** 仍为 **`legacyRestApi`** `/api/v1/agents/.../tools/effective`。

2. **`channel/v1`**  
   - RPC：`ListChannelCatalog`、`ListChannels`、`Get/Create/Update/Delete`、`Toggle`、`Test`、`Credentials`。  
   - 后端：对照 `pkg/backend` `ChannelService` + 表结构（`0001_init.sql`）。

3. **`usage/v1`**（✅ 管理 UI 已收口）  
   - RPC：`GetUsageOverview`、`ListUsageTrends`、`ListTopModels`、`ListTopAgents`、`ListUsageEvents`（`/v1/usage/*`）。  
   - 前端：`features/usage/api.ts` → **`createUsageService()`**；响应 **`range_summary` → `range`** 等映射为遗留 **`ModelUsage*`** snake_case 形状。

4. **`monitor/v1`**（✅ 管理 UI 读路径已收口）  
   - RPC：`ListAuditLogs`、`ListMonitorEvents`、`GetMonitorEvent`、`ListMonitorTraces`、`GetMonitorTrace`、`GetMonitorLogs`（占位）。HTTP：`/v1/monitor/*`。  
   - 前端：`features/monitor/api.ts` → **`createMonitorService()`**；**Trace 表格**数据语义对齐 **`usage/v1`**（`listModelUsageEvents`）。  
   - **前端分层**（与 Playbook **B5b**、`vue-design.md` §2）：展示 `.vue` 在 **`components/monitor/`**；**`features/monitor/`** 仅存 **`api.ts`、`types.ts`、`utils.ts`**。  
   - **SSE**：`/sse/monitor/logs/stream`（tx7do）、`/sse/team-run-events`（broker + 手写帧），见 **`configs.server.sse`** 与 Quasar **`/sse` → `server.sse.addr`**。

5. **Skill import**  
   - 对齐 `POST /skills/import`、poll job、conflict refine、apply；** multipart** 遵守主文档 §2.4。

6. **`team-run-events` SSE（端点已迁）**  
   - **`biz.TeamRunEventBroker.Publish`**：待团队运行写入迁入 **`cmd/admin`** 会话/编排栈后从运行路径调用；可选后续：`team/v1` 流式扩展或独立 RPC，CLI `capability` 种子路径一并更新。

---

## 3. 每域机械验收（与 Playbook §8 一致）

- [ ] `api/kratos/<x>/v1/*.proto` 覆盖本迭代 **全部** 对外 HTTP 能力（含 SSE/multipart 策略说明）。  
- [ ] `make api` + `internal/service` + `Register*HTTPServer` + **`wire`** + **`go build ./cmd/admin`**。  
- [ ] `web/src/services/index.ts`：`create*Service`。  
- [ ] `web/src/features/<域>/api.ts`：仅此层接触生成客户端；Page 不入展示组件违规。  
- [ ] 回写 **`pkg-backend-to-kratos.md` §6.3** 与 **Playbook §6** 行 2 / 行 7～9。

---

## 4. 与「核心对话链」关系

**`chat/v1`**（`/chat/messages`、`stream`、`options`）与上述 **catalog/平台收口** 在依赖上相对独立；可并行立项，但不要混在同一个 proto 包里 unless 产品确认合并为 `conversation/v1`。

---

*版本：2026-04-29 · 随 PR 更新表 1「优先级」各行状态。*
