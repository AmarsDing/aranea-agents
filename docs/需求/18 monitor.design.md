# Monitor 监控模块 — 实现设计文档

> 对应需求：`18 monitor.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`
> 2026-05-18 现状对齐：本文档已与当前代码实现完全对齐，补充了分页/过滤/Usage 整合等设计。

---

## 一、模块概述

运行时监控：审计日志、实时事件、模型用量总览、LLM 调用追踪、日志流。通过 `MonitorService`（Kratos HTTP/gRPC）提供结构化查询，通过 WebSocket 推送实时事件与日志。

### 1.1 功能边界

| 子模块 | 数据来源 | 传输方式 | 说明 |
|--------|----------|----------|------|
| Audit | `audit_logs` 表 | HTTP REST | 管理操作审计，支持分页/过滤 |
| Events | `monitor_events` 表 + WS 推送 | HTTP REST + WebSocket | 持久化事件 + 实时运行事件 |
| Usage | `model_token_usage_events` / `model_token_usage_daily` | HTTP REST（`UsageService`） | 模型用量总览、趋势、Top 排行 |
| Traces | `monitor_traces` 表 + `model_token_usage_events` | HTTP REST + WebSocket | LLM 调用链追踪与 Span 树 |
| Logs | WS 推送 + 内存快照 | WebSocket + HTTP REST | Gateway/运行时文本日志流 |

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
}
```

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
- `GetMonitorLogs`：返回 WS 提示信息（实际日志通过 WS 推送）

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
├── pages/MonitorPage.vue              ← 页面壳、5 Tab（Usage/Audit/Events/Traces/Logs）
├── components/monitor/
│   ├── UsageOverview.vue              ← ★ 新增：模型用量总览卡片
│   ├── AuditTable.vue                 ← 活动日志表格（增强：事件类型/实体类型筛选 + 分页）
│   ├── RealtimeEvents.vue             ← WS 事件流
│   ├── EventTimeline.vue              ← Envelope 事件时间线
│   ├── TraceList.vue                  ← Trace 列表与详情
│   ├── LogStream.vue                  ← 日志流
│   ├── MonitorHeroSection.vue         ← 页面头部
│   ├── MonitorGlassPanel.vue          ← 玻璃态面板
│   └── MonitorErrorBanner.vue         ← 错误提示
├── features/monitor/
│   ├── api.ts                         ← Monitor API（含分页/过滤参数）
│   ├── types.ts                       ← 类型定义（含 AuditQuery/PaginatedResult）
│   └── utils.ts                       ← 格式化工具
├── features/usage/
│   ├── api.ts                         ← Usage API
│   └── types.ts                       ← Usage 类型
└── stores/monitor/index.ts            ← Pinia Store
```

### 7.2 页面 Tab 布局

| Tab | 组件 | 数据来源 |
|-----|------|----------|
| **Usage** | `UsageOverview` | `UsageService.GetUsageOverview` |
| **Audit** | `AuditTable` | `MonitorService.ListAuditLogs` |
| **Events** | `RealtimeEvents` | `MonitorService.ListMonitorEvents` + WS |
| **Traces** | `TraceList` | `UsageService.ListUsageEvents` |
| **Logs** | `LogStream` | `MonitorService.GetMonitorLogs` + WS |

### 7.3 API

```typescript
listMonitorAudit(query: AuditQuery): Promise<PaginatedResult<AuditLog>>
listMonitorEvents(): Promise<PlatformResource[]>
getMonitorEvent(id: string): Promise<PlatformResource>
getMonitorLogs(): Promise<MonitorLogSnapshot>
subscribeMonitorLogsWs(sessionId, onLine, onError?, onConnected?): { close, connected, enableLog }
subscribeMonitorRuntimeEventsWs(sessionId, onEvent, onError?): { close, connected }
listMonitorTraceEvents(query: ModelUsageQuery): Promise<MonitorTraceEvent[]>
```

### 7.4 UsageOverview 组件

展示内容：
- **指标卡**：今日请求数、成功率、Token 总量、今日费用
- **Top 模型**：按成本/调用数排序，展示 provider/model/成功率
- **Top Agent**：按调用量/成本排序，展示 agent/tokens/成功率
- **最近异常**：最近失败模型调用，显示时间/Agent/Provider/错误信息

---

## 八、数据保留与脱敏

- JSON 详情默认折叠大字段，单字段超过 2,000 字符时显示「展开」
- 密钥、Token、Authorization、Cookie、API Key 等字段统一用 `******` 脱敏
- WS 前端缓冲默认最多 1,000 条事件；Logs 默认最多 5,000 行
