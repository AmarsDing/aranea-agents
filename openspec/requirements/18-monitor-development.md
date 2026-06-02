# Monitor 监控 — 开发计划

> **版本**：2026-05-29-v3 | **状态**：🟢 核心已通 + **MON-OPT-01~06 ✅ LOG-01/TRACE-01 ✅ DIAG-01/02 ✅ Latency P50/P95/P99 ✅ LOG-03 P0/P1/P2 ✅ REDLINE ✅ QUALITY ✅**；待办为 LOG-02（跨 pkg）、LOOP-01（P3）
> **需求**：[18 monitor.md](./18%20monitor.md) · **设计**：[18 monitor.design.md](./18%20monitor.design.md)（§九 方案 C）
> **进度真相**：[execution-plan.md](../guides/execution-plan.md)（I8-MON-01/02、MON-01、I5-MON-01/02）· **页面索引**：[frontend-pages.md](./frontend-pages.md) §监控

---

## 1. 模块定位

系统监控：审计、实时事件、模型用量、单次运行排障（Runs）、流程/进程日志、Runner 窗口指标与告警。

**代码锚点**：

| 层 | 路径 |
|----|------|
| Proto | `api/kratos/monitor/v1/monitor.proto` |
| Biz | `internal/biz/monitor.go`、`internal/biz/monitor/`（alert_eval_worker、metric_ring_buffer、trace_projector、flow_file_appender、alert_metric_registry、root_cause_engine、diag_bundle）、`runner_completion.go` |
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

---

## 3. 差距与优化（P2+）

1. **P2 — UI 命名**：路由 Tab `traces` → `runs` 别名；Events 服务端 `hide_linked_completions`（减轻前端过滤）。
2. **P2 — LOG-02**：框架层 zap 日志结构化（JSON Encoder）— 跨 `pkg/trpc-agent-go` 修改，需独立 PR。
3. **P2 — LOOP-01**：系统调试日志闭环（用 FlowLog 替代 `fmt.Println`/`log.Printf`，让系统运行信息直接显示在 Monitor Logs 界面）。需求：[18-monitor-loop-01-requirement.md](./18-monitor-loop-01-requirement.md) · 设计：[18-monitor-loop-01-design.md](./18-monitor-loop-01-design.md)

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
| LOOP-01 | 系统调试日志闭环 | P2 | ❌ 待实施 | 需求：[18-monitor-loop-01-requirement.md](./18-monitor-loop-01-requirement.md) · 设计：[18-monitor-loop-01-design.md](./18-monitor-loop-01-design.md) |

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
| 10 | LOOP-01 系统调试日志闭环 | P2 | ❌ 待实施（需求+设计已完成） |

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

---

## 7. 依赖与风险

- Dashboard 可复用 `docs/observability/grafana-aranea.json` 或前端图表库。
- 告警出站依赖 [17-channel-development.md](./17-channel-development.md) Channel `webhook_url`。
- **方案 C**：`trace_id` / `usage_event_id` 缺失时仍落库 completion，Events 仅降级展示。
- **Runs 列表**以 `recordTurnUsage` 为准，与 `CHAT_RECORD_RUNNER_USAGE` 环境变量无关。
- 前端跳转遵守 [frontend-guide.md](../guides/frontend-guide.md) — `useMonitorRunNavigation` 编排，组件不直连 router。
- **LOG-02** 跨 `pkg/trpc-agent-go` 修改，需独立 PR，不在本迭代范围。
