# Catalog / 平台残余 + 可观测性 收口清单（逐个 PR）

> **母文档**：[`AI-full-stack-migration-playbook.md`](./AI-full-stack-migration-playbook.md)、[`pkg-backend-to-kratos.md`](./pkg-backend-to-kratos.md) §6.3。  
> **目的**：在 **`session/v1` 与 Avatar/平台 CRUD 已落地** 的前提下，把仍走 **`pkg/backend`** `registerRoutes`（`/api/v1/...`）的 **通道 / 工具 / 监控 / 用量** 按 **依赖由浅入深、先读后写、先 JSON 后 SSE/multipart** 收口到 **`cmd/admin` + `api/kratos/*/v1`**。

---

## 1. 现状速览（旧路由 → 前端）

| 优先级 | 遗留分组 | 旧前缀（`handler.go`） | 前端落点 | 备注 |
|--------|----------|------------------------|----------|------|
| **①** | **Tools（capability 目录）** | `/api/v1/tools`、`/tools/*/runs` 等 | [`web/src/features/tools/api.ts`](../../web/src/features/tools/api.ts)，`ToolsPage.vue`、`ToolRunsPage.vue` | **目录 CRUD + runs** 已迁 **`tool/v1`**（`createToolService()` → `/v1/tools*`）；**`GET .../agents/:id/tools/effective`** 仍 **`legacyRestApi`** `/api/v1/...` |
| **②** | **Channels 通道** | `/channels/catalog`、`/channels`、`/channels/*` | [`web/src/features/channels/api.ts`](../../web/src/features/channels/api.ts)，`ChannelsPage.vue` | catalog 只读 + 实例 CRUD / test / credentials |
| **③** | **Skill 导入（multipart / 多段 JSON）** | `/skills/import`、`/skills/import/*` | [`web/src/features/skills/api.ts`](../../web/src/features/skills/api.ts)，`SkillsPage.vue` | Playbook **A9**：须写明兼容策略 |
| **④** | **用量 model-usage** | ~~`/model-usage/...`~~ → **`/v1/usage/*`**（Kratos） | [`web/src/features/usage/api.ts`](../../web/src/features/usage/api.ts)：`**createUsageService()`** + snake_case 映射；`OverviewPage`、`ProviderTrendDialog` | ✅ **`usage/v1`**（后端 `UsageService` + `biz/data usage`） |
| **⑤** | **Monitor 监控** | `/monitor/audit`、`/monitor/events`、`/monitor/traces`、`/monitor/logs*` | [`web/src/features/monitor/api.ts`](../../web/src/features/monitor/api.ts)（`legacyRestApi`）；`MonitorPage.vue` | 含 **SSE**：`/monitor/logs/stream`；audit 依赖 **AuditService**（非 Platform 通用 List） |
| **⑥（横切）** | **Team 运行 SSE** | `/team-run-events` | `features/monitor/api.ts` `subscribeMonitorRuntimeEvents` | 可与 `team/v1` 流式补齐同一阶段 |

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

4. **`monitor/v1`**  
   - 只读：`ListAudit`、`ListMonitorEvents`、`GetMonitorEvent`、`ListMonitorTraces`、`GetMonitorTrace`（结构与 `sanitizePlatformResource` 对齐）。  
   - **SSE**：`StreamMonitorLogs` 或网关层保留 SSE 路径（须在 proto/README 单列）。  
   - 注意：**`listMonitorTraceEvents` 在前端当前实现里调用的是 `/model-usage/events`**（非 `/monitor/traces`），收口时要么统一语义要么拆两个 RPC。

5. **Skill import**  
   - 对齐 `POST /skills/import`、poll job、conflict refine、apply；** multipart** 遵守主文档 §2.4。

6. **`team-run-events` SSE**  
   - 归入 `team/v1` 流式扩展或独立 `SubscribeTeamRuns` RPC；CLI `capability` 种子里的路径一并更新。

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
