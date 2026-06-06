# Monitor 监控模块 — 实现设计文档

> 对应需求：`18 monitor.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`
> 2026-05-20 更新：Logs Tab 拆分为 **流程日志** / **进程日志** 二级 Tab；共享 `LogStreamHub`（单 WS）；legacy `EnvelopeTypeLog` 重复发射点迁移至 `flow_log`。
> 2026-05-21 更新：对齐代码 — **6 Tab**（含 Alerts）、`GetRunnerMetrics` / 告警规则 API、方案 C Phase 1d ✅（[changelog](../changelog/2026-05-20-Monitor-Phase1d-PlanC.md)）。
> 2026-05-29 更新：MON-OPT-01~06 ✅（FlowLog Bus 分离、告警冷却持久化、评估批量化、WS 反压、Trace 写入回路、告警注册表）；LOG-01/TRACE-01/DIAG-01/02 ✅（文件落盘、诊断包、根因引擎）；Latency P50/P95/P99 ✅；LOG-03 P0/P1/P2 ✅（红线修复、路径补全、fmt.Errorf 清理）；REDLINE ✅；QUALITY ✅（27 项质量修复）。

---

## 一、模块概述

运行时监控：审计日志、实时事件、模型用量总览、LLM 调用追踪、日志流。通过 `MonitorService`（Kratos HTTP/gRPC）提供结构化查询，通过 WebSocket 推送实时事件与日志。

### 1.1 功能边界

| 子模块 | 数据来源 | 传输方式 | 说明 |
|--------|----------|----------|------|
| Audit | `audit_logs` 表 | HTTP REST | 管理操作审计，支持分页/过滤 |
| Alerts | `monitor_alert_rules` 表 | HTTP REST | `runner.error_rate` 规则；评估后 `alert.fired` + 出站通知 |
| Runner 指标 | `monitor_events`（`runner.completion` 聚合） | HTTP REST | `GetRunnerMetrics`；Usage Tab 顶部面板 |
| Events | `monitor_events` 表 + WS 推送 | HTTP REST + WebSocket | 持久化事件 + 实时运行事件（方案 C 过滤 completion） |
| Usage | `model_token_usage_events` / `model_token_usage_daily` | HTTP REST（`UsageService`） | 模型用量总览、趋势、Top 排行 |
| Runs（Traces Tab） | `model_token_usage_events`（`metadata_json.spans`） | HTTP REST + WS `flow_log` | 单次运行真相源；瀑布图与 FlowLog **同源 Span 投影** |
| **Flow 流程日志** | WS `flow_log` | WebSocket | 业务时间线；Logs **流程** 二级 Tab；[52-flow-logger.design](./52-flow-logger.design.md) |
| **Process 进程日志** | WS `log` | WebSocket + `enable_log` | Gateway/插件 stderr；Logs **进程** 二级 Tab |

> **Tracing 与 Flow 分工**（v2）：OTel → Jaeger（运维）；Monitor 内 **FlowLog**（Logs Tab）+ **Span**（瀑布图），一次 `TraceEmitter` 写入。见 [52-flow-logger.design.md](./52-flow-logger.design.md)。

---

## 二、Proto 层

### 2.1 Monitor Proto

文件：`api/kratos/monitor/v1/monitor.proto`

```protobuf
service MonitorService {
  rpc ListAuditLogs(ListAuditLogsRequest) returns (ListAuditLogsResponse) {
    option (google.api.http) = {get: "/v1/monitor/audit"};
  }
  rpc ListMonitorEvents(ListMonitorEventsRequest) returns (ListMonitorEventsResponse) {
    option (google.api.http) = {get: "/v1/monitor/events"};
  }
  rpc GetMonitorEvent(GetMonitorEventRequest) returns (MonitorPlatformRow) {
    option (google.api.http) = {get: "/v1/monitor/events/{id}"};
  }
  rpc ListMonitorTraces(ListMonitorTracesRequest) returns (ListMonitorTracesResponse) {
    option (google.api.http) = {get: "/v1/monitor/traces"};
  }
  rpc GetMonitorTrace(GetMonitorTraceRequest) returns (MonitorTraceDetail) {
    option (google.api.http) = {get: "/v1/monitor/traces/{id}"};
  }
  rpc GetMonitorLogs(GetMonitorLogsRequest) returns (GetMonitorLogsResponse) {
    option (google.api.http) = {get: "/v1/monitor/logs"};
  }
  rpc ListMonitorAlertRules(GetMonitorLogsRequest) returns (ListMonitorAlertRulesResponse) {
    option (google.api.http) = {get: "/v1/monitor/alert-rules"};
  }
  rpc PutMonitorAlertRules(PutMonitorAlertRulesRequest) returns (PutMonitorAlertRulesResponse) {
    option (google.api.http) = { put: "/v1/monitor/alert-rules"; body: "*" };
  }
  rpc GetRunnerMetrics(GetRunnerMetricsRequest) returns (RunnerMetricsSummary) {
    option (google.api.http) = {get: "/v1/monitor/runner-metrics"};
  }
}
```

> `ListMonitorAlertRules` 复用空 `GetMonitorLogsRequest` 为占位入参（生成代码约定，无业务字段）。

### 2.5 告警与 Runner 指标消息

| 消息 | 说明 |
|------|------|
| `MonitorAlertRule` | `id`、`name`、`metric_key`、`threshold`、`window_minutes`、`enabled`、`severity`、`notify_webhook_url`、`notify_channel_id`、`cooldown_minutes` |
| `RunnerMetricsSummary` | `window_minutes`、`total_runs`、`error_runs`、`error_rate`、`success_rate` |

**评估**：`MonitorUsecase.EvaluateAlerts` 在 `runner.completion` 落库后由 EventBus Handler 触发；当前内置指标键 **`runner.error_rate`**。出站：`internal/service/monitor_notify.go`（Webhook POST + Channel `webhook_url`，尊重 `cooldown_minutes`）。

### 2.2 Audit 扩展字段

`AuditLog` 消息在原有 7 字段基础上新增：

| 字段 | Proto 编号 | 说明 |
|------|-----------|------|
| `actor` | 8 | 操作者（用户 ID / system） |
| `ip` | 9 | 客户端 IP |
| `user_agent` | 10 | 浏览器/客户端标识 |
| `severity` | 11 | 严重级别（info/warning/critical） |
| `metadata_json` | 12 | 扩展元数据 JSON |

### 2.3 分页与过滤

**ListAuditLogsRequest**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `limit` | int32 | 每页条数，默认 200 |
| `offset` | int32 | 偏移量 |
| `action` | string | 按事件类型过滤（如 create/delete/update） |
| `resource` | string | 按实体类型过滤（如 agent/team/channel） |
| `actor` | string | 按操作者过滤 |
| `keyword` | string | 全文模糊搜索（action/resource/resource_id/detail） |

**ListAuditLogsResponse**：新增 `total` 字段（int32），表示符合条件的总记录数。

**ListMonitorEventsRequest**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `limit` | int32 | 默认 100 |
| `offset` | int32 | 偏移量 |
| `event_type` | string | 按 event_key 前缀匹配 |
| `agent_id` | string | 按 metadata_json 中 agent_id 模糊匹配 |
| `status` | string | 按状态精确匹配 |

**ListMonitorTracesRequest**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `limit` | int32 | 默认 100 |
| `offset` | int32 | 偏移量 |
| `agent_id` | string | 按 metadata_json 中 agent_id 模糊匹配 |
| `provider` | string | 按 metadata_json 中 provider 模糊匹配 |
| `model` | string | 按 metadata_json 中 model 模糊匹配 |
| `status` | string | 按状态精确匹配 |

### 2.4 Usage Proto

文件：`api/kratos/usage/v1/usage.proto`

```protobuf
service UsageService {
  rpc GetUsageOverview(UsageQuery) returns (UsageOverview) { get: "/v1/usage/overview" }
  rpc ListUsageTrends(UsageQuery) returns (ListUsageTrendsResponse) { get: "/v1/usage/trends" }
  rpc ListTopModels(UsageQuery) returns (ListBreakdownResponse) { get: "/v1/usage/top-models" }
  rpc ListTopAgents(UsageQuery) returns (ListBreakdownResponse) { get: "/v1/usage/top-agents" }
  rpc ListUsageEvents(UsageQuery) returns (ListUsageEventsResponse) { get: "/v1/usage/events" }
  rpc RecordTokenUsageEvent(TokenUsageEvent) returns (TokenUsageEvent) { post: "/v1/usage/token-events" }
}
```

Monitor 前端 Usage Tab 通过 `UsageService` 获取数据，不经过 `MonitorService`。

---

## 三、Biz 层

### 3.1 领域模型

```go
type AuditLog struct {
    ID, Action, Resource, ResourceID, RequestID, Detail, CreatedAt string
    Actor, IP, UserAgent, Severity, MetadataJSON                  string
}

type AuditQuery struct {
    Limit int32; Offset int32
    Action, Resource, Actor, Keyword string
}

type AuditListResult struct {
    Items []AuditLog; Total int32
}

type MonitorPlatformRow struct {
    Resource, ID, Key, Name, Description, Status string
    Enabled bool; SortOrder int; ParentID string
    Level, AgentID, Provider, Model string
    ConfigJSON, MetadataJSON string
    CreatedAt, UpdatedAt, DeletedAt string
}

type MonitorEventsQuery struct {
    Limit, Offset int32
    EventType, AgentID, Status string
}

type MonitorTracesQuery struct {
    Limit, Offset int32
    AgentID, Provider, Model, Status string
}

type MonitorListResult struct {
    Items []MonitorPlatformRow; Total int32
}
```

### 3.2 Usecase

```go
type MonitorUsecase struct { repo MonitorRepo }

func (u *MonitorUsecase) ListAuditLogs(ctx, query AuditQuery) (AuditListResult, error)
func (u *MonitorUsecase) ListMonitorEvents(ctx, query MonitorEventsQuery) (MonitorListResult, error)
func (u *MonitorUsecase) GetMonitorEvent(ctx, id string) (MonitorPlatformRow, error)
func (u *MonitorUsecase) ListMonitorTraces(ctx, query MonitorTracesQuery) (MonitorListResult, error)
func (u *MonitorUsecase) GetMonitorTrace(ctx, id string) (MonitorPlatformRow, error)
```

### 3.3 Usage Usecase（独立模块）

```go
type UsageUsecase struct { repo UsageRepo }

func (u *UsageUsecase) Overview(ctx, query UsageQuery) (UsageOverview, error)
func (u *UsageUsecase) Trends(ctx, query UsageQuery) ([]UsageTrendPoint, error)
func (u *UsageUsecase) TopModels(ctx, query UsageQuery) ([]UsageBreakdownRow, error)
func (u *UsageUsecase) TopAgents(ctx, query UsageQuery) ([]UsageBreakdownRow, error)
func (u *UsageUsecase) Events(ctx, query UsageQuery) ([]TokenUsageEvent, error)
func (u *UsageUsecase) RecordTokenUsageEvent(ctx, e TokenUsageEvent) (TokenUsageEvent, error)
```

---

## 四、Data 层

### 4.1 表结构

**audit_logs**：

| 列 | 类型 | 说明 |
|----|------|------|
| id | TEXT PK | UUID |
| action | TEXT NOT NULL | 操作类型（create/delete/update/toggle/credentials） |
| resource | TEXT NOT NULL | 实体类型（agent/team/channel/provider/config/session） |
| resource_id | TEXT NOT NULL | 实体 ID |
| request_id | TEXT NOT NULL | 请求追踪 ID |
| detail | TEXT NOT NULL | 变更详情 |
| created_at | TEXT NOT NULL | ISO8601 时间戳 |
| actor | TEXT DEFAULT '' | 操作者 |
| ip | TEXT DEFAULT '' | 客户端 IP |
| user_agent | TEXT DEFAULT '' | 浏览器/客户端标识 |
| severity | TEXT DEFAULT '' | 严重级别 |
| metadata_json | TEXT DEFAULT '' | 扩展元数据 |

索引：`idx_audit_logs_action`、`idx_audit_logs_resource`、`idx_audit_logs_created_at`

**monitor_events** / **monitor_traces**：同原设计，新增 `idx_monitor_events_event_key`、`idx_monitor_events_status`、`idx_monitor_events_created_at`、`idx_monitor_traces_status`、`idx_monitor_traces_created_at` 索引。

**monitor_alert_rules**（`docs/sql/14_monitor_alert.sql`）：规则持久化；索引 `idx_monitor_alert_rules_enabled`。

### 4.2 查询模式

- **Audit**：`WHERE` 动态拼接（action/resource/actor/keyword），`COUNT(*)` 获取总数，`LIMIT/OFFSET` 分页
- **Events/Traces**：基础 `WHERE deleted_at = ''` + 动态条件追加，`COUNT(*)` 获取总数
- **Usage**：独立 `UsageRepo`，查询 `model_token_usage_events` 聚合

---

## 五、Service 层

### 5.1 MonitorService

- `ListAuditLogs`：接收 `ListAuditLogsRequest`，构造 `AuditQuery`，调用 Usecase，返回分页结果
- `ListMonitorEvents` / `ListMonitorTraces`：同上模式
- `GetMonitorTrace`：额外提取 `config_json` 中的 `spans`，组装 `MonitorTraceDetail`
- `GetMonitorLogs`：返回 `enabled`（镜像 `server.monitor.process_log_enabled`）+ hint；实时行走 WS
- `ListMonitorAlertRules` / `PutMonitorAlertRules`：告警规则 CRUD；PUT 时无规则则写入默认 `runner.error_rate` 种子
- `GetRunnerMetrics`：窗口内 completion 计数聚合 → `RunnerMetricsSummary`

### 5.2 JSON 脱敏

`sanitizeJSONString` 递归遍历 JSON，将包含 `api_key`/`token`/`secret`/`password`/`authorization`/`cookie` 的键值替换为 `******`。

### 5.3 Span 提取

`traceSpansRaw` 从 `config_json` 中提取 `spans` 或 `trace.spans` 数组，供 `MonitorTraceDetail.spans_json` 使用。

---

## 六、Wire 注入

已有，无需新增。`NewMonitorUsecase(repo) → NewMonitorService(uc)`。

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/
├── pages/MonitorPage.vue              ← 页面壳、6 Tab；query tab / usage_event_id
├── components/monitor/
│   ├── MonitorRunnerMetrics.vue       ← Usage Tab 容器：useRunnerMetrics + RunnerMetricsPanel
│   ├── RunnerMetricsPanel.vue         ← Runner 指标纯展示（props/emits）
│   ├── MonitorUsageDashboardLink.vue  ← 跳转 /overview、/usage/events
│   ├── MonitorAlertRules.vue          ← 告警规则 CRUD
│   ├── AuditTable.vue                 ← 活动日志（筛选 + 分页）
│   ├── RealtimeEvents.vue             ← WS 事件流（方案 C completion 过滤）
│   ├── TraceList.vue                  ← Runs 列表与详情
│   ├── TraceWaterfall.vue             ← 详情瀑布图
│   ├── FlowTracePanel.vue             ← 详情 Flow Tab
│   ├── FlowLogExportButton.vue        ← Flow JSONL 导出
│   ├── LogStreamPanel.vue             ← Logs 二级 Tab + 共享 Hub
│   ├── FlowLogStream.vue              ← 流程日志（flow_log）
│   ├── ProcessLogStream.vue           ← 进程日志（process_log_enabled）
│   ├── MonitorHeroSection.vue         ← 页面头部
│   ├── MonitorGlassPanel.vue          ← 玻璃态面板
│   └── MonitorErrorBanner.vue         ← 错误提示
├── features/monitor/
│   ├── api.ts                         ← Monitor + alert + runner-metrics API
│   ├── useRunnerMetrics.ts            ← Runner 指标 composable → Store
│   ├── runCorrelation.ts              ← 方案 C 关联与过滤
│   ├── useMonitorRunNavigation.ts     ← Chat / Runs / Monitor Tab 深链
│   ├── useLogStreamHub.ts             ← 共享 Logs WS
│   ├── types.ts                       ← RunnerMetricsSummary 等
│   └── utils.ts                       ← 格式化工具
└── stores/monitor/index.ts            ← Pinia Store（含 loadRunnerMetrics）
```

### 7.2 页面 Tab 布局

| Tab | 组件 | 数据来源 |
|-----|------|----------|
| **Usage** | `MonitorRunnerMetrics` + `MonitorUsageDashboardLink` | `GetRunnerMetrics`；用量大盘见 `/overview` |
| **Alerts** | `MonitorAlertRules` | `ListMonitorAlertRules` / `PutMonitorAlertRules` |
| **Audit** | `AuditTable` | `MonitorService.ListAuditLogs` |
| **Events** | `RealtimeEvents` | WS + `ListMonitorEvents`（告警；completion 降级） |
| **Runs（Traces）** | `TraceList` | `UsageService.ListUsageEvents`（单次运行真相源） |
| **Logs** | `LogStreamPanel` → `FlowLogStream` / `ProcessLogStream` | 共享 WS Hub + `flow_log` / `log` 分流 |

### 7.3 LogStreamHub（共享 WS）

Monitor Logs 使用 **单条** `session_id=*` WebSocket（全局上限 3），由 `useLogStreamHub` 管理生命周期：

```text
LogStreamPanel (mount)
    └─ useLogStreamHub
           ├─ createEnvelopeStream(channels: monitor, system)
           ├─ onConnected → state=connected（不依赖首条日志）
           ├─ onType(flow_log) → FlowLogStream 缓冲（可 paused）
           ├─ onType(log)      → ProcessLogStream 缓冲（需 process_log_enabled + 非 paused Tab）
           └─ enableLog(bool)  → 与 config 联动；config 关时 WS 忽略 enable_log(true)
```

**配置**（`configs/config.yaml`）：

```yaml
server:
  monitor:
    process_log_enabled: true   # 默认 true；false 时服务端不推送 EnvelopeTypeLog
```

| 操作 | 流程 Tab | 进程 Tab | WS |
|------|----------|----------|-----|
| 进入 Logs Tab | 自动 connect | 同左 | 1 连接 |
| 暂停 | `flowPaused=true`，丢弃入站 flow | 切离进程 Tab → `processPaused=true`，**丢弃**入站 log（不缓冲） | 保持 |
| 进程日志开关 | — | **无 UI**；由 `process_log_enabled` 控制 | globalMode 连接时 mirror config |
| 切到进程 Tab | — | `processPaused=false`，自动恢复显示 | 保持 |
| 离开 Logs Tab | Hub disconnect | 同左 | 释放 |

**后端约束**（`internal/server/ws.go`）：`enable_log(false)` 在 `globalMode` 下 **不得** 删除 `monitor` channel（否则误伤 `flow_log`）。

### 7.4 API

```typescript
listMonitorAudit(query: AuditQuery): Promise<PaginatedResult<AuditLog>>
listMonitorEvents(): Promise<PlatformResource[]>
getMonitorEvent(id: string): Promise<PlatformResource>
getMonitorLogs(): Promise<MonitorLogSnapshot>
subscribeMonitorLogsWs(...)  // 兼容；新代码用 createMonitorLogHub
createMonitorLogHub(opts): MonitorLogHub
subscribeMonitorRuntimeEventsWs(sessionId, onEvent, onError?): { close, connected }
listMonitorTraceEvents(query: ModelUsageQuery): Promise<MonitorTraceEvent[]>
listMonitorAlertRules(): Promise<MonitorAlertRule[]>
putMonitorAlertRules(rules: MonitorAlertRule[]): Promise<MonitorAlertRule[]>
getRunnerMetrics(windowMinutes?: number): Promise<RunnerMetricsSummary>
// runCorrelation.ts: shouldHideCompletionInEvents, completionCanOpenInRuns, ...
// useMonitorRunNavigation.ts: openRunsTab, openChatSession, ...
```

### 7.4 Usage Tab 与 Dashboard 分工

| 页面 | 组件 | 数据 |
|------|------|------|
| Monitor **Usage** | `MonitorRunnerMetrics`、`MonitorUsageDashboardLink` | `GET /v1/monitor/runner-metrics`；跳转携带 `filters.range` |
| **概览** `/overview` | `UsageMetricCards`、`UsageTrendChart`、`UsageBreakdownCharts` 等 | `GET /v1/usage/overview` 等 — 见 [18 monitor-dashboard.design.md](./18%20monitor-dashboard.design.md) |

已删除 `UsageOverview.vue`（避免与概览重复维护）。

---

## 八、数据保留与脱敏

- JSON 详情默认折叠大字段，单字段超过 2,000 字符时显示「展开」
- 密钥、Token、Authorization、Cookie、API Key 等字段统一用 `******` 脱敏
- WS 前端缓冲默认最多 1,000 条事件；Logs 默认最多 5,000 行

---

## 九、方案 C：Runs + Events + `runner.completion`

> 对应需求：[18 monitor.md §3–§4](./18%20monitor.md) · 开发计划：[18-monitor-development.md](./18-monitor-development.md) Phase 1d  
> **决策（2026-05-20）**：Chat 排障以 **Runs（Traces Tab / `model_token_usage_events`）** 为唯一详情壳；Events 收窄为实时/告警；`runner.completion` 仅落库 + correlation，不建平行 Events 详情页。

### 9.1 重复度结论

| 路径 | 写入 | UI 入口 | 与 Runs 关系 |
|------|------|---------|--------------|
| Chat Turn 结束 | `recordTurnUsage` → usage 行 | Runs 列表 + Trace 详情 | **真相源** |
| 同 Turn 结束 | `runner.completion` → `monitor_events` | 原 Events 列表 | **重复**（信息子集） |
| 告警 / Runner 指标 | COUNT `monitor_events` | Usage `RunnerMetricsPanel` | **保留落库** |

`TraceList` 详情已含：Summary、Flow（`trace_id` 过滤）、Waterfall、Span tree — **禁止在 Events 再实现一套**。

### 9.2 目标架构

```text
Chat Turn 结束
  ├─ recordTurnUsage (usage_kind=chat) ──► Runs 列表 / 详情（主排障）
  └─ runner.completion ──► monitor_events（告警、指标、correlation 元数据）
                              └─ Events：默认不列表；无 Runs 行时降级展示
```

### 9.3 `metadata_json`（`runner.completion/v1`，以关联为主）

| 字段 | 必填 | 说明 |
|------|------|------|
| `schema_version` | 是 | `runner.completion/v1` |
| `session_id` | 是 | 会话 |
| `trace_id` | 否 | 与 `recordTurnUsage` / FlowLogger 同 Turn |
| `usage_event_id` | 否 | 对应 `model_token_usage_events.id`（**Runs 行主键**） |
| `invocation_id` | 否 | 幂等键 |
| `request_id` | 否 | 链路 |
| `agent_id` / `agent_key` | 否 | 解析展示 |
| `status` / `duration_ms` / `usage` / `error` | 否 | 告警与降级卡片 |

**行级 `name`/`description`**：可选简短中文；**不**作为 Runs 列表数据源。

**写入时机**：`runnerCompletionHandler` 在落库前尽量从同 Turn 的 `TraceEmitter` / 已写入的 usage 行补齐 `trace_id`、`usage_event_id`（`internal/service/trpc_turn.go` + `turn_usage.go` 为锚点）。

### 9.4 Biz / Data

| 项 | 说明 |
|----|------|
| DomainEvent 扩展 | `RequestID`、`InvocationID` 等进入 `envelopeToDomainEvent` 或 Handler 直读 Envelope |
| `monitorRunnerCompletionMeta` | 输出 v1，**优先 correlation 字段** |
| 幂等 | `(event_key, session_id, invocation_id)` |
| 告警 | `EvaluateAlerts` / `runner.error_rate` **行为不变** |

### 9.5 Web（方案 C）

| 组件 | 变更 |
|------|------|
| `RealtimeEvents.vue` | 过滤 persisted `runner.completion`（有 `usage_event_id` 或可对上 Runs）；仅降级场景展示 |
| `TraceList.vue` | ✅ **打开会话**；列表标题 Runs；路由 Tab 仍为 `traces`（P2 可改标签） |
| `RunnerMetricsPanel.vue` | ✅ 点击下钻 `?tab=traces` |
| `features/monitor/runCorrelation.ts` | ✅ `shouldHideCompletionInEvents`、`completionCanOpenInRuns` 等 |
| `pages/MonitorPage.vue` | ✅ `tab` / `usage_event_id` query |

**不新建** `MonitorEventDetailDialog` 用于 Chat completion。

### 9.6 与其它模块

| 模块 | 关系 |
|------|------|
| [52-flow-logger](./52-flow-logger.design.md) | `trace_id` 对齐；排障在 Runs 详情 Flow Tab |
| Usage | Runs 列表即 `ListUsageEvents`；与 Events 分流 |
| Alerts / Memory | 仍消费 `runner.completion` 落库 |

### 9.7 非目标（Phase 1d）

- 不在 Events 为 Chat 建结构化 completion 详情（与 Runs 重复）。
- 不新增 `monitor_events` 表列（P2 再评估）。
- 不改变 WS `runner_completion` Envelope 类型。

### 9.8 后续（P2，可选）

- UI 标签 `Traces` → `Runs`，query `?tab=runs` 别名。
- `ListMonitorEvents` 服务端过滤 `hide_linked_completions`（减轻前端过滤）。


---

## 子模块：Monitor Dashboard 设计

> 对应需求：[18 monitor-dashboard.md](./18%20monitor-dashboard.md)  
> 用量契约：[29 token.design.md](./29%20token.design.md) · 运维页：[18 monitor.design.md](./18%20monitor.design.md)  
> **版本**：2026-05-21（Phase 2/3 + 前端分层整改）

---

## 一、架构定位

```text
OverviewPage (/overview)
  ├─ useOverviewPage → useUsageStore → features/usage/api → UsageService
  └─ OverviewRunnerMetrics → useRunnerMetrics → useMonitorStore → GetRunnerMetrics

MonitorPage Usage Tab
  ├─ MonitorRunnerMetrics → useRunnerMetrics（同上 Store）
  └─ MonitorUsageDashboardLink → /overview?range=（顶栏 filters.range）
```

```
                    ┌─────────────────────────────────────┐
                    │  OverviewPage (/overview)           │
                    └──────────────┬──────────────────────┘
                                   │
          ┌────────────────────────┼────────────────────────┐
          │                        │                        │
          ▼                        ▼                        ▼
   useUsageStore            useMonitorStore          components (props)
   UsageService            MonitorService
```

Runner 指标 **不写入 Usage 表**；只读 `GET /v1/monitor/runner-metrics`。

---

## 二、后端

### 2.1 已有 API

| RPC | HTTP | 响应要点 |
|-----|------|----------|
| `GetUsageOverview` | `GET /v1/usage/overview` | `today` / `yesterday` / `month` / `range` / `trends` / `top_models` / `top_agents` / `anomalies` / `inefficient_models` / `quota_dashboard` |
| `ListUsageTrends` | `GET /v1/usage/trends` | `granularity=hour` → `model_token_usage_hourly` |
| `ListUsageEvents` | `GET /v1/usage/events` | 明细页 |
| `GetRunnerMetrics` | `GET /v1/monitor/runner-metrics` | `RunnerMetricsSummary`（Dashboard / Monitor Usage 共用） |

### 2.2 聚合规则（读路径）

- `UsageUsecase.Overview` → `usageWhere(..., billableOnly=true)` 排除 `team_turn`。
- 状态归一：`usage_status_sql.go`。
- 费用：`usage_pricing.go` + Provider 模型 `config_json`。

### 2.3 待扩展 API

| 能力 | 说明 |
|------|------|
| `top_providers` 独立聚合 | Provider 饼图当前基于 `top_models` 样本，非全量 Provider rollup |
| P50/P95 延迟 | `UsageOverview` 增 `latency_percentiles`（MDB-02-06） |

---

## 三、前端分层（`frontend-guide.md`）

### 3.1 合法数据流

```text
用量大盘：
  Page → useOverviewPage → useUsageStore.loadOverview → features/usage/api

Runner 指标：
  容器组件 → useRunnerMetrics → useMonitorStore.loadRunnerMetrics → features/monitor/api

图表：
  UsageTrendChart / UsageBreakdownCharts（props only）
    → useUsageChart（ECharts 生命周期）
    → usageTrendMetrics / usageBreakdownSlices（纯函数）
```

**红线遵守**：

- `RunnerMetricsPanel.vue`：**仅 props/emits**，不 import `api` / `store`。
- `OverviewPage` / `MonitorPage`：不直连 `features/*/api`（Runner 经容器 composable）。

### 3.2 目录结构（当前）

```text
web/src/
├── pages/OverviewPage.vue
├── features/usage/
│   ├── useOverviewPage.ts
│   ├── useUsageChart.ts              ← ECharts 宿主 + resize debounce
│   ├── usageEcharts.ts               ← 按需注册 + 主题色（--color-success/danger）
│   ├── usageTrendMetrics.ts
│   ├── usageBreakdownSlices.ts
│   └── api.ts / types.ts
├── features/monitor/
│   ├── useRunnerMetrics.ts           ← Runner 唯一请求入口（页面/容器）
│   └── useMonitorRunNavigation.ts
├── stores/
│   ├── usage/index.ts
│   └── monitor/index.ts              ← loadRunnerMetrics
└── components/
    ├── usage/
    │   ├── OverviewPageHero.vue
    │   ├── OverviewRunnerMetrics.vue ← 容器：composable + RunnerMetricsPanel
    │   ├── OverviewMonitorQuickLinks.vue
    │   ├── UsageMetricCards.vue
    │   ├── UsageTrendChart.vue       ← async chunk
    │   ├── UsageBreakdownCharts.vue  ← async chunk
    │   └── …
    └── monitor/
        ├── RunnerMetricsPanel.vue    ← 纯展示
        ├── MonitorRunnerMetrics.vue  ← 容器
        └── MonitorUsageDashboardLink.vue
```

已删除：`UsageTrendPanel.vue`、`UsageOverview.vue`。

### 3.3 用量数据流

```text
筛选变更 / onMounted
  → useOverviewPage.loadOverview()
  → useUsageStore.loadOverview(query, granularity)
  → getModelUsageOverview + [hour] listModelUsageTrends
  → 子组件 props（overview.trends / top_models / …）
```

### 3.4 路由与深链

| Query | 行为 |
|-------|------|
| `range` | `useOverviewPage` 初始化筛选（`?range=30d` 等） |
| — | 「打开概览」从 Monitor 携带 `range`，与顶栏 `filters.range` 一致 |

### 3.5 图表实现

| 模块 | 实现 |
|------|------|
| 趋势 | `UsageTrendChart`：metric 切换 tokens / calls / cost / success_rate（堆叠 %） |
| 占比 | `UsageBreakdownCharts`：模型 Top5 费用环图；Provider 由 Top 模型样本聚合（UI 已标注） |
| 分包 | `defineAsyncComponent` + `useUsageChart` 独立 chunk（`usageEcharts`） |

主题色：`usageChartPalette()` 读取 `--color-accent`、`--color-success`、`--color-danger`。

---

## 四、Monitor Usage Tab（已实现）

```text
Monitor → Usage Tab
  ├── MonitorRunnerMetrics（Store + RunnerMetricsPanel）
  └── MonitorUsageDashboardLink（打开概览 / 查看明细；range 用页面顶栏，无重复下拉）
```

不再维护 `UsageOverview.vue`。

---

## 五、测试

| 层 | 内容 |
|----|------|
| Web | `usageTrendMetrics.spec.ts`、`usageBreakdownSlices.spec.ts` |
| Web（P2） | Overview 筛选 → store mock；E2E `/overview` → `/usage/events` |
| Go | `usage_*_test.go` 聚合口径 |

---

*任务与验收见 [18-monitor-dashboard-development.md](./18-monitor-dashboard-development.md)。*


---

## 子模块：Monitor Loop 01 设计

> **版本**：2026-05-29-v2 | **状态**：🟡 待实施
> **需求**：[`18-monitor-loop-01-requirement.md`](./18-monitor-loop-01-requirement.md)
> **规则真相源**：[`project_rules.md`](../../.trae/rules/project_rules.md) · [`aranea-coding-guide`](../../.trae/skills/aranea-coding-guide.md)

---

## 1. 设计目标

用 FlowLog/SysLog 替代 `fmt.Println`/`log.Printf`，让系统运行信息直接显示在 Monitor Logs 界面，方便开发时定位问题。

**核心思路**：不是新增功能，而是**补全和统一**——把散落在 `log.Printf`、Kratos `log.Helper` 中的调试信息统一收敛到 FlowLog 体系。

---

## 2. 数据流

```
系统运行时
    │
    ├── event.SysLogInfo(stepID, msg, ...Pair)    ← 正确方式
    ├── event.SysLogWarn(stepID, msg, ...Pair)    ← 正确方式
    ├── event.SysLogError(stepID, msg, ...Pair)   ← 正确方式
    │
    ├── log.Printf(...)                            ← ❌ 红线违规，需消除
    ├── w.log.Warnf(...)                           ← ❌ 冗余，需消除
    └── fmt.Println(...)                           ← ❌ 调试残留，需消除
          │
          ▼
    MonitorBus (channel="monitor")
          │
          ├── FlowFileAppender → JSONL 文件落盘
          ├── TraceProjector → monitor_traces 表
          ├── flowLogPersistConsumer → monitor_events 表
          └── WS 推送 → 前端 Monitor Logs 页面
```

---

## 3. 修复方案

### 3.1 FR-01：消除 `log.Printf` 红线违规

#### 3.1.1 `internal/biz/evolution.go`

**当前**：
```go
import "log"

log.Printf("[EVOLUTION] GetToolSuccessRate agent=%s err=%v", agentID, err)
log.Printf("[EVOLUTION] GetRetrievalQuality agent=%s err=%v", agentID, err)
log.Printf("[EVOLUTION] GetEpisodeCount agent=%s err=%v", agentID, err)
log.Printf("[EVOLUTION] GetNegativeFeedbackCount agent=%s err=%v", agentID, err)
```

**修复**：
```go
import "event"

event.SysLogWarn("system.evolution.metrics_fail", "evolution metric query failed",
    event.P("metric", "tool_success_rate"), event.P("agent_id", agentID), event.P("error", err.Error()))
event.SysLogWarn("system.evolution.metrics_fail", "evolution metric query failed",
    event.P("metric", "retrieval_quality"), event.P("agent_id", agentID), event.P("error", err.Error()))
event.SysLogWarn("system.evolution.metrics_fail", "evolution metric query failed",
    event.P("metric", "episode_count"), event.P("agent_id", agentID), event.P("error", err.Error()))
event.SysLogWarn("system.evolution.metrics_fail", "evolution metric query failed",
    event.P("metric", "negative_feedback_count"), event.P("agent_id", agentID), event.P("error", err.Error()))
```

**改动**：移除 `import "log"`，新增 `import "event"`（路径 `aranea-agents/internal/event`）。

#### 3.1.2 `internal/modelcatalog/runner.go`

**当前**：
```go
logger *log.Logger

r.logger.Printf("model-catalog: store resolve failed: %v", err)
r.logger.Printf("model-catalog: schedule check failed: %v", err)
r.logger.Printf("model-catalog: scheduled sync failed: %v", err)
r.logger.Printf("model-catalog: scheduled sync apply failed: %v", applyRes.Errors)
r.logger.Printf("model-catalog: scheduled sync ok providers=%d models=%d policy=%s", ...)
```

**修复**：
```go
event.SysLogWarn("system.model_catalog.resolve_fail", "model catalog store resolve failed",
    event.P("error", err.Error()))
event.SysLogWarn("system.model_catalog.sync_fail", "model catalog schedule check failed",
    event.P("error", err.Error()))
event.SysLogWarn("system.model_catalog.sync_fail", "model catalog scheduled sync failed",
    event.P("error", err.Error()))
event.SysLogWarn("system.model_catalog.sync_fail", "model catalog sync apply failed",
    event.P("errors", fmt.Sprintf("%v", applyRes.Errors)))
event.SysLogInfo("system.model_catalog.sync_ok", "model catalog sync completed",
    event.P("providers", providers), event.P("models", models), event.P("policy", policy))
```

**改动**：移除 `*log.Logger` 字段和 `log.New(...)` 初始化，新增 `import "event"`。注意 `fmt.Sprintf` 仍保留用于 `applyRes.Errors` 的格式化（非红线违规）。

### 3.2 FR-02：清理 cronrunner 双重日志

**模式 A — 已有 FlowLog 的冗余调用**（12 处）：

直接删除 `w.log.*` 行，保留 `event.SysLogWarn`。

```go
// Before:
event.SysLogWarn("memory.l4_decay", "list targets failed", event.P("error", err.Error()))
w.log.Warnf("memory l4 decay: list targets: %v", err)  // ← 删除

// After:
event.SysLogWarn("memory.l4_decay", "list targets failed", event.P("error", err.Error()))
```

**模式 B — 仅有 Kratos 日志的缺口**（17 处）：

先补充 FlowLog，再删除 Kratos 日志。

```go
// Before:
w.log.Infof("memory l4 decay: %d agents, importance=%d", len(targets), importance)  // ← 仅 Kratos

// After:
event.SysLogInfo("memory.l4_decay", "decay completed",
    event.P("agents", len(targets)), event.P("importance", importance))
```

**最终**：移除 `cronrunner` 中所有 `*log.Helper` 字段和构造函数参数。

### 3.3 FR-03：补全 stepTitleRegistry

在 `internal/event/flow_log.go` 的 `stepTitleRegistry` 中新增 22 个条目：

```go
"system.evolution.metrics_fail":    "进化指标查询失败",
"system.model_catalog.resolve_fail": "模型目录解析失败",
"system.model_catalog.sync_fail":    "模型目录同步失败",
"system.model_catalog.sync_ok":      "模型目录同步完成",
"memory.l4_decay":                   "L4 图谱衰减",
"memory.l2_decay":                   "L2 情景衰减",
"memory.l3_decay":                   "L3 事实衰减",
"memory.index_reconcile":            "记忆索引对账",
"memory.dead_letter_replay":         "记忆死信重放",
"memory.data_migration":             "记忆数据迁移",
"memory.episode_backfill":           "情景嵌入回填",
"event_store.cleanup":               "事件存储清理",
"flow_log.cleanup":                  "流程日志清理",
"tool_audit.cleanup":                "工具审计清理",
"channel.delivery":                  "渠道投递",
"channel.health":                    "渠道健康检查",
"provider.health":                   "模型供应商健康检查",
"evolution.scanner":                 "进化扫描",
"monitor.alert_cooldown_cleanup":    "告警冷却清理",
"webresearch.proxy_parse":           "网络研究代理解析",
"knowledge_reflect.eval_fail":       "知识反思评估失败",
"graph.event_bridge":                "图事件桥接",
```

---

## 4. 实施分期

| 阶段 | 内容 | 文件数 | 改动量 |
|------|------|--------|--------|
| **Phase 1** | FR-01：消除 `log.Printf` 红线违规 | 2 | 9 处替换 |
| **Phase 2** | FR-02：清理 cronrunner 双重日志 | 15 | 29 处替换/删除 |
| **Phase 3** | FR-03：补全 stepTitleRegistry | 1 | 22 条注册 |

---

## 5. 验证方案

| 阶段 | 验证命令 |
|------|----------|
| Phase 1 | `go build ./internal/biz/... ./internal/modelcatalog/...` + `go vet` |
| Phase 2 | `go build ./internal/cronrunner/...` + `go vet` |
| Phase 3 | `go build ./internal/event/...` |
| 全量 | `grep -rn 'log\.Printf\|log\.Infof\|log\.Warnf\|log\.Errorf' internal/biz/ internal/modelcatalog/` → 0 结果 |

---

## 6. 远期展望：AI 辅助分析

当 FR-01~03 完成后，系统日志覆盖率和结构化程度将足够支撑 AI 分析。远期可考虑：

1. **AI 日志分析 Agent**：内置 `__system_optimizer__` Agent，读取 JSONL 日志，识别错误模式
2. **代码修复建议**：AI 定位到具体代码文件和行号，生成 diff
3. **人工审批闭环**：AI 建议经人工审批后执行，自动验证

此为独立需求，不在 LOOP-01 范围内，需另行设计。
