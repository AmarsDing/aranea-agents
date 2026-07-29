# Monitor 监控模块 — 实现设计文档

> 对应需求：`18 monitor.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`
> 2026-05-20 更新：Logs Tab 拆分为 **流程日志** / **进程日志** 二级 Tab；共享 `LogStreamHub`（单 WS）；legacy `EnvelopeTypeLog` 重复发射点迁移至 `flow_log`。
> 2026-05-21 更新：对齐代码 — **6 Tab**（含 Alerts）、`GetRunnerMetrics` / 告警规则 API、方案 C Phase 1d（[changelog](../changelog/2026-05-20-Monitor-Phase1d-PlanC.md)）。
> 2026-05-29 更新：MON-OPT-01~06 设计方案；LOG-01/TRACE-01/DIAG-01/02 设计方案；Latency P50/P95/P99；LOG-03 P0/P1/P2 红线修复方案；REDLINE；QUALITY。
> 2026-06-06 更新：自检/自愈/模式挖掘设计方案；LOOP-01 设计方案。

> 实现进度、任务清单、状态标记见 [18-monitor.development.md](./18-monitor.development.md)；用户故事、功能需求、验收标准见 [18 monitor.md](./18%20monitor.md)。

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
| **Runs（Traces Tab）** | `monitor_traces`（OPT-05 投影；列表含 duration/tokens/cost）+ WS `flow_log` | HTTP REST + WS `flow_log` | 单次运行真相源（方案 C 原写 usage events；OPT-05 后以 traces 投影为主，usage 经 correlation 关联） |
| **Flow 流程日志** | WS `flow_log` | WebSocket | 业务时间线；Logs **流程** 二级 Tab；[52-flow-logger.design](./52-flow-logger.design.md) |
| **Process 进程日志** | WS `log` | WebSocket + `enable_log` | Gateway/插件 stderr；Logs **进程** 二级 Tab |
| **自检 SelfCheck** | 内置 Checker 插件 | 定时（5 min） | 周期性子系统健康检查 + 自动修复 |
| **自愈 SelfHeal** | FlowLog 错误事件 + 诊断包 | 事件驱动 | 诊断→根因→修复闭环；含预测性自愈与模式挖掘 |

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
  rpc ListFlowLogs(ListFlowLogsRequest) returns (ListFlowLogsResponse) {
    option (google.api.http) = {get: "/v1/monitor/flow-logs"};
  }
  rpc ListMonitorAlertRules(GetMonitorLogsRequest) returns (ListMonitorAlertRulesResponse) {
    option (google.api.http) = {get: "/v1/monitor/alert-rules"};
  }
  rpc ListAlertMetrics(GetMonitorLogsRequest) returns (ListAlertMetricsResponse) {
    option (google.api.http) = {get: "/v1/monitor/alert-metrics"};
  }
  rpc PutMonitorAlertRules(PutMonitorAlertRulesRequest) returns (PutMonitorAlertRulesResponse) {
    option (google.api.http) = { put: "/v1/monitor/alert-rules"; body: "*" };
  }
  rpc GetRunnerMetrics(GetRunnerMetricsRequest) returns (RunnerMetricsSummary) {
    option (google.api.http) = {get: "/v1/monitor/runner-metrics"};
  }
  rpc GetCodeExecutorCapabilities(GetMonitorLogsRequest) returns (GetCodeExecutorCapabilitiesResponse) {
    option (google.api.http) = {get: "/v1/monitor/code-executor-capabilities"};
  }
  rpc GenerateDiagnosticBundle(GenerateDiagnosticBundleRequest) returns (GenerateDiagnosticBundleResponse) {
    option (google.api.http) = { post: "/v1/monitor/diagnostic-bundle"; body: "*" };
  }
  rpc DiagnoseAndHeal(DiagnoseAndHealRequest) returns (DiagnoseAndHealResponse) {
    option (google.api.http) = { post: "/v1/monitor/diagnose-and-heal"; body: "*" };
  }
  rpc TriggerSelfCheck(TriggerSelfCheckRequest) returns (TriggerSelfCheckResponse) {
    option (google.api.http) = { post: "/v1/monitor/self-check"; body: "*" };
  }
  rpc ListSelfCheckReports(ListSelfCheckReportsRequest) returns (ListSelfCheckReportsResponse) {
    option (google.api.http) = {get: "/v1/monitor/self-check-reports"};
  }
  rpc GetHealStats(HealStatsRequest) returns (HealStatsResponse) {
    option (google.api.http) = {get: "/v1/monitor/heal-stats"};
  }
  rpc ListHealRecords(ListHealRecordsRequest) returns (ListHealRecordsResponse) {
    option (google.api.http) = {get: "/v1/monitor/heal-records"};
  }
}
```

> `ListMonitorAlertRules` / `GetCodeExecutorCapabilities` 复用空 `GetMonitorLogsRequest` 为占位入参（生成代码约定，无业务字段）。

### 2.2 告警与 Runner 指标消息

| 消息 | 说明 |
|------|------|
| `MonitorAlertRule` | `id`、`name`、`metric_key`、`threshold`、`window_minutes`、`enabled`、`severity`、`notify_webhook_url`、`notify_channel_id`、`cooldown_minutes` |
| `AlertMetricInfo` | 指标目录条目：`key`、`name`、`description`、`unit`（`ratio`/`count`）、`default_window_minutes`、`suggested_threshold`、`current_value`、`evaluated_at` |
| `RunnerMetricsSummary` | `window_minutes`、`total_runs`、`error_runs`、`error_rate`、`success_rate`、`avg_duration_ms`、`p50_duration_ms`、`p95_duration_ms`、`p99_duration_ms` |

**评估**：`AlertEvalWorker` 在 `runner.completion` 落库后由 EventBus Handler 触发，并周期评估全部已注册指标；出站：`internal/service/monitor_notify.go`（Webhook POST + Channel `webhook_url`，尊重 `cooldown_minutes`）。

**指标注册表（AlertMetricRegistry，2026-07-29 合并为 Wire 单例）**：所有指标经 `Catalog()` 暴露目录元数据，`ListAlertMetrics` API 聚合成含当前值的目录供 Alerts 页渲染。内置 4 个指标：

| 指标键 | 单位 | 含义 | 建议阈值 / 默认窗口 |
|--------|------|------|--------------------|
| `runner.error_rate` | ratio | 窗口内对话与团队运行失败比例 | 0.25 / 60 min |
| `skill.filesystem_missing_count` | count | 已安装但磁盘文件缺失的 Skill 数 | 1 / 5 min |
| `sequencer.dead_letter_count` | count | 重试后仍无法持久化的活动事件数 | 1 / 5 min |
| `monitor.selfcheck_unhealthy_count` | count | 当前未通过的子系统自检项数 | 1 / 5 min |

### 2.3 Audit 扩展字段

`AuditLog` 消息在原有 7 字段基础上新增：

| 字段 | Proto 编号 | 说明 |
|------|-----------|------|
| `actor` | 8 | 操作者（用户 ID / system） |
| `ip` | 9 | 客户端 IP |
| `user_agent` | 10 | 浏览器/客户端标识 |
| `severity` | 11 | 严重级别（info/warning/critical） |
| `metadata_json` | 12 | 扩展元数据 JSON |

**Audit 数据契约（2026-07-29 规范化）**：

- **action 契约**：统一 `verb.resource` 格式，由 `biz.AuditAction(verb, resource)` 生成；动词枚举 `AuditVerbCreate/Update/Delete/Toggle/Credentials/Archive/Sync`。历史遗留 `resource.verb` / 散装写法由迁移 **20260729**（`audit_action_migrate.go`，幂等）批量规范化
- **detail 契约**：统一 JSON `{"summary": "人类可读摘要", "before": {...}, "after": {...}}`；前端列表只渲染 `summary`，完整 JSON 走详情弹窗。历史纯文本 detail 原样展示（兼容）
- **元数据**：`ip` / `user_agent` 由 `internal/service/audit_meta.go` 从 Kratos transport HTTP Header 提取（`X-Forwarded-For` / `User-Agent`）
- **埋点覆盖**：agent、team（CRUD + Duplicate）、channel（CRUD + Toggle + Credentials upsert/delete）、provider（CRUD + Reveal 凭据）、config（系统设置更新，summary 记录变更 section）、session、tool、mcp_server、skill 的管理操作均落 `AdminAuditEntry`；service 层通过 `recordAudit(ctx, mon, entry)` 统一入口，失败仅记日志不阻断主流程

### 2.4 分页与过滤

**ListAuditLogsRequest**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `limit` | int32 | 每页条数，默认 200 |
| `offset` | int32 | 偏移量 |
| `action` | string | 按事件类型过滤（如 create/delete/update）。**动词级过滤**：不含 `.` 时按 `<verb>.%` 前缀匹配规范化 action；含 `.` 视为完整 action 精确匹配（见 §2.3 数据契约） |
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
| `event_types` | repeated string | 前缀匹配集合（OR，与 `event_type` 取并集），如 `["alert.","runner.completion"]`（2026-07-29 EVT-R） |
| `exclude_event_types` | repeated string | 前缀排除集合，在 include 结果上应用，如 `["skill.filesystem."]`（2026-07-29 EVT-R） |

**ListMonitorTracesRequest**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `limit` | int32 | 默认 100 |
| `offset` | int32 | 偏移量 |
| `agent_id` | string | 按 metadata_json 中 agent_id 模糊匹配 |
| `provider` | string | 按 metadata_json 中 provider 模糊匹配 |
| `model` | string | 按 metadata_json 中 model 模糊匹配 |
| `status` | string | 按状态精确匹配 |

### 2.5 Usage Proto

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
type Usecase struct {
    auditRepo        AuditRepo
    eventRepo        EventRepo
    traceRepo        TraceRepo
    alertRepo        AlertRepo
    runnerCompletion RunnerCompletionRepo
    notifier         AlertNotifier
    fsHealth         FilesystemHealthReader
    lg               loggateway.Logger
    lastFired        sync.Map
    rulesCache       []AlertRule
    rulesExpire      time.Time
    rulesMu          sync.RWMutex
    ringBuffer       *MetricRingBuffer
    evalWorker       *AlertEvalWorker
    registry         *AlertMetricRegistry
}

type UsecaseOption func(*Usecase)
```

构造函数：`NewUsecase(audit, event, trace, alert, runner, notifier, ...UsecaseOption)` — 6 个必选依赖 + 可选注入。

```go
func (u *Usecase) ListAuditLogs(ctx, query AuditQuery) (AuditListResult, error)
func (u *Usecase) ListMonitorEvents(ctx, query MonitorEventsQuery) (MonitorListResult, error)
func (u *Usecase) GetMonitorEvent(ctx, id string) (MonitorPlatformRow, error)
func (u *Usecase) ListMonitorTraces(ctx, query MonitorTracesQuery) (MonitorListResult, error)
func (u *Usecase) GetMonitorTrace(ctx, id string) (MonitorPlatformRow, error)
func (u *Usecase) GetRunnerMetrics(ctx, windowMinutes int32) (RunnerMetricsSummary, error)
func (u *Usecase) EvaluateAlerts(ctx) error
func (u *Usecase) GenerateDiagnosticBundle(ctx, req DiagnosticBundleRequest) (DiagnosticBundleResponse, error)
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

**monitor_traces** 扩展列（MON-OPT-05）：

| 列 | 类型 | 说明 |
|----|------|------|
| session_id | TEXT | 会话 ID |
| run_id | TEXT | 运行 ID |
| invocation_id | TEXT | 调用 ID |
| agent_id | TEXT | Agent ID |
| provider | TEXT | Provider |
| model | TEXT | 模型 |
| team_id | TEXT | Team ID |
| parent_trace_id | TEXT | 跨 turn/跨 team 关联 |
| duration_ms | INTEGER | 耗时 |
| span_count | INTEGER | Span 数 |
| error_count | INTEGER | 错误数 |
| total_tokens | INTEGER | Token 总数 |
| total_cost_usd | REAL | 成本 |

索引：`idx_monitor_traces_session_id`、`idx_monitor_traces_run_id`、`idx_monitor_traces_agent_id`、`idx_monitor_traces_provider`、`idx_monitor_traces_model`

**monitor_trace_spans**（MON-OPT-05 新增）：

| 列 | 类型 | 说明 |
|----|------|------|
| id | INTEGER PK AUTOINCREMENT | 自增主键 |
| trace_id | TEXT NOT NULL | Trace ID |
| span_id | TEXT NOT NULL | Span ID |
| parent_span_id | TEXT | 父 Span ID |
| kind | TEXT NOT NULL | span 类型（llm/tool/retrieve/graph_node/hitl/subteam） |
| name | TEXT NOT NULL | Span 名称 |
| started_at | INTEGER NOT NULL | 开始时间 |
| ended_at | INTEGER | 结束时间 |
| status | TEXT NOT NULL | 状态（ok/error） |
| attributes_json | TEXT | 属性 JSON |
| error_json | TEXT | 错误 JSON |

索引：`idx_monitor_trace_spans_trace_id`、`idx_monitor_trace_spans_kind`
约束：`UNIQUE(trace_id, span_id)`

**monitor_alert_rules**（`internal/data/sql/monitor_alert.sql`）：规则持久化；索引 `idx_monitor_alert_rules_enabled`。

**monitor_alert_rules** 扩展列（MON-OPT-02）：

| 列 | 类型 | 说明 |
|----|------|------|
| last_fired_at | INTEGER | 上次触发时间（unix ms） |
| last_fired_value | REAL | 命中时的指标值 |
| last_fired_window_start | INTEGER | 窗口起始 |
| firing_state | TEXT DEFAULT 'idle' | 状态机（idle/firing/recovered） |
| recovered_at | INTEGER | 恢复时间 |

### 4.2 查询模式

- **Audit**：`WHERE` 动态拼接（action/resource/actor/keyword），`COUNT(*)` 获取总数，`LIMIT/OFFSET` 分页
- **Events/Traces**：基础 `WHERE deleted_at = ''` + 动态条件追加，`COUNT(*)` 获取总数
- **Traces 显示名解析**：`ListMonitorTraces`/`GetMonitorTrace` 以标量子查询解析 `agents.display_name`/`teams.display_name`（id 或 key 匹配，dangling 回退空串）；行 `Name` 输出解析后显示名，原存储域保留在 `config_json.domain` 供前端类型标签使用
- **Usage**：独立 `UsageRepo`，查询 `model_token_usage_events` 聚合；trace 维度聚合（`AggregateUsageByTrace`）走表达式索引 `idx_model_token_usage_events_trace_id`（迁移 20261114，与查询同一 `Dialect.JSONExtract` 表达式保证规划器匹配）
- **Latency 聚合**：`LatencyPercentilesSince` + `meta_duration_ms` generated column + `LIMIT 10000`

---

## 五、Service 层

### 5.1 MonitorService

- `ListAuditLogs`：接收 `ListAuditLogsRequest`，构造 `AuditQuery`，调用 Usecase，返回分页结果
- `ListMonitorEvents` / `ListMonitorTraces`：同上模式
- `GetMonitorTrace`：额外提取 `config_json` 中的 `spans`，组装 `MonitorTraceDetail`
- `GetMonitorLogs`：返回 `enabled`（镜像 `server.monitor.process_log_enabled`）+ hint；实时行走 WS
- `ListFlowLogs`：HTTP 历史 FlowLog 查询（`FlowLogService` + `biz.FlowLogUsecase` + Ent Repo）
- `ListMonitorAlertRules` / `PutMonitorAlertRules`：告警规则 CRUD；PUT 时无规则则写入默认 `runner.error_rate` 种子
- `GetRunnerMetrics`：窗口内 completion 计数聚合 → `RunnerMetricsSummary`（含 P50/P95/P99）
- `GenerateDiagnosticBundle`：诊断包生成 API
- `DiagnoseAndHeal`：诊断+自愈 API
- `TriggerSelfCheck` / `ListSelfCheckReports`：自检 API
- `GetHealStats` / `ListHealRecords`：自愈记录 API

### 5.2 JSON 脱敏

`sanitizeJSONString` 递归遍历 JSON，将包含 `api_key`/`token`/`secret`/`password`/`authorization`/`cookie` 的键值替换为 `******`。

### 5.3 Span 提取

`traceSpansRaw` 从 `config_json` 中提取 `spans` 或 `trace.spans` 数组，供 `MonitorTraceDetail.spans_json` 使用。

---

## 六、Wire 注入

已有，无需新增。`NewMonitorUsecase(repo) → NewMonitorService(uc)`。自检/自愈组件通过 `cmd/admin/wire.go` 集成。

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/
├── pages/MonitorPage.vue              ← 页面壳、6 Tab；query tab / usage_event_id
├── components/monitor/
│   ├── MonitorRunnerMetrics.vue       ← Usage Tab 容器：useRunnerMetrics + RunnerMetricsPanel
│   ├── RunnerMetricsPanel.vue         ← Runner 指标纯展示（props/emits）
│   ├── MonitorAlertRules.vue          ← 告警规则 CRUD
│   ├── AuditTable.vue                 ← 活动日志（服务端筛选 + 分页 + 详情弹窗）
│   ├── RealtimeEvents.vue             ← Events 页：脉搏（WS chip 流）+ 历史（服务端分页表）+ 详情弹窗
│   ├── TraceList.vue                  ← Runs 列表与详情
│   ├── TraceWaterfall.vue             ← 详情瀑布图
│   ├── FlowTracePanel.vue             ← 详情 Flow Tab
│   ├── FlowLogExportButton.vue        ← Flow JSONL 导出
│   ├── LogStreamPanel.vue             ← Logs 二级 Tab + 共享 Hub
│   ├── FlowLogStream.vue              ← 流程日志（flow_log）
│   ├── ProcessLogStream.vue           ← 进程日志（process_log_enabled）
│   ├── SelfCheckStatusPanel.vue       ← 自检状态面板
│   ├── MonitorHeroSection.vue         ← 页面头部
│   ├── MonitorGlassPanel.vue          ← 玻璃态面板
│   └── MonitorErrorBanner.vue         ← 错误提示
├── features/monitor/
│   ├── api.ts                         ← Monitor + alert + runner-metrics API
│   ├── eventView.ts                   ← 事件归一化视图模型（纯函数：人话标题/severity/分类/筛选查询组装；EVT-R）
│   ├── useRunnerMetrics.ts            ← Runner 指标 composable → Store
│   ├── runCorrelation.ts              ← 方案 C 关联与过滤
│   ├── useMonitorRunNavigation.ts     ← Chat / Runs / Monitor Tab 深链
│   ├── useLogStreamHub.ts             ← 共享 Logs WS
│   ├── useMonitorPage.ts              ← 页面状态管理
│   ├── useMonitorRealtimeEvents.ts    ← 实时事件 composable
│   ├── useMonitorTraceFlow.ts         ← Trace Flow composable
│   ├── useMonitorLogStreamPanel.ts    ← Log Stream Panel composable
│   ├── useMonitorAlertRules.ts        ← 告警规则 composable
│   ├── flow.ts                        ← Flow 工具
│   ├── types.ts                       ← RunnerMetricsSummary 等
│   └── utils.ts                       ← 格式化工具
└── stores/monitor/index.ts            ← Pinia Store（含 loadRunnerMetrics）
```

### 7.2 页面 Tab 布局

| Tab | 组件 | 数据来源 |
|-----|------|----------|
| **Usage** | `SelfCheckStatusPanel` + `MonitorRunnerMetrics` | `ListSelfCheckReports` / `TriggerSelfCheck` / `GetRunnerMetrics`；用量大盘见 `/overview` |
| **Alerts** | `MonitorAlertRules` → `MonitorAlertMetricCatalog` + `MonitorAlertRuleCard` | `ListAlertMetrics`（指标目录 + 当前值）/ `ListMonitorAlertRules` / `PutMonitorAlertRules` |
| **Audit** | `AuditTable` | `MonitorService.ListAuditLogs`（2026-07-29 重设计：服务端分页/筛选、summary 摘要列、详情弹窗、全量 i18n） |
| **Events** | `RealtimeEvents` | 脉搏：WS 运行时事件；历史：`ListMonitorEvents`（服务端分页 + `event_types`/`status` 过滤；2026-07-29 EVT-R 重设计） |
| **Runs（Traces）** | `TraceList` | `MonitorService.ListMonitorTraces`（OPT-05；含 duration/tokens/cost；2026-07-29 重设计：6 态状态模型 + 语义色、显示名解析、指标条 + 错误面板、全量 i18n） |
| **Logs** | `LogStreamPanel` → `FlowLogStream` / `ProcessLogStream` | 共享 WS Hub + `flow_log` / `log` 分流 |

### 7.3 Quasar 组件映射

| 区域 | Quasar 组件 |
|------|-------------|
| 二级 Tab | `QTabs` + `QTabPanels`（嵌在 Logs 一级 Tab 内） |
| 状态 | `QBadge` |
| 工具行 | `QInput`、`QBtnToggle`、`QBtn` |
| 日志主体 | `QCard` + 等宽行列表（流程行带 severity class） |
| 详情弹窗 | `QDialog`（最大化） |

### 7.4 LogStreamHub（共享 WS）

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

### 7.5 API

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

### 7.6 API 返回格式

| 类型 | 字段 |
|------|------|
| `PaginatedResult<T>` | `items: T[]`、`total: number` |
| `LoadState` | `idle` / `loading` / `success` / `empty` / `error` |
| `StreamState` | `connecting` / `connected` / `live` / `paused` / `error` |

### 7.7 Usage Tab 与 Dashboard 分工

| 页面 | 组件 | 数据 |
|------|------|------|
| Monitor **Usage** | `SelfCheckStatusPanel`、`MonitorRunnerMetrics` | `GET /v1/monitor/self-check-reports`、`GET /v1/monitor/runner-metrics` |
| **概览** `/overview` | `UsageMetricCards`、`UsageTrendChart`、`UsageBreakdownCharts` 等 | `GET /v1/usage/overview` 等 — 见 [18 monitor-dashboard.design.md](./18%20monitor-dashboard.design.md) |

已删除 `UsageOverview.vue`（避免与概览重复维护）。`MonitorUsageDashboardLink`（「打开概览 / 查看明细」跳转面板）已于 2026-07-29 移除，Usage Tab 只保留自检状态与 Runner 指标。

---

## 八、数据保留与脱敏

- JSON 详情默认折叠大字段，单字段超过 2,000 字符时显示「展开」
- 密钥、Token、Authorization、Cookie、API Key 等字段统一用 `******` 脱敏
- WS 前端缓冲默认最多 1,000 条事件；Logs 默认最多 5,000 行
- **`monitor_events` 保留 30 天**（2026-07-29 EVT-R P3）：`MonitorEventsCleanup` 后台任务（默认 24h 周期，`MONITOR_EVENTS_CLEANUP_DISABLED=1` 可关）硬删除 `created_at` 早于 cutoff 的行。安全性依据：告警窗口/Runner 指标按分钟-小时聚合（`CountMonitorEventsSince`），Runs 真相源为 `monitor_traces`/usage 行（OPT-05），长期审计由 `audit_logs` 承担——30 天外无业务消费方

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
| `TraceList.vue` | **打开会话**；列表标题 Runs；路由 Tab 仍为 `traces`（P2 可改标签） |
| `RunnerMetricsPanel.vue` | 点击下钻 `?tab=traces` |
| `features/monitor/runCorrelation.ts` | `shouldHideCompletionInEvents`、`completionCanOpenInRuns` 等 |
| `pages/MonitorPage.vue` | `tab` / `usage_event_id` query |

**不新建** `MonitorEventDetailDialog` 用于 Chat completion。

### 9.6 与其它模块

| 模块 | 关系 |
|------|------|
| [52-flow-logger](./52-flow-logger.design.md) | `trace_id` 对齐；排障在 Runs 详情 Flow Tab |
| Usage | Runs 列表为 `ListMonitorTraces`（OPT-05）；用量明细仍走 `ListUsageEvents`；与 Events 分流 |
| Alerts / Memory | 仍消费 `runner.completion` 落库 |

### 9.7 非目标（Phase 1d）

- 不在 Events 为 Chat 建结构化 completion 详情（与 Runs 重复）。
- 不新增 `monitor_events` 表列（P2 再评估）。
- 不改变 WS `runner_completion` Envelope 类型。

### 9.8 后续（P2，可选）

- UI 标签 `Traces` → `Runs`，query `?tab=runs` 别名。
- `ListMonitorEvents` 服务端过滤 `hide_linked_completions`（减轻前端过滤）。


---

## 十、Events 页重设计（2026-07-29 · EVT-R）

> 对应需求：[18 monitor.md §3.7](./18%20monitor.md) · 开发计划：[18-monitor.development.md](./18-monitor.development.md) EVT-R 任务表

**设计决策**：Events = 「值得注意的事」。原实现把 WS 实时流与 `monitor_events` 落库行混在一个卡片流里——实时洪峰冲掉历史、原始 event_type 直出、无级别语义、skill 磁盘高频事件刷屏、历史无服务端过滤。重设计拆为 **脉搏（Pulse）+ 历史（History）** 双区，统一经 **归一化视图模型** 渲染。

### 10.1 目标架构

```text
WS 运行时事件（team_run_*/intent_pass/...）
  └─ wsEventToView() ──► Pulse 区（chip 流，容量上限 FIFO，不落库）

monitor_events 落库行（runner.completion/alert.*/skill.filesystem.*/usage.budget_alert/chat.user_feedback）
  └─ ListMonitorEvents（服务端分页 + event_types/status 过滤）
       └─ persistedEventToView() ──► History 区（表格 + 翻页保持）

skill.filesystem.updated（info 级高频）
  └─ watch.Reporter: SkipPersist=true ──► 仅 MonitorBus 实时事件（不进 monitor_events）
```

两类数据源共用 `MonitorViewEvent` 视图模型（`features/monitor/eventView.ts`，纯函数模块，i18n 通过注入 `t()` 完成）：

| 字段 | 来源 | 业务意义 |
|------|------|---------|
| `title` | 按 event_type 映射 i18n 人话标题（如 `team_run_failed`→「团队运行失败」）；未知类型回退原始 type | 一眼识别发生了什么 |
| `subtitle` | 错误信息 / 耗时 / Token / 会话短号（取首个可用，无占位废文案） | 一行判断影响面 |
| `severity` | 持久化：`status`→critical/warn/success/info；WS：failed→warn、finished→success、其余 info | 色点分级，异常优先 |
| `category` | event_type 前缀 → task/message/agent/tool/system（§3.2 映射） | 分类筛选 |
| `actor` | Agent/规则/Skill 名（`step.agent_name` 或行 `name`） | 定位责任主体 |
| `completionMeta`/`canOpenInRuns` | 方案 C `runCorrelation` | 降级卡片跳 Runs |

### 10.2 服务端过滤（Proto/Data）

- `ListMonitorEventsRequest` 新增 `event_types`（前缀 OR，与 `event_type` 并集）与 `exclude_event_types`（前缀排除）——见 §2.4。
- 前端筛选组装 `buildMonitorEventsQuery()`：类型筛选直接传前缀；**级别筛选映射 `severity→status`**（critical→error / warn→warn / success→ok / info→info，对齐写库方取值）。
- 类型筛选选项 `EVENT_TYPE_FILTERS` 对齐真实落库 keyspace（`RecordMonitorEvent` 调用点），杜绝永远无结果的选项。

### 10.3 翻页保持

新 WS 事件到达只刷新 Pulse 区，不重置 History 分页；仅当用户修改类型/级别筛选条件时重置到第 1 页（`useMonitorRealtimeEvents`）。

### 10.4 Skill 事件洪泛控制（SkipPersist）

`internal/skill/watch/reporter.go`：`Report.SkipPersist=true` 时仅发布 MonitorBus 实时事件，不写 `monitor_events`/`admin_audit`。当前应用于 `skill.filesystem.updated` 且 severity=info（编辑器/外部工具高频改动场景）；`imported`/`missing`/`recovered`/`rejected` 仍正常落库（见 [20-skill.design.md §监控事件](./20-skill.design.md)）。

### 10.5 详情弹窗

结构化元数据表（类型/级别/分类/主体/时间/会话）+ 原始 JSON（`raw` 含解析后的 config/metadata，可复制）。不在 Events 为 Chat 建平行排障详情（§9.7 非目标不变）。

### 10.6 保留策略

见 §八：`MonitorEventsCleanup`（`internal/cronrunner/jobs/monitor_events_cleanup.go`）每日硬删 30 天前 `monitor_events` 行；`EventRepo.DeleteMonitorEventsOlderThan` 经 `RWDB().WriteDB` 事务感知写路径 + `entErrToBizErr` 翻译。

### 10.7 非目标

- 不引入新的落库事件类型；不改变现有 `monitor_events` 表结构。
- 不做历史区实时追加（历史定位是「查」，实时定位是「看」）。
- Pulse 区不做持久化/回放（刷新即清空，查历史走 History 区）。


---

## 子模块：Monitor Dashboard 设计

> 对应需求：[18 monitor-dashboard.md](./18%20monitor-dashboard.md)  
> 用量契约：[29 token.design.md](./29%20token.design.md) · 运维页：[18 monitor.design.md](./18%20monitor.design.md)  

---

## 一、架构定位

```text
OverviewPage (/overview)
  ├─ useOverviewPage → useUsageStore → features/usage/api → UsageService
  └─ OverviewRunnerMetrics → useRunnerMetrics → useMonitorStore → GetRunnerMetrics

MonitorPage Usage Tab
  ├─ SelfCheckStatusPanel → useMonitorStore（self-check-reports + 手动触发）
  └─ MonitorRunnerMetrics → useRunnerMetrics（同上 Store）
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
        └── SelfCheckStatusPanel.vue  ← 自检状态 + 手动触发
```

已删除：`UsageTrendPanel.vue`、`UsageOverview.vue`、`MonitorUsageDashboardLink.vue`（2026-07-29）。

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

### 3.5 图表实现

| 模块 | 实现 |
|------|------|
| 趋势 | `UsageTrendChart`：metric 切换 tokens / calls / cost / success_rate（堆叠 %） |
| 占比 | `UsageBreakdownCharts`：模型 Top5 费用环图；Provider 由 Top 模型样本聚合（UI 已标注） |
| 分包 | `defineAsyncComponent` + `useUsageChart` 独立 chunk（`usageEcharts`） |

主题色：`usageChartPalette()` 读取 `--color-accent`、`--color-success`、`--color-danger`。

---

## 四、Monitor Usage Tab

```text
Monitor → Usage Tab
  ├── SelfCheckStatusPanel（自检报告 + 手动触发）
  └── MonitorRunnerMetrics（Store + RunnerMetricsPanel）
```

不再维护 `UsageOverview.vue`。

---

## 五、测试

| 层 | 内容 |
|----|------|
| Web | `usageTrendMetrics.spec.ts`、`usageBreakdownSlices.spec.ts` |
| Web（P2） | Overview 筛选 → store mock；E2E `/overview` → `/usage/events` |
| Go | `usage_*_test.go` 聚合口径 |

> 任务与验收见 [18-monitor.development.md §子模块 Monitor Dashboard](./18-monitor.development.md)。


---

## 子模块：Monitor Loop 01 设计

> **版本**：2026-05-29-v2
> **需求**：[`18 monitor.md`](./18%20monitor.md) §子模块 Monitor Loop 01 需求
> **规则真相源**：[`project_rules.md`](../../.trae/rules/project_rules.md) · [`aranea-coding-guide`](../../.trae/skills/aranea-coding-guide.md)

> 实现进度见 [18-monitor.development.md §子模块 LOOP-01](./18-monitor.development.md)。

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

## 4. 验证方案

| 阶段 | 验证命令 |
|------|----------|
| Phase 1 | `go build ./internal/biz/...` + `go vet` |
| Phase 2 | `go build ./internal/cronrunner/...` + `go vet` |
| Phase 3 | `go build ./internal/event/...` |
| 全量 | `grep -rn 'log\.Printf\|log\.Infof\|log\.Warnf\|log\.Errorf' internal/biz/` → 0 结果 |

> 实施分期与状态见 [18-monitor.development.md §子模块 LOOP-01](./18-monitor.development.md)。

---

## 5. AI 辅助分析（已实现）

原远期目标已通过以下组件实现：

1. **AI 诊断包**（DIAG-01）：`DiagBundleGenerator` + `GenerateDiagnosticBundle` API — 自动聚合错误上下文
2. **根因分析引擎**（DIAG-02）：`RootCauseEngine` 5 条内置规则 + 置信度评分 — 自动推导错误根因
3. **自检体系**：`SelfCheckScheduler`（5 min 周期）+ `SelfCheckRepairDispatcher`（4 个修复器）— 自动检测+修复
4. **自愈体系**：`SelfHealObserver`（事件驱动修复）+ `PredictiveHeal`（预测性自愈）— 闭环工作流
5. **模式挖掘**：`PatternMiningUsecase`（故障聚类 + 自动修复模板生成）— 从历史中学习

详见 §子模块「自检与自愈设计」。


---

## 子模块：自检与自愈设计

> **版本**：2026-06-06
> **需求**：[18 monitor.md](./18%20monitor.md) §0.2 自检与自愈
> **代码锚点**：`internal/biz/monitor/self_check*.go`、`self_heal*.go`、`predictive_heal.go`、`pattern_mining.go`

> 实现进度见 [18-monitor.development.md §子模块 自检与自愈](./18-monitor.development.md)。

---

### 1. 架构概览

```text
SelfCheckScheduler（5 min ticker）
    ├── SelfChecker 插件（每个子系统一个，ProvideSelfCheckers 装配，nil 依赖自动跳过）
    │   ├── DBHealthChecker（db_health：Postgres 读写池 ping）
    │   ├── TraceProjectorChecker（trace_projector：投影滞后/积压）
    │   ├── AlertEvalChecker（alert_eval：告警评估 worker 存活）
    │   ├── EventBusChecker（eventbus：MonitorBus 订阅者数，GenericBus.SubscriberCount）
    │   ├── WebSocketChecker（websocket：WS 连接计数）
    │   ├── FlowFileChecker（flow_file：流文件磁盘占用）
    │   └── RunnerCompletionFlowChecker（runner_completion_flow：flow 活跃但无 completion 记录 → 指标静默故障）
    ├── SelfCheckRepairDispatcher
    │   ├── FlowFileRepairer（清理过期流文件）
    │   ├── TraceProjectorRepairer（触发 trace 回填）
    │   ├── AlertEvalRepairer（重启告警评估 worker）
    │   └── EventBusRepairer（重新订阅事件总线）
    └── SelfCheckReport（聚合报告；unhealthy 计数缓存供 monitor.selfcheck_unhealthy_count 指标）

SelfHealObserver（事件驱动）
    ├── 订阅 FlowLog severity=error/critical
    ├── DiagBundleGenerator → 诊断包
    ├── RootCauseEngine → 根因分析
    └── 执行修复 + 记录 HealRecord

PredictiveHealUsecase（预测性自愈）
    ├── 读取系统指标（provider 延迟、内存使用率、会话积压）
    ├── 匹配活跃故障模式（FailurePattern）
    └── 置信度 > 0.8 时执行预防性修复

PatternMiningUsecase（模式挖掘）
    ├── 从历史 HealRecord 聚类相似故障
    ├── 聚类 >= 3 次成功修复 → 自动生成修复模板
    └── 写入 failure_pattern 表（source="mined"）
```

### 2. 核心接口

```go
// SelfChecker — 每个子系统一个检查插件
type SelfChecker interface {
    Name() string
    Check(ctx context.Context) SelfCheckResult
}

type SelfCheckResult struct {
    Name      string
    Healthy   bool
    Message   string
    RepairKey string  // 路由到对应 Repairer
}

// SelfCheckRepairer — 检查-修复解耦（SRP）
type SelfCheckRepairer interface {
    RepairKey() string
    Repair(ctx context.Context, result SelfCheckResult) RepairOutcome
}

type RepairOutcome struct {
    Success  bool
    Message  string
    Duration time.Duration
}
```

### 3. 自愈闭环

```text
FlowLog error/critical
    → SelfHealObserver.OnFlowLogError()
    → DiagBundleGenerator.Generate()
    → RootCauseEngine.Evaluate()
    → 执行修复（基于 FailurePattern 或内置规则）
    → 记录 HealRecord
    → PatternMining 异步聚类
```

**置信度阈值**：根因置信度 < 0.5 时不自动修复，仅记录诊断包。

**Cooldown**：同一 (step_id, session_id) 5 分钟内不重复修复。

### 4. 故障模式

```go
type FailurePattern struct {
    ID           string
    PatternHash  string    // error_code + 归一化 stack_trace
    Source       string    // runtime | ci | mined
    ErrorCode    string
    Description  string
    FixTemplate  string    // 修复步骤模板
    Confidence   float64   // 0.0~1.0
    SuccessCount int
    FailCount    int
    Active       bool
}
```

**来源**：
- `runtime`：运维手动录入
- `ci`：从 CI 日志解析（`FailureReportParser`）
- `mined`：`PatternMiningUsecase` 自动挖掘

**置信度晋升**：mined 模式初始 0.5，连续 3 次成功修复后晋升到 0.8。

**自动停用**：失败率 > 50% 且尝试 >= 5 次时自动停用。

### 5. 预测性自愈

`PredictiveHealUsecase` 读取系统指标，匹配活跃故障模式：

| 指标 | 阈值 | 匹配模式 |
|------|------|----------|
| Provider 平均延迟 | > 5s | Provider 降级模式 |
| 内存使用率 | > 90% | 内存压力模式 |
| 会话积压数 | > 100 | 队列积压模式 |

当匹配到模式且置信度 > 0.8 时，执行预防性修复（如提前切换 Provider、触发 GC、扩容 Worker）。

### 6. 故障报告

`FailureReport` 统一 CI 和 runtime 错误格式：

| 类型 | 说明 |
|------|------|
| `lint_error` | 代码风格违规 |
| `test_failure` | 测试失败 |
| `build_failure` | 编译失败 |
| `proto_sync` | Proto 生成物不同步 |
| `runtime_error` | 运行时 panic/nil pointer/connection refused |

`FailureReportParser` 支持 Go build error、test failure、lint error、proto sync、runtime panic 的正则识别。

### 7. 与已有模块的关系

| 模块 | 关系 |
|------|------|
| DIAG-01/02 | 自愈的前置步骤：诊断包 + 根因分析 |
| MON-OPT-01~06 | 自检的检查对象：Bus 分离、告警评估、Trace 写入等 |
| LOOP-01 | 自愈是 LOOP-01「闭环」的具体实现 |
| `failure_pattern` 表 | 模式挖掘的持久化存储 |


---

## 子模块：Monitor 优化设计（MON-OPT-01~06）

> **关联**：[`18 monitor.md`](./18%20monitor.md) · [`18 monitor.design.md`](./18%20monitor.design.md) · 代码 Review [2026-05-26-Monitor-Code-Review](../review/2026-05-26-Monitor-Code-Review.md)
> **规则真相源**：[`monitor-streams-wire.mdc`](../../.cursor/rules/monitor-streams-wire.mdc) · [`AGENT_RUNTIME_BOUNDARY.md`](../AGENT_RUNTIME_BOUNDARY.md)
> **范围**：本文聚焦 **业务逻辑层面** 的优化设计（用户/运维实际感受得到的能力差异）。代码风格、命名、格式等问题已在 review 文档 P3 收敛，本文不重复。

> 实现进度见 [18-monitor.development.md §子模块 MON-OPT](./18-monitor.development.md)。

---

## 0. 背景

Monitor 已实现六大 Tab（Audit / Alerts / Events / Runs / Usage / Logs）的基础能力，但 review 暴露 **6 项业务正确性 / 运维体验** 缺陷。本方案给出可执行设计：

| 编号 | 主题 | 业务问题（运维 / 用户视角） | 优先级 |
|------|------|---------------------------|--------|
| MON-OPT-01 | **FlowLog 流彻底分离到 MonitorBus** | 高 QPS 时 chat 业务事件挤掉 flow_log → Monitor 页缺关键步骤 | **P1** |
| MON-OPT-02 | **告警冷却持久化 + 多实例分布式去重** | 进程重启 / 多副本 → 同一窗口 Webhook 重复多次轰炸 → IM 限流封禁 | **P1** |
| MON-OPT-03 | **告警评估批量化 + 滑动窗口 + 单飞** | 每次 completion 全规则扫 + 2× COUNT → 高 QPS 时监控反而拖垮 DB | **P1** |
| MON-OPT-04 | **WS 反压可观测 + 客户端可见反馈** | 满 buffer 静默丢事件 → 前端"看不见"问题 → 误判系统正常 | **P1** |
| MON-OPT-05 | **Trace 写入回路 + Run 全链路视图** | `monitor_traces` 表只读不写 → Traces Tab 长期空白 | **P1** |
| MON-OPT-06 | **告警规则注册表 + 自定义指标 DSL** | 加新指标必改 Usecase + repo + Wire；不可热扩展 | **P2** |

---

## 1. MON-OPT-01：FlowLog 流彻底分离到 MonitorBus

> **迁移完成说明（2026-06-27，ADR-03 Phase 5）**：本节描述的 DualBus/Envelope 路由方案是当时的设计方案。最终实现采用更彻底的方案——删除 legacy Envelope Bus，FlowLog 通过 `contract.MonitorEvent` 在 `MonitorEventBus` 上传输（不再走 Envelope）。`TraceEmitter`/`FlowTracker` 已迁移到 `MonitorEventBus`，`SelfHealObserver`/`TraceProjector` 也已从旧 envelope bus 迁移到 `MonitorEventBus`（修复了死订阅 bug）。下方设计方案保留作为历史记录，当前实际架构见 ADR-03（统一总线架构，已归档）和 [34-event-system.development.md](./34-event-system.development.md)。

### 1.1 现状与业务问题

| 来源 | 目标 Bus | 是否合规 |
|------|----------|---------|
| `event/system_flow.go::emitSystem` | **MonitorBus** | ✅ |
| `event/trace_emitter.go::TraceEmitter`（chat / team） | `Pipeline.Bus`（**SessionBus**） | ⚠️ 与 `monitor-streams-wire.mdc` 「flow_log 走 MonitorBus」P0 意图冲突 |

**业务后果**：
- 全局 Monitor 连接（`session_id=*`）必须订阅 **双 Bus**（参见 `ws.go::eventPump`）才能收齐 flow_log；任一 Bus 丢事件，运维就缺一段。
- chat 高 QPS（每秒多 turn）下，SessionBus buffer 128 优先被 chat envelope 占满 → flow_log 被 `DropNewest`/`DropOldest` → 运维"看见 turn 完成但看不见中间步骤"。
- Pipeline 不同业务（chat / team / channel ingress）各持一个 Bus 引用，配置散落，未来加新业务流极易踩坑。

### 1.2 设计方案

#### 1.2.1 Envelope 双发模式

`event.Bus` 接口扩展（不破坏兼容）：

```go
type DualBus struct {
    Session Bus  // 业务运行时（必收：team_run_*, intent_pass, chat envelope）
    Monitor Bus  // 监控运维（必收：flow_log, log, alert.fired）
}
```

新增 `Publish` 路由策略表（编译期决定）：

| EnvelopeType | 路由 |
|--------------|------|
| `flow_log` | **MonitorBus only** |
| `log` | **MonitorBus only** |
| `alert.fired` / `alert.notify` | **MonitorBus** + SessionBus（前端 Chat 也可弹) |
| `team_run_*` / `team_step_*` | **SessionBus only** |
| `intent_pass` / `runner.completion` Envelope（不是 monitor_events 行） | **SessionBus only** |
| `usage.*` | **SessionBus** + MonitorBus（Usage 大盘需要） |

实现：`event.Infra.Publish(env)` 内部按 `env.Type` 查表选 Bus；调用方不再自选。

#### 1.2.2 Pipeline 重构

`internal/chat/Pipeline` / `internal/team` / `internal/channel` 中所有持 `Bus` 字段的结构体，统一替换为 `*event.DualBus`（或保留 `Bus` 字段但内部用 `Infra` 单例）。

`TraceEmitter` 改为：

```go
func (e *TraceEmitter) emit(...) {
    env := buildFlowLogEnvelope(...)
    e.infra.Publish(ctx, env)   // 路由表自动送到 MonitorBus
}
```

#### 1.2.3 WS 订阅简化

`internal/server/ws.go::handleSession`：
- 全局连接（`session_id=*`）：**仅订 MonitorBus**（不再启第二个 pump）。
- 单 session 连接：**仅订 SessionBus**。
- 删除 `globalMode && monitorBus != sessionBus` 双 pump 分支 → 代码 -80 行，减少竞争。

#### 1.2.4 迁移与回滚

| 阶段 | 行为 | 开关 |
|------|------|------|
| Phase 0 | 路由表上线，但 `flow_log` **同时**发 Session + Monitor | `MONITOR_BUS_ROUTING=dual` |
| Phase 1 | 灰度切换：MonitorBus 唯一接收 flow_log | `MONITOR_BUS_ROUTING=split`（默认） |
| Phase 2 | 删除 SessionBus 上的 flow_log 路径与双 pump 代码 | 永久 |

回滚：env flag 单步回退；不需要 DB 迁移。

### 1.3 验收标准

| 指标 | 目标 |
|------|------|
| chat 高峰（>50 turn/s）下 flow_log 丢失率 | < 0.1% |
| `system.ws.send_drop` 上 flow_log 类型占比 | 减少 ≥ 80% |
| WS 全局连接 goroutine 数 | -50%（单 pump） |
| 集成测 `TestDualBusRouting_NoFlowLogOnSessionBus` | ✅ |

---

## 2. MON-OPT-02：告警冷却持久化 + 多实例分布式去重

### 2.1 现状与业务问题

| 现状 | 问题 |
|------|------|
| `Usecase.lastFired sync.Map` 仅内存 | 进程重启 → 同一阈值再发 Webhook |
| 无分布式锁 / DB 记录 | 多副本部署 → N 个进程同时触发 → N 次 Webhook |
| `Cooldown` 比较以本进程 `now` 为准 | 跨实例时钟漂移可能跳冷却 |

**业务后果**：
- 凌晨例行重启 → 早 8 点的告警在重启后**立即重发**给值班群。
- HPA 副本扩到 3 个 → 1 次错误率超阈引发 3 个 Webhook + 3 条 IM 推送 → 值班疲劳。
- Webhook 接收方限流（如飞书机器人每分钟 100 次）→ 重要告警被丢弃。

### 2.2 设计方案

#### 2.2.1 DB 持久化 `last_fired_at`

`monitor_alert_rules` 加列：

```sql
ALTER TABLE monitor_alert_rules ADD COLUMN last_fired_at INTEGER;       -- unix ms
ALTER TABLE monitor_alert_rules ADD COLUMN last_fired_value REAL;        -- 命中时的指标值
ALTER TABLE monitor_alert_rules ADD COLUMN last_fired_window_start INTEGER; -- 窗口起始
ALTER TABLE monitor_alert_rules ADD COLUMN firing_state TEXT
  NOT NULL DEFAULT 'idle'
  CHECK(firing_state IN ('idle','firing','recovered'));
ALTER TABLE monitor_alert_rules ADD COLUMN recovered_at INTEGER;
```

#### 2.2.2 firing 状态机

```text
idle --(metric ≥ threshold AND cooldown 过)--> firing
firing --(metric < threshold × recovery_factor)--> recovered
recovered --(冷却结束)--> idle
```

| 状态 | 行为 |
|------|------|
| idle → firing | 发 `alert.fired` + Webhook；写 `last_fired_at` |
| firing 期间持续命中 | 仅每 N 分钟（`reminder_minutes`，默认 30）重发提醒；不重置冷却 |
| firing → recovered | 发 `alert.recovered` + Webhook（恢复通知）；进入 cooldown |
| recovered → idle | cooldown 过后允许下次 firing |

`recovery_factor` 默认 0.9（阈值 0.25 → 跌到 0.225 以下才算恢复，防抖动）。

#### 2.2.3 多实例去重锁

SQLite（单写）：写入前 `BEGIN IMMEDIATE`，读取最新 `last_fired_at` 后判断。

Postgres / 分布式部署：
```sql
SELECT id, last_fired_at, firing_state
FROM monitor_alert_rules
WHERE id = $1
FOR UPDATE;
```

并发安全：所有 `ShouldFireAlert / MarkAlertFired` 操作在同一事务内完成。

#### 2.2.4 业务化告警分级

`AlertRule` 增加 `severity_escalation`：

| 持续时间 | 行为 |
|----------|------|
| 0 ~ 10 min | severity=warn，仅 Webhook |
| 10 ~ 30 min | severity=critical，Webhook + IM @值班 |
| > 30 min | severity=critical + 自动创建 incident（如已接 incident 系统） |

#### 2.2.5 静默窗口

`AlertRule` 增加 `silence_windows`（数组）：

```json
[{"cron": "0 2-4 * * *", "duration_minutes": 180, "reason": "maintenance"}]
```

匹配窗口内的告警不发 Webhook（仍写 `alert.fired` 事件供回看）。

### 2.3 验收标准

| 指标 | 目标 |
|------|------|
| 进程重启后 1 分钟内重复 Webhook | 0 次 |
| 3 副本同时部署，单次告警 Webhook 数 | 1 次 |
| `alert.recovered` 事件覆盖率 | ≥ 95% |
| 集成测 `TestAlertCooldownPersistedAcrossRestart` | ✅ |
| 集成测 `TestAlertConcurrentEvaluation_SingleNotification` | ✅ |

---

## 3. MON-OPT-03：告警评估批量化 + 滑动窗口 + 单飞

### 3.1 现状与业务问题

```go
// 每次 runner.completion handler 结束 →
safego.Go("monitor.evaluate-alerts", func() {
    monitor.EvaluateAlerts(ctx)  // 全规则 + 2× COUNT/规则
})
```

| 问题 | 影响 |
|------|------|
| 每次 completion 触发 | 1000 QPS completion × 5 规则 = **每秒 5000 次 COUNT** |
| 同步阻塞 SQL | DB 连接池被告警吃满 → 业务读写慢 |
| 无 singleflight | 同一规则被 N 个 goroutine 并行评估 |
| Window 内全表 `json_extract` | SQLite 文件锁竞争 |

**业务后果**：监控系统在系统真正出问题（高 QPS / 错误率上升）时反而**自我拖垮**。

### 3.2 设计方案

#### 3.2.1 独立 `MonitorAlertEvalWorker`

```go
type MonitorAlertEvalWorker struct {
    usecase  *monitor.Usecase
    interval time.Duration  // 默认 30s
}
```

- 启动单 goroutine ticker，每 30 s 统一评估所有 enabled 规则。
- 移除 `event_bus_runner_handler` 中的 `safego.Go("monitor.evaluate-alerts")`。
- 评估失败有 backoff（指数退避，最多 5 min）。

#### 3.2.2 内存滑动窗口

`MonitorAlertEvalWorker` 持有 ring buffer：

```go
type MetricRingBuffer struct {
    buckets    []MetricBucket   // 每 1 min 一个桶
    bucketSize time.Duration    // 1 min
    capacity   int              // 60（即 1 小时窗口）
}

type MetricBucket struct {
    startUnix int64
    totals    map[string]int64  // event_key → count
    errors    map[string]int64
    durations map[string]struct{ sum, count int64 }
}
```

事件订阅：`event.Bus.Subscribe("monitor.*")` → 实时增量更新 buckets（O(1)）。

评估时（每 30 s）：

```text
For each enabled rule:
    window = rule.WindowMinutes
    [error, total] = buffer.SumLastN(window)
    rate = error / total
    if rate >= threshold: try-fire（按 OPT-02 状态机）
```

DB COUNT 退化为定期对账（每小时 1 次），用于校正内存与 DB 偏差。

#### 3.2.3 Singleflight

即使评估器内部，对同 rule 的 fire 操作走 `singleflight.Group`，防止极端情况下并发问题：

```go
sf.Do(rule.ID, func() (interface{}, error) {
    return nil, u.tryFire(ctx, rule)
})
```

#### 3.2.4 历史数据加载

进程启动时：
- 从 `monitor_events` 最近 1 小时 load 进 buckets（rebuild）。
- 完成前不评估（避免误判）。

#### 3.2.5 退化模式

事件订阅断流（Bus 异常）→ Worker 自动切回 DB COUNT 模式 + 发 `monitor.eval_degraded` 事件。

### 3.3 验收标准

| 指标 | 目标 |
|------|------|
| 评估对 DB QPS | -99%（从 N×K/s 降到 ≤ 1/h 对账） |
| 1000 QPS completion 下评估 CPU 占用 | < 5% 单核 |
| 评估延迟（事件 → 触发 alert） | ≤ 60 s（30 s 评估周期 + 30 s 入桶延迟） |
| 集成测 `TestAlertEval_RingBuffer_ConsistentWithDB` | ✅ |

---

## 4. MON-OPT-04：WS 反压可观测 + 客户端可见反馈

### 4.1 现状与业务问题

```go
select {
case wc.send <- data:
default:
    event.SessionSysLogWarn(..., "system.ws.send_drop", ...)
}
```

| 问题 | 影响 |
|------|------|
| 客户端无感知 | Monitor 页一切如常，运维以为系统正常实际丢了关键事件 |
| 无优先级 | `alert.fired` 与 `flow_log` 平等竞争 buffer → 关键告警可能被一般 flow log 挤掉 |
| 无丢弃统计入 metric | drop 累计不可监控 |

**业务后果**：
- 重大故障时大量 flow_log 涌入 → wc.send 满 → `alert.fired` 被丢弃 → 运维**根本看不到**告警 → 错过响应窗口。

### 4.2 设计方案

#### 4.2.1 按 EnvelopeType 优先级队列

替换 `wc.send` 单 channel 为三优先级 channel：

```go
type connQueues struct {
    high   chan []byte  // alert.fired, alert.notify, system.fatal — cap 64
    normal chan []byte  // team_run_*, runner.completion, intent_pass — cap 128
    low    chan []byte  // flow_log, log, usage.* — cap 256
}
```

`writePump` 按 `high → normal → low` 顺序取（每轮最多 N 个 low 避免饿死）。

满策略：

| 优先级 | 满时行为 |
|--------|----------|
| high | **永不丢**：阻塞至超时（5 s），仍满则关闭连接（让 client 重连） |
| normal | 丢弃尾部（DropNewest）+ 计数 |
| low | 丢弃尾部（DropNewest）+ 计数 |

#### 4.2.2 反压事件回流客户端

当一段时间（如 10 s）内任一优先级 drop > N 次：

发送 `monitor.backpressure` envelope 给该连接：

```json
{
  "type": "monitor.backpressure",
  "metadata": {
    "dropped_high": 0,
    "dropped_normal": 23,
    "dropped_low": 412,
    "window_seconds": 10,
    "advice": "reduce subscribed channels or pause non-critical streams"
  }
}
```

Monitor 页面拿到后顶部展示 banner：「监控流过载，最近 10 s 丢弃 N 条非关键事件，可能影响实时性」。

#### 4.2.3 Lossless 订阅模式

WS 升级握手时可上行：

```json
{"action":"set_mode","mode":"lossless","scope":["high","normal"]}
```

服务器记 `wc.lossless=true`：
- 满时不丢弃，等待 5 s 写超时；超时关闭连接。
- 客户端通过断重连 + last_event_id 补拉（需要 OPT-05 支持回放）。

#### 4.2.4 Metric 化

新增 metrics（写入 `monitor_events` 或 Prometheus exporter，按现有体系）：

| metric | 含义 |
|--------|------|
| `monitor.ws.drop_high` | high 优先级丢弃数 |
| `monitor.ws.drop_normal` | normal 丢弃数 |
| `monitor.ws.drop_low` | low 丢弃数 |
| `monitor.ws.lossless_disconnect` | 主动断连数 |
| `monitor.ws.send_blocked_ms` | 写阻塞时长直方图 |

### 4.3 验收标准

| 指标 | 目标 |
|------|------|
| 故障场景下 `alert.fired` 推送成功率 | ≥ 99.9% |
| 高峰丢弃集中在 low 优先级 | ≥ 95% |
| 客户端能感知反压并展示 banner | ✅ |
| 集成测 `TestWSPriorityQueue_HighNeverDropped` | ✅ |

---

## 5. MON-OPT-05：Trace 写入回路 + Run 全链路视图

### 5.1 现状与业务问题

| 现状 | 问题 |
|------|------|
| `monitor_traces` 表存在但**无 INSERT 代码路径** | Traces Tab 永远空 |
| Run 详情依赖 `model_token_usage_events` + `flow_log_events` 各自查询 | 数据散落，需要前端 N+1 拼接 |
| 跨 Agent / 跨 Team 调用链无统一 span 关联 | "为什么这次回答慢"无法定位到某 tool / 某 LLM 调用 |

**业务后果**：
- 用户 / 运维点 Traces Tab → 看到空表 → 失去信任。
- 错误分析时只能在 flow_log + usage 两边切换比对。

### 5.2 设计方案

#### 5.2.1 统一 Trace 模型

`monitor_traces` 扩展（见 §四.1 表结构）。

新增 `monitor_trace_spans`（见 §四.1 表结构）。

#### 5.2.2 `MonitorTraceProjector`

新 goroutine consumer，订阅 EventBus：

```text
event.Subscribe(filter: trace_id != "")
    → trace_id 首次出现 → INSERT monitor_traces(status='running')
    → 收到 LLM call event → INSERT span(kind=llm, ...)
    → 收到 tool call event → INSERT span(kind=tool, ...)
    → 收到 runner.completion → UPDATE traces SET status='ok'|'error', duration_ms, totals
```

事件源：
- `model_token_usage_events`（已有）→ kind=llm span
- `flow_log` step（已有 step_id 注册表）→ kind=graph_node / tool / hitl span
- `runner.completion`（已有）→ trace 关闭

#### 5.2.3 跨 turn / 跨 team 关联

| 场景 | parent_trace_id 来源 |
|------|---------------------|
| chat 续接对话 | 取上一 turn 的 trace_id |
| Team Graph 调度 subteam | subteam 第一个 trace.parent = team_run.trace_id |
| Resume 自 HITL | resume 后第一个 trace.parent = pre-HITL trace_id |

UI：Traces 详情可点 parent → 跳转上一段 trace；Waterfall 跨 trace 视图（可选）。

#### 5.2.4 Token 与成本聚合

`monitor_traces.total_tokens` / `total_cost_usd` 在 trace 关闭时计算（sum spans）。Usage 大盘可直接按 trace 聚合，不再需要 `model_token_usage_events` 单独 query（性能优化）。

**实现（2026-07-29）**：关闭时以 `model_token_usage_events` 为权威成本源聚合——`TraceProjector.OnRunnerCompletion` 与 `MonitorTraceBackfillWorker` 均调 `TraceUsageRepo.AggregateUsageByTrace(traceID)`，结果经 `TraceCompletion` 结构写入：

- **聚合**：`SUM(total_tokens)`、`SUM(total_cost_micro_usd)/1e6`、`COUNT(*)`；provider/model 取 `occurred_at DESC` 最新一条非空值（标量子查询，非 `MAX()`）
- **tokens 取大**：usage 聚合 tokens 与 flow_log 累计 tokens 取较大者（两者口径不同，flow_log 可能缺 turn）
- **provider/model 空值回填**：`UpdateMonitorTraceCompletion` 用 `CASE WHEN provider = '' AND $x != ''` 仅在存储列为空时回填（flow_log 元数据常缺，usage 事件权威）

#### 5.2.5 历史数据回填

新增 cron `MonitorTraceBackfillWorker`：
- 从 `model_token_usage_events` 倒序扫最近 30 天
- 按 `session_id` + `invocation_id` 分组生成 trace 行
- 完成后置 `backfill_done=true`

#### 5.2.6 僵尸 running 清扫（InterruptStaleTraces，2026-07-29）

进程崩溃 / runner 未完成的 trace 会永久停在 `running`。`MonitorTraceBackfillWorker` 每轮先 `sweepStaleRunning`：

- **TTL**：`staleRunningTraceTTL = 30min`（`created_at` 早于 cutoff 才候选）
- **span 活跃守卫**：仅当 `NOT EXISTS` TTL 窗口内 span 活动（`monitor_trace_spans.started_at`/`ended_at` 毫秒时间戳 ≥ cutoffMs）才置 `interrupted`——长运行 team run 持续产 span，不会被误杀
- **已知边界**：HITL 人工等待超 TTL 且无 span 产出的 run 会被标记 `interrupted`；仅影响监控展示，不影响运行时恢复（运行真相源在 session_runs/checkpoint）

### 5.3 验收标准

| 指标 | 目标 |
|------|------|
| 新产生的 turn 100% 落 trace 行 | ✅ |
| Trace 详情 Waterfall 渲染数据完整率 | ≥ 95% |
| Run 详情前端请求数 | 从 N+1 降到 2 次（trace + spans） |
| 历史回填覆盖率 | ≥ 99%（30 天内） |
| 集成测 `TestTraceProjector_RunnerCompletion_BuildsTraceWithSpans` | ✅ |

---

## 6. MON-OPT-06：告警规则注册表 + 自定义指标 DSL

### 6.1 现状与业务问题

```go
switch strings.TrimSpace(rule.MetricKey) {
case "runner.error_rate": u.evaluateRunnerErrorRate(...)
case "skill.filesystem_missing_count": u.evaluateSkillFilesystemMissingCount(...)
}
```

| 问题 | 影响 |
|------|------|
| 新增指标需改 Usecase + repo + Wire | 业务需求"我想告 token 成本超阈" → 工程介入 |
| 无表达式能力 | 不能配 "5 min 内同一 user 错误数 > 3" 这种复合条件 |
| 阈值固定 number | 不能配 "对比上一周同时段" |

### 6.2 设计方案

#### 6.2.1 Metric Registry

```go
type AlertMetric interface {
    Key() string                                          // "runner.error_rate"
    Description() string
    Inputs() []string                                     // 依赖事件类型
    Evaluate(ctx context.Context, window time.Duration, scope ScopeFilter) (value float64, err error)
}

type AlertMetricRegistry struct {
    mu sync.RWMutex
    m  map[string]AlertMetric
}

func (r *AlertMetricRegistry) Register(m AlertMetric)
func (r *AlertMetricRegistry) Get(key string) (AlertMetric, bool)
func (r *AlertMetricRegistry) List() []AlertMetric
```

启动时注册 built-in metrics（取代当前 switch）：

| key | Evaluate |
|-----|----------|
| `runner.error_rate` | window 内 error / total |
| `runner.avg_duration_ms` | window 内 duration AVG |
| `runner.p95_duration_ms` | 直方图分位 |
| `skill.filesystem_missing_count` | 从 FilesystemHealthReader 取 |
| `token.cost_per_hour_usd` | usage event 聚合 |
| `chat.user_negative_feedback_count` | `chat.user_feedback` 中 negative |

后续新增 metric → 实现 `AlertMetric` + `Register` 即可，规则配置无需代码改动。

#### 6.2.2 表达式 DSL（简版）

`AlertRule.Expression` 字符串：

```text
runner.error_rate(window=10m, scope=agent:foo) > 0.25
chat.user_negative_feedback_count(window=1h, scope=team:bar) >= 5
token.cost_per_hour_usd() > 50 AND token.cost_per_hour_usd(window=24h) > 800
```

文法（简化 BNF）：

```bnf
Expr        := Compare (Logical Compare)*
Compare     := MetricCall Op Number
Logical     := "AND" | "OR"
Op          := ">" | ">=" | "<" | "<=" | "==" | "!="
MetricCall  := Identifier "(" ArgList ")"
ArgList     := (Arg ("," Arg)*)?
Arg         := Identifier "=" Value
Value       := Number | String | "agent:" Id | "team:" Id | "user:" Id
```

实现：直接用 `expr-lang/expr` 或自写小递归下降解析器；评估器拿 AST → 调注册表 metric.Evaluate。

#### 6.2.3 规则 CRUD 升级

`AlertRule` proto 扩展：

```protobuf
message MonitorAlertRule {
  // existing ...
  string expression = 20;          // 新表达式，与 metric_key+threshold 二选一
  string scope_json = 21;          // {"agent_ids":["foo"],"team_ids":["bar"]}
  repeated string silence_windows = 22;  // cron 表达式数组
  string reminder_minutes = 23;
}
```

兼容：旧 `metric_key + threshold` 自动转换为 `metric_key(window=W) > T` 表达式。

#### 6.2.4 自定义指标插件（可选 Phase 2）

允许用户上传 Go plugin（admin only）：
- 实现 `AlertMetric` 接口
- 通过 `plugin.Open` 动态加载
- 沙箱：超时 1 s / 内存限制 / 仅读取 monitor.* 事件
（Go plugin 限制多，可改为 WASM 评估器，详见 Phase 2 设计）

### 6.3 验收标准

| 指标 | 目标 |
|------|------|
| 新增 built-in metric 改动文件数 | ≤ 2（实现 + Register） |
| 表达式 DSL 覆盖现有 runner.error_rate + filesystem 两规则 | ✅ |
| 旧 metric_key + threshold 规则向后兼容 | 100% |
| 集成测 `TestAlertExpressionDSL_RunnerErrorRate_WithScope` | ✅ |

---

## 7. 跨方案的统一原则

| 原则 | 落地点 |
|------|--------|
| **关键路径不可静默失败** | OPT-01 (drop 入 metric) / OPT-02 (firing 状态机) / OPT-04 (反压可见) |
| **业务用结构化协议而非字符串 switch** | OPT-06 (DSL + Registry) |
| **多实例 / 高可用一等公民** | OPT-02 (分布式锁) / OPT-03 (评估幂等 + 单飞) |
| **可观测自闭环**：监控系统自身的健康度可监控 | OPT-04 metrics / OPT-03 degraded 事件 / OPT-05 trace projector status |
| **每个优化项可独立 ship / 灰度** | 所有 DDL 加列默认值；行为开关有 env flag |

---

## 8. 不在本方案范围

| 项 | 理由 |
|----|------|
| 用量大盘（`/overview`）改版 | 见独立需求 `18 monitor-dashboard.md` |
| Audit 表 schema 改动 | 现有满足合规，本轮不动 |
| 自定义 metric 的 WASM 评估器 | OPT-06 Phase 2，本方案仅占位 |
| 接入外部 incident 系统（PagerDuty 等） | 通过 Webhook 即可，无需平台内置 |
| 前端 ECharts 改型 | 本方案聚焦后端业务流，前端按需对齐 |

---

## 9. 与监控分流规则的对照（`monitor-streams-wire.mdc`）

| 规则约定 | 本方案落地 |
|----------|-----------|
| Audit / Logs / Events 不混表 | 保持 ✅；OPT-05 traces 独立表 |
| 实时主通道 WS / 禁止独立 SSE | 保持 ✅；OPT-04 在 WS 内做反压 |
| flow_log 走 MonitorBus | **OPT-01 彻底落地** |
| TeamRunEvent snake_case + payload 扩展 | 保持 ✅ |
| 重要配置变更写 audit_logs（detail 不脱敏密钥） | 不变；OPT-02 alert rule 变更将额外写一条 audit |
| `cmd/admin/wire_gen.go` 不手改 | 严格遵守；OPT-03 Worker 通过 wire provider 注入 |


---

## 子模块：Monitor AI 闭环设计

> **关联**：[`18 monitor.md`](./18%20monitor.md) · [`18-monitor.design.md`](./18%20monitor.design.md) · [`52-flow-logger.design.md`](./52-flow-logger.design.md) · 代码 Review [`2026-05-26-Monitor-Code-Review.md`](../review/2026-05-26-Monitor-Code-Review.md)
> **创建**：2026-05-28

> 实现进度见 [18-monitor.development.md §子模块 AI 闭环](./18-monitor.development.md)。

---

## 0. 需求原文与问题定义

### 0.1 原始需求

> 通过后台的 logs 日志，记录服务的所有运行状态，AI 可以根据日志运行的记录文件追踪到问题，定位问题，形成闭环。

### 0.2 需求拆解

| 子需求 | 含义 | 设计方案 |
|--------|------|----------|
| **记录所有运行状态** | 每个关键业务动作都有结构化日志 | LOG-03 路径补全 |
| **日志持久化到文件** | 日志写入磁盘文件，进程重启后可回溯 | LOG-01 文件落盘 |
| **AI 可读取日志** | 日志格式对 AI 友好（结构化、可检索、带关联 ID） | LOG-01 JSON Lines + LOG-02 zap 结构化 |
| **AI 追踪到问题** | 从一条错误日志出发，沿 trace_id / session_id 回溯完整链路 | DIAG-01 诊断包自动聚合 |
| **定位问题** | AI 能给出根因分析 + 修复建议 | DIAG-02 根因规则引擎 |
| **形成闭环** | 问题从发现 → 追踪 → 定位 → 修复 → 验证 全链路可追溯 | LOOP-01 闭环工作流 + 自检/自愈 |

### 0.3 闭环定义

```
┌──────────────────────────────────────────────────────────────────────┐
│                                                                      │
│  ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌────┐ │
│  │ 1.发现   │───▶│ 2.追踪   │───▶│ 3.定位   │───▶│ 4.修复   │───▶│5.验 │ │
│  │ Detect  │    │ Trace   │    │ Root    │    │ Fix     │    │证   │ │
│  │         │    │         │    │ Cause   │    │         │    │Verify│ │
│  └─────────┘    └─────────┘    └─────────┘    └─────────┘    └────┘ │
│       ▲                                                    │       │
│       └────────────────────────────────────────────────────┘       │
│                                                                      │
│  数据源：结构化日志文件（JSON Lines）                                   │
│  关联键：trace_id + session_id + run_id                              │
│  AI 角色：自动执行 1→2→3，辅助 4，自动执行 5                           │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 1. 日志体系现状

项目存在 **三套并行的日志体系**，尚未统一：

| 体系 | 实现 | 输出目标 | 结构化 | 关联 ID | AI 可读 |
|------|------|----------|--------|---------|---------|
| **框架层 zap** | `pkg/trpc-agent-go/log` | stdout（ConsoleEncoder） | ❌ 彩色控制台 | ❌ 无 trace_id | ❌ |
| **应用层 FlowLog** | `internal/event/trace_emitter.go` | EventBus → WS + DB | ✅ `flow_log/v1` | ✅ trace_id/session_id/run_id | ✅ |
| **系统域 SysLog** | `internal/event/system_flow.go` | EventBus → MonitorBus | ✅ `flow_log/v1` | 🟡 部分（system 域无 session） | ✅ |

---

## 2. 方案设计

### 2.1 总体架构

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        服务运行时                                        │
│                                                                         │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐            │
│  │  业务代码     │     │  框架层       │     │  系统域       │            │
│  │  TraceEmitter │     │  zap Logger  │     │  SysLog*     │            │
│  └──────┬───────┘     └──────┬───────┘     └──────┬───────┘            │
│         │                    │                    │                     │
│         ▼                    ▼                    ▼                     │
│  ┌──────────────────────────────────────────────────────┐              │
│  │              EventBus（MonitorBus + SessionBus）       │              │
│  └──────────────────────┬───────────────────────────────┘              │
│                         │                                               │
│         ┌───────────────┼───────────────┐                              │
│         ▼               ▼               ▼                              │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐                      │
│  │ WS 推送      │ │ DB 持久化    │ │ 文件 Appender│  ← LOG-01 新增     │
│  │ (现有)       │ │ (现有+增强)  │ │ (新增)       │                      │
│  └─────────────┘ └─────────────┘ └──────┬──────┘                      │
│                                          │                              │
│                                          ▼                              │
│                                 ┌─────────────────┐                    │
│                                 │ JSON Lines 文件  │                    │
│                                 │ /var/log/aranea/ │                    │
│                                 │   flow-*.jsonl   │                    │
│                                 │   system-*.jsonl │                    │
│                                 │   trace-*.jsonl  │                    │
│                                 └────────┬────────┘                    │
│                                          │                              │
└──────────────────────────────────────────┼──────────────────────────────┘
                                           │
                                           ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                        AI 闭环追踪层                                      │
│                                                                          │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐             │
│  │ 1.日志扫描    │────▶│ 2.链路追踪    │────▶│ 3.根因分析    │             │
│  │ LogScanner   │     │ TraceWalker  │     │ RootCause    │             │
│  │              │     │              │     │ Engine       │             │
│  └──────────────┘     └──────────────┘     └──────┬───────┘             │
│                                                    │                    │
│                              ┌─────────────────────┼──────────────┐     │
│                              ▼                     ▼              ▼     │
│                       ┌──────────┐          ┌──────────┐   ┌─────────┐ │
│                       │ 4.诊断包  │          │ 5.修复建议 │   │ 6.验证   │ │
│                       │ DiagPack │          │ FixSuggest│   │ Verify  │ │
│                       └──────────┘          └──────────┘   └─────────┘ │
└──────────────────────────────────────────────────────────────────────────┘
```

### 2.2 设计原则

| 原则 | 说明 |
|------|------|
| **日志即真相源** | 所有运行状态以 JSON Lines 文件为最终持久化形态，DB/WS 为投影 |
| **一条链路一个 trace_id** | 沿用 FlowLog 现有设计，trace_id 贯穿日志、Trace、Usage |
| **AI First 格式** | 日志输出 JSON Lines，每行自描述（含 schema_version），无需额外 schema 文件 |
| **零侵入追加** | 文件 Appender 作为 EventBus 消费者接入，不修改现有 FlowLog/TraceEmitter 代码 |
| **闭环可验证** | 每个闭环步骤有明确的输入/输出契约，可自动化测试 |

---

## 3. LOG-01：FlowLog 文件落盘

### 3.1 目标

将所有 FlowLog + 系统日志持久化到磁盘 JSON Lines 文件，确保进程重启、DB 异常后仍可回溯。

### 3.2 文件布局

```
/var/log/aranea/
├── flow-2026-05-28.jsonl          # 当日 FlowLog（业务域 + 系统域）
├── flow-2026-05-27.jsonl          # 昨日（轮转后）
├── system-2026-05-28.jsonl        # 当日系统域日志（独立文件，高频）
├── trace-2026-05-28.jsonl         # 当日 Trace 完成事件（span 聚合后）
└── alert-2026-05-28.jsonl         # 当日告警事件
```

### 3.3 FlowFileAppender

```go
type FlowFileAppender struct {
    dir        string
    flowFile   *rotatingFile
    systemFile *rotatingFile
    traceFile  *rotatingFile
    alertFile  *rotatingFile
}

type rotatingFile struct {
    mu       sync.Mutex
    path     string
    file     *os.File
    encoder  *json.Encoder
    date     string
    maxSize  int64
}
```

**路由规则**：

| Envelope Type | Channel | 目标文件 |
|---------------|---------|----------|
| `flow_log` | `monitor` | `system-YYYY-MM-DD.jsonl` |
| `flow_log` | 其他（chat/team/...） | `flow-YYYY-MM-DD.jsonl` |
| `alert.fired` / `alert.recovered` / `alert.notify` | 任意 | `alert-YYYY-MM-DD.jsonl` |
| `runner.completion` | 任意 | `trace-YYYY-MM-DD.jsonl` |

### 3.4 文件轮转

| 参数 | 默认值 | 配置项 |
|------|--------|--------|
| 轮转周期 | 每日 | `server.monitor.log_rotation` |
| 单文件最大 | 500 MB | `server.monitor.log_max_size_mb` |
| 保留天数 | 30 天 | `server.monitor.log_retention_days` |
| 压缩 | gzip（>1 天的文件） | `server.monitor.log_compress` |

### 3.5 接入方式

作为 EventBus 消费者，与现有 `flowLogPersistConsumer` 并行：

```go
func newFlowFileAppender(infra *event.Infra, cfg *conf.Monitor) *FlowFileAppender {
    a := &FlowFileAppender{dir: cfg.LogDir}
    infra.MonitorBus().Subscribe(event.SubscribeOptions{
        Channel:   "monitor",
        BufferSize: 4096,
        DropPolicy: event.DropOldest,
        Handler:   a.onEnvelope,
    })
    return a
}
```

### 3.6 验收标准

| 指标 | 目标 |
|------|------|
| FlowLog 写入文件延迟 | < 10 ms（异步） |
| 文件轮转无数据丢失 | ✅ |
| 30 天内任意历史日志可查 | ✅ |
| 磁盘异常时服务不受影响 | ✅ 降级为 SysLogWarn |

---

## 4. LOG-02：框架层 zap 日志结构化

### 4.1 目标

将 `pkg/trpc-agent-go/log` 的 ConsoleEncoder 替换为 JSON Encoder，使框架层日志也可被 AI 解析。

### 4.2 方案

```go
var Default Logger = zap.New(
    zapcore.NewCore(
        zapcore.NewJSONEncoder(jsonEncoderConfig),  // Console → JSON
        zapcore.NewMultiWriteSyncer(
            zapcore.AddSync(os.Stdout),
            zapcore.AddSync(fileSync),              // 同时写文件
        ),
        zapLevel,
    ),
    zap.AddCaller(),
    zap.AddCallerSkip(1),
).Sugar()
```

**JSON 编码器配置**：

```go
jsonEncoderConfig := zap.NewProductionEncoderConfig()
jsonEncoderConfig.TimeKey = "ts"
jsonEncoderConfig.LevelKey = "level"
jsonEncoderConfig.MessageKey = "msg"
jsonEncoderConfig.CallerKey = "caller"
jsonEncoderConfig.StacktraceKey = "stack"
jsonEncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
jsonEncoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder
```

### 4.3 Context 注入关联 ID

扩展 zap Logger，从 context 中提取 `trace_id` / `session_id` / `run_id` 并注入日志字段：

```go
func WithTraceFields(ctx context.Context, logger *zap.SugaredLogger) *zap.SugaredLogger {
    if tc, ok := event.TraceContextFromContext(ctx); ok {
        return logger.With(
            "trace_id", tc.TraceID,
            "session_id", tc.SessionID,
            "run_id", tc.RunID,
        )
    }
    return logger
}
```

### 4.4 输出示例

```json
{"ts":"2026-05-28T10:00:01.123+0800","level":"warn","caller":"trpc-agent-go/agent.go:42","msg":"tool execution timeout","trace_id":"tr_abc123","session_id":"sess_def456","tool":"search","duration_ms":30000}
```

### 4.5 验收标准

| 指标 | 目标 |
|------|------|
| 框架层日志 100% JSON 输出 | ✅ |
| 含 trace_id 的日志占比（Turn 内路径） | ≥ 90% |
| JSON 输出与现有 Console 输出性能差异 | < 5% |

---

## 5. LOG-03：关键路径 FlowLog 补全

### 5.1 目标

将仍使用 slog/zap 的关键业务路径迁移到 FlowLog，确保 AI 可通过 trace_id 追踪完整链路。

### 5.2 需补全的路径

| 路径 | 当前方式 | 迁移目标 | 优先级 |
|------|----------|----------|--------|
| Provider 调用失败/重试 | slog | `provider.call.error` / `provider.call.retry` | P1 |
| Memory 读写（L0-L4） | 部分 FlowLog | 补全 `memory.read.miss` / `memory.write.error` | P1 |
| MCP 连接/调用 | slog | `mcp.session.connect` / `mcp.tool.invoke` | P1 |
| Knowledge 检索失败 | 部分 FlowLog | `knowledge.search.error` / `knowledge.chunk.empty` | P2 |
| Plugin 沙箱执行 | slog | `plugin.sandbox.execute` / `plugin.sandbox.timeout` | P2 |
| Graph 节点执行 | 部分 FlowLog | `graph.node.enter` / `graph.node.error` | P2 |
| Session 状态持久化 | slog | `session.state.persist` / `session.state.restore` | P2 |
| Token 配额检查 | slog | `token.quota.check` / `token.quota.exceeded` | P3 |

### 5.3 补全原则

1. **只补关键路径**：start/done/error 三阶段，skip 可选
2. **复用 TraceEmitter**：从 context 获取，不新建
3. **slog 保留**：非业务路径（如 Kratos 框架内部）保留 slog，通过 LOG-02 结构化
4. **step_id 注册表同步**：每新增 step_id 必须更新 `flow_log.go` 的 `stepTitleRegistry`

### 5.4 验收标准

| 指标 | 目标 |
|------|------|
| P1 路径 100% 覆盖 FlowLog | ✅ |
| 关键错误（provider/memory/mcp）均有 trace_id | ≥ 95% |
| 步骤注册表与实际调用点对齐 | ✅ |

---

## 6. TRACE-01：Trace 写入回路

> 本节引用 [MON-OPT-05](#5-mon-opt-05trace-写入回路--run-全链路视图) 的设计，不重复。仅补充 AI 闭环所需的接口。

### 6.1 AI 闭环依赖的 Trace 能力

| 能力 | 用途 | MON-OPT-05 覆盖 |
|------|------|-----------------|
| `monitor_traces` 写入 | AI 按 trace_id 查询完整运行 | ✅ MonitorTraceProjector |
| `monitor_trace_spans` 写入 | AI 查看每步耗时和状态 | ✅ span 投影 |
| 跨 turn/跨 team 关联 | AI 追踪跨 Agent 调用链 | ✅ parent_trace_id |
| Trace 文件落盘 | AI 直接读文件，不依赖 DB | ❌ 本方案 TRACE-01 补充 |

### 6.2 Trace 文件落盘

在 `FlowFileAppender` 中增加 Trace 完成事件写入：

```jsonl
{"schema_version":"trace_complete/v1","trace_id":"tr_abc","session_id":"sess_def","run_id":"run_ghi","status":"error","duration_ms":5230,"span_count":5,"error_count":1,"total_tokens":1520,"spans":[{"id":"s1","name":"chat.turn","kind":"root","status":"ok","duration_ms":5230},{"id":"s2","name":"llm.call","kind":"llm","status":"ok","duration_ms":3200},{"id":"s3","name":"tool.search","kind":"tool","status":"error","duration_ms":1500,"error":"timeout"}]}
```

### 6.3 验收标准

| 指标 | 目标 |
|------|------|
| 新 Turn 100% 产生 trace 行 + 文件记录 | ✅ |
| trace-*.jsonl 可被 AI 直接解析 | ✅ |

---

## 7. DIAG-01：AI 诊断包

### 7.1 目标

从一条错误日志出发，自动聚合相关联的所有上下文信息，生成 AI 可直接消费的诊断包。

### 7.2 诊断包结构

```
diagnostic_bundle/
├── manifest.json              # 元数据
├── flow.jsonl                 # 按 trace_id 过滤的 FlowLog 条目
├── trace.json                 # Trace + Spans 完整数据
├── usage.json                 # Token/Cost 用量
├── alerts.jsonl               # 相关告警事件
├── system.jsonl               # 相关系统日志（按时间窗口）
├── config_redacted.json       # 脱敏后的 Agent/Provider 配置快照
└── summary.json               # AI 生成的摘要（可选）
```

### 7.3 manifest.json

```json
{
  "schema_version": "diag_bundle/v1",
  "bundle_id": "db_01J...",
  "created_at": "2026-05-28T10:05:00Z",
  "trigger": {
    "type": "error",
    "source": "flow_log",
    "trace_id": "tr_abc123",
    "session_id": "sess_def456",
    "run_id": "run_ghi789",
    "step_id": "chat.llm.invoke",
    "severity": "error",
    "message": "Provider timeout after 30s",
    "timestamp": "2026-05-28T10:00:05Z"
  },
  "scope": {
    "time_range": ["2026-05-28T09:59:00Z", "2026-05-28T10:05:00Z"],
    "trace_ids": ["tr_abc123"],
    "session_ids": ["sess_def456"],
    "run_ids": ["run_ghi789"]
  },
  "files": {
    "flow.jsonl": { "entries": 23, "size_bytes": 4096 },
    "trace.json": { "spans": 5 },
    "usage.json": { "records": 1 },
    "alerts.jsonl": { "entries": 1 },
    "system.jsonl": { "entries": 8 },
    "config_redacted.json": { "agents": 1, "providers": 1 }
  }
}
```

### 7.4 诊断包生成 API

```protobuf
service MonitorService {
  rpc GenerateDiagnosticBundle(GenerateDiagnosticBundleRequest) returns (GenerateDiagnosticBundleResponse);
}

message GenerateDiagnosticBundleRequest {
  string trigger_type = 1;    // error | alert | manual
  string trace_id = 2;        // 入口关联键
  string session_id = 3;
  string run_id = 4;
  string step_id = 5;
  int32  context_minutes = 6; // 前后时间窗口（默认 5 分钟）
}

message GenerateDiagnosticBundleResponse {
  string bundle_id = 1;
  string download_url = 2;    // 临时下载链接
  string manifest_json = 3;   // 内联 manifest
  int32  total_entries = 4;
}
```

### 7.5 自动触发规则

| 触发条件 | 动作 |
|----------|------|
| FlowLog severity=critical | 自动生成诊断包 + 写入 `alert-*.jsonl` |
| 告警规则 firing | 自动生成诊断包 + 附加到告警通知 |
| 用户在 Monitor 页面点击「诊断」 | 手动触发，可指定 trace_id |
| API 调用 `GenerateDiagnosticBundle` | 外部 AI 工具触发 |

### 7.6 验收标准

| 指标 | 目标 |
|------|------|
| 从一条 critical FlowLog 到诊断包生成 | < 5 s |
| 诊断包包含完整 trace 链路 | ≥ 95% |
| 诊断包可被 GPT-4/Claude 直接解析 | ✅ |
| 诊断包大小 | < 1 MB（单次运行） |

---

## 8. DIAG-02：根因分析规则引擎

### 8.1 目标

基于诊断包中的结构化数据，自动推导错误根因，减少 AI 的推理负担。

### 8.2 规则模型

```go
type RootCauseRule struct {
    ID          string
    Name        string
    Description string
    Condition   RootCauseCondition
    RootCause   string
    FixSuggest  string
    Severity    string
}

type RootCauseCondition struct {
    StepID      string            // 匹配的 step_id
    Phase       string            // error / critical
    ErrorCodes  []string          // 匹配的 error.code
    Pattern     string            // 正则匹配 error.message
    Prerequisites []Prerequisite  // 前置条件（增强准确率）
}

type Prerequisite struct {
    StepID   string
    Phase    string
    Severity string
    Pattern  string
}
```

### 8.3 内置规则

| 规则 ID | 匹配条件 | 根因 | 修复建议 |
|---------|----------|------|----------|
| `RC-001` | step=`chat.llm.invoke`, phase=error, pattern=`timeout` | Provider 响应超时 | 1. 检查 Provider 状态 2. 增大超时 3. 切换 Provider |
| `RC-002` | step=`chat.first_byte_timeout`, phase=error | 模型首 Token 延迟过高 | 1. 检查网络 2. 切换更快的模型 3. 减小 max_tokens |
| `RC-003` | step=`chat.turn.empty_reply`, phase=error | 模型返回空响应 | 1. 检查 prompt 长度 2. 检查 content filter 3. 重试 |
| `RC-004` | step=`provider.call.error`, pattern=`429\|rate_limit` | Provider 限流 | 1. 降低并发 2. 启用多 Provider 轮换 3. 检查配额 |
| `RC-005` | step=`provider.call.error`, pattern=`401\|invalid_api_key` | API Key 无效 | 1. 检查 Provider 配置 2. 更新 API Key |
| `RC-006` | step=`knowledge.search.error` | 知识库检索失败 | 1. 检查 Embedding 服务 2. 检查索引状态 3. 重建索引 |
| `RC-007` | step=`mcp.tool.invoke`, phase=error | MCP 工具调用失败 | 1. 检查 MCP 服务状态 2. 检查工具参数 3. 重连 MCP |
| `RC-008` | step=`memory.write.error` | 记忆写入失败 | 1. 检查 DB 连接 2. 检查存储空间 3. 检查 schema |
| `RC-009` | step=`chat.turn.timeout` | Turn 整体超时 | 1. 检查是否有死循环工具调用 2. 增大 turn 超时 3. 检查 Agent 配置 |
| `RC-010` | step=`system.bus.drop`, pattern=`flow_log` | FlowLog 被丢弃 | 1. 检查 Bus buffer 配置 2. 检查消费者处理速度 |
| `RC-011` | step=`plugin.sandbox.timeout` | 插件沙箱超时 | 1. 优化插件逻辑 2. 增大超时 3. 检查资源限制 |
| `RC-012` | step=`token.quota.exceeded` | Token 配额耗尽 | 1. 充值/提升配额 2. 优化 prompt 3. 启用缓存 |

### 8.4 规则评估

```go
func (e *RootCauseEngine) Evaluate(bundle *DiagnosticBundle) []RootCauseResult {
    var results []RootCauseResult
    for _, entry := range bundle.FlowLogEntries {
        if entry.Severity != "error" && entry.Severity != "critical" {
            continue
        }
        for _, rule := range e.rules {
            if matchRule(rule, entry, bundle) {
                results = append(results, RootCauseResult{
                    Rule:      rule,
                    Entry:     entry,
                    Confidence: calcConfidence(rule, bundle),
                })
            }
        }
    }
    sort.Slice(results, func(i, j int) bool {
        return results[i].Confidence > results[j].Confidence
    })
    return results
}
```

### 8.5 置信度计算

| 因素 | 权重 | 说明 |
|------|------|------|
| 规则直接匹配 | 0.4 | step_id + error pattern 完全匹配 |
| 前置条件满足 | 0.3 | Prerequisites 全部满足 |
| 时间关联性 | 0.2 | 错误发生在相关步骤之后 |
| 频率关联性 | 0.1 | 同类错误在近期重复出现 |

### 8.6 验收标准

| 指标 | 目标 |
|------|------|
| 内置规则覆盖 Top 12 常见错误 | ✅ |
| 根因命中率（人工标注） | ≥ 80% |
| 规则评估延迟 | < 100 ms |
| 新增规则无需改代码 | ✅（配置驱动） |

---

## 9. LOOP-01：闭环工作流

### 9.1 目标

将「发现 → 追踪 → 定位 → 修复 → 验证」串联为自动化工作流，AI 可端到端执行。

### 9.2 闭环状态机

```
detected ──[auto/manual]──▶ tracing ──[bundle ready]──▶ analyzing ──[root cause found]──▶ suggested
    │                          │                         │                              │
    │                          │                         │                              ▼
    │                          │                         │                         fixing ──[fix applied]──▶ verifying
    │                          │                         │                                              │
    │                          │                         │                              ┌───────────────┘
    │                          │                         │                              ▼
    │                          │                         │                         verified ──[pass]──▶ closed
    │                          │                         │                              │
    └──────────────────────────┴─────────────────────────┴──────────────────────────────┘
                                                                                          │
                                                                                    [fail] └──▶ reopened
```

### 9.3 闭环事件

| 事件 | 触发 | 数据 |
|------|------|------|
| `loop.detected` | FlowLog critical / Alert firing | trace_id, severity, step_id |
| `loop.tracing` | 自动/手动触发诊断包生成 | bundle_id |
| `loop.analyzed` | 根因引擎完成评估 | root_cause_id, confidence, fix_suggest |
| `loop.fix_suggested` | AI 生成修复建议 | fix_actions[] |
| `loop.fix_applied` | 人工/AI 执行修复 | fix_result |
| `loop.verifying` | 修复后自动验证 | verify_plan |
| `loop.verified` | 验证通过 | verify_result |
| `loop.closed` | 闭环完成 | summary |
| `loop.reopened` | 验证失败 | fail_reason |

### 9.4 验证策略

| 验证类型 | 说明 | 示例 |
|----------|------|------|
| **重放验证** | 用相同输入重试失败步骤 | 重新调用 Provider 检查是否恢复 |
| **指标验证** | 检查相关指标是否恢复正常 | error_rate < threshold |
| **日志验证** | 检查后续日志是否无同类错误 | 5 分钟内无同 step_id error |
| **功能验证** | 执行健康检查端点 | `GET /healthz` 返回 200 |

### 9.5 闭环记录

每次闭环产生一条 `loop_record`，持久化到 `monitor_events`：

```json
{
  "event_key": "loop.closed",
  "status": "ok",
  "metadata_json": {
    "loop_id": "lp_01J...",
    "trigger_trace_id": "tr_abc123",
    "trigger_step_id": "chat.llm.invoke",
    "root_cause_rule": "RC-001",
    "confidence": 0.85,
    "fix_actions": ["增大 Provider 超时至 60s"],
    "verify_result": "pass",
    "duration_ms": 125000,
    "total_entries_analyzed": 23
  }
}
```

### 9.6 AI Prompt 模板

AI 在执行闭环时的系统 Prompt 模板：

```markdown
## 角色
你是 Aranea 平台的运维 AI 助手，负责根据日志诊断和修复服务问题。

## 输入
- 诊断包 manifest.json
- flow.jsonl（按时间排序的 FlowLog 条目）
- trace.json（Span 树）
- 根因分析结果（规则 ID + 置信度）

## 工作流
1. 阅读 manifest.json 了解问题概要
2. 扫描 flow.jsonl 中 severity=error/critical 的条目
3. 根据 trace_id 追踪完整调用链
4. 对照根因分析结果，确认或修正根因
5. 给出具体修复建议（含操作步骤）
6. 建议验证方案

## 输出格式
```json
{
  "root_cause": "...",
  "confidence": 0.0-1.0,
  "evidence": ["step_id:xxx -> ...", "..."],
  "fix_suggestions": [
    {"action": "...", "priority": "high/medium/low", "steps": ["..."]}
  ],
  "verify_plan": {"type": "metric|replay|log|functional", "params": {...}}
}
```

## 注意
- 不要猜测，基于日志证据推导
- 如果证据不足，明确说明需要哪些额外信息
- 修复建议必须是可执行的操作，不要模糊描述
- 敏感信息（API Key、Token）不要出现在输出中
```

### 9.7 验收标准

| 指标 | 目标 |
|------|------|
| 从 critical 错误到诊断包生成 | < 10 s |
| 从诊断包到根因分析完成 | < 5 s |
| 闭环记录 100% 写入 monitor_events | ✅ |
| 闭环记录可通过 Monitor Events Tab 查看 | ✅ |

---

## 10. 配置汇总

### 10.1 新增配置项

```yaml
server:
  monitor:
    log_dir: "/var/log/aranea"           # 日志文件目录
    log_rotation: "daily"                # daily | hourly | size
    log_max_size_mb: 500                 # 单文件最大 MB
    log_retention_days: 30               # 保留天数
    log_compress: true                   # 轮转后 gzip 压缩
    log_file_enabled: true               # 文件落盘开关
    diagnostic_auto_trigger: true        # critical 时自动生成诊断包
    diagnostic_context_minutes: 5        # 诊断包时间窗口
    root_cause_engine_enabled: true      # 根因引擎开关
    loop_workflow_enabled: true          # 闭环工作流开关
```

### 10.2 环境变量

| 变量 | 默认 | 说明 |
|------|------|------|
| `ARANEA_LOG_DIR` | `/var/log/aranea` | 日志目录（覆盖 config.yaml） |
| `ARANEA_LOG_FILE_ENABLED` | `true` | 文件落盘开关 |
| `ARANEA_DIAG_AUTO_TRIGGER` | `true` | 自动诊断触发 |
| `ARANEA_ROOT_CAUSE_ENABLED` | `true` | 根因引擎开关 |

---

## 11. 与已有优化方案的关系

| 已有方案 | 本方案关系 | 冲突 |
|----------|-----------|------|
| MON-OPT-01（Bus 分离） | **前置依赖**：FlowFileAppender 订阅 MonitorBus，需 Bus 分离完成 | 无 |
| MON-OPT-02（告警冷却持久化） | **协作**：闭环工作流的 `loop.detected` 可由 alert.fired 触发 | 无 |
| MON-OPT-03（告警批量化） | **协作**：RingBuffer 数据可被根因引擎复用 | 无 |
| MON-OPT-04（WS 反压） | **独立**：文件 Appender 不走 WS，不受反压影响 | 无 |
| MON-OPT-05（Trace 写入） | **前置依赖**：诊断包依赖 trace 数据 | 无 |
| MON-OPT-06（规则注册表） | **协作**：根因规则可注册到 AlertMetricRegistry | 无 |
| 52-flow-logger Phase 2 | **前置依赖**：FlowLog 落库是文件落盘的基础 | 无 |

### 建议实施顺序

```
MON-OPT-01 (Bus 分离)
    │
    ├──▶ LOG-01 (文件落盘)
    │       │
    │       └──▶ LOG-02 (zap 结构化)
    │
    ├──▶ MON-OPT-05 (Trace 写入)
    │       │
    │       └──▶ TRACE-01 (Trace 文件落盘)
    │
    ├──▶ LOG-03 (路径补全)
    │
    └──▶ DIAG-01 (诊断包)
            │
            ├──▶ DIAG-02 (根因引擎)
            │
            └──▶ LOOP-01 (闭环工作流)
```

---

## 12. 不在本方案范围

| 项 | 理由 |
|----|------|
| 日志搜索服务（ELK/Loki） | 本方案聚焦文件落盘 + AI 直接消费，不引入额外基础设施 |
| 实时流式 AI 分析 | 当前为按需生成诊断包，实时分析需更大架构变更 |
| 自动修复执行 | 闭环到「修复建议」为止，自动执行修复需人工审批 |
| 多语言日志 | 当前日志以中文 title 为主，AI Prompt 为英文，暂不调整 |
| 前端闭环 UI | 本方案聚焦后端能力，前端展示在后续迭代规划 |
