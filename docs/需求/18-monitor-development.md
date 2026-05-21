# Monitor 监控 — 开发计划

> **版本**：2026-05-21 | **状态**：🟢 核心已通（6 Tab + 告警 + Logs 1c + **Phase 1d 方案 C** ✅）；概览 Dashboard Phase 2～3b ✅；待办为 latency 聚合等 P2
> **需求**：[18 monitor.md](./18%20monitor.md) · **设计**：[18 monitor.design.md](./18%20monitor.design.md)（§九 方案 C）
> **进度真相**：[execution-plan.md](../guides/execution-plan.md)（I8-MON-01/02、MON-01、I5-MON-01/02）· **页面索引**：[frontend-pages.md](./frontend-pages.md) §监控

---

## 1. 模块定位

系统监控：审计、实时事件、模型用量、单次运行排障（Runs）、流程/进程日志、Runner 窗口指标与告警。

**代码锚点**：

| 层 | 路径 |
|----|------|
| Proto | `api/kratos/monitor/v1/monitor.proto` |
| Biz | `internal/biz/monitor.go`、`monitor_alert` 逻辑、`runner_completion.go` |
| Data | `internal/data/monitor.go`、`internal/data/monitor_alert.go` |
| Service | `internal/service/monitor.go`、`monitor_notify.go` |
| 用量 | `internal/biz/usage.go`、`internal/service/turn_usage.go`、`chat_usage_ingress.go` |
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
| 错误率统计（窗口） | ✅ | `GetRunnerMetrics` 聚合 `runner.completion`；非独立 latency 指标 |
| 告警规则 | ✅ | `monitor_alert_rules` + `GET/PUT /v1/monitor/alert-rules` |
| 告警评估 | ✅ | `EvaluateAlerts`：`runner.error_rate` → `alert.fired` |
| 告警通知 | ✅ | `monitor_notify.go`：Webhook + Channel；`cooldown_minutes` |
| Events / Runs 分工（方案 C） | ✅ | `runCorrelation.ts`、`RealtimeEvents` 过滤、Runs「打开会话」 |
| Logs 流程/进程拆分 | ✅ | `useLogStreamHub`、`LogStreamPanel`、[changelog](../changelog/2026-05-20-Monitor-Logs-Split.md) |
| 监控 Dashboard（`/overview`） | ✅ | Phase 0～3b 完成；见 [18-monitor-dashboard-development.md](./18-monitor-dashboard-development.md) |
| 响应时间（latency）专用指标 | ❌ | Runs 行有 `latency_ms`；无全局 latency 聚合 API |

---

## 3. 差距与优化（P2+）

1. **P2 — Latency 指标**：全局 P50/P95、按 Agent/Model 聚合（Runs 行字段已有，缺聚合 API）。
2. **P2 — UI 命名**：路由 Tab `traces` → `runs` 别名；Events 服务端 `hide_linked_completions`（减轻前端过滤）。
3. **P2 — FlowLogger Phase 2**：`ListFlowLogs` HTTP 历史（流程 Tab 当前仅 WS 实时）。

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
| Phase 1（原） | 全局 latency / error_rate 聚合 API | ❌ 部分由 Runner 指标覆盖 |

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
| 1 | 全局 latency 聚合 API | P2 | ❌ |
| 2 | Dashboard 图表/占比/Runner（`/overview`） | P2 | ✅ [18-monitor-dashboard-development.md](./18-monitor-dashboard-development.md) |
| 3 | 告警规则引擎 | P2 | ✅ |
| 4 | 通知渠道 Webhook/Channel | P3 | ✅ |
| 5 | 方案 C Runs/Events + correlation | P1 | ✅ |
| 6 | `ListFlowLogs` HTTP 历史 | P2 | ❌（FlowLogger Phase 2） |
| 7 | Tab 命名 `traces`→`runs`、服务端 completion 过滤 | P2 | ❌ |

---

## 6. 验收标准

- [x] Usage：Runner 指标 + 跳转概览（用量大盘在 `/overview`，见 Dashboard 三件套）
- [x] Alerts：规则可配置；超阈 `alert.fired`；Webhook/Channel 出站（冷却生效）
- [x] Audit / Events / Traces / Logs：见 [18 monitor.md §7](./18%20monitor.md#7-验收要点)
- [x] 方案 C（Phase 1d）：Runs 主排障 + Events 不重复 completion + correlation（RUN-01～06）
- [x] Dashboard（`/overview`）ECharts + Runner + Monitor Usage 去重 — [18 monitor-dashboard.md](./18%20monitor-dashboard.md)
- [ ] 全局 latency 聚合（P2）

---

## 7. 依赖与风险

- Dashboard 可复用 `docs/observability/grafana-aranea.json` 或前端图表库。
- 告警出站依赖 [17-channel-development.md](./17-channel-development.md) Channel `webhook_url`。
- **方案 C**：`trace_id` / `usage_event_id` 缺失时仍落库 completion，Events 仅降级展示。
- **Runs 列表**以 `recordTurnUsage` 为准，与 `CHAT_RECORD_RUNNER_USAGE` 环境变量无关。
- 前端跳转遵守 [frontend-guide.md](../guides/frontend-guide.md) — `useMonitorRunNavigation` 编排，组件不直连 router。
