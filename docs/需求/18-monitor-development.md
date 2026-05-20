# Monitor 监控 — 开发计划

> **版本**：2026-05-20 | **状态**：🟡 Usage 总览 + 告警规则 + Webhook/Channel 出站 ✅
> **需求**：[18 monitor.md](./18%20monitor.md) · **设计**：[18 monitor.design.md](./18%20monitor.design.md)
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
| 告警规则 | ✅ | `monitor_alert_rules` + `GET/PUT /v1/monitor/alert-rules`；`runner.error_rate` 评估 |
| 告警通知 | ✅ | `AlertNotifier`：Webhook POST + Channel `webhook_url`；`alert.notify` 事件；规则级 `cooldown_minutes` |
| 响应时间追踪 | ❌ | 无 latency 指标采集 |
| 错误率统计 | ❌ | 无 error_rate 指标聚合 |

---

## 3. 差距与优化

1. **P2**：无监控 Dashboard，用户无法直观查看系统运行状态。
2. **P2**：无响应时间和错误率指标，无法评估 Agent 质量。
3. ~~**P2**：告警仅落 Events，无出站通知。~~ ✅ 迭代 4：`internal/service/monitor_notify.go`

---

## 4. 开发阶段

- **Phase 1**：响应时间 + 错误率指标采集
- **Phase 2**：监控 Dashboard 前端页面
- **Phase 3**：告警规则 + 通知机制

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | 运行指标采集：latency / error_rate / success_rate | P2 | — |
| 2 | Dashboard 前端页面 | P2 | — |
| 3 | 告警规则引擎 | P2 | ✅ MON-01 |
| 4 | 通知渠道（Webhook/Channel） | P3 | ✅ I4-MON-02 |

---

## 6. 验收标准

- [x] Usage 总览可展示 Token 用量（`/usage/events` 聚合）
- [x] 告警规则可配置并写入 `alert.fired` 事件
- [x] 告警可触发 Channel/Webhook 通知（冷却期内不重复）

---

## 7. 依赖与风险

- Dashboard 可复用现有前端图表库
- 告警需与 Channel 模块联动
