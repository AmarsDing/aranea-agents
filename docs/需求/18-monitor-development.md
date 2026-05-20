# Monitor 监控 — 开发计划

> **版本**：2026-05-20 | **状态**：🟡 Usage + 告警 + Logs 1c ✅；**Phase 1d 方案 C（Runs+Events 分工）✅**
> **需求**：[18 monitor.md](./18%20monitor.md) · **设计**：[18 monitor.design.md](./18%20monitor.design.md)（§九 方案 C）
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

系统监控：采集和展示 Agent/Team 运行指标，包括 Token 用量、响应时间、成功率、错误率等。

**代码锚点**：
- `internal/biz/usage.go` — UsageUsecase（Token 用量统计）
- `internal/service/chat_usage_ingress.go` — 用量记录入口
- `internal/data/usage.go` — UsageRepo
- **FlowLogger v2**：[52-flow-logger.md](./52-flow-logger.md) · [design](./52-flow-logger.design.md) · [开发计划](./52-flow-logger-development.md) — Logs 已收 `flow_log`；Traces「流程」Tab 见 Phase 1b

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Token 用量记录 | ✅ | `chat_usage_ingress.go` → `UsageUsecase` |
| 用量查询 API | ✅ | `GetUsage` / `ListUsage` RPC |
| Dashboard | 🟡 | `MonitorPage` Usage 总览 + Audit/Events/Traces |
| Events / Runs 分工 | ✅ | 方案 C：Runs=真相源，Events 过滤已关联 completion；见 Phase 1d |
| 告警规则 | ✅ | `monitor_alert_rules` + `GET/PUT /v1/monitor/alert-rules`；`runner.error_rate` 评估 |
| 告警通知 | ✅ | `AlertNotifier`：Webhook POST + Channel `webhook_url`；`alert.notify` 事件；规则级 `cooldown_minutes` |
| 响应时间追踪 | ❌ | 无 latency 指标采集 |
| 错误率统计 | ❌ | 无 error_rate 指标聚合 |

---

## 3. 差距与优化

1. ~~**P1（Phase 1d · 方案 C）**~~：✅ correlation + Events 过滤 + Runs「打开会话」+ Runner 指标下钻。
2. **P2**：无监控 Dashboard，用户无法直观查看系统运行状态。
3. **P2**：无响应时间和错误率指标，无法评估 Agent 质量。
4. ~~**P2**：告警仅落 Events，无出站通知。~~ ✅ 迭代 4：`internal/service/monitor_notify.go`

---

## 4. 开发阶段

- **Phase 1d（方案 C · Runs + Events）**：completion correlation、Events 过滤、Runs 详情「打开会话」、Runner 指标下钻 — ✅
- **Phase 1c（Logs Tab 拆分）**：流程/进程二级 Tab + LogStreamHub + WS `enable_log` 修复 + legacy `log` 清理 + `process_log_enabled` 配置 — ✅
- **Phase 1**：响应时间 + 错误率指标采集
- **Phase 2**：监控 Dashboard 前端页面
- **Phase 3**：告警规则 + 通知机制

### Phase 1c — Logs 流程/进程拆分

| ID | 任务 | 优先级 | 状态 |
|----|------|--------|------|
| MON-1c-01 | `useLogStreamHub` 共享 WS + `connected` 状态 | P1 | ✅ |
| MON-1c-02 | `LogStreamPanel` + `FlowLogStream` + `ProcessLogStream` | P1 | ✅ |
| MON-1c-03 | `ws.go`：`enable_log(false)` 保留 global monitor channel | P0 | ✅ |
| MON-1c-04 | 删除重复 `EnvelopeTypeLog`（intent/team-runner/chat-native/compress） | P1 | ✅ |
| MON-1c-05 | 文档 + changelog + execution-plan 同步 | P1 | ✅ |
| MON-1c-06 | `server.monitor.process_log_enabled` + 进程 Tab UI 简化 + 全高布局 | P1 | ✅ |

**验收**：

1. Logs Tab 内两个二级 Tab 独立缓冲；流程 Tab 可手动暂停；进程 Tab 切离时暂停（丢弃入站）、切回自动恢复。
2. 连接成功即显示「已连接」，不依赖首条日志。
3. `process_log_enabled: false` 时服务端不推送进程 log；`enable_log(true)` 被忽略。
4. 关闭进程推送后流程 `flow_log` 仍正常；global `*` 连接不误删 monitor channel。
5. 发起 Chat 后流程 Tab 见中文 `flow_log`；config 开启且切到进程 Tab 见插件 `log`。

### Phase 1d — 方案 C：Runs 真相源 + Events 收窄 + completion 关联

> 设计：[18 monitor.design.md §九](./18%20monitor.design.md#九方案-cruns--events--runnercompletion) · 验收：[18 monitor.md RUN-01～06](./18%20monitor.md#35-用户故事与验收phase-1d--方案-c)

| ID | 任务 | 层 | 优先级 | 状态 | 依赖 |
|----|------|-----|--------|------|------|
| MON-1d-01 | `DomainEvent`/Handler 保留 `request_id`、`invocation_id`、`usage`；补齐 `trace_id` | biz | P0 | ✅ | — |
| MON-1d-02 | `monitorRunnerCompletionMeta` → v1（**`trace_id`、`usage_event_id`** 为主） | biz | P0 | ✅ | MON-1d-01 |
| MON-1d-03 | `recordTurnUsage` 与 completion 共享 correlation（同 Turn 写 `usage_event_id`） | service + biz | P0 | ✅ | MON-1d-02 |
| MON-1d-04 | 落库幂等：`(event_key, session_id, invocation_id)` | data + biz | P1 | ✅ | MON-1d-02 |
| MON-1d-05 | `features/monitor/runCorrelation.ts` + `RunnerCompletionMeta` 类型 | web | P0 | ✅ | MON-1d-02 |
| MON-1d-06 | `RealtimeEvents`：**过滤** 已关联 Runs 的 persisted `runner.completion`；降级卡片 +「在 Runs 中查看」 | web | P0 | ✅ | MON-1d-05 |
| MON-1d-07 | `TraceList` 详情：**打开会话**；列表副标题 Runs 语义 | web | P0 | ✅ | — |
| MON-1d-08 | `RunnerMetricsPanel` 点击下钻 `?tab=traces` | web | P1 | ✅ | — |
| MON-1d-09 | `MonitorPage` / `useMonitorRunNavigation`：session、trace、打开 Runs 详情 | web | P1 | ✅ | MON-1d-06, MON-1d-07 |
| MON-1d-10 | persisted + WS 去重；changelog 落地标记 | docs | P1 | ✅ | MON-1d-06 |

**明确不做（方案 C）**：`MonitorEventDetailDialog` 承载 Chat completion 完整排障；Events 与 Runs 平行详情页。

**Phase 1d 验收**（RUN-01～06）：

1. Chat 结束后 **Runs（Traces）** 列表有 usage 行，详情可开 Flow/Waterfall/Span。
2. Runs 详情可 **打开会话**（`/chat?session=…`）。
3. Events **主列表不重复** 显示已关联 Runs 的 Chat `runner.completion`。
4. `runner.error_rate` 与 Runner 指标卡统计不变。
5. 无 Runs 行时 Events 显示降级 completion，并可跳转会话或 Runs（若有 `usage_event_id`）。
6. metadata 含 `schema_version=v1` 且带 `trace_id` 或 `usage_event_id`（在可采集时）。

**验证命令**：

```bash
make build && make test
cd web && pnpm lint && pnpm test && pnpm build
```

**手工**：Chat 一轮 → Monitor **Traces** 见行并打开详情 → Events 无重复 completion → Runner 指标点击下钻 Traces。

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | 运行指标采集：latency / error_rate / success_rate | P2 | — |
| 2 | Dashboard 前端页面 | P2 | — |
| 3 | 告警规则引擎 | P2 | ✅ MON-01 |
| 4 | 通知渠道（Webhook/Channel） | P3 | ✅ I4-MON-02 |
| 5 | 方案 C：Runs/Events 分工 + completion 关联 | P1 | ✅ Phase 1d |

---

## 6. 验收标准

- [x] Usage 总览可展示 Token 用量（`/usage/events` 聚合）
- [x] 告警规则可配置并写入 `alert.fired` 事件
- [x] 告警可触发 Channel/Webhook 通知（冷却期内不重复）
- [ ] 方案 C：Runs 主排障 + Events 不重复 completion + correlation（Phase 1d）

---

## 7. 依赖与风险

- Dashboard 可复用现有前端图表库
- 告警需与 Channel 模块联动
- **Phase 1d**：`trace_id` / `usage_event_id` 依赖 Turn 上下文；缺失时仍落库，仅降级 Events 展示
- **Phase 1d**：Runs 列表以 `recordTurnUsage` 为准，与 `CHAT_RECORD_RUNNER_USAGE` 无关
- **Phase 1d**：遵守 [frontend-guide.md](../guides/frontend-guide.md) — 跳转由 Page / `useMonitorRunNavigation` 编排
