## Context

Monitor 模块当前具备被动监控能力（告警评估、根因分析、事件驱动自愈），但缺乏主动巡检机制。以下子系统可能静默退化而无法被现有告警捕获：

- **FlowFileAppender**：磁盘满或权限丢失导致落盘静默失败
- **TraceProjector**：EventBus 订阅断开后 Trace 投影停止，但无告警
- **AlertEvalWorker**：评估循环停滞（goroutine 泄漏或死锁）
- **EventBus 订阅**：Handler panic 导致订阅静默退出
- **WebSocket 连接**：前端推送通道断开
- **数据库**：SQLite 锁竞争或 Schema 损坏

现有 `monitor-self-healing` 变更解决的是**运行时错误的自愈**（LLM 重试、MCP 重连），而本变更解决的是**基础设施层面的主动巡检与修复**。两者互补，不重叠。

## Goals / Non-Goals

**Goals:**
- 定时主动检查 Monitor 所有子系统健康状态
- 自检发现问题后自动执行修复动作
- 自检结果持久化，可查询历史
- 自检失败集成到告警体系
- 自检数据集成到诊断包
- 暴露 API 供前端展示

**Non-Goals:**
- 不做跨实例自检协调（SQLite 单实例）
- 不做用户自定义自检规则（内置检查器，后续可扩展）
- 不做运行时层面的自愈（由 monitor-self-healing 覆盖）
- 不做前端完整自检仪表盘 UI（仅基础状态面板）

## Decisions

### D1: SelfChecker 接口设计

**选择**: 定义 `SelfChecker` 接口，每个子系统实现一个 Checker。

**代码实际实现**（`internal/biz/monitor/self_check.go` + `internal/biz/types/monitor_condition.go`）：

```go
type SelfChecker interface {
    Name() string
    Check(ctx context.Context) types.SelfCheckResult
}

type SelfCheckResult struct {
    CheckID    string                  `json:"check_id"`
    Checker    string                  `json:"checker"`
    Status     SelfCheckStatus         `json:"status"`
    Message    string                  `json:"message,omitempty"`
    Details    map[string]any          `json:"details,omitempty"`
    Conditions []SelfCheckStatusCondition `json:"conditions,omitempty"`
    CheckedAt  time.Time               `json:"checked_at"`
}

type SelfCheckStatus string
const (
    SelfCheckStatusPassed  SelfCheckStatus = "passed"
    SelfCheckStatusWarning SelfCheckStatus = "warning"
    SelfCheckStatusFailed  SelfCheckStatus = "failed"
)
```

**与原设计差异**：
- 状态值从 `healthy/degraded/unhealthy` 改为 `passed/warning/failed`，更符合测试语义
- `SelfCheckResult` 额外包含 `CheckID`（UUID 唯一标识）和 `Conditions`（用于 RootCauseCondition 扩展）
- `SelfCheckResult` 类型定义在 `internal/biz/types/monitor_condition.go`（跨模块共享类型），而非 `self_check.go`

**替代方案**: 使用统一函数 + switch case → 拒绝，违反 OCP，新增检查器需修改调度器代码。

**理由**: 接口模式符合 OCP，新增检查器只需实现接口并注册，不修改调度器。

### D2: 修复动作与检查器解耦

**选择**: 修复动作独立于检查器，通过 `SelfCheckRepairer` 接口实现。调度器检查 → 发现问题 → 调用对应 Repairer。

**代码实际实现**（`internal/biz/monitor/self_check.go`）：

```go
type SelfCheckRepairer interface {
    CanRepair(checkName string, status types.SelfCheckStatus) bool
    Repair(ctx context.Context, result types.SelfCheckResult) RepairOutcome
}

type RepairOutcome struct {
    Success bool   `json:"success"`
    Action  string `json:"action"`
    Message string `json:"message,omitempty"`
}
```

与原设计一致，无差异。

**替代方案**: 修复逻辑内嵌在 Checker 中 → 拒绝，违反 SRP，检查和修复是不同关注点。

**理由**: 检查和修复分离，同一个检查结果可以有多种修复策略，修复策略可独立演进。

### D3: 自检报告持久化

**选择**: 新增 `self_check_reports` 表，每次自检周期写入一条聚合报告。

**代码实际实现**（`internal/data/ent/schema/self_check_report.go` + `internal/data/sql/migrations/20260715_self_check_report_schema.sql`）：

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT | UUID 主键 |
| check_results_json | TEXT | 各检查器的结果数组（JSON 序列化） |
| overall_status | TEXT | passed / warning / failed（取最差） |
| repair_actions_json | TEXT | 执行的修复动作数组（JSON 序列化） |
| started_at | TEXT | 自检开始时间（RFC3339Nano） |
| finished_at | TEXT | 自检结束时间（RFC3339Nano） |
| duration_ms | INTEGER | 自检耗时 |
| created_at | TEXT | 记录创建时间 |

**与原设计差异**：
- 字段名使用 `_json` 后缀（`check_results_json`、`repair_actions_json`）明确标识 JSON 序列化字段
- 新增 `created_at` 字段用于清理排序
- 使用 raw SQL DDL（`20260715_self_check_report_schema.sql`）而非纯 Ent 代码生成
- 索引：`idx_self_check_reports_status_created`（overall_status + created_at）、`idx_self_check_reports_created`（created_at）

**替代方案**: 每个检查器一条记录 → 拒绝，数据量大，查询聚合复杂。

**理由**: 聚合报告更符合运维查看习惯，一条记录看全貌。保留 30 天，CronJob 清理。

### D4: 自检触发方式

**选择**: 双模式触发 — 定时自动 + 手动 API 触发。

- 定时：SelfCheckScheduler 内置 ticker，默认 5 分钟间隔，可通过 `SELF_CHECK_INTERVAL` 环境变量配置（最小 1 分钟）
- 手动：`POST /v1/monitor/self-check` API，立即触发一次自检

**与原设计差异**：
- 原设计使用 CronRunner Job 触发，代码实现改为 SelfCheckScheduler 内置 ticker + Start/Stop 生命周期
- 间隔配置通过环境变量 `SELF_CHECK_INTERVAL` 而非配置文件
- Start 时立即执行一次 RunOnce（不等第一个 ticker 周期）

**替代方案**: 仅定时 → 拒绝，运维需要按需触发（如部署后验证）。

**理由**: 双模式兼顾自动化和运维灵活性。手动触发有并发锁防止重复执行。

### D5: 自检失败告警集成

**选择**: 自检结果通过现有 `AlertMetricRegistry` 注册新指标 `monitor.selfcheck_unhealthy_count`，告警规则可基于此指标触发。

**代码实际实现**（`internal/biz/monitor/self_check_scheduler.go`）：

```go
type SelfCheckUnhealthyCountMetric struct {
    scheduler *SelfCheckScheduler
}

func (m *SelfCheckUnhealthyCountMetric) Key() string        { return "monitor.selfcheck_unhealthy_count" }
func (m *SelfCheckUnhealthyCountMetric) Description() string { return "Number of unhealthy self-check results" }
func (m *SelfCheckUnhealthyCountMetric) Evaluate(ctx context.Context, _ time.Duration) (float64, error) {
    // 实时统计当前 unhealthy checker 数量
}
```

**与原设计差异**：
- 指标通过 `SelfCheckUnhealthyCountMetric` 结构体实现 `AlertMetric` 接口
- `Evaluate` 方法实时调用各 Checker 的 `Check` 方法统计 unhealthy 数量（而非读取上次自检缓存值）
- `updateMetric` 在 `RunOnce` 中调用，但当前实现中获取 metric 后仅调用 `Evaluate`，未显式更新存储值

**替代方案**: 自检直接调用 AlertNotifier → 拒绝，耦合告警通知逻辑，且无法利用现有告警规则评估流程。

**理由**: 复用现有告警体系，自检只是指标提供者，告警规则和通知策略由用户配置。

### D6: Proto API 设计

新增 2 个 RPC（已实现在 `api/kratos/monitor/v1/monitor.proto`）：

```protobuf
// 触发自检（手动）
rpc TriggerSelfCheck(TriggerSelfCheckRequest) returns (TriggerSelfCheckResponse) {
    option (google.api.http) = { post: "/v1/monitor/self-check" body: "*" };
}

// 查询自检报告
rpc ListSelfCheckReports(ListSelfCheckReportsRequest) returns (ListSelfCheckReportsResponse) {
    option (google.api.http) = { get: "/v1/monitor/self-check-reports" };
}
```

**代码实际实现**：Proto 中还定义了以下辅助 message 类型：
- `SelfCheckResultEntry`：单条检查结果（含 check_id, checker, status, message, details_json, checked_at）
- `RepairActionEntry`：修复动作记录（含 success, action, message）
- `SelfCheckReportEntry`：聚合报告（含 id, check_results, overall_status, repair_actions, started_at, finished_at, duration_ms）
- `SelfCheckStatusCondition`：根因条件扩展（含 check_name, status, message）

### D7: 6 个内置检查器

**代码实际实现**（`internal/biz/monitor/checker_builtins.go`）：

| 检查器 | Name() | 检查内容 | 修复动作 | 与原设计差异 |
|--------|--------|----------|----------|-------------|
| `DBHealthChecker` | `db_health` | SQLite Ping + Schema 完整性（monitor_events 表） | 无自动修复（仅告警） | 名称从 `db_health_checker` 简化为 `db_health` |
| `FlowFileChecker` | `flow_file` | 写入测试（临时文件） | 清理过期压缩文件（FlowFileRepairer 已实现） | 未实现磁盘空间检查（>100MB=passed, <100MB=warning），仅做写入测试 |
| `TraceProjectorChecker` | `trace_projector` | TraceProjector 活跃 Trace 数量 | 触发 TraceBackfill（Repairer 未实现） | 检查 `TraceCount()` 而非"最近 5 分钟是否有新 Trace 投影" |
| `AlertEvalChecker` | `alert_eval` | AlertEvalWorker Ready 状态 | 重启 Worker goroutine（Repairer 未实现） | 检查 `Ready()` 而非"最近评估时间" |
| `EventBusChecker` | `eventbus` | MonitorBus 订阅者存活状态 | 重新订阅（Repairer 未实现） | 使用 `EventBusHealthChecker` 接口（SubscriberCount + IsHealthy） |
| `WebSocketChecker` | `websocket` | WS 连接数 | 无自动修复（仅告警） | 未检查"最近推送时间"，仅检查连接数 |

**注意**：6 个 Checker 全部实现在同一个文件 `checker_builtins.go` 中，而非原设计的 6 个独立文件。

### D8: SelfCheckScheduler 并发控制

**代码实际实现**（`internal/biz/monitor/self_check_scheduler.go`）：

- 使用 `sync.Mutex` + `TryLock()` 实现并发锁，防止 RunOnce 重复执行
- 每个 Checker 使用独立 context 超时（10s），整体自检超时 60s
- Checker 执行使用 `safego.Go` 防止 panic 传播
- Checker 超时或 panic 时返回 `SelfCheckResult{Status: "failed"}`

### D9: SelfCheckRepairDispatcher 冷却期

**代码实际实现**（`internal/biz/monitor/self_check_repair.go`）：

- 冷却期 300 秒（5 分钟），通过 `map[string]time.Time` 记录每个 Checker 的最后修复时间
- 修复成功后设置冷却期，修复失败不设置
- 冷却期内跳过修复，返回 `RepairOutcome{Success: false, Action: "skipped_cooldown"}`

### D10: Wire 集成（部分实现）

**代码实际实现**（`internal/biz/monitor/wire.go`）：

```go
var WireProviderSet = wire.NewSet(
    NewSelfCheckScheduler,
    NewAlertMetricRegistry,
    ProvideSelfCheckers,      // 返回空数组（占位）
    ProvideSelfCheckRepairers, // 返回空数组（占位）
)
```

**注意**：`ProvideSelfCheckers()` 和 `ProvideSelfCheckRepairers()` 当前返回空数组，需要在 Wire 集成阶段填充实际的 Checker 和 Repairer 实例。`cmd/admin/wire.go` 中尚未集成 SelfCheck 相关注入。

## Risks / Trade-offs

- **[自检修复可能引入新问题]** → 修复动作必须有幂等性保证，修复失败只记录不重试，避免级联故障
- **[自检本身影响性能]** → 每个检查器设置 10s 超时，整体自检 60s 超时；磁盘写入测试使用临时文件
- **[修复动作与运行时状态冲突]** → 修复前检查目标组件当前状态，避免重复修复；修复动作记录到 HealRecord
- **[自检报告数据量]** → 保留 30 天，CronJob 清理；聚合报告模式减少记录数
- **[SelfCheckUnhealthyCountMetric 实时调用 Checker]** → 当前 Evaluate 方法每次调用都执行所有 Checker，高频告警评估可能影响性能，后续可改为缓存上次自检结果

## Migration Plan

1. **Phase 1**: 新增 SelfCheckScheduler + 6 个 Checker + SelfCheckRepair（仅 log_only 修复）— **已完成核心实现**
2. **Phase 2**: 逐个启用修复动作（从低风险到高风险：flow_file → trace_projector → alert_eval → eventbus）— **FlowFileRepairer 已实现，其余待实现**
3. **Phase 3**: 前端状态面板 + API 集成 — **Proto 已定义，Service 层和前端待实现**

## Open Questions

- 自检修复动作是否需要人工确认机制（如 critical 级别修复需审批）？→ 初期不需要，所有修复自动执行，通过 HealRecord 可追溯
- SelfCheckUnhealthyCountMetric.Evaluate 实时调用 Checker 的性能影响？→ 需评估是否改为缓存模式
