## Why

Monitor 模块当前只有**被动响应**能力（告警评估、根因分析、事件驱动自愈），缺乏**主动巡检**机制。系统中的多个子系统（FlowFileAppender、TraceProjector、AlertEvalWorker、EventBus 订阅、WebSocket 连接）可能静默退化（如磁盘满导致落盘失败、Trace 投影积压、告警评估停滞），直到用户手动发现。需要一套主动自检 + 自动修复机制，在问题影响用户前发现并修复。

## What Changes

- 新增 `SelfCheckScheduler`：定时触发自检任务（默认 5 分钟间隔，可通过 `SELF_CHECK_INTERVAL` 环境变量配置），覆盖 Monitor 所有子系统健康状态
- 新增 `SelfChecker` 接口 + 6 个内置检查器：DB 连接/Schema（`db_health`）、FlowFileAppender 写入测试（`flow_file`）、TraceProjector 活跃 Trace 数（`trace_projector`）、AlertEvalWorker Ready 状态（`alert_eval`）、EventBus 订阅健康（`eventbus`）、WebSocket 连接数（`websocket`）
- 新增 `SelfCheckRepairDispatcher` + `FlowFileRepairer`：自检发现问题后自动执行修复动作（当前仅实现 FlowFileRepairer：清理过期压缩文件）
- 新增 `SelfCheckReport`：自检结果持久化到 `self_check_reports` 表 + Proto API 定义（TriggerSelfCheck / ListSelfCheckReports），Service 层实现待完成
- 自检结果集成到现有告警体系：`monitor.selfcheck_unhealthy_count` 指标通过 `SelfCheckUnhealthyCountMetric` 实现
- 自检结果集成到 DiagBundle：诊断包包含自检快照（待实现）
- 自检状态值使用 `passed/warning/failed`（而非 healthy/degraded/unhealthy）

## Capabilities

### New Capabilities
- `self-check-scheduler`: 定时自检调度器，管理自检任务的生命周期（注册、调度、超时、并发控制）
- `self-check-repair`: 自检修复引擎，根据自检结果执行自动修复动作并记录修复历史

### Modified Capabilities
- `root-cause-engine`: RootCauseCondition 增加 SelfCheckStatus 维度（自检通过/失败/修复中），根因分析可参考自检结果
- `diag-bundle`: DiagBundle 增加自检快照数据

## Impact

- **Biz 层**（internal/biz/monitor）: 新增 SelfCheckScheduler、SelfChecker 接口、6 个 Checker 实现（`checker_builtins.go`）、SelfCheckRepairDispatcher + FlowFileRepairer（`self_check_repair.go`）、SelfCheckReport 聚合类型、SelfCheckUnhealthyCountMetric
- **Types 层**（internal/biz/types）: 新增 SelfCheckResult、SelfCheckStatus、SelfCheckStatusCondition 类型
- **Data 层**（internal/data）: 新增 self_check_reports Ent Schema + SQL DDL migration + SelfCheckReportRepo
- **Service 层**（internal/service）: MonitorService 已注入 selfCheckScheduler 和 selfCheckRepo，但 TriggerSelfCheck / ListSelfCheckReports 方法未实现
- **Proto**（api/kratos/monitor/v1）: 新增 2 个 RPC + SelfCheckResultEntry / RepairActionEntry / SelfCheckReportEntry / SelfCheckStatusCondition message
- **CronRunner**（internal/cronrunner/jobs）: SelfCheckJob 未实现（SelfCheckScheduler 使用内置 ticker 替代 CronRunner）
- **前端**（web）: 自检状态面板未实现
- **Wire**（internal/biz/monitor/wire.go）: WireProviderSet 已包含 NewSelfCheckScheduler、NewAlertMetricRegistry、ProvideSelfCheckers（空数组占位）、ProvideSelfCheckRepairers（空数组占位）；cmd/admin/wire.go 未集成

## Non-goals

- 不替代现有告警体系（自检是补充，不是替代）
- 不做跨实例自检协调（当前 SQLite 单实例）
- 不做自检规则的用户自定义（内置检查器，后续可扩展）
- 不修改 trpc-agent-go 运行时（与 monitor-self-healing 变更互补，不重叠）
- 不做前端自检仪表盘的完整 UI（仅基础状态面板，详细 UI 后续变更）
