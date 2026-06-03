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

```go
type SelfChecker interface {
    Name() string
    Check(ctx context.Context) SelfCheckResult
}

type SelfCheckResult struct {
    Status    SelfCheckStatus // healthy / degraded / unhealthy
    Checker   string
    Message   string
    Details   map[string]any
    CheckedAt time.Time
}

type SelfCheckStatus string
const (
    SelfCheckHealthy   SelfCheckStatus = "healthy"
    SelfCheckDegraded  SelfCheckStatus = "degraded"
    SelfCheckUnhealthy SelfCheckStatus = "unhealthy"
)
```

**替代方案**: 使用统一函数 + switch case → 拒绝，违反 OCP，新增检查器需修改调度器代码。

**理由**: 接口模式符合 OCP，新增检查器只需实现接口并注册，不修改调度器。

### D2: 修复动作与检查器解耦

**选择**: 修复动作独立于检查器，通过 `SelfCheckRepairer` 接口实现。调度器检查 → 发现问题 → 调用对应 Repairer。

```go
type SelfCheckRepairer interface {
    CanRepair(checkName string, status SelfCheckStatus) bool
    Repair(ctx context.Context, result SelfCheckResult) RepairOutcome
}

type RepairOutcome struct {
    Success  bool
    Action   string // e.g. "restarted_worker", "reconnected_subscription"
    Message  string
}
```

**替代方案**: 修复逻辑内嵌在 Checker 中 → 拒绝，违反 SRP，检查和修复是不同关注点。

**理由**: 检查和修复分离，同一个检查结果可以有多种修复策略，修复策略可独立演进。

### D3: 自检报告持久化

**选择**: 新增 `self_check_reports` Ent Schema，每次自检周期写入一条聚合报告。

| 字段 | 类型 | 说明 |
|------|------|------|
| id | string | UUID |
| check_results | JSON | 各检查器的结果数组 |
| overall_status | string | healthy / degraded / unhealthy（取最差） |
| repair_actions | JSON | 执行的修复动作数组 |
| started_at | time.Time | 自检开始时间 |
| finished_at | time.Time | 自检结束时间 |
| duration_ms | int64 | 自检耗时 |

**替代方案**: 每个检查器一条记录 → 拒绝，数据量大，查询聚合复杂。

**理由**: 聚合报告更符合运维查看习惯，一条记录看全貌。保留 30 天，CronJob 清理。

### D4: 自检触发方式

**选择**: 双模式触发 — 定时自动 + 手动 API 触发。

- 定时：CronRunner Job，默认 5 分钟间隔，可通过配置调整
- 手动：`POST /v1/monitor/self-check` API，立即触发一次自检

**替代方案**: 仅定时 → 拒绝，运维需要按需触发（如部署后验证）。

**理由**: 双模式兼顾自动化和运维灵活性。手动触发有并发锁防止重复执行。

### D5: 自检失败告警集成

**选择**: 自检结果通过现有 `AlertMetricRegistry` 注册新指标 `monitor.selfcheck_unhealthy_count`，告警规则可基于此指标触发。

**替代方案**: 自检直接调用 AlertNotifier → 拒绝，耦合告警通知逻辑，且无法利用现有告警规则评估流程。

**理由**: 复用现有告警体系，自检只是指标提供者，告警规则和通知策略由用户配置。

### D6: Proto API 设计

新增 2 个 RPC：

```protobuf
// 触发自检（手动）
rpc TriggerSelfCheck(TriggerSelfCheckRequest) returns (TriggerSelfCheckResponse) {
    option (google.api.http) = { post: "/v1/monitor/self-check" };
}

// 查询自检报告
rpc ListSelfCheckReports(ListSelfCheckReportsRequest) returns (ListSelfCheckReportsResponse) {
    option (google.api.http) = { get: "/v1/monitor/self-check-reports" };
}
```

### D7: 6 个内置检查器

| 检查器 | 检查内容 | 修复动作 |
|--------|----------|----------|
| `db_health_checker` | SQLite 连接 + Schema 完整性 | 无自动修复（仅告警） |
| `flow_file_checker` | 磁盘空间 + 写入测试 + 文件轮转状态 | 清理过期压缩文件 |
| `trace_projector_checker` | 最近 5 分钟是否有新 Trace 投影 + 积压检测 | 触发 TraceBackfill |
| `alert_eval_checker` | AlertEvalWorker 最近评估时间 + 评估耗时 | 重启 Worker goroutine |
| `eventbus_checker` | 订阅者存活检查 + 事件积压 | 重新订阅 |
| `websocket_checker` | WS 连接数 + 最近推送时间 | 无自动修复（仅告警） |

## Risks / Trade-offs

- **[自检修复可能引入新问题]** → 修复动作必须有幂等性保证，修复失败只记录不重试，避免级联故障
- **[自检本身影响性能]** → 每个检查器设置 10s 超时，整体自检 60s 超时；磁盘写入测试使用临时文件
- **[修复动作与运行时状态冲突]** → 修复前检查目标组件当前状态，避免重复修复；修复动作记录到 HealRecord
- **[自检报告数据量]** → 保留 30 天，CronJob 清理；聚合报告模式减少记录数

## Migration Plan

1. **Phase 1**: 新增 SelfCheckScheduler + 6 个 Checker + SelfCheckRepair（仅 log_only 修复）
2. **Phase 2**: 逐个启用修复动作（从低风险到高风险：flow_file → trace_projector → alert_eval → eventbus）
3. **Phase 3**: 前端状态面板 + API 集成

## Open Questions

- 自检修复动作是否需要人工确认机制（如 critical 级别修复需审批）？→ 初期不需要，所有修复自动执行，通过 HealRecord 可追溯
