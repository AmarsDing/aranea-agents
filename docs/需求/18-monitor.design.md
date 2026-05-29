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
