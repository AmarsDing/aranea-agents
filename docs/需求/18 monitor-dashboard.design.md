# 监控 Dashboard（概览页）— 技术设计

> 对应需求：[18 monitor-dashboard.md](./18%20monitor-dashboard.md)  
> 用量契约：[29 token.design.md](./29%20token.design.md) · 运维页：[18 monitor.design.md](./18%20monitor.design.md)  
> **版本**：2026-05-21（Phase 2/3 + 前端分层整改）

---

## 一、架构定位

```text
OverviewPage (/overview)
  ├─ useOverviewPage → useUsageStore → features/usage/api → UsageService
  └─ OverviewRunnerMetrics → useRunnerMetrics → useMonitorStore → GetRunnerMetrics

MonitorPage Usage Tab
  ├─ MonitorRunnerMetrics → useRunnerMetrics（同上 Store）
  └─ MonitorUsageDashboardLink → /overview?range=（顶栏 filters.range）
```

```
                    ┌─────────────────────────────────────┐
                    │  OverviewPage (/overview)           │
                    └──────────────┬──────────────────────┘
                                   │
          ┌────────────────────────┼────────────────────────┐
          │                        │                        │
          ▼                        ▼                        ▼
   useUsageStore            useMonitorStore          components (props)
   UsageService            MonitorService
```

Runner 指标 **不写入 Usage 表**；只读 `GET /v1/monitor/runner-metrics`。

---

## 二、后端

### 2.1 已有 API

| RPC | HTTP | 响应要点 |
|-----|------|----------|
| `GetUsageOverview` | `GET /v1/usage/overview` | `today` / `yesterday` / `month` / `range` / `trends` / `top_models` / `top_agents` / `anomalies` / `inefficient_models` / `quota_dashboard` |
| `ListUsageTrends` | `GET /v1/usage/trends` | `granularity=hour` → `model_token_usage_hourly` |
| `ListUsageEvents` | `GET /v1/usage/events` | 明细页 |
| `GetRunnerMetrics` | `GET /v1/monitor/runner-metrics` | `RunnerMetricsSummary`（Dashboard / Monitor Usage 共用） |

### 2.2 聚合规则（读路径）

- `UsageUsecase.Overview` → `usageWhere(..., billableOnly=true)` 排除 `team_turn`。
- 状态归一：`usage_status_sql.go`。
- 费用：`usage_pricing.go` + Provider 模型 `config_json`。

### 2.3 待扩展 API

| 能力 | 说明 |
|------|------|
| `top_providers` 独立聚合 | Provider 饼图当前基于 `top_models` 样本，非全量 Provider rollup |
| P50/P95 延迟 | `UsageOverview` 增 `latency_percentiles`（MDB-02-06） |

---

## 三、前端分层（`frontend-guide.md`）

### 3.1 合法数据流

```text
用量大盘：
  Page → useOverviewPage → useUsageStore.loadOverview → features/usage/api

Runner 指标：
  容器组件 → useRunnerMetrics → useMonitorStore.loadRunnerMetrics → features/monitor/api

图表：
  UsageTrendChart / UsageBreakdownCharts（props only）
    → useUsageChart（ECharts 生命周期）
    → usageTrendMetrics / usageBreakdownSlices（纯函数）
```

**红线遵守**：

- `RunnerMetricsPanel.vue`：**仅 props/emits**，不 import `api` / `store`。
- `OverviewPage` / `MonitorPage`：不直连 `features/*/api`（Runner 经容器 composable）。

### 3.2 目录结构（当前）

```text
web/src/
├── pages/OverviewPage.vue
├── features/usage/
│   ├── useOverviewPage.ts
│   ├── useUsageChart.ts              ← ECharts 宿主 + resize debounce
│   ├── usageEcharts.ts               ← 按需注册 + 主题色（--color-success/danger）
│   ├── usageTrendMetrics.ts
│   ├── usageBreakdownSlices.ts
│   └── api.ts / types.ts
├── features/monitor/
│   ├── useRunnerMetrics.ts           ← Runner 唯一请求入口（页面/容器）
│   └── useMonitorRunNavigation.ts
├── stores/
│   ├── usage/index.ts
│   └── monitor/index.ts              ← loadRunnerMetrics
└── components/
    ├── usage/
    │   ├── OverviewPageHero.vue
    │   ├── OverviewRunnerMetrics.vue ← 容器：composable + RunnerMetricsPanel
    │   ├── OverviewMonitorQuickLinks.vue
    │   ├── UsageMetricCards.vue
    │   ├── UsageTrendChart.vue       ← async chunk
    │   ├── UsageBreakdownCharts.vue  ← async chunk
    │   └── …
    └── monitor/
        ├── RunnerMetricsPanel.vue    ← 纯展示
        ├── MonitorRunnerMetrics.vue  ← 容器
        └── MonitorUsageDashboardLink.vue
```

已删除：`UsageTrendPanel.vue`、`UsageOverview.vue`。

### 3.3 用量数据流

```text
筛选变更 / onMounted
  → useOverviewPage.loadOverview()
  → useUsageStore.loadOverview(query, granularity)
  → getModelUsageOverview + [hour] listModelUsageTrends
  → 子组件 props（overview.trends / top_models / …）
```

### 3.4 路由与深链

| Query | 行为 |
|-------|------|
| `range` | `useOverviewPage` 初始化筛选（`?range=30d` 等） |
| — | 「打开概览」从 Monitor 携带 `range`，与顶栏 `filters.range` 一致 |

### 3.5 图表实现

| 模块 | 实现 |
|------|------|
| 趋势 | `UsageTrendChart`：metric 切换 tokens / calls / cost / success_rate（堆叠 %） |
| 占比 | `UsageBreakdownCharts`：模型 Top5 费用环图；Provider 由 Top 模型样本聚合（UI 已标注） |
| 分包 | `defineAsyncComponent` + `useUsageChart` 独立 chunk（`usageEcharts`） |

主题色：`usageChartPalette()` 读取 `--color-accent`、`--color-success`、`--color-danger`。

---

## 四、Monitor Usage Tab（已实现）

```text
Monitor → Usage Tab
  ├── MonitorRunnerMetrics（Store + RunnerMetricsPanel）
  └── MonitorUsageDashboardLink（打开概览 / 查看明细；range 用页面顶栏，无重复下拉）
```

不再维护 `UsageOverview.vue`。

---

## 五、测试

| 层 | 内容 |
|----|------|
| Web | `usageTrendMetrics.spec.ts`、`usageBreakdownSlices.spec.ts` |
| Web（P2） | Overview 筛选 → store mock；E2E `/overview` → `/usage/events` |
| Go | `usage_*_test.go` 聚合口径 |

---

*任务与验收见 [18-monitor-dashboard-development.md](./18-monitor-dashboard-development.md)。*
