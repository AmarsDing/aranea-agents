# Overview 指挥中心 — 设计文档

## 1. 页面布局设计

### 1.1 整体结构

升级后的 OverviewPage 自上而下分为 6 个区域：

```
┌─────────────────────────────────────────────────────────┐
│  ① Hero 欢迎区（替换原 OverviewPageHero）                │
│  问候语 + 6 核心指标 + 快捷操作按钮（slot）               │
├─────────────────────────────────────────────────────────┤
│  ② 快捷入口区（新增）                                    │
│  [新建对话] [创建Agent] [定时任务] [系统设置]              │
├─────────────────────────────────────────────────────────┤
│  ③ 实时状态面板行（新增）                                 │
│  [Agent状态] [会话活跃] [Provider健康] [Runner运行]        │
├─────────────────────────────────────────────────────────┤
│  ④ 筛选栏（保留）                                        │
│  时间范围 | Provider | 模型 | 状态 | 趋势粒度             │
├─────────────────────────────────────────────────────────┤
│  ⑤ 用量分析区（保留，原指标卡片+趋势+分布+排行+告警）      │
│  MetricCards → 趋势图+摘要 → 分布+Provider → 排行 →      │
│  Runner指标 → 告警栈                                     │
└─────────────────────────────────────────────────────────┘
```

### 1.2 区域详细设计

#### ① Hero 欢迎区

**组件**：`CommandCenterHero.vue`（替换 `OverviewPageHero.vue`）

```
┌──────────────────────────────────────────────────────────┐
│  Command Center                                          │
│  你好，Admin                                    🌙 12:34  │
│                                                          │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐    │
│  │ 活跃Agent │ │ 模型提供商│ │ 公司数量  │ │ 团队数量  │    │
│  │    5     │ │    4     │ │    3     │ │    2     │    │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘    │
│  ┌──────────┐ ┌──────────┐                               │
│  │ 今日调用  │ │ 今日Token │                               │
│  │   128    │ │  52.3K   │                               │
│  └──────────┘ └──────────┘                               │
│                                                          │
│  [⚠️ 查看告警]  [📄 查看明细]  （slot: actions）          │
└──────────────────────────────────────────────────────────┘
```

**Props**：

| Prop | 类型 | 说明 |
|------|------|------|
| `username` | `string` | 当前登录用户名 |
| `activeAgentCount` | `number` | 活跃 Agent 数 |
| `providerCount` | `number` | 模型提供商数 |
| `categoryCount` | `number` | 公司（组织）数量 |
| `teamCount` | `number` | 团队数量 |
| `todaySessionCount` | `number` | 今日对话数 |
| `todayTokenCount` | `number` | 今日 Token 消耗 |

**Emits**：

| Event | Payload | 说明 |
|-------|---------|------|
| `navigate` | `[action: string]` | 点击指标卡片导航（如 `'tokens'` 跳转用量明细） |

**Slots**：

| Slot | 说明 |
|------|------|
| `actions` | Hero 右上角操作区，由父组件注入快捷链接与按钮 |

**数据来源**：
- `username`：从 `localStorage.auth_user` 解析获取（fallback `'Admin'`）
- `activeAgentCount`：通过 `listAgentsPaged({ status: 'active', limit: 1 })` 取 `total`
- `providerCount`：从 `usePlatformStore.providerModels` 去重统计 provider 编码
- `categoryCount`：通过 `usePlatformStore.loadTaxonomyTree('organization')` 取树长度
- `teamCount`：通过 `listTeams()` 取长度
- `todaySessionCount`：从 `useUsageStore.overview.today.call_count` 获取
- `todayTokenCount`：从 `useUsageStore.overview.today.total_tokens` 获取

**i18n**：所有文案通过 `useI18n()` 的 `t('overviewPage.xxx')` 国际化，键值定义在 `web/src/i18n/locales/zh-CN.ts` 与 `en-US.ts` 的 `overviewPage` 命名空间下。

**指标卡片可点击**：每个统计卡片是 `router-link`，点击跳转对应管理页（`/agents`、`/models`、`/settings/organization`、`/team`、`/chat`）；Token 卡片通过 `@click.prevent="$emit('navigate', 'tokens')"` 触发父组件跳转用量明细。

#### ② 快捷入口区

**组件**：`CommandCenterQuickActions.vue`

```
┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐
│  💬        │ │  🤖        │ │  ⏰        │ │  ⚙️        │
│  新建对话   │ │  创建Agent │ │  定时任务   │ │  系统设置   │
│  与AI对话   │ │  配置新智能体│ │  管理定时任务│ │  系统参数   │
└────────────┘ └────────────┘ └────────────┘ └────────────┘
```

**路由映射**：

| 入口 | 图标 | 路由 | Material Icon |
|------|------|------|---------------|
| 新建对话 | chat | `/chat` | `chat` |
| 创建 Agent | smart_toy | `/agents` | `smart_toy` |
| 定时任务 | schedule | `/cron` | `schedule` |
| 系统设置 | settings | `/settings` | `settings` |

**Props**：无（纯展示 + 路由跳转，不需要外部数据）

#### ③ 实时状态面板行

**组件**：`CommandCenterStatusPanels.vue`（容器）+ 4 个子面板组件

```
┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐
│  Agent 状态 │ │  会话活跃   │ │ Provider健康│ │ Runner运行  │
│   ◔ 5/8    │ │   12       │ │  ● 3/4     │ │ 成功率 98% │
│  在线5 离线3│ │  ≈≈≈≈≈    │ │  降级:1     │ │ 错误率 2%  │
└────────────┘ └────────────┘ └────────────┘ └────────────┘
```

**子组件拆分**：

| 组件 | 职责 | 数据源 |
|------|------|--------|
| `StatusPanelAgent.vue` | Agent 在线/离线统计（环形进度条） | `useOverviewPage.agentStats`（来自 `listAgentsPaged`） |
| `StatusPanelSession.vue` | 今日活跃会话数 + Sparkline | `useOverviewPage.sessionActiveCount` + `sessionSparkline`（来自 usage overview） |
| `StatusPanelProvider.vue` | Provider 活跃/降级统计 | `useOverviewPage.providerHealthSummary`（来自 `usePlatformStore.providerModels`） |
| `StatusPanelRunner.vue` | Runner 成功率/错误率/运行数 | `useOverviewPage.runnerStats`（来自 `useMonitorStore.runnerMetrics`） |

> **设计调整说明**：原设计为 `StatusPanelSystem.vue`（CPU/内存使用率），因后端 `RunnerMetricsSummary` 无 `cpu_percent`/`memory_percent` 字段，改为 `StatusPanelRunner.vue` 展示 Runner 运行成功率/错误率/总运行数/错误数。详见 [开发计划文档](./62-overview-command-center.development.md#2-现状评估)。

**容器组件 Props**：

| Prop | 类型 | 说明 |
|------|------|------|
| `agentStats` | `{ active: number; total: number }` | Agent 统计 |
| `sessionActiveCount` | `number` | 今日活跃会话数 |
| `sessionSparkline` | `number[]` | 24h 趋势数据点 |
| `providerHealth` | `{ active: number; degraded: number; total: number }` | Provider 健康 |
| `runnerStats` | `{ totalRuns: number; errorRuns: number; successRate: number; errorRate: number }` | Runner 运行统计 |
| `loading` | `boolean` | 整体加载态 |

**子面板 Props**：

| 组件 | Props |
|------|-------|
| `StatusPanelAgent` | `active: number; total: number; loading: boolean` |
| `StatusPanelSession` | `activeCount: number; sparkline: number[]; loading: boolean` |
| `StatusPanelProvider` | `active: number; degraded: number; total: number; loading: boolean` |
| `StatusPanelRunner` | `totalRuns: number; errorRuns: number; successRate: number; errorRate: number; loading: boolean` |

## 2. 数据流设计

### 2.1 Composable 层

升级 `useOverviewPage.ts`，新增以下数据获取逻辑（基于实际实现）：

```typescript
// useOverviewPage.ts 新增部分
import { listAgentsPaged } from '../agents/api';
import { listTeams } from '../teams/api';

export function useOverviewPage() {
  // ... 现有逻辑保留 ...

  // 新增：Agent 统计（仅查 total，不拉全量列表）
  const agentStats = ref({ active: 0, total: 0 });

  async function loadAgentStats() {
    try {
      const [activeResult, totalResult] = await Promise.all([
        listAgentsPaged({ status: 'active', limit: 1, offset: 0 }),
        listAgentsPaged({ limit: 1, offset: 0 }),
      ]);
      agentStats.value = {
        active: activeResult.total,
        total: totalResult.total,
      };
    } catch { /* silent */ }
  }

  // 新增：Provider 数量（去重统计 provider 编码）
  const providerCount = computed(() => {
    const seen = new Set<string>();
    for (const m of providerModels.value) {
      const code = m.provider ?? '';
      if (code) seen.add(code);
    }
    return seen.size;
  });

  // 新增：公司（组织）数量
  const categoryCount = ref(0);
  async function loadCategoryCount() {
    try {
      await platformStore.loadTaxonomyTree('organization');
      categoryCount.value = platformStore.taxonomyTree.length;
    } catch { /* silent */ }
  }

  // 新增：团队数量
  const teamCount = ref(0);
  async function loadTeamCount() {
    try {
      const teams = await listTeams();
      teamCount.value = teams.length;
    } catch { /* silent */ }
  }

  // 新增：用户名
  const username = computed(() => {
    try {
      const raw = localStorage.getItem('auth_user');
      if (raw) {
        const parsed = JSON.parse(raw);
        if (parsed.username) return parsed.username;
      }
    } catch { /* silent */ }
    return 'Admin';
  });

  // 新增：Provider 健康摘要
  const providerHealthSummary = computed(() => {
    const models = providerModels.value;
    const active = models.filter((m) => m.status === 'active' || !m.status).length;
    const degraded = models.filter((m) => m.status === 'degraded').length;
    return { active, degraded, total: models.length };
  });

  // 新增：会话活跃数 + Sparkline
  const sessionActiveCount = computed(() => overview.value?.today?.call_count ?? 0);
  const sessionSparkline = computed(() => {
    const trends = overview.value?.trends ?? [];
    return trends.slice(-24).map((t) => t.call_count ?? 0);
  });

  // 新增：Runner 统计（成功率/错误率转换为百分比）
  const runnerStats = computed(() => ({
    totalRuns: runnerMetrics.value?.total_runs ?? 0,
    errorRuns: runnerMetrics.value?.error_runs ?? 0,
    successRate: (runnerMetrics.value?.success_rate ?? 0) * 100,
    errorRate: (runnerMetrics.value?.error_rate ?? 0) * 100,
  }));

  // onMounted 中追加
  onMounted(() => {
    void loadAgentStats();
    void loadCategoryCount();
    void loadTeamCount();
  });

  return {
    // ... 现有返回值 ...
    agentStats,
    providerCount,
    categoryCount,
    teamCount,
    username,
    providerHealthSummary,
    sessionActiveCount,
    sessionSparkline,
    runnerStats,
  };
}
```

### 2.2 数据流合规检查

| 检查项 | 结果 |
|--------|------|
| 展示组件不 import Store | ✅ 新组件通过 props 接收数据 |
| 展示组件不直接调 API | ✅ 请求在 composable 中触发 |
| 状态收敛在 Store | ✅ 数据来自 useUsageStore/usePlatformStore/useMonitorStore |
| composable 组合 Store | ✅ useOverviewPage 组合多个 Store + 直接 API 调用（listAgentsPaged/listTeams） |

## 3. 组件分层

```
pages/OverviewPage.vue                    ← 页面容器
  ├── components/usage/
  │   ├── CommandCenterHero.vue           ← Hero 欢迎区（含 6 指标 + actions slot）
  │   ├── CommandCenterQuickActions.vue   ← 快捷入口区（4 卡片）
  │   ├── CommandCenterStatusPanels.vue   ← 状态面板行容器
  │   │   ├── StatusPanelAgent.vue        ← Agent 状态面板（环形进度条）
  │   │   ├── StatusPanelSession.vue      ← 会话活跃面板（大数字 + Sparkline）
  │   │   ├── StatusPanelProvider.vue     ← Provider 健康面板（状态灯）
  │   │   └── StatusPanelRunner.vue       ← Runner 运行面板（成功率/错误率仪表盘）
  │   ├── OverviewMonitorQuickLinks.vue   ← 保留（注入 Hero actions slot）
  │   ├── OverviewPageHero.vue            ← 保留文件但不再引用（被 CommandCenterHero 替代）
  │   ├── OverviewProviderHealth.vue      ← 保留在用量分析区
  │   ├── OverviewRunnerMetrics.vue       ← 保留在用量分析区
  │   ├── UsageMetricCards.vue            ← 保留
  │   ├── UsageTrendChart.vue             ← 保留（defineAsyncComponent）
  │   └── ... 其他现有组件保留
  features/usage/
  │   └── useOverviewPage.ts              ← 升级：新增 agentStats/providerCount/categoryCount/teamCount/username/providerHealthSummary/sessionActiveCount/sessionSparkline/runnerStats
```

## 4. CSS 设计

### 4.1 CSS 变量使用

新组件复用项目现有 UX 主题变量，不新增独立 CSS 变量文件：

| 变量类别 | 示例变量 | 用途 |
|---------|---------|------|
| 背景与边框 | `--color-background-elevated`、`--glass-border`、`--glass-border-hover` | 卡片背景与边框 |
| 强调色 | `--color-accent-indigo`、`--color-accent-blue`、`--color-accent-cyan`、`--color-accent-green`、`--color-accent` | 图标背景渐变 |
| 文本色 | `--color-text-primary`、`--color-text-secondary` | 文本颜色 |
| 状态色 | `--color-success`、`--color-warning`、`--color-danger` | 状态灯与仪表盘填充 |
| 间距 | `--space-4`、`--space-5` | 布局间距 |

所有变量定义在 `web/src/css/theme/_css-vars-light.sass` 与 `_css-vars-dark.sass` 中，Dark mode 通过 `body.body--dark` 选择器覆盖。

### 4.2 命名规范

所有新增 CSS class 遵循 BEM 命名，前缀 `command-center-`：

```css
.command-center-hero { ... }
.command-center-hero__greeting { ... }
.command-center-hero__stats { ... }
.command-center-hero__stat { ... }
.command-center-hero__stat-icon { ... }
.command-center-stat-panel { ... }
.command-center-stat-panel__ring { ... }
.command-center-stat-panel__gauge { ... }
.command-center-quick-actions { ... }
.command-center-quick-action { ... }
```

### 4.3 响应式断点

| 断点 | Hero 指标行 | 状态面板行 | 快捷入口区 |
|------|------------|-----------|-----------|
| ≥ 1200px | 6 列 | 4 列 | 4 列 |
| 600-1199px | 3 列 | 2 列 | 2 列 |
| < 600px | 2 列 | 1 列 | 1 列 |

## 5. 与现有组件的关系

| 现有组件 | 变更 |
|----------|------|
| `OverviewPageHero.vue` | 被 `CommandCenterHero.vue` 替换，原文件保留但不再在 OverviewPage 中引用 |
| `OverviewMonitorQuickLinks.vue` | 保留，注入 `CommandCenterHero` 的 `actions` slot |
| `OverviewProviderHealth.vue` | 保留在用量分析区，同时 `StatusPanelProvider.vue` 复用其数据源（`providerModels`） |
| `OverviewRunnerMetrics.vue` | 保留在用量分析区，同时 `StatusPanelRunner.vue` 复用其数据源（`runnerMetrics`） |
| `UsageMetricCards.vue` | 保留不变 |
| 其他现有组件 | 保留不变 |

## 6. 性能考虑

| 场景 | 策略 |
|------|------|
| Agent 统计获取 | 使用 `listAgentsPaged({ limit: 1 })` 仅取 `total` 字段，不拉全量列表 |
| 团队数量获取 | 使用 `listTeams()` 取数组长度 |
| 状态面板刷新 | 复用现有 Store 的数据，不新增独立轮询 |
| Sparkline 渲染 | 使用纯 SVG 迷你图，不引入图表库 |
| 异步组件 | `UsageTrendChart` 保持 `defineAsyncComponent` |
| 时间更新 | Hero 内部 `setInterval` 每 60s 更新一次，`onUnmounted` 清理 |
