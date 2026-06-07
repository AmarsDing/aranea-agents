# Monitor 监控 — 开发计划

> **版本**：2026-06-06-v4 | **状态**：🟢 核心已通 + **MON-OPT-01~06 ✅ LOG-01/TRACE-01 ✅ DIAG-01/02 ✅ Latency P50/P95/P99 ✅ LOG-03 P0/P1/P2 ✅ REDLINE ✅ QUALITY ✅ 自检/自愈 ✅**；待办为 LOG-02（跨 pkg）、LOOP-01 FR-02/FR-03（P3）
> **需求**：[18 monitor.md](./18%20monitor.md) · **设计**：[18 monitor.design.md](./18%20monitor.design.md)（§九 方案 C）
> **进度真相**：[execution-plan.md](../guides/execution-plan.md)（I8-MON-01/02、MON-01、I5-MON-01/02）· **页面索引**：[frontend-pages.md](./frontend-pages.md) §监控

---

## 1. 模块定位

系统监控：审计、实时事件、模型用量、单次运行排障（Runs）、流程/进程日志、Runner 窗口指标与告警。

**代码锚点**：

| 层 | 路径 |
|----|------|
| Proto | `api/kratos/monitor/v1/monitor.proto` |
| Biz | `internal/biz/monitor.go`、`internal/biz/monitor/`（alert_eval_worker、metric_ring_buffer、trace_projector、flow_file_appender、alert_metric_registry、root_cause_engine、diag_bundle、self_check、self_heal、self_check_repair、self_check_scheduler、predictive_heal、pattern_mining、failure_pattern_repo、failure_report）、`runner_completion.go` |
| Data | `internal/data/monitor.go`、`internal/data/monitor_alert.go`、`internal/data/monitor_trace.go` |
| Service | `internal/service/monitor.go`、`monitor_notify.go`、`monitor_flow_log.go` |
| Cron | `internal/cronrunner/jobs/monitor_trace_backfill.go` |
| 用量 | `internal/biz/usage.go`、`internal/service/turn_usage.go`、`chat_usage_ingress.go` |
| FlowLog | `internal/biz/flowlog/`、`internal/data/flow_log_repo.go` |
| 前端 | `web/src/pages/MonitorPage.vue`、`web/src/features/monitor/*`、`web/src/components/monitor/*` |
| SQL | `docs/sql/07_monitor.sql`、`docs/sql/14_monitor_alert.sql` |
| FlowLogger | [52-flow-logger-development.md](./52-flow-logger-development.md) — Logs 流程 Tab 与 Runs 详情 Flow |

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Monitor 页面 6 Tab | ✅ | `MonitorPage.vue`：Usage / Alerts / Audit / Events / Traces / Logs |
| Token 用量记录 | ✅ | `turn_usage.go` / `chat_usage_ingress.go` → `UsageUsecase` |
| 用量查询 API | ✅ | `UsageService`：`/v1/usage/overview`、`/events` 等 |
| Runner 窗口指标 | ✅ | `GET /v1/monitor/runner-metrics` + `MonitorRunnerMetrics` / `RunnerMetricsPanel`（Store + composable） |
| 错误率统计（窗口） | ✅ | `GetRunnerMetrics` 聚合 `runner.completion` |
| 全局 Latency 聚合 | ✅ | `LatencyPercentilesSince` + `meta_duration_ms` generated column；P50/P95/P99 |
| 告警规则 | ✅ | `monitor_alert_rules` + `GET/PUT /v1/monitor/alert-rules` |
| 告警评估 | ✅ | `EvaluateAlerts`：`runner.error_rate` → `alert.fired`；MON-OPT-03 RingBuffer + EvalWorker |
| 告警冷却持久化 | ✅ | MON-OPT-02：`firing` 状态机 + DB 持久化 `last_fired_at` |
| 告警注册表 | ✅ | MON-OPT-06：`AlertMetricRegistry` 替代 switch/case |
| 告警通知 | ✅ | `monitor_notify.go`：Webhook + Channel；`cooldown_minutes` |
| FlowLog Bus 分离 | ✅ | MON-OPT-01：`flow_log`/`log` 路由到 MonitorBus；WS 全局连接单 pump |
| Trace 写入回路 | ✅ | MON-OPT-05：`TraceProjector` + `MonitorTraceProjector` + 历史回填 |
| FlowLog 文件落盘 | ✅ | LOG-01：`FlowFileAppender` + 按日/大小轮转 + gzip + 30 天清理 |
| Trace 文件落盘 | ✅ | TRACE-01：`runner.completion` → `trace-*.jsonl` |
| AI 诊断包 | ✅ | DIAG-01：`DiagBundleGenerator` + `GenerateDiagnosticBundle` API |
| 根因分析引擎 | ✅ | DIAG-02：`RootCauseEngine` 5 条内置规则 + 置信度评分 |
| WS 反压可观测 | ✅ | MON-OPT-04：优先级队列 + drop 计数 + 反压事件 |
| Events / Runs 分工（方案 C） | ✅ | `runCorrelation.ts`、`RealtimeEvents` 过滤、Runs「打开会话」 |
| Logs 流程/进程拆分 | ✅ | `useLogStreamHub`、`LogStreamPanel`、[changelog](../changelog/2026-05-20-Monitor-Logs-Split.md) |
| 监控 Dashboard（`/overview`） | ✅ | Phase 0～3b 完成；见 [18-monitor-dashboard-development.md](./18-monitor-dashboard-development.md) |
| ListFlowLogs HTTP 历史 | ✅ | `FlowLogService.ListFlowLogs` + `biz.FlowLogUsecase` + Ent Repo |
| LOG-03 P0 红线修复 | ✅ | 9 处 `log.Warnf`/`log.Errorf` → `event.SysLogWarn`/`SysLogError`（Graph/Task/Channel 域） |
| LOG-03 P1 关键路径补全 | ✅ | Graph runtime、Session title/rollback、Knowledge embedder FlowLog 补全 |
| LOG-03 P2 biz 层 fmt.Errorf 清理 | ✅ | session_run.go 13 处 + shared.go 1 处 + agent_settings_helpers.go 1 处 → `kerrors` |
| LOG-03 P2 admin.go log.Infof 修复 | ✅ | → `event.SysLogInfo`（红线 #16） |
| step_id 注册表扩展 | ✅ | 新增 15 个 step_id（graph/session/task/channel/knowledge 域） |
| UsecaseOption 构造器注入 | ✅ | `UsecaseOption` 函数选项模式替代 4 个 `Set*`（保留 2 个循环依赖 setter） |
| RebuildRingBuffer 逐分钟重建 | ✅ | `ensureBucketAt` + 60 桶逐分钟从 DB 重建 |
| 自检 SelfCheck | ✅ | `SelfCheckScheduler`（5 min）+ `SelfCheckRepairDispatcher`（4 个修复器）+ `SelfChecker` 插件接口 |
| 自愈 SelfHeal | ✅ | `SelfHealObserver`（事件驱动修复）+ cooldown + 置信度阈值 |
| 预测性自愈 PredictiveHeal | ✅ | `PredictiveHealUsecase`（系统指标 + 故障模式匹配 + 预防性修复） |
| 模式挖掘 PatternMining | ✅ | `PatternMiningUsecase`（故障聚类 + 自动修复模板 + 置信度晋升/停用） |
| 故障报告 FailureReport | ✅ | `FailureReport` 统一 CI/runtime 错误格式 + `FailureReportParser` 正则识别 |
| LOOP-01 FR-01 | ✅ | `log.Printf` 红线违规已清零（evolution.go + modelcatalog 已移除） |
| LOOP-01 FR-02 | 🟡 | cronrunner Kratos `log.Helper` 从 29 处降至 7 处（5 个文件残留） |
| LOOP-01 FR-03 | ❌ | stepTitleRegistry 22 个 step_id 未注册 |

---

## 3. 差距与优化（P2+）

1. **P2 — UI 命名**：路由 Tab `traces` → `runs` 别名；Events 服务端 `hide_linked_completions`（减轻前端过滤）。
2. **P2 — LOG-02**：框架层 zap 日志结构化（JSON Encoder）— 跨 `pkg/trpc-agent-go` 修改，需独立 PR。
3. **P3 — LOOP-01 FR-02**：清理 cronrunner 剩余 7 处 Kratos `log.Helper`（5 个文件：monitor_alert_cooldown、memory_dead_letter_replayer、provider_health、channel_health、evolution_scanner、channel_delivery）。
4. **P3 — LOOP-01 FR-03**：补全 stepTitleRegistry 22 个缺失 step_id 注册。

---

## 4. 开发阶段（已完成 vs 待办）

| 阶段 | 内容 | 状态 |
|------|------|------|
| Phase 1c | Logs 流程/进程 + LogStreamHub + `process_log_enabled` | ✅ |
| Phase 1d | 方案 C：Runs 真相源 + Events 收窄 + completion correlation | ✅ [changelog](../changelog/2026-05-20-Monitor-Phase1d-PlanC.md) |
| MON-01 / I4 | 告警规则 + `alert.fired` + 出站通知 | ✅ |
| I5-MON-01/02 | Runner 指标 + Alerts Channel 下拉 | ✅ |
| I6-TEL-02 | Trace 瀑布图 + usage spans | ✅ |
| Phase 2 | Dashboard 增强（`/overview`） | ✅ 见 [18-monitor-dashboard-development.md](./18-monitor-dashboard-development.md) |
| Phase 3 | 告警（已提前完成，见上） | ✅ |
| Phase 1（原） | 全局 latency / error_rate 聚合 API | ✅ P50/P95/P99 + `meta_duration_ms` generated column |

### Phase 1c — Logs 流程/进程拆分

| ID | 任务 | 优先级 | 状态 |
|----|------|--------|------|
| MON-1c-01 | `useLogStreamHub` 共享 WS + `connected` 状态 | P1 | ✅ |
| MON-1c-02 | `LogStreamPanel` + `FlowLogStream` + `ProcessLogStream` | P1 | ✅ |
| MON-1c-03 | `ws.go`：`enable_log(false)` 保留 global monitor channel | P0 | ✅ |
| MON-1c-04 | 删除重复 `EnvelopeTypeLog` 发射点 | P1 | ✅ |
| MON-1c-05 | 文档 + changelog + execution-plan 同步 | P1 | ✅ |
| MON-1c-06 | `server.monitor.process_log_enabled` + 进程 Tab UI 简化 | P1 | ✅ |

### Phase 1d — 方案 C：Runs 真相源 + Events 收窄 + completion 关联

| ID | 任务 | 层 | 优先级 | 状态 |
|----|------|-----|--------|------|
| MON-1d-01 | `DomainEvent`/Handler correlation 字段 | biz | P0 | ✅ |
| MON-1d-02 | `monitorRunnerCompletionMeta` v1 | biz | P0 | ✅ |
| MON-1d-03 | `recordTurnUsage` 与 completion 关联 | service | P0 | ✅ |
| MON-1d-04 | 落库幂等 `(event_key, session_id, invocation_id)` | data | P1 | ✅ |
| MON-1d-05 | `runCorrelation.ts` | web | P0 | ✅ |
| MON-1d-06 | `RealtimeEvents` 过滤 + 降级卡片 | web | P0 | ✅ |
| MON-1d-07 | `TraceList`「打开会话」+ Runs 语义 | web | P0 | ✅ |
| MON-1d-08 | `RunnerMetricsPanel` 下钻 `?tab=traces` | web | P1 | ✅ |
| MON-1d-09 | `MonitorPage` / `useMonitorRunNavigation` 深链 | web | P1 | ✅ |
| MON-1d-10 | changelog + 文档同步 | docs | P1 | ✅ |

### MON-OPT — 业务逻辑优化（2026-05-26 方案落地）

> 方案详见 [18-monitor-optimization-2026-05-26.md](./18-monitor-optimization-2026-05-26.md)

| ID | 任务 | 优先级 | 状态 | 关键实现 |
|----|------|--------|------|----------|
| MON-OPT-01 | FlowLog 流彻底分离到 MonitorBus | P1 | ✅ | `event.Infra` 路由表 split 模式；WS 全局连接单 pump |
| MON-OPT-02 | 告警冷却持久化 + firing 状态机 | P1 | ✅ | `UpdateAlertFiringState` + `MarkAlertFiredPersistent` + `MarkAlertRecovered` |
| MON-OPT-03 | 告警评估批量化 + 滑动窗口 | P1 | ✅ | `MetricRingBuffer` + `AlertEvalWorker`（30s ticker）+ singleflight |
| MON-OPT-04 | WS 反压可观测 + 优先级队列 | P1 | ✅ | 优先级 channel + drop 计数 + 反压事件 |
| MON-OPT-05 | Trace 写入回路 + 历史回填 | P1 | ✅ | `TraceProjector` + `EnsureTraceSchema` + `MonitorTraceBackfillWorker` |
| MON-OPT-06 | 告警规则注册表 | P2 | ✅ | `AlertMetricRegistry` + `RunnerErrorRateMetric` + `SkillFilesystemMissingMetric` |

### AI 闭环追踪（2026-05-28 方案落地）

> 方案详见 [18-monitor-ai-closed-loop-2026-05-28.md](./18-monitor-ai-closed-loop-2026-05-28.md)

| ID | 任务 | 优先级 | 状态 | 关键实现 |
|----|------|--------|------|----------|
| LOG-01 | FlowLog 文件落盘 | P1 | ✅ | `FlowFileAppender` + 按日/大小轮转 + gzip + 30 天清理 |
| LOG-02 | 框架层 zap 日志结构化 | P2 | ❌ | 跨 `pkg/trpc-agent-go` 修改，需独立 PR |
| LOG-03 | 关键路径 FlowLog 补全 | P2 | ✅ P0/P1/P2 完成 | P0 红线修复 9 处；P1 补全 Graph/Session/Knowledge；P2 biz 层 fmt.Errorf 全量清理 + admin.go log.Infof |
| TRACE-01 | Trace 文件落盘 | P1 | ✅ | `runner.completion` → `trace-*.jsonl` |
| DIAG-01 | AI 诊断包 | P1 | ✅ | `DiagBundleGenerator` + `GenerateDiagnosticBundle` RPC |
| DIAG-02 | 根因分析规则引擎 | P1 | ✅ | `RootCauseEngine` 5 条内置规则 + 置信度评分 |
| LOOP-01 | 系统调试日志闭环 | P2 | 🟡 FR-01 ✅ FR-02 🟡 FR-03 ❌ | FR-01 已完成；FR-02 剩余 7 处 cronrunner `log.Helper`；FR-03 22 个 step_id 未注册 |

### Latency 聚合（2026-05-28 新增）

| ID | 任务 | 优先级 | 状态 | 关键实现 |
|----|------|--------|------|----------|
| LATENCY-01 | 全局 P50/P95/P99 聚合 API | P2 | ✅ | `LatencyPercentilesSince` + `percentileIndex` + `meta_duration_ms` generated column |
| LATENCY-02 | `RunnerMetricsSummary` 扩展 | P2 | ✅ | Proto 新增 `p50_duration_ms`/`p95_duration_ms`/`p99_duration_ms` |

### 红线修复（2026-05-28 新增）

| ID | 问题 | 状态 | 修复 |
|----|------|------|------|
| REDLINE-01 | Graph 域 3 处 `log.Warnf`（红线 #16） | ✅ | → `event.SysLogWarn`（graph_task_start_fail/task_status_fail/task_resume_fail） |
| REDLINE-02 | Task 域 5 处 `log.Warnf`/`log.Errorf`（红线 #16） | ✅ | → `event.SysLogWarn`/`SysLogError`（task.timeout_update_fail/release_claim_fail/dispatcher_tick_fail/check_timeout_fail/claim_fail/dispatch_run_fail） |
| REDLINE-03 | Channel 域 1 处 `log.Warnf`（红线 #16） | ✅ | → `event.SysLogWarn`（channel.dead_letter） |
| REDLINE-04 | Admin 域 1 处 `log.Infof`（红线 #16） | ✅ | → `event.SysLogInfo`（system.admin.logout） |
| REDLINE-05 | biz 层 15 处 `fmt.Errorf`（红线 #14） | ✅ | session_run.go 13 处 → `kerrors.InternalServer`/`BadRequest`；shared.go 1 处 → `errors.BadRequest`；agent_settings_helpers.go 1 处 → `kerrors.InternalServer` |

### 质量修复

| ID | 问题 | 状态 | 修复 |
|----|------|------|------|
| MON-Q-09 | 空库返回未持久化的合成默认规则 | ✅ | `ListMonitorAlertRules` 空库时自动 `ReplaceAlertRules` |
| MON-Q-11 | `json_extract` 过滤无法走索引 | ✅ | generated columns（`meta_session_id`/`meta_duration_ms` 等）+ `COALESCE` 查询 |
| MON-Q-12 | `FlowFileAppender.writeRow` 无锁保护（P0 data race） | ✅ | `writeRow` → `writeRowLocked`，统一在 `mu.Lock` 范围内 |
| MON-Q-13 | `AlertEvalWorker.ready` bool 跨 goroutine 读写（P0 data race） | ✅ | `ready bool` → `ready atomic.Bool` |
| MON-Q-14 | `TraceProjector.dropCount` 非 atomic（P0 data race） | ✅ | `dropCount int64` → `dropCount atomic.Int64` |
| MON-Q-15 | `DiagBundleGenerator` fmt.Errorf（红线 #14）+ usageData 长度 bug | ✅ | → `kerrors.InternalServer`；`len(usageData)` → `len(usageRows)` |
| MON-Q-16 | `service/monitor.go` 3 处 fmt.Errorf + ListMonitorAlertRules 吞错 | ✅ | → `kerrors.BadRequest`；添加 `event.SysLogWarn` 错误记录 |
| MON-Q-17 | `ReplaceAlertRules` DELETE ALL 丢失 firing_state | ✅ | 增量 upsert：已存在→UPDATE 保留状态，新增→INSERT，删除→DELETE BY ID |
| MON-Q-18 | `LatencyPercentilesSince` 无 LIMIT 可能全量加载 | ✅ | SQL 增加 `LIMIT 10000` |
| MON-Q-19 | `UsecaseOption` 构造器注入重构 | ✅ | 函数选项模式替代 4 个 `Set*`（保留 `SetEvalWorker`/`SetRegistry` 循环依赖） |
| MON-Q-20 | `RebuildRingBuffer` 只填当前桶 | ✅ | `ensureBucketAt` 逐分钟从 DB 重建 60 个桶 |
| MON-Q-21 | `monitor_trace_backfill` watermark 用 `time.Now()` | ✅ | 使用最后一行的实际 `created_at` 作为 watermark |
| MON-Q-22 | `RootCauseEngine` 正则编译错误无日志 | ✅ | `regexp.Compile` 失败时添加 `event.SysLogError` |
| MON-Q-23 | `TraceProjector` hasPrefix/hasSuffix 包装函数冗余 | ✅ | 删除包装，直接使用 `strings.HasPrefix`/`strings.HasSuffix` |
| MON-Q-24 | `RebuildRingBuffer` 累积计数 bug（Critical） | ✅ | `CountMonitorEventsSince` 增加 `untilRFC3339` 参数，SQL 加 `AND created_at < ?` 上界 |
| MON-Q-25 | `AlertEvalWorker.rebuildFromDB` 无错误处理 | ✅ | `RebuildRingBuffer` 返回重建桶数，0 桶时记录 `SysLogWarn` |
| MON-Q-26 | `DiagBundleGenerator` 脆弱 key 匹配 | ✅ | `strings.Contains` → `strings.HasPrefix`（`alert.`/`usage`） |
| MON-Q-27 | `FlowFileAppender` 写入后无定时 Sync | ✅ | `maintenance` 周期添加 `syncOpenFiles`，每小时 Sync 所有打开文件 |

**验证命令**：

```bash
make build && make test
cd web && pnpm lint && pnpm test && pnpm build
```

**手工**：Chat 一轮 → Monitor **Traces** 见 Runs 行 → 详情 Flow/Waterfall → Events 无重复 completion → Usage Runner 指标下钻 → Alerts 保存规则。

---

## 5. 任务清单（ backlog ）

| # | 任务 | 优先级 | 状态 |
|---|------|--------|------|
| 1 | 全局 latency 聚合 API | P2 | ✅ P50/P95/P99 |
| 2 | Dashboard 图表/占比/Runner（`/overview`） | P2 | ✅ [18-monitor-dashboard-development.md](./18-monitor-dashboard-development.md) |
| 3 | 告警规则引擎 | P2 | ✅ |
| 4 | 通知渠道 Webhook/Channel | P3 | ✅ |
| 5 | 方案 C Runs/Events + correlation | P1 | ✅ |
| 6 | `ListFlowLogs` HTTP 历史 | P2 | ✅ `FlowLogService.ListFlowLogs` |
| 7 | Tab 命名 `traces`→`runs`、服务端 completion 过滤 | P2 | ❌ |
| 8 | LOG-02 框架层 zap 结构化 | P2 | ❌（跨 pkg 修改） |
| 9 | LOG-03 关键路径 FlowLog 补全 | P2 | ✅ P0/P1/P2 完成 |
| 10 | LOOP-01 FR-02 cronrunner `log.Helper` 残留清理 | P3 | 🟡 7 处残留 |
| 11 | LOOP-01 FR-03 stepTitleRegistry 22 个 step_id 注册 | P3 | ❌ |
| 12 | 自检/自愈/模式挖掘 | P2 | ✅ |

---

## 6. 验收标准

- [x] Usage：Runner 指标 + 跳转概览（用量大盘在 `/overview`，见 Dashboard 三件套）
- [x] Alerts：规则可配置；超阈 `alert.fired`；Webhook/Channel 出站（冷却生效）
- [x] 告警冷却持久化 + firing 状态机（MON-OPT-02）
- [x] 告警评估批量化 + RingBuffer + EvalWorker（MON-OPT-03）
- [x] 告警注册表 AlertMetricRegistry（MON-OPT-06）
- [x] FlowLog Bus 分离到 MonitorBus（MON-OPT-01）
- [x] Trace 写入回路 + 历史回填（MON-OPT-05）
- [x] FlowLog 文件落盘 + gzip + 30 天清理（LOG-01）
- [x] Trace 文件落盘（TRACE-01）
- [x] AI 诊断包 GenerateDiagnosticBundle API（DIAG-01）
- [x] 根因分析规则引擎 5 条内置规则（DIAG-02）
- [x] WS 反压可观测 + 优先级队列（MON-OPT-04）
- [x] Audit / Events / Traces / Logs：见 [18 monitor.md §7](./18%20monitor.md#7-验收要点)
- [x] 方案 C（Phase 1d）：Runs 主排障 + Events 不重复 completion + correlation（RUN-01～06）
- [x] Dashboard（`/overview`）ECharts + Runner + Monitor Usage 去重 — [18 monitor-dashboard.md](./18%20monitor-dashboard.md)
- [x] 全局 latency 聚合 P50/P95/P99（LATENCY-01/02）
- [x] ListFlowLogs HTTP 历史 API
- [x] LOG-03 P0 红线修复（9 处 `log.Warnf`/`log.Errorf` → FlowLog）
- [x] LOG-03 P1 关键路径补全（Graph/Session/Knowledge）
- [x] LOG-03 P2 biz 层 fmt.Errorf 全量清理（15 处 → kerrors）
- [x] LOG-03 P2 admin.go log.Infof → FlowLog
- [x] UsecaseOption 构造器注入重构
- [x] RebuildRingBuffer 逐分钟重建
- [x] 3 处 P0 data race 修复（FlowFileAppender/AlertEvalWorker/TraceProjector）
- [x] ReplaceAlertRules 增量 upsert 保留 firing_state
- [x] 自检 SelfCheck 周期性检查 + 4 个修复器
- [x] 自愈 SelfHeal 事件驱动修复 + cooldown + 置信度阈值
- [x] 预测性自愈 PredictiveHeal 系统指标 + 故障模式匹配
- [x] 模式挖掘 PatternMining 故障聚类 + 自动修复模板
- [x] 故障报告 FailureReport 统一 CI/runtime 错误格式
- [x] LOOP-01 FR-01 `log.Printf` 红线违规清零
- [ ] LOOP-01 FR-02 cronrunner `log.Helper` 残留清理（7 处）
- [ ] LOOP-01 FR-03 stepTitleRegistry 22 个 step_id 注册

---

## 7. 依赖与风险

- Dashboard 可复用 `docs/observability/grafana-aranea.json` 或前端图表库。
- 告警出站依赖 [17-channel-development.md](./17-channel-development.md) Channel `webhook_url`。
- **方案 C**：`trace_id` / `usage_event_id` 缺失时仍落库 completion，Events 仅降级展示。
- **Runs 列表**以 `recordTurnUsage` 为准，与 `CHAT_RECORD_RUNNER_USAGE` 环境变量无关。
- 前端跳转遵守 [frontend-guide.md](../guides/frontend-guide.md) — `useMonitorRunNavigation` 编排，组件不直连 router。
- **LOG-02** 跨 `pkg/trpc-agent-go` 修改，需独立 PR，不在本迭代范围。


---

## 子模块：Monitor Dashboard 开发计划

> **版本**：2026-05-21 | **状态**：🟢 Phase 0～3 + 分层整改（MDB-02～03、Store/composable）已完成；Phase 4 待办  
> **需求**：[18 monitor-dashboard.md](./18%20monitor-dashboard.md) · **设计**：[18 monitor-dashboard.design.md](./18%20monitor-dashboard.design.md)  
> **用量真相**：[29-token-development.md](./29-token-development.md) · **运维页**：[18-monitor-development.md](./18-monitor-development.md)

---

## 1. 模块定位

**监控 Dashboard** = `/overview`（`OverviewPage`），**不是** `/monitor/logs`。

| 页面 | 路由 | 职责 |
|------|------|------|
| Dashboard | `/overview` | 用量/成本大盘、ECharts 趋势/占比、Runner 条、运维快捷入口 |
| Monitor 运维 | `/monitor/logs` | Usage Tab 仅 Runner + 跳转概览 |
| 用量明细 | `/usage/events` | 逐条事件、CSV |

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| 路由与侧栏 | ✅ | `/` → `/overview` |
| 用量数据流 | ✅ | `useOverviewPage` → `useUsageStore` → `features/usage/api` |
| Runner 数据流 | ✅ | `useRunnerMetrics` → `useMonitorStore.loadRunnerMetrics` |
| Runner 展示组件 | ✅ | `RunnerMetricsPanel` 纯 props；容器 `OverviewRunnerMetrics` / `MonitorRunnerMetrics` |
| ECharts 趋势 | ✅ | `UsageTrendChart` + `usageTrendMetrics` |
| 占比饼图 | ✅ | `UsageBreakdownCharts` + `usageBreakdownSlices`（Provider 基于 Top 模型样本） |
| Monitor Usage 去重 | ✅ | `MonitorUsageDashboardLink`；已删 `UsageOverview.vue` |
| 运维快捷链 | ✅ | `OverviewMonitorQuickLinks` |
| 死代码清理 | ✅ | 已删 `UsageTrendPanel.vue` |
| 单测 | ✅ | `usageTrendMetrics.spec.ts`、`usageBreakdownSlices.spec.ts`（66 tests pass） |
| Provider 全量占比 API | ❌ | 待 `top_providers` 或后端 rollup |
| 自动刷新 / 全量 URL 筛选 | ❌ | MDB-04 |

---

## 3. 开发阶段

| 阶段 | 目标 | 状态 |
|------|------|------|
| Phase 0 — MVP | 概览 + Usage API | ✅ |
| Phase 1 — 文档 | 三文档 + 交叉引用 | ✅ |
| Phase 2 — 图表与 Runner | ECharts + 占比 + 概览 Runner 条 | ✅ |
| Phase 3 — Monitor 整合 | Usage 去重 + 快捷入口 | ✅ |
| Phase 3b — 分层整改 | Store/composable、`useUsageChart`、容器组件 | ✅ |
| Phase 4 — 运营扩展 | 自动刷新、快照、Grafana | ❌ |

---

## 4. 任务清单

### Phase 2 — 图表与指标（✅）

| ID | 任务 | 状态 |
|----|------|------|
| MDB-02-01 | `echarts` + `usageEcharts.ts` | ✅ |
| MDB-02-02 | `UsageTrendChart` 多 metric | ✅ |
| MDB-02-03 | 成功率堆叠柱 | ✅ |
| MDB-02-04 | `UsageBreakdownCharts` + `usageBreakdownSlices.ts` | ✅ |
| MDB-02-05 | `OverviewRunnerMetrics` + 下钻 Traces | ✅ |
| MDB-02-06 | latency P50/P95 API | ❌ |

### Phase 3 — Monitor 整合（✅）

| ID | 任务 | 状态 |
|----|------|------|
| MDB-03-01 | `MonitorUsageDashboardLink`；顶栏共用 `range` | ✅ |
| MDB-03-02 | `OverviewMonitorQuickLinks` | ✅ |
| MDB-03-03 | 异常行跳转明细/Runs | ❌ |
| MDB-03-04 | 自动刷新 5min | ❌ |

### Phase 3b — 前端分层整改（✅，2026-05-21）

| ID | 任务 | 状态 |
|----|------|------|
| MDB-03b-01 | `useMonitorStore.loadRunnerMetrics` | ✅ |
| MDB-03b-02 | `features/monitor/useRunnerMetrics.ts` | ✅ |
| MDB-03b-03 | `RunnerMetricsPanel` 纯展示 | ✅ |
| MDB-03b-04 | `useUsageChart.ts` + resize debounce | ✅ |
| MDB-03b-05 | 趋势/占比逻辑迁入 `usageTrendMetrics` / `usageBreakdownSlices` | ✅ |
| MDB-03b-06 | `RunnerMetricsSummary` → `features/monitor/types.ts` | ✅ |
| MDB-03b-07 | 删除 `UsageTrendPanel.vue` | ✅ |

### Phase 4 — 待办

| ID | 任务 | 状态 |
|----|------|------|
| MDB-04-01 | URL 持久化全部筛选 | ❌ |
| MDB-04-02 | 概览 PNG/PDF 导出 | ❌ |
| MDB-04-03 | Grafana iframe | ❌ |
| MDB-04-04 | `top_providers` API + Provider 全量饼图 | ❌ |

---

## 5. 验收标准

- [x] Token / 调用 / 费用 / 成功率趋势可切换
- [x] 模型 + Provider 费用占比图（Provider 样本口径已标注）
- [x] Runner 条 + 下钻 `/monitor/logs?tab=traces`
- [x] Monitor Usage 无重复用量卡片
- [x] `pnpm test` + `pnpm build` 通过
- [x] Runner 请求经 Store，展示组件不直连 API

---

## 6. 验证命令

```bash
cd web && pnpm test && pnpm build
```

**手工**：`/overview` 改筛选 → 趋势 metric 切换 → 占比图 → Runner 下钻 → Monitor Usage「打开概览」带 `range` → 运维监控下拉进 Events/Traces。

---

## 7. 依赖与风险

| 风险 | 缓解 |
|------|------|
| Provider 饼图非全量 | UI 文案 + 后续 `top_providers` |
| Runner 窗口 vs 用量 `range` 独立 | `OverviewRunnerMetrics` scopeHint |
| ECharts 体积 | async import + `useUsageChart` chunk |

---

## 8. 建议下一步

1. MDB-04-04 `top_providers`（后端 + 饼图）
2. MDB-03-03 异常行深链
3. MDB-03-04 自动刷新
4. Monitor 其他组件（`MonitorAlertRules`）迁入 Store（独立任务）

---

*维护：实现状态以本文件 §2、§4 为准。*


---

## 子模块：Monitor 优化 2026-05-26

> **关联**：[`18 monitor.md`](./18%20monitor.md) · [`18 monitor.design.md`](./18%20monitor.design.md) · [`18-monitor-development.md`](./18-monitor-development.md) · 代码 Review [2026-05-26-Monitor-Code-Review](../review/2026-05-26-Monitor-Code-Review.md)
> **规则真相源**：[`monitor-streams-wire.mdc`](../../.cursor/rules/monitor-streams-wire.mdc) · [`AGENT_RUNTIME_BOUNDARY.md`](../AGENT_RUNTIME_BOUNDARY.md)
> **范围**：本文聚焦 **业务逻辑层面** 的优化（用户/运维实际感受得到的能力差异）。代码风格、命名、格式等问题已在 review 文档 P3 收敛，本文不重复。
> **状态**：✅ 已全部落地（2026-05-28）

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

### 1.4 实现落地（2026-05-28）

- `event.Infra.Publish()` 按 EnvelopeType 路由表自动分发（`flow_log`/`log` → MonitorBus，其余 → SessionBus）
- 默认 `split` 模式，`dual` 模式通过 env flag 可回退
- `ws.go` 全局连接仅订阅 MonitorBus，单 pump
- 移除 `globalMode && monitorBus != sessionBus` 双 pump 分支

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

### 2.4 实现落地（2026-05-28）

- `monitor_alert_rules` 新增 `last_fired_at`/`last_fired_value`/`last_fired_window_start`/`firing_state`/`recovered_at` 列
- `Usecase` 实现 `ShouldFireAlert`（DB 持久化优先 + 内存 fallback）、`MarkAlertFiredPersistent`、`MarkAlertRecovered`
- `evaluateRunnerErrorRate` / `evaluateSkillFilesystemMissingCount` / `evaluateMetricValue` 统一 recovery 逻辑
- `recovery_factor` 默认 0.9，`recoveryThreshold()` 计算

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

### 3.4 实现落地（2026-05-28）

- `MetricRingBuffer`：内存滑动窗口，O(1) 增量更新，60 个 1 分钟桶
- `AlertEvalWorker`：独立 goroutine 30s ticker 统一评估，替代每次 completion 触发
- `singleflight.Group` 防并发评估
- `event_bus_runner_handler` 移除 `safego.Go("monitor.evaluate-alerts")`，改为 `OnCompletion` 更新 RingBuffer
- `RebuildRingBuffer` 启动时从 DB 加载最近 1 小时数据

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

### 4.4 实现落地（2026-05-28）

- WS 连接按 EnvelopeType 分优先级 channel（high/normal/low）
- `writePump` 按 high → normal → low 顺序取
- high 优先级永不丢弃（阻塞超时后关闭连接）
- normal/low 满时 DropNewest + 计数
- `monitor.backpressure` envelope 反馈客户端

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

`monitor_traces` 扩展：

```sql
ALTER TABLE monitor_traces ADD COLUMN session_id TEXT;
ALTER TABLE monitor_traces ADD COLUMN run_id TEXT;
ALTER TABLE monitor_traces ADD COLUMN invocation_id TEXT;
ALTER TABLE monitor_traces ADD COLUMN agent_id TEXT;
ALTER TABLE monitor_traces ADD COLUMN team_id TEXT;
ALTER TABLE monitor_traces ADD COLUMN parent_trace_id TEXT;  -- 跨 turn / 跨 team 关联
ALTER TABLE monitor_traces ADD COLUMN status TEXT;            -- ok | error | partial
ALTER TABLE monitor_traces ADD COLUMN duration_ms INTEGER;
ALTER TABLE monitor_traces ADD COLUMN span_count INTEGER;
ALTER TABLE monitor_traces ADD COLUMN error_count INTEGER;
ALTER TABLE monitor_traces ADD COLUMN total_tokens INTEGER;
ALTER TABLE monitor_traces ADD COLUMN total_cost_usd REAL;
CREATE INDEX idx_monitor_traces_session ON monitor_traces(session_id, started_at);
CREATE INDEX idx_monitor_traces_run ON monitor_traces(run_id);
```

新增 `monitor_trace_spans`（如果暂未独立表）：

```sql
CREATE TABLE monitor_trace_spans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    trace_id TEXT NOT NULL,
    span_id TEXT NOT NULL,
    parent_span_id TEXT,
    kind TEXT NOT NULL,         -- llm | tool | retrieve | graph_node | hitl | subteam
    name TEXT NOT NULL,
    started_at INTEGER NOT NULL,
    ended_at INTEGER,
    status TEXT NOT NULL,        -- ok | error
    attributes_json TEXT,
    error_json TEXT,
    UNIQUE(trace_id, span_id)
);
CREATE INDEX idx_trace_spans_trace ON monitor_trace_spans(trace_id, started_at);
```

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

#### 5.2.5 历史数据回填

新增 cron `MonitorTraceBackfillWorker`：
- 从 `model_token_usage_events` 倒序扫最近 30 天
- 按 `session_id` + `invocation_id` 分组生成 trace 行
- 完成后置 `backfill_done=true`

### 5.3 验收标准

| 指标 | 目标 |
|------|------|
| 新产生的 turn 100% 落 trace 行 | ✅ |
| Trace 详情 Waterfall 渲染数据完整率 | ≥ 95% |
| Run 详情前端请求数 | 从 N+1 降到 2 次（trace + spans） |
| 历史回填覆盖率 | ≥ 99%（30 天内） |
| 集成测 `TestTraceProjector_RunnerCompletion_BuildsTraceWithSpans` | ✅ |

### 5.4 实现落地（2026-05-28）

- `TraceProjector`：订阅 EventBus，首次 trace_id 出现 INSERT `monitor_traces`，后续 span UPSERT
- `spanKindFromStep`：`HasPrefix`/`HasSuffix` 精确匹配（已修复 Contains 误分类）
- `OnRunnerCompletion`：关闭 trace（UPDATE status/duration/tokens）
- `evictStaleTraces`：1 分钟清理 ticker，10 分钟 TTL 淘汰孤儿 trace
- `EnsureTraceSchema`：`monitor_traces` 扩展列 + `monitor_trace_spans` 建表 + `monitor_events` generated columns
- `MonitorTraceBackfillWorker`：6 小时间隔 cron，从 `monitor_events` 回填历史 trace
- `traceProjectorWorker`：256 buffer + drop 计数（每 100 次打印 SysLogWarn）

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

### 6.4 实现落地（2026-05-28）

- `AlertMetric` 接口：`Key()`/`Description()`/`Evaluate(ctx, window)`
- `AlertMetricRegistry`：`Register`/`Get`/`List`，线程安全（`sync.RWMutex`）
- `RunnerErrorRateMetric`：优先 RingBuffer，fallback DB COUNT
- `SkillFilesystemMissingMetric`：从 `FilesystemHealthReader` 取
- `Usecase.EvaluateAlerts` 优先使用注册表，fallback 到 switch
- `evaluateMetricValue` 通用评估方法（recovery + threshold + fire 逻辑统一）
- DSL 解析器（Phase 2）暂未实现，当前仅 Registry

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

## 8. 排期建议（参考）

| 迭代 | 内容 | 预估 |
|------|------|------|
| Sprint A（2 周） | OPT-01 Bus 路由表 + Phase 0/1 灰度 + OPT-04 优先级 channel | M |
| Sprint B（2 周） | OPT-02 firing 状态机 + DB 持久化 + OPT-03 RingBuffer Worker | M |
| Sprint C（2 周） | OPT-05 MonitorTraceProjector + Traces Tab 数据接通 | M |
| Sprint D（3 周） | OPT-05 跨 trace 关联 + 历史回填 + OPT-04 lossless 模式 | L |
| Sprint E（2 周） | OPT-06 Registry + DSL 解析器 + 规则迁移 | M |
| Sprint F（2 周） | OPT-02 silence_windows + escalation + 前端配置 UI | M |

---

## 9. 不在本方案范围

| 项 | 理由 |
|----|------|
| 用量大盘（`/overview`）改版 | 见独立需求 `18 monitor-dashboard.md` |
| Audit 表 schema 改动 | 现有满足合规，本轮不动 |
| 自定义 metric 的 WASM 评估器 | OPT-06 Phase 2，本方案仅占位 |
| 接入外部 incident 系统（PagerDuty 等） | 通过 Webhook 即可，无需平台内置 |
| 前端 ECharts 改型 | 本方案聚焦后端业务流，前端按需对齐 |

---

## 10. 与监控分流规则的对照（`monitor-streams-wire.mdc`）

| 规则约定 | 本方案落地 |
|----------|-----------|
| Audit / Logs / Events 不混表 | 保持 ✅；OPT-05 traces 独立表 |
| 实时主通道 WS / 禁止独立 SSE | 保持 ✅；OPT-04 在 WS 内做反压 |
| flow_log 走 MonitorBus | **OPT-01 彻底落地** |
| TeamRunEvent snake_case + payload 扩展 | 保持 ✅ |
| 重要配置变更写 audit_logs（detail 不脱敏密钥） | 不变；OPT-02 alert rule 变更将额外写一条 audit |
| `cmd/admin/wire_gen.go` 不手改 | 严格遵守；OPT-03 Worker 通过 wire provider 注入 |

---

## 11. 关联文档

- 业务需求总则：[`18 monitor.md`](./18%20monitor.md)
- 当前实现设计：[`18 monitor.design.md`](./18%20monitor.design.md)
- 进度跟踪（本方案落地后追加章节）：[`18-monitor-development.md`](./18-monitor-development.md)
- 代码 Review（问题来源）：[`2026-05-26-Monitor-Code-Review.md`](../review/2026-05-26-Monitor-Code-Review.md)
- 历史 Review：[`18-monitor-review.md`](../review/18-monitor-review.md)
- 流分流规则：[`monitor-streams-wire.mdc`](../../.cursor/rules/monitor-streams-wire.mdc)


---

## 子模块：Monitor AI 闭环 2026-05-28

> **关联**：[`18 monitor.md`](./18%20monitor.md) · [`18-monitor-optimization-2026-05-26.md`](./18-monitor-optimization-2026-05-26.md) · [`52-flow-logger.design.md`](./52-flow-logger.design.md) · 代码 Review [`2026-05-26-Monitor-Code-Review.md`](../review/2026-05-26-Monitor-Code-Review.md)
> **状态**：🟢 Phase A~D 已落地（LOG-01 ✅ TRACE-01 ✅ DIAG-01 ✅ DIAG-02 ✅ LOG-03 ✅ P0/P1/P2 ✅ 自检/自愈 ✅）；LOG-02（跨 pkg）待实施；LOOP-01 FR-01 ✅ FR-02 🟡 FR-03 ❌
> **创建**：2026-05-28

---

## 0. 需求原文与问题定义

### 0.1 原始需求

> 通过后台的 logs 日志，记录服务的所有运行状态，AI 可以根据日志运行的记录文件追踪到问题，定位问题，形成闭环。

### 0.2 需求拆解

| 子需求 | 含义 | 当前状态 | 差距 |
|--------|------|----------|------|
| **记录所有运行状态** | 每个关键业务动作都有结构化日志 | 🟡 FlowLog 覆盖 ~60 个 step_id，但部分关键路径仍用 slog/zap | 需补全遗漏路径 + 统一日志出口 |
| **日志持久化到文件** | 日志写入磁盘文件，进程重启后可回溯 | ❌ FlowLog 仅走 EventBus → WS/DB，无文件落盘 | 需新增文件 Appender |
| **AI 可读取日志** | 日志格式对 AI 友好（结构化、可检索、带关联 ID） | 🟡 FlowLogEntry 已结构化，但文件输出为 ConsoleEncoder | 需 JSON Lines 文件输出 |
| **AI 追踪到问题** | 从一条错误日志出发，沿 trace_id / session_id 回溯完整链路 | 🟡 FlowLog 有 trace_id 关联，但跨表/跨文件追踪需手动拼接 | 需诊断包自动聚合 |
| **定位问题** | AI 能给出根因分析 + 修复建议 | ❌ 无自动化根因分析能力 | 需诊断规则引擎 + AI Prompt 模板 |
| **形成闭环** | 问题从发现 → 追踪 → 定位 → 修复 → 验证 全链路可追溯 | ❌ 发现靠人工看 Monitor 页面，修复后无自动验证 | 需闭环工作流 |

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

## 1. 现状分析

### 1.1 日志体系现状

项目存在 **三套并行的日志体系**，尚未统一：

| 体系 | 实现 | 输出目标 | 结构化 | 关联 ID | AI 可读 |
|------|------|----------|--------|---------|---------|
| **框架层 zap** | `pkg/trpc-agent-go/log` | stdout（ConsoleEncoder） | ❌ 彩色控制台 | ❌ 无 trace_id | ❌ |
| **应用层 FlowLog** | `internal/event/trace_emitter.go` | EventBus → WS + DB | ✅ `flow_log/v1` | ✅ trace_id/session_id/run_id | ✅ |
| **系统域 SysLog** | `internal/event/system_flow.go` | EventBus → MonitorBus | ✅ `flow_log/v1` | 🟡 部分（system 域无 session） | ✅ |

### 1.2 关键差距

| 差距编号 | 描述 | 影响 | 关联 |
|----------|------|------|------|
| **GAP-01** | FlowLog 无文件落盘 | ✅ LOG-01 已落地 | ✅ |
| **GAP-02** | 框架层 zap 日志无结构化输出 | ❌ AI 无法解析 stdout 彩色文本 | 新增（LOG-02） |
| **GAP-03** | 部分关键路径仍用 slog 而非 FlowLog | ✅ LOG-03 P0/P1/P2 已完成 | ✅ |
| **GAP-04** | `monitor_traces` 表无写入路径 | ✅ MON-OPT-05 已落地 | ✅ |
| **GAP-05** | 无诊断包自动聚合 | ✅ DIAG-01 已落地 | ✅ |
| **GAP-06** | 无根因分析规则引擎 | ✅ DIAG-02 已落地 | ✅ |
| **GAP-07** | 告警触发后无自动追踪动作 | ✅ 自检/自愈体系已落地 | ✅ |
| **GAP-08** | Chat FlowLog 仍走 SessionBus | ✅ MON-OPT-01 已落地 | ✅ |

### 1.3 已有优化方案覆盖情况

| 本方案编号 | 已有方案覆盖 | 说明 |
|------------|-------------|------|
| LOG-01（文件落盘） | ✅ 已落地 | `FlowFileAppender` + 按日/大小轮转 + gzip + 30 天清理 |
| LOG-02（zap 结构化） | ❌ 未实施 | 跨 `pkg/trpc-agent-go` 修改，需独立 PR |
| LOG-03（路径补全） | ✅ 已落地 | P0 红线修复 9 处；P1 补全 Graph/Session/Knowledge；P2 biz 层 fmt.Errorf 全量清理 |
| TRACE-01（Trace 写入） | ✅ 已落地 | `runner.completion` → `trace-*.jsonl` |
| DIAG-01（诊断包） | ✅ 已落地 | `DiagBundleGenerator` + `GenerateDiagnosticBundle` RPC |
| DIAG-02（根因引擎） | ✅ 已落地 | `RootCauseEngine` 5 条内置规则 + 置信度评分 |
| LOOP-01（闭环工作流） | 🟡 FR-01 ✅ FR-02 🟡（7 处残留）FR-03 ❌（22 个 step_id 未注册） | 需求+设计已完成，FR-02/03 待实施 |

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

> 本节引用 [MON-OPT-05](./18-monitor-optimization-2026-05-26.md#5-mon-opt-05trace-写入回路--run-全链路视图) 的设计，不重复。仅补充 AI 闭环所需的接口。

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

## 12. 排期建议

| 阶段 | 内容 | 依赖 |
|------|------|------|
| **Phase A** | LOG-01 文件落盘 + LOG-02 zap 结构化 | MON-OPT-01 |
| **Phase B** | LOG-03 路径补全（P1 部分）+ TRACE-01 Trace 文件落盘 | MON-OPT-05 |
| **Phase C** | DIAG-01 诊断包生成 API + 自动触发 | Phase A + B |
| **Phase D** | DIAG-02 根因引擎（12 条内置规则）+ LOG-03 P2 部分 | Phase C |
| **Phase E** | LOOP-01 闭环工作流 + AI Prompt 模板 | Phase D |

---

## 13. 不在本方案范围

| 项 | 理由 |
|----|------|
| 日志搜索服务（ELK/Loki） | 本方案聚焦文件落盘 + AI 直接消费，不引入额外基础设施 |
| 实时流式 AI 分析 | 当前为按需生成诊断包，实时分析需更大架构变更 |
| 自动修复执行 | 闭环到「修复建议」为止，自动执行修复需人工审批 |
| 多语言日志 | 当前日志以中文 title 为主，AI Prompt 为英文，暂不调整 |
| 前端闭环 UI | 本方案聚焦后端能力，前端展示在后续迭代规划 |

---

## 14. 关联文档

- 业务需求：[`18 monitor.md`](./18%20monitor.md)
- 设计文档：[`18 monitor.design.md`](./18%20monitor.design.md)
- 优化方案：[`18-monitor-optimization-2026-05-26.md`](./18-monitor-optimization-2026-05-26.md)
- FlowLogger 设计：[`52-flow-logger.design.md`](./52-flow-logger.design.md)
- 代码 Review：[`2026-05-26-Monitor-Code-Review.md`](../review/2026-05-26-Monitor-Code-Review.md)
- 历史 Review：[`18-monitor-review.md`](../review/18-monitor-review.md)
- Bus 分流规则：[`monitor-streams-wire.mdc`](../../.cursor/rules/monitor-streams-wire.mdc)


---

## 子模块：自检与自愈开发计划

> **版本**：2026-06-06 | **状态**：✅ 已实现
> **需求**：[18 monitor.md](./18%20monitor.md) §0.2 自检与自愈 · **设计**：[18 monitor.design.md](./18%20monitor.design.md) §十
> **代码锚点**：`internal/biz/monitor/self_check*.go`、`self_heal*.go`、`predictive_heal.go`、`pattern_mining.go`

---

### 1. 模块定位

Monitor 自检与自愈体系：周期性健康检查 + 事件驱动修复 + 预测性自愈 + 故障模式挖掘。是 DIAG-01/02 的延伸，将「诊断→根因→修复」串联为自动化闭环。

---

### 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| SelfCheckScheduler | ✅ | 5 min 周期 + 4 个 Checker 插件 + SelfCheckRepairDispatcher |
| SelfHealObserver | ✅ | 事件驱动修复 + cooldown + 置信度阈值 |
| PredictiveHeal | ✅ | 系统指标 + 故障模式匹配 + 预防性修复 |
| PatternMining | ✅ | 故障聚类 + 自动修复模板 + 置信度晋升/停用 |
| FailureReport | ✅ | 统一 CI/runtime 错误格式 + FailureReportParser |
| failure_pattern 表 | ✅ | FailurePatternReader/Writer 接口 + data 层实现 |
| Wire 注入 | ✅ | cmd/admin/wire.go 集成 |

---

### 3. 任务清单

| ID | 任务 | 优先级 | 状态 | 关键实现 |
|----|------|--------|------|----------|
| HEAL-01 | SelfChecker 插件接口 + 4 个内置 Checker | P1 | ✅ | TraceProjectorChecker/FlowFileChecker/AlertEvalChecker/EventBusChecker |
| HEAL-02 | SelfCheckRepairDispatcher + 4 个修复器 | P1 | ✅ | FlowFile/TraceProjector/AlertEval/EventBus Repairer |
| HEAL-03 | SelfCheckScheduler 周期调度 | P1 | ✅ | 5 min ticker + `SELF_CHECK_INTERVAL` 环境变量 |
| HEAL-04 | SelfHealObserver 事件驱动修复 | P1 | ✅ | 订阅 FlowLog error/critical + DiagBundle + RootCauseEngine |
| HEAL-05 | PredictiveHeal 预测性自愈 | P2 | ✅ | SystemMetricsReader + 故障模式匹配 + 置信度 > 0.8 |
| HEAL-06 | PatternMining 模式挖掘 | P2 | ✅ | HealRecord 聚类 + 自动修复模板 + 置信度晋升/停用 |
| HEAL-07 | FailureReport + Parser | P2 | ✅ | 5 种故障类型 + CI/runtime 正则识别 |
| HEAL-08 | failure_pattern 表 + Repo | P2 | ✅ | FailurePatternReader/Writer + 3 种来源（runtime/ci/mined） |

---

### 4. 验收标准

- [x] SelfCheck 每 5 分钟执行一次，4 个 Checker 全部运行
- [x] Checker 检测到异常时自动触发对应 Repairer
- [x] SelfHealObserver 订阅 FlowLog 错误事件，自动生成诊断包并尝试修复
- [x] PredictiveHeal 在系统指标异常时执行预防性修复
- [x] PatternMining 从历史修复记录中自动挖掘故障模式
- [x] FailureReport 统一 CI 和 runtime 错误格式
- [x] Wire 注入完整，`go build ./cmd/admin` 通过

---

### 5. 验证命令

```bash
make build && make test
```

**手工**：触发一次 Provider 超时 → Monitor Logs 看到错误 → SelfHealObserver 自动生成诊断包 → 根因引擎匹配规则 → 执行修复 → PatternMining 异步聚类。
