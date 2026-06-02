# 监控 Dashboard（概览页）— 开发计划

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
