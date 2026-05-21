# Monitor Dashboard — 实现与文档同步（2026-05-21）

## 摘要

监控 **概览 Dashboard**（`/overview`）完成 Phase 2～3b（ECharts 趋势/占比、Runner 条、运维快捷链、前端 Store/composable 分层）。**Monitor 运维页** Usage Tab 不再重复用量卡片，仅保留 Runner + 跳转概览/明细。

## 产品分工

| 页面 | 路由 | 职责 |
|------|------|------|
| Dashboard | `/overview` | 用量/成本大盘、趋势、占比、Runner、Monitor 快捷入口 |
| Monitor | `/monitor/logs` | Usage：Runner + `MonitorUsageDashboardLink` |
| 明细 | `/usage/events` | 逐条事件 + CSV |

## 前端变更（要点）

- 新增：`UsageTrendChart`、`UsageBreakdownCharts`、`usageTrendMetrics`、`usageBreakdownSlices`、`useUsageChart`、`useRunnerMetrics`
- 容器：`OverviewRunnerMetrics`、`MonitorRunnerMetrics`、`MonitorUsageDashboardLink`、`OverviewMonitorQuickLinks`
- Store：`useMonitorStore.loadRunnerMetrics`
- 删除：`UsageOverview.vue`、`UsageTrendPanel.vue`
- `RunnerMetricsPanel` 改为纯展示（props/emits）

## 文档同步

- `docs/需求/18 monitor.md` — Usage Tab §4.0、文件清单
- `docs/需求/18 monitor.design.md` — §7.1～7.4
- `docs/需求/18-monitor-development.md` — Phase 2 ✅、验收
- `docs/需求/18 monitor-dashboard*.md` — 已实现态（上轮）
- `docs/需求/frontend-pages.md` — `/overview` 与 Monitor Usage Tab
- `docs/需求/29-token-development.md`、`29 token.design.md` — 趋势组件路径
- `docs/需求/README-development.md`、`docs/guides/execution-plan.md` — 进度索引

## 待办（文档已记录）

- `top_providers` API（Provider 全量饼图）
- 异常行深链、概览自动刷新（MDB-03-03/04、MDB-04）
- 全局 latency P50/P95（MDB-02-06）
- `MonitorAlertRules` 迁 Store（既有技术债）
