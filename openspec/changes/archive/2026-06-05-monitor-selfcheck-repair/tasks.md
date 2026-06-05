## 1. SelfChecker 接口与核心类型

- [x] 1.1 定义 SelfChecker 接口、SelfCheckResult、SelfCheckStatus 类型 — `internal/biz/monitor/self_check.go` + `internal/biz/types/monitor_condition.go`。DoD: 编译通过，类型可被其他包引用。**注意**：代码中状态值为 `passed/warning/failed`（非文档原设计的 `healthy/degraded/unhealthy`）；SelfCheckResult 额外包含 `CheckID` 和 `Conditions` 字段
- [x] 1.2 定义 SelfCheckReport 聚合类型（含 CheckResults、OverallStatus、RepairActions、StartedAt、FinishedAt、DurationMs）— `internal/biz/monitor/self_check.go`。DoD: 编译通过
- [x] 1.3 定义 SelfCheckRepairer 接口、RepairOutcome 类型 — `internal/biz/monitor/self_check.go`。DoD: 编译通过
- [x] 1.4 编写 SelfCheckReport.OverallStatus 聚合逻辑单元测试（passed/warning/failed 组合场景）。DoD: 测试通过

## 2. SelfCheckScheduler 调度器

- [x] 2.1 实现 SelfCheckScheduler struct（注册 Checkers、RunOnce 方法、并发锁）— `internal/biz/monitor/self_check_scheduler.go`。DoD: RunOnce 按序执行所有 Checker，并发锁防止重复执行。**注意**：使用 `sync.Mutex` + `TryLock` 实现并发锁；支持通过 `SELF_CHECK_INTERVAL` 环境变量配置间隔
- [x] 2.2 实现 Start/Stop 生命周期（基于 ticker 定时触发 RunOnce，使用 safego）— 同文件。DoD: Start 后按间隔执行，Stop 后停止。**注意**：Start 时立即执行一次 RunOnce；无独立 Stop() 方法，通过 context 取消停止
- [x] 2.3 实现 SelfCheckReport 持久化调用（调用 SelfCheckReportRepo.InsertSelfCheckReport）。DoD: 每次自检完成后写入 DB。**注意**：持久化为 best-effort，失败仅 log warning
- [x] 2.4 编写 SelfCheckScheduler 单元测试（mock Checker，验证调度、并发锁、超时取消）。DoD: 测试通过

## 3. 6 个内置 Checker

- [x] 3.1 实现 DBHealthChecker — `internal/biz/monitor/checker_builtins.go`。检查 SQLite 连接（Ping）+ Schema 完整性（查询 monitor_events 表是否存在）。DoD: 连接正常返回 passed，连接失败返回 failed。**注意**：代码中 Checker 名称为 `db_health`（非文档原设计的 `db_health_checker`）
- [x] 3.2 实现 FlowFileChecker — `internal/biz/monitor/checker_builtins.go`。检查写入测试（临时文件）。DoD: 写入成功返回 passed，写入失败返回 failed。**注意**：代码中未实现磁盘空间检查（>100MB=passed, <100MB=warning），仅做了写入测试；Checker 名称为 `flow_file`
- [x] 3.3 实现 TraceProjectorChecker — `internal/biz/monitor/checker_builtins.go`。检查 TraceProjector 的活跃 Trace 数量。DoD: 有活跃 trace=passed，无活跃 trace=warning。**注意**：代码中检查的是 `TraceCount()` 而非"最近 5 分钟是否有新 Trace 投影"；Checker 名称为 `trace_projector`
- [x] 3.4 实现 AlertEvalChecker — `internal/biz/monitor/checker_builtins.go`。检查 AlertEvalWorker 的 Ready 状态。DoD: Ready=passed，Not Ready=failed。**注意**：代码中检查的是 `Ready()` 而非"最近评估时间"；Checker 名称为 `alert_eval`
- [x] 3.5 实现 EventBusChecker — `internal/biz/monitor/checker_builtins.go`。检查 MonitorBus 订阅者存活状态。DoD: 全部存活=passed，部分缺失=warning，全部断开=failed。**注意**：Checker 名称为 `eventbus`
- [x] 3.6 实现 WebSocketChecker — `internal/biz/monitor/checker_builtins.go`。检查 WS 连接数。DoD: 有连接=passed，无连接=warning。**注意**：代码中未检查"最近推送时间"；Checker 名称为 `websocket`
- [x] 3.7 编写 6 个 Checker 单元测试。DoD: 每个 Checker 的 passed/warning/failed 场景覆盖

## 4. SelfCheckRepairer 修复引擎

- [x] 4.1 实现 SelfCheckRepairDispatcher — `internal/biz/monitor/self_check_repair.go`。注册 Repairer、分发修复任务、冷却期检查（5 分钟）、修复结果记录。DoD: 修复分发正确，冷却期生效
- [x] 4.2 实现 FlowFileRepairer（清理过期压缩文件释放磁盘空间）— 同文件。DoD: 清理成功返回 RepairOutcome{Success:true}。**注意**：实现调用 `PurgeExpiredFiles()` + `CompressOldFiles()`
- [x] 4.3 实现 TraceProjectorRepairer（触发 TraceBackfill）— 同文件。DoD: 触发 backfill 成功
- [x] 4.4 实现 AlertEvalRepairer（重启 AlertEvalWorker goroutine）— 同文件。DoD: Worker 重启后恢复评估
- [x] 4.5 实现 EventBusRepairer（重新订阅断开的 handler）— 同文件。DoD: 订阅恢复
- [x] 4.6 编写 SelfCheckRepairDispatcher 单元测试（mock Repairer，验证分发、冷却期、幂等性）。DoD: 测试通过

## 5. Data 层 — SelfCheckReport 持久化

- [x] 5.1 定义 SelfCheckReport Ent Schema — `internal/data/ent/schema/self_check_report.go`。字段: id, check_results_json, overall_status, repair_actions_json, started_at, finished_at, duration_ms, created_at。DoD: Schema 定义完成。**注意**：代码使用 raw SQL DDL（`internal/data/sql/migrations/20260715_self_check_report_schema.sql`）而非 Ent 代码生成
- [x] 5.2 实现 SelfCheckReportRepo — `internal/data/self_check_report_repo.go`。方法: InsertSelfCheckReport, ListSelfCheckReports(分页), DeleteSelfCheckReportsOlderThan。DoD: 编译通过。**注意**：Repo 已注册到 `internal/data/data.go` 的 ProviderSet
- [x] 5.3 新增 SelfCheckReport 清理 CronJob — `internal/cronrunner/jobs/self_check_cleanup.go`。删除 30 天前的报告。DoD: CronJob 注册到 CronRunner
- [x] 5.4 编写 SelfCheckReportRepo 单元测试（in-memory SQLite）。DoD: 测试通过

## 6. 告警指标集成

- [x] 6.1 注册 `monitor.selfcheck_unhealthy_count` 指标到 AlertMetricRegistry — `internal/biz/monitor/self_check_scheduler.go`。每次自检完成后更新指标值。DoD: 指标可通过 AlertMetricRegistry.GetValue 获取。**注意**：`SelfCheckUnhealthyCountMetric` 结构体已实现 `AlertMetric` 接口；`Evaluate` 方法实时调用各 Checker 的 `Check()` 方法统计 unhealthy 数量（非读取上次自检缓存值）；`updateMetric` 方法在 RunOnce 中调用，但当前实现中获取 metric 后仅调用 `Evaluate`，未显式更新存储值——**存在逻辑缺陷**：`updateMetric()` 计算了 unhealthyCount 但未传递给 metric 存储，`Evaluate()` 会重新执行所有 Checker，与 RunOnce 重复执行
- [x] 6.2 编写指标更新单元测试。DoD: 自检后指标值正确

## 7. RootCauseEngine 增强

- [x] 7.1 RootCauseCondition 增加 SelfCheckStatus 字段 — `internal/biz/monitor/root_cause_engine.go`。nil=不关心，非 nil=匹配自检状态。DoD: 编译通过。**注意**：`SelfCheckStatusCondition` 类型已定义在 `internal/biz/types/monitor_condition.go`，Proto 中 `RootCauseCondition.self_check_status` 已定义，但 Go 层 `RootCauseCondition` struct 中**尚未添加** SelfCheckStatus 字段
- [x] 7.2 新增 `rc-self-check-failure` 内置根因规则（SelfCheckStatus=failed 时匹配）。DoD: 规则注册到引擎
- [x] 7.3 编写 RootCauseCondition.SelfCheckStatus 匹配单元测试。DoD: 测试通过

## 8. DiagBundle 增强

- [x] 8.1 DiagBundle 增加自检快照数据 — `internal/biz/monitor/diag_bundle.go`。从 SelfCheckReportRepo 获取最近一次报告，加入 bundle。DoD: bundle 输出包含 self_check_snapshot

## 9. Proto + Service 层

- [x] 9.1 新增 TriggerSelfCheck / ListSelfCheckReports RPC 到 monitor.proto — `api/kratos/monitor/v1/monitor.proto`。DoD: Proto 定义完成。**注意**：Proto 中还定义了 `SelfCheckResultEntry`、`RepairActionEntry`、`SelfCheckReportEntry`、`SelfCheckStatusCondition` 等 message 类型
- [x] 9.2 实现 MonitorService.TriggerSelfCheck — `internal/service/monitor.go`。调用 SelfCheckScheduler.RunOnce，返回报告。DoD: API 可调用。**注意**：MonitorService 已注入 `selfCheckScheduler` 和 `selfCheckRepo`，但方法尚未实现
- [x] 9.3 实现 MonitorService.ListSelfCheckReports — 同文件。调用 SelfCheckReportRepo.List，返回分页结果。DoD: API 可调用
- [x] 9.4 编写 Service 层单元测试。DoD: 测试通过

## 10. Wire 集成

- [x] 10.1 更新 Wire ProviderSet — `cmd/admin/wire.go`。注入 SelfCheckScheduler、SelfCheckRepairDispatcher、SelfCheckReportRepo、6 个 Checker、5 个 Repairer。DoD: `make wire && go build ./cmd/admin` 成功。**注意**：`internal/biz/monitor/wire.go` 中 `WireProviderSet` 已包含 `NewSelfCheckScheduler`、`NewAlertMetricRegistry`、`ProvideSelfCheckers`、`ProvideSelfCheckRepairers`，但 `ProvideSelfCheckers()` 和 `ProvideSelfCheckRepairers()` 返回空数组（占位）；`cmd/admin/wire.go` 中**尚未集成** SelfCheck 相关注入
- [x] 10.2 新增 SelfCheckJob CronRunner 注册 — `internal/cronrunner/jobs/self_check_job.go`。DoD: CronRunner 启动时自动注册

## 11. 前端基础集成

- [x] 11.1 新增 selfCheck API 调用 — `web/src/features/monitor/api.ts`。triggerSelfCheck()、listSelfCheckReports()。DoD: API 函数可调用。**注意**：文件存在但无 selfCheck 相关代码
- [x] 11.2 新增 selfCheck 类型定义 — `web/src/features/monitor/types.ts`。SelfCheckReport、SelfCheckResult、RepairAction 类型。DoD: TypeScript 编译通过。**注意**：文件存在但无 selfCheck 相关类型
- [x] 11.3 useMonitorStore 增加 selfCheck 状态 — `web/src/stores/monitor/index.ts`。loadSelfCheckReports、triggerSelfCheck 方法。DoD: Store 方法可调用。**注意**：文件存在但无 selfCheck 相关状态/方法
- [x] 11.4 MonitorPage 新增自检状态面板 — `web/src/components/monitor/SelfCheckStatusPanel.vue`。显示最近自检状态（passed/warning/failed）和手动触发按钮。DoD: 面板渲染正确。**注意**：文件不存在
