## 1. SelfChecker 接口与核心类型

- [ ] 1.1 定义 SelfChecker 接口、SelfCheckResult、SelfCheckStatus 类型 — `internal/biz/monitor/self_check.go`。DoD: 编译通过，类型可被其他包引用
- [ ] 1.2 定义 SelfCheckReport 聚合类型（含 CheckResults、OverallStatus、RepairActions、StartedAt、FinishedAt、DurationMs）— 同文件。DoD: 编译通过
- [ ] 1.3 定义 SelfCheckRepairer 接口、RepairOutcome 类型 — 同文件。DoD: 编译通过
- [ ] 1.4 编写 SelfCheckReport.OverallStatus 聚合逻辑单元测试（healthy/degraded/unhealthy 组合场景）。DoD: 测试通过

## 2. SelfCheckScheduler 调度器

- [ ] 2.1 实现 SelfCheckScheduler struct（注册 Checkers、RunOnce 方法、并发锁）— `internal/biz/monitor/self_check_scheduler.go`。DoD: RunOnce 按序执行所有 Checker，并发锁防止重复执行
- [ ] 2.2 实现 Start/Stop 生命周期（基于 ticker 定时触发 RunOnce，使用 safego）— 同文件。DoD: Start 后按间隔执行，Stop 后停止
- [ ] 2.3 实现 SelfCheckReport 持久化调用（调用 SelfCheckReportRepo.Insert）。DoD: 每次自检完成后写入 DB
- [ ] 2.4 编写 SelfCheckScheduler 单元测试（mock Checker，验证调度、并发锁、超时取消）。DoD: 测试通过

## 3. 6 个内置 Checker

- [ ] 3.1 实现 DBHealthChecker — `internal/biz/monitor/checker_db_health.go`。检查 SQLite 连接 + Schema 完整性（查询 monitor_events 表是否存在）。DoD: 连接正常返回 healthy，连接失败返回 unhealthy
- [ ] 3.2 实现 FlowFileChecker — `internal/biz/monitor/checker_flow_file.go`。检查磁盘空间（>100MB=healthy, <100MB=degraded）、写入测试（临时文件）、文件轮转状态。DoD: 三种状态正确返回
- [ ] 3.3 实现 TraceProjectorChecker — `internal/biz/monitor/checker_trace_projector.go`。检查最近 5 分钟是否有新 Trace 投影、EventBus 订阅状态。DoD: 正常=healthy，无新投影=degraded，订阅断开=unhealthy
- [ ] 3.4 实现 AlertEvalChecker — `internal/biz/monitor/checker_alert_eval.go`。检查 AlertEvalWorker 最近评估时间（>60s=unhealthy）。DoD: 正常=healthy，停滞=unhealthy
- [ ] 3.5 实现 EventBusChecker — `internal/biz/monitor/checker_eventbus.go`。检查 MonitorBus 订阅者存活状态。DoD: 全部存活=healthy，部分缺失=degraded，全部断开=unhealthy
- [ ] 3.6 实现 WebSocketChecker — `internal/biz/monitor/checker_websocket.go`。检查 WS 连接数和最近推送时间。DoD: 有连接=healthy，无连接=degraded
- [ ] 3.7 编写 6 个 Checker 单元测试。DoD: 每个 Checker 的 healthy/degraded/unhealthy 场景覆盖

## 4. SelfCheckRepairer 修复引擎

- [ ] 4.1 实现 SelfCheckRepairDispatcher — `internal/biz/monitor/self_check_repair.go`。注册 Repairer、分发修复任务、冷却期检查（5 分钟）、修复结果记录。DoD: 修复分发正确，冷却期生效
- [ ] 4.2 实现 FlowFileRepairer（清理过期压缩文件释放磁盘空间）— 同文件。DoD: 清理成功返回 RepairOutcome{Success:true}
- [ ] 4.3 实现 TraceProjectorRepairer（触发 TraceBackfill）— 同文件。DoD: 触发 backfill 成功
- [ ] 4.4 实现 AlertEvalRepairer（重启 AlertEvalWorker goroutine）— 同文件。DoD: Worker 重启后恢复评估
- [ ] 4.5 实现 EventBusRepairer（重新订阅断开的 handler）— 同文件。DoD: 订阅恢复
- [ ] 4.6 编写 SelfCheckRepairDispatcher 单元测试（mock Repairer，验证分发、冷却期、幂等性）。DoD: 测试通过

## 5. Data 层 — SelfCheckReport 持久化

- [ ] 5.1 定义 SelfCheckReport Ent Schema — `internal/data/ent/schema/self_check_report.go`。字段: id, check_results(JSON), overall_status, repair_actions(JSON), started_at, finished_at, duration_ms。DoD: `go generate ./internal/data/ent` 成功
- [ ] 5.2 实现 SelfCheckReportRepo — `internal/data/self_check_report_repo.go`。方法: Insert, List(分页), DeleteOlderThan。DoD: 编译通过
- [ ] 5.3 新增 SelfCheckReport 清理 CronJob — `internal/cronrunner/jobs/self_check_cleanup.go`。删除 30 天前的报告。DoD: CronJob 注册到 CronRunner
- [ ] 5.4 编写 SelfCheckReportRepo 单元测试（in-memory SQLite）。DoD: 测试通过

## 6. 告警指标集成

- [ ] 6.1 注册 `monitor.selfcheck_unhealthy_count` 指标到 AlertMetricRegistry — `internal/biz/monitor/alert_metric_registry.go`。每次自检完成后更新指标值。DoD: 指标可通过 AlertMetricRegistry.GetValue 获取
- [ ] 6.2 编写指标更新单元测试。DoD: 自检后指标值正确

## 7. RootCauseEngine 增强

- [ ] 7.1 RootCauseCondition 增加 SelfCheckStatus 字段 — `internal/biz/monitor/root_cause_engine.go`。nil=不关心，非 nil=匹配自检状态。DoD: 编译通过
- [ ] 7.2 新增 `rc-self-check-failure` 内置根因规则（SelfCheckStatus=unhealthy 时匹配）。DoD: 规则注册到引擎
- [ ] 7.3 编写 RootCauseCondition.SelfCheckStatus 匹配单元测试。DoD: 测试通过

## 8. DiagBundle 增强

- [ ] 8.1 DiagBundle 增加自检快照数据 — `internal/biz/monitor/diag_bundle.go`。从 SelfCheckReportRepo 获取最近一次报告，加入 bundle。DoD: bundle 输出包含 self_check_snapshot

## 9. Proto + Service 层

- [ ] 9.1 新增 TriggerSelfCheck / ListSelfCheckReports RPC 到 monitor.proto — `api/kratos/monitor/v1/monitor.proto`。DoD: `make api` 成功
- [ ] 9.2 实现 MonitorService.TriggerSelfCheck — `internal/service/monitor.go`。调用 SelfCheckScheduler.RunOnce，返回报告。DoD: API 可调用
- [ ] 9.3 实现 MonitorService.ListSelfCheckReports — 同文件。调用 SelfCheckReportRepo.List，返回分页结果。DoD: API 可调用
- [ ] 9.4 编写 Service 层单元测试。DoD: 测试通过

## 10. Wire 集成

- [ ] 10.1 更新 Wire ProviderSet — `cmd/admin/wire.go`。注入 SelfCheckScheduler、SelfCheckRepairDispatcher、SelfCheckReportRepo、6 个 Checker、5 个 Repairer。DoD: `make wire && go build ./cmd/admin` 成功
- [ ] 10.2 新增 SelfCheckJob CronRunner 注册 — `internal/cronrunner/jobs/self_check_job.go`。DoD: CronRunner 启动时自动注册

## 11. 前端基础集成

- [ ] 11.1 新增 selfCheck API 调用 — `web/src/features/monitor/api.ts`。triggerSelfCheck()、listSelfCheckReports()。DoD: API 函数可调用
- [ ] 11.2 新增 selfCheck 类型定义 — `web/src/features/monitor/types.ts`。SelfCheckReport、SelfCheckResult、RepairAction 类型。DoD: TypeScript 编译通过
- [ ] 11.3 useMonitorStore 增加 selfCheck 状态 — `web/src/stores/monitor/index.ts`。loadSelfCheckReports、triggerSelfCheck 方法。DoD: Store 方法可调用
- [ ] 11.4 MonitorPage 新增自检状态面板 — `web/src/components/monitor/SelfCheckStatusPanel.vue`。显示最近自检状态（healthy/degraded/unhealthy）和手动触发按钮。DoD: 面板渲染正确
