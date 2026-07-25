# Monitor 监控 — 开发计划

> **版本**：2026-06-06-v4 | **状态**：🟢 核心已通 + **MON-OPT-01~05 ✅ OPT-06 🟡（Registry 有 / DSL 未做）LOG-01/TRACE-01 ✅ DIAG-01/02 ✅ Latency P50/P95/P99 ✅ LOG-03 P0/P1/P2 ✅ REDLINE ✅ QUALITY ✅ 自检/自愈 ✅**；2026-07-16 修复 completion 副作用接线、firing reminder、Runs 列、反压 banner、进程 Tab；待办为 LOG-02（跨 pkg）、LOOP-01 FR-02/FR-03（P3）、Heal/Diagnose UI
> **需求**：[18 monitor.md](./18%20monitor.md) · **设计**：[18 monitor.design.md](./18%20monitor.design.md)（§九 方案 C / 子模块 自检与自愈设计 / 子模块 Monitor 优化设计 / 子模块 Monitor AI 闭环设计）
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
| SQL | `internal/data/sql/monitor_alert.sql`、`internal/data/sql/monitor_alert_firing_state.sql`、`internal/data/memory_helpers.go`（audit_logs/monitor_events/monitor_traces 建表）、`internal/data/monitor_trace.go`（monitor_traces 扩展列 + monitor_trace_spans 建表） |
| FlowLogger | [52-flow-logger-development.md](./52-flow-logger-development.md) — Logs 流程 Tab 与 Runs 详情 Flow |

> **架构与接口设计**：见 [18 monitor.design.md](./18%20monitor.design.md) §一~§九（Proto 契约、Biz/Data/Service 分层、Wire 注入、Web 前端、方案 C）。

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
| 可观测性 Dashboard（`/admin/observability`） | ✅ | 5 Tab（Plans/TeamRuns/GraphExecutions/Metrics/FlowLogs）整合页；`ObservabilityDashboardPage.vue` + `features/observability/`；链接跳转现有模块页面；T3.4/T4.1-4.3/T5.4 已完成 |
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
| Completion→RingBuffer/TraceClose 接线 | ✅ | 2026-07-16：`RecordRunnerCompletion` → `evalWorker.OnCompletion` + `traceProjector.OnRunnerCompletion` + TRACE-01 `WriteTraceComplete` |
| Firing 态 reminder（防出站轰炸） | ✅ | 2026-07-16：firing 期间按 `reminder_minutes`（默认 30）提醒，不再非法重入 `threshold_exceeded` |
| Trace backfill 关闭 running 行 | ✅ | 2026-07-16：`UpdateMonitorTraceCompletion` 覆盖 INSERT OR IGNORE 已有行 |
| Runs 列表 Token/延迟/成本列 | ✅ | 2026-07-16：`ListMonitorTraces` 投影 duration/tokens/cost；FE TraceList 展示（OPT-05 后真相源为 `monitor_traces`） |
| WS 反压前端 banner | ✅ | 2026-07-16：消费 `monitor.backpressure` 并在 MonitorPage 展示 |
| 进程日志 Tab 暂停控制 | ✅ | 2026-07-16：切离暂停（丢弃入站）；2026-07-24：应用户要求恢复手动「暂停/恢复」按钮，默认关闭（paused=true），切回不自动恢复 |

---

## 3. 差距与优化（P2+）

1. **P2 — UI 命名**：路由 Tab `traces` → `runs` 别名；Events 服务端 `hide_linked_completions`（减轻前端过滤）。
2. **P2 — LOG-02**：框架层 zap 日志结构化（JSON Encoder）— 跨 `pkg/trpc-agent-go` 修改，需独立 PR。
3. **P3 — LOOP-01 FR-02**：清理 cronrunner 剩余 7 处 Kratos `log.Helper`（5 个文件：monitor_alert_cooldown、memory_dead_letter_replayer、provider_health、channel_health、evolution_scanner、channel_delivery）。
4. **P3 — LOOP-01 FR-03**：补全 stepTitleRegistry 22 个缺失 step_id 注册。

> **优化项详细设计**：见 [18 monitor.design.md](./18%20monitor.design.md) 子模块 Monitor 优化设计（MON-OPT-01~06）。

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

> **方案 C 详细设计**：见 [18 monitor.design.md](./18%20monitor.design.md) §九。

### MON-OPT — 业务逻辑优化（2026-05-26 方案落地）

| ID | 任务 | 优先级 | 状态 | 关键实现 |
|----|------|--------|------|----------|
| MON-OPT-01 | FlowLog 流彻底分离到 MonitorBus | P1 | ✅ | `event.Infra` 路由表 split 模式；WS 全局连接单 pump |
| MON-OPT-02 | 告警冷却持久化 + firing 状态机 | P1 | ✅ | `UpdateAlertFiringState` + `MarkAlertFiredPersistent` + `MarkAlertRecovered` |
| MON-OPT-03 | 告警评估批量化 + 滑动窗口 | P1 | ✅ | `MetricRingBuffer` + `AlertEvalWorker`（30s ticker）+ singleflight |
| MON-OPT-04 | WS 反压可观测 + 优先级队列 | P1 | ✅ | 优先级 channel + drop 计数 + 反压事件 |
| MON-OPT-05 | Trace 写入回路 + 历史回填 | P1 | ✅ | `TraceProjector` + `EnsureTraceSchema` + `MonitorTraceBackfillWorker` |
| MON-OPT-06 | 告警规则注册表 | P2 | ✅ | `AlertMetricRegistry` + `RunnerErrorRateMetric` + `SkillFilesystemMissingMetric` |

> **MON-OPT 详细设计**：见 [18 monitor.design.md](./18%20monitor.design.md) 子模块 Monitor 优化设计。

### AI 闭环追踪（2026-05-28 方案落地）

| ID | 任务 | 优先级 | 状态 | 关键实现 |
|----|------|--------|------|----------|
| LOG-01 | FlowLog 文件落盘 | P1 | ✅ | `FlowFileAppender` + 按日/大小轮转 + gzip + 30 天清理 |
| LOG-02 | 框架层 zap 日志结构化 | P2 | ❌ | 跨 `pkg/trpc-agent-go` 修改，需独立 PR |
| LOG-03 | 关键路径 FlowLog 补全 | P2 | ✅ P0/P1/P2 完成 | P0 红线修复 9 处；P1 补全 Graph/Session/Knowledge；P2 biz 层 fmt.Errorf 全量清理 + admin.go log.Infof |
| TRACE-01 | Trace 文件落盘 | P1 | ✅ | `runner.completion` → `trace-*.jsonl` |
| DIAG-01 | AI 诊断包 | P1 | ✅ | `DiagBundleGenerator` + `GenerateDiagnosticBundle` RPC |
| DIAG-02 | 根因分析规则引擎 | P1 | ✅ | `RootCauseEngine` **12** 条内置规则 + 置信度评分（与设计 RC-001 清单 taxonomy 不完全相同，以代码为准） |
| LOOP-01 | 系统调试日志闭环 | P2 | 🟡 FR-01 ✅ FR-02 🟡 FR-03 ❌ | FR-01 已完成；FR-02 剩余 7 处 cronrunner `log.Helper`；FR-03 22 个 step_id 未注册 |

> **AI 闭环详细设计**：见 [18 monitor.design.md](./18%20monitor.design.md) 子模块 Monitor AI 闭环设计。

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

## 子模块：Monitor 优化 2026-05-26 开发计划

> **关联**：[`18 monitor.md`](./18%20monitor.md) · [`18 monitor.design.md`](./18%20monitor.design.md) 子模块 Monitor 优化设计 · [`18-monitor-development.md`](./18-monitor-development.md) · 代码 Review [2026-05-26-Monitor-Code-Review](../review/2026-05-26-Monitor-Code-Review.md)
> **规则真相源**：[`monitor-streams-wire.mdc`](../../.cursor/rules/monitor-streams-wire.mdc) · [`AGENT_RUNTIME_BOUNDARY.md`](../AGENT_RUNTIME_BOUNDARY.md)
> **范围**：本文聚焦 **业务逻辑层面** 的优化（用户/运维实际感受得到的能力差异）。代码风格、命名、格式等问题已在 review 文档 P3 收敛，本文不重复。
> **状态**：✅ 已全部落地（2026-05-28）

---

## 0. 背景

Monitor 已实现六大 Tab（Audit / Alerts / Events / Runs / Usage / Logs）的基础能力，但 review 暴露 **6 项业务正确性 / 运维体验** 缺陷。本计划跟踪其落地进度。

| 编号 | 主题 | 业务问题（运维 / 用户视角） | 优先级 |
|------|------|---------------------------|--------|
| MON-OPT-01 | **FlowLog 流彻底分离到 MonitorBus** | 高 QPS 时 chat 业务事件挤掉 flow_log → Monitor 页缺关键步骤 | **P1** |
| MON-OPT-02 | **告警冷却持久化 + 多实例分布式去重** | 进程重启 / 多副本 → 同一窗口 Webhook 重复多次轰炸 → IM 限流封禁 | **P1** |
| MON-OPT-03 | **告警评估批量化 + 滑动窗口 + 单飞** | 每次 completion 全规则扫 + 2× COUNT → 高 QPS 时监控反而拖垮 DB | **P1** |
| MON-OPT-04 | **WS 反压可观测 + 客户端可见反馈** | 满 buffer 静默丢事件 → 前端"看不见"问题 → 误判系统正常 | **P1** |
| MON-OPT-05 | **Trace 写入回路 + Run 全链路视图** | `monitor_traces` 表只读不写 → Traces Tab 长期空白 | **P1** |
| MON-OPT-06 | **告警规则注册表 + 自定义指标 DSL** | 加新指标必改 Usecase + repo + Wire；不可热扩展 | **P2** |

> **详细设计方案**（架构、状态机、SQL DDL、代码示例）：见 [18 monitor.design.md](./18%20monitor.design.md) 子模块 Monitor 优化设计。

---

## 1. 任务清单与落地状态

| ID | 任务 | 优先级 | 状态 | 关键实现 |
|----|------|--------|------|----------|
| MON-OPT-01 | FlowLog 流彻底分离到 MonitorBus | P1 | ✅ | ADR-03 Phase 5 后：`FlowTracker` 直接发布到 `MonitorEventBus`（`contract.MonitorEvent`），不再走 `event.Infra.Publish()` 路由表；`SelfHealObserver`/`TraceProjector` 已迁移到 `MonitorEventBus` 订阅；WS 全局连接单 pump |
| MON-OPT-02 | 告警冷却持久化 + firing 状态机 | P1 | ✅ | `monitor_alert_rules` 新增 `last_fired_at`/`firing_state` 等列；`ShouldFireAlert`/`MarkAlertFiredPersistent`/`MarkAlertRecovered`；`recovery_factor` 默认 0.9；**2026-07-16** firing 内 reminder 路径防重入轰炸 |
| MON-OPT-03 | 告警评估批量化 + 滑动窗口 | P1 | ✅ | `MetricRingBuffer`（60 个 1 分钟桶）+ `AlertEvalWorker`（30s ticker）+ `singleflight.Group`；`RebuildRingBuffer` 启动时从 DB 加载；**2026-07-16** live `OnCompletion` 已接到 `RecordRunnerCompletion` |
| MON-OPT-04 | WS 反压可观测 + 优先级队列 | P1 | ✅ | WS 连接按 ActivityKind/MonitorEventType 分 high/normal/low 优先级 channel；high 永不丢弃；`monitor.backpressure` 反馈客户端；**2026-07-16** 前端 banner 已接 |
| MON-OPT-05 | Trace 写入回路 + 历史回填 | P1 | ✅ | `TraceProjector` 订阅 EventBus；`EnsureTraceSchema` 扩展列 + `monitor_trace_spans` 建表；`MonitorTraceBackfillWorker`；**2026-07-16** completion 关 Trace + backfill UPDATE running |
| MON-OPT-06 | 告警规则注册表 | P2 | 🟡 | `AlertMetric` 接口 + `AlertMetricRegistry`；`RunnerErrorRateMetric`/`SkillFilesystemMissingMetric`；**DSL 解析器 / silence_windows / severity_escalation 仍为 Phase 2 未实现** |

---

## 2. 排期建议（参考）

| 迭代 | 内容 | 预估 |
|------|------|------|
| Sprint A（2 周） | OPT-01 Bus 路由表 + Phase 0/1 灰度 + OPT-04 优先级 channel | M |
| Sprint B（2 周） | OPT-02 firing 状态机 + DB 持久化 + OPT-03 RingBuffer Worker | M |
| Sprint C（2 周） | OPT-05 MonitorTraceProjector + Traces Tab 数据接通 | M |
| Sprint D（3 周） | OPT-05 跨 trace 关联 + 历史回填 + OPT-04 lossless 模式 | L |
| Sprint E（2 周） | OPT-06 Registry + DSL 解析器 + 规则迁移 | M |
| Sprint F（2 周） | OPT-02 silence_windows + escalation + 前端配置 UI | M |

---

## 3. 不在本方案范围

| 项 | 理由 |
|----|------|
| 用量大盘（`/overview`）改版 | 见独立需求 `18 monitor-dashboard.md` |
| Audit 表 schema 改动 | 现有满足合规，本轮不动 |
| 自定义 metric 的 WASM 评估器 | OPT-06 Phase 2，本方案仅占位 |
| 接入外部 incident 系统（PagerDuty 等） | 通过 Webhook 即可，无需平台内置 |
| 前端 ECharts 改型 | 本方案聚焦后端业务流，前端按需对齐 |

---

## 4. 关联文档

- 业务需求总则：[`18 monitor.md`](./18%20monitor.md)
- 设计文档（含 MON-OPT 详细设计）：[`18 monitor.design.md`](./18%20monitor.design.md) 子模块 Monitor 优化设计
- 进度跟踪（本方案落地后追加章节）：[`18-monitor-development.md`](./18-monitor-development.md)
- 代码 Review（问题来源）：[`2026-05-26-Monitor-Code-Review.md`](../review/2026-05-26-Monitor-Code-Review.md)
- 历史 Review：[`18-monitor-review.md`](../review/18-monitor-review.md)
- 流分流规则：[`monitor-streams-wire.mdc`](../../.cursor/rules/monitor-streams-wire.mdc)


---

## 子模块：Monitor AI 闭环 2026-05-28 开发计划

> **关联**：[`18 monitor.md`](./18%20monitor.md) · [`18 monitor.design.md`](./18%20monitor.design.md) 子模块 Monitor AI 闭环设计 · [`18-monitor-optimization-2026-05-26.md`](./18-monitor-optimization-2026-05-26.md) · [`52-flow-logger.design.md`](./52-flow-logger.design.md) · 代码 Review [`2026-05-26-Monitor-Code-Review.md`](../review/2026-05-26-Monitor-Code-Review.md)
> **状态**：🟢 Phase A~D 已落地（LOG-01 ✅ TRACE-01 ✅ DIAG-01 ✅ DIAG-02 ✅ LOG-03 ✅ P0/P1/P2 ✅ 自检/自愈 ✅）；LOG-02（跨 pkg）待实施；LOOP-01 FR-01 ✅ FR-02 🟡 FR-03 ❌
> **创建**：2026-05-28

---

## 0. 现状分析

### 0.1 日志体系现状

项目存在 **三套并行的日志体系**，尚未统一：

| 体系 | 实现 | 输出目标 | 结构化 | 关联 ID | AI 可读 |
|------|------|----------|--------|---------|---------|
| **框架层 zap** | `pkg/trpc-agent-go/log` | stdout（ConsoleEncoder） | ❌ 彩色控制台 | ❌ 无 trace_id | ❌ |
| **应用层 FlowLog** | `internal/event/trace_emitter.go` | EventBus → WS + DB | ✅ `flow_log/v1` | ✅ trace_id/session_id/run_id | ✅ |
| **系统域 SysLog** | `internal/event/system_flow.go` | EventBus → MonitorBus | ✅ `flow_log/v1` | 🟡 部分（system 域无 session） | ✅ |

### 0.2 关键差距

| 差距编号 | 描述 | 影响 | 关联 | 状态 |
|----------|------|------|------|------|
| **GAP-01** | FlowLog 无文件落盘 | ✅ LOG-01 已落地 | ✅ | ✅ |
| **GAP-02** | 框架层 zap 日志无结构化输出 | ❌ AI 无法解析 stdout 彩色文本 | 新增（LOG-02） | ❌ |
| **GAP-03** | 部分关键路径仍用 slog 而非 FlowLog | ✅ LOG-03 P0/P1/P2 已完成 | ✅ | ✅ |
| **GAP-04** | `monitor_traces` 表无写入路径 | ✅ MON-OPT-05 已落地 | ✅ | ✅ |
| **GAP-05** | 无诊断包自动聚合 | ✅ DIAG-01 已落地 | ✅ | ✅ |
| **GAP-06** | 无根因分析规则引擎 | ✅ DIAG-02 已落地 | ✅ | ✅ |
| **GAP-07** | 告警触发后无自动追踪动作 | ✅ 自检/自愈体系已落地 | ✅ | ✅ |
| **GAP-08** | Chat FlowLog 仍走 SessionBus | ✅ MON-OPT-01 已落地 | ✅ | ✅ |

> **AI 闭环详细设计方案**（架构图、文件布局、Proto 契约、规则模型、闭环状态机、AI Prompt 模板）：见 [18 monitor.design.md](./18%20monitor.design.md) 子模块 Monitor AI 闭环设计。

---

## 1. 任务清单与落地状态

| ID | 任务 | 优先级 | 状态 | 关键实现 |
|----|------|--------|------|----------|
| LOG-01 | FlowLog 文件落盘 | P1 | ✅ | `FlowFileAppender` + 按日/大小轮转 + gzip + 30 天清理；路由规则按 EnvelopeType 分发到 flow/system/trace/alert 文件 |
| LOG-02 | 框架层 zap 日志结构化 | P2 | ❌ | 跨 `pkg/trpc-agent-go` 修改，需独立 PR；目标：ConsoleEncoder → JSONEncoder + context 注入 trace_id |
| LOG-03 | 关键路径 FlowLog 补全 | P2 | ✅ P0/P1/P2 完成 | P0 红线修复 9 处（Graph/Task/Channel/Admin 域）；P1 补全 Graph/Session/Knowledge；P2 biz 层 fmt.Errorf 全量清理（15 处 → kerrors）+ admin.go log.Infof |
| TRACE-01 | Trace 文件落盘 | P1 | ✅ | `RecordRunnerCompletion` → `FlowFileAppender.WriteTraceComplete` → `trace-*.jsonl`（2026-07-16 接通） |
| DIAG-01 | AI 诊断包 | P1 | ✅ | `DiagBundleGenerator` + `GenerateDiagnosticBundle` RPC；自动触发规则（critical FlowLog / 告警 firing / 手动） |
| DIAG-02 | 根因分析规则引擎 | P1 | ✅ | `RootCauseEngine` 5 条内置规则 + 置信度评分（直接匹配 0.4 + 前置条件 0.3 + 时间关联 0.2 + 频率关联 0.1） |
| LOOP-01 | 系统调试日志闭环 | P2 | 🟡 FR-01 ✅ FR-02 🟡 FR-03 ❌ | FR-01 `log.Printf` 红线违规清零；FR-02 剩余 7 处 cronrunner `log.Helper`（5 个文件）；FR-03 22 个 step_id 未注册 |

### 1.1 LOOP-01 FR-02 残留清单

| 文件 | 行号 | 说明 |
|------|------|------|
| `internal/cronrunner/jobs/memory_dead_letter_replayer.go` | 68, 94 | 2 处 `w.log.*` |
| `internal/cronrunner/jobs/monitor_alert_cooldown.go` | 96 | 1 处 `w.log.*` |
| `internal/cronrunner/jobs/provider_health.go` | 49 | 1 处 `w.log.*` |
| `internal/cronrunner/jobs/channel_health.go` | 49 | 1 处 `w.log.*` |
| `internal/cronrunner/jobs/evolution_scanner.go` | 49 | 1 处 `w.log.*` |
| `internal/cronrunner/jobs/channel_delivery.go` | 50 | 1 处 `w.log.*` |

### 1.2 LOOP-01 FR-03 待注册 step_id

`internal/event/flow_log.go` 中 `stepTitleRegistry` 共 22 个 step_id 未注册中文标题（详见代码）。

---

## 2. 排期建议

| 阶段 | 内容 | 依赖 | 状态 |
|------|------|------|------|
| **Phase A** | LOG-01 文件落盘 + LOG-02 zap 结构化 | MON-OPT-01 | LOG-01 ✅ / LOG-02 ❌ |
| **Phase B** | LOG-03 路径补全（P1 部分）+ TRACE-01 Trace 文件落盘 | MON-OPT-05 | ✅ |
| **Phase C** | DIAG-01 诊断包生成 API + 自动触发 | Phase A + B | ✅ |
| **Phase D** | DIAG-02 根因引擎（12 条内置规则）+ LOG-03 P2 部分 | Phase C | ✅（5 条内置规则） |
| **Phase E** | LOOP-01 闭环工作流 + AI Prompt 模板 | Phase D | 🟡 FR-01 ✅ FR-02/03 待办 |

---

## 3. 不在本方案范围

| 项 | 理由 |
|----|------|
| 日志搜索服务（ELK/Loki） | 本方案聚焦文件落盘 + AI 直接消费，不引入额外基础设施 |
| 实时流式 AI 分析 | 当前为按需生成诊断包，实时分析需更大架构变更 |
| 自动修复执行 | 闭环到「修复建议」为止，自动执行修复需人工审批 |
| 多语言日志 | 当前日志以中文 title 为主，AI Prompt 为英文，暂不调整 |
| 前端闭环 UI | 本方案聚焦后端能力，前端展示在后续迭代规划 |

---

## 4. 关联文档

- 业务需求：[`18 monitor.md`](./18%20monitor.md)
- 设计文档（含 AI 闭环详细设计）：[`18 monitor.design.md`](./18%20monitor.design.md) 子模块 Monitor AI 闭环设计
- 优化方案：[`18-monitor-optimization-2026-05-26.md`](./18-monitor-optimization-2026-05-26.md)
- FlowLogger 设计：[`52-flow-logger.design.md`](./52-flow-logger.design.md)
- 代码 Review：[`2026-05-26-Monitor-Code-Review.md`](../review/2026-05-26-Monitor-Code-Review.md)
- 历史 Review：[`18-monitor-review.md`](../review/18-monitor-review.md)
- Bus 分流规则：[`monitor-streams-wire.mdc`](../../.cursor/rules/monitor-streams-wire.mdc)


---

## 子模块：自检与自愈开发计划

> **版本**：2026-06-06 | **状态**：✅ 已实现
> **需求**：[18 monitor.md](./18%20monitor.md) §0.2 自检与自愈 · **设计**：[18 monitor.design.md](./18%20monitor.design.md) 子模块 自检与自愈设计
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

> **自检与自愈详细设计**：见 [18 monitor.design.md](./18%20monitor.design.md) 子模块 自检与自愈设计。

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
