# Overview 指挥中心 — 设计文档

## 1. 页面布局设计

### 1.1 整体结构

升级后的 OverviewPage 自上而下分为 6 个区域：

```
┌─────────────────────────────────────────────────────────┐
│  ① Hero 欢迎区（替换原 OverviewPageHero）                │
│  问候语 + 3 核心指标 + 快捷操作按钮                       │
├─────────────────────────────────────────────────────────┤
│  ② 实时状态面板行（新增）                                 │
│  [Agent状态] [会话活跃] [Provider健康] [系统负载]          │
├─────────────────────────────────────────────────────────┤
│  ③ 筛选栏（保留）                                        │
│  时间范围 | Provider | 模型 | 状态 | 趋势粒度             │
├─────────────────────────────────────────────────────────┤
│  ④ 用量分析区（保留，原指标卡片+趋势+分布+排行+告警）      │
│  MetricCards → 趋势图+摘要 → 分布+Provider → 排行 →      │
│  Runner指标 → 告警栈                                     │
├─────────────────────────────────────────────────────────┤
│  ⑤ 快捷入口区（新增）                                    │
│  [新建对话] [创建Agent] [定时任务] [系统设置]              │
└─────────────────────────────────────────────────────────┘
```

### 1.2 区域详细设计

#### ① Hero 欢迎区

**组件**：`CommandCenterHero.vue`（替换 `OverviewPageHero.vue`）

```
┌──────────────────────────────────────────────────────────┐
│  你好，Admin                                    🌙 12:34  │
│  系统已运行 3 天 2 小时                                   │
│                                                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐               │
│  │ 活跃Agent │  │ 今日对话  │  │ 今日Token │               │
│  │    5     │  │    128   │  │  52.3K   │               │
│  └──────────┘  └──────────┘  └──────────┘               │
│                                                          │
│  [💬 开始对话]  [⚠️ 查看告警]                             │
└──────────────────────────────────────────────────────────┘
```

**Props**：

| Prop | 类型 | 说明 |
|------|------|------|
| `username` | `string` | 当前登录用户名 |
| `uptime` | `string` | 系统运行时长（格式化后） |
| `activeAgentCount` | `number` | 活跃 Agent 数 |
| `todaySessionCount` | `number` | 今日对话数 |
| `todayTokenCount` | `number` | 今日 Token 消耗 |

**Emits**：

| Event | Payload | 说明 |
|-------|---------|------|
| `start-chat` | — | 点击"开始对话"按钮 |
| `view-alerts` | — | 点击"查看告警"按钮 |

**数据来源**：
- `username`：从 auth store 或 localStorage 获取
- `uptime`：从 monitor store 的 runnerMetrics 获取
- `activeAgentCount`：从 agents catalog store 获取（统计 status=active 的 Agent）
- `todaySessionCount`：从 usage store 的 overview.today.call_count 获取
- `todayTokenCount`：从 usage store 的 overview.today.total_tokens 获取

#### ② 实时状态面板行

**组件**：`CommandCenterStatusPanels.vue`（容器）+ 4 个子面板组件

```
┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐
│  Agent 状态 │ │  会话活跃   │ │ Provider健康│ │  系统负载   │
│   ◔ 5/8    │ │   12       │ │  ● 3/4     │ │  CPU 23%   │
│  在线5 离线3│ │  ≈≈≈≈≈    │ │  降级:1     │ │  MEM 45%   │
└────────────┘ └────────────┘ └────────────┘ └────────────┘
```

**子组件拆分**：

| 组件 | 职责 | 数据源 |
|------|------|--------|
| `StatusPanelAgent.vue` | Agent 在线/离线/忙碌统计 | `useAgentsCatalogStore.fetchAgents()` |
| `StatusPanelSession.vue` | 活跃会话数 + Sparkline | `useSessionStore.loadSessions({limit:1})` + usage overview |
| `StatusPanelProvider.vue` | Provider 可用/降级统计 | `usePlatformStore.providerModels` |
| `StatusPanelSystem.vue` | CPU/内存使用率 | `useMonitorStore.runnerMetrics` |

**统一 Props 接口**（每个子面板遵循）：

```typescript
interface StatusPanelProps {
  loading: boolean
}
```

**容器组件 Props**：

| Prop | 类型 | 说明 |
|------|------|------|
| `agentCount` | `{ active: number; total: number }` | Agent 统计 |
| `sessionCount` | `number` | 活跃会话数 |
| `providerHealth` | `{ active: number; degraded: number; total: number }` | Provider 健康 |
| `systemLoad` | `{ cpuPercent: number; memPercent: number }` | 系统负载 |
| `loading` | `boolean` | 整体加载态 |

#### ⑤ 快捷入口区

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

## 2. 数据流设计

### 2.1 Composable 层

升级 `useOverviewPage.ts`，新增以下数据获取逻辑：

```typescript
// useOverviewPage.ts 新增部分
import { useAgentsCatalogStore } from "../../stores/agents/catalog"

export function useOverviewPage() {
  // ... 现有逻辑保留 ...

  // 新增：Agent 统计
  const agentsCatalogStore = useAgentsCatalogStore()
  const agentStats = ref({ active: 0, total: 0 })

  async function loadAgentStats() {
    try {
      const agents = await agentsCatalogStore.fetchAgents({ limit: 1000 })
      agentStats.value = {
        active: agents.filter(a => a.status === 'active').length,
        total: agents.length
      }
    } catch { /* silent */ }
  }

  // 新增：用户名
  const username = computed(() => {
    // 从 localStorage 或 auth store 获取
    return localStorage.getItem('username') || 'Admin'
  })

  // 新增：系统运行时长
  const uptime = computed(() => {
    if (!runnerMetrics.value) return '--'
    return formatUptime(runnerMetrics.value.uptime_seconds)
  })

  // 新增：系统负载
  const systemLoad = computed(() => ({
    cpuPercent: runnerMetrics.value?.cpu_percent ?? 0,
    memPercent: runnerMetrics.value?.memory_percent ?? 0
  }))

  // onMounted 中追加
  onMounted(() => {
    void loadAgentStats()
  })

  return {
    // ... 现有返回值 ...
    agentStats,
    username,
    uptime,
    systemLoad,
  }
}
```

### 2.2 数据流合规检查

| 检查项 | 结果 |
|--------|------|
| 展示组件不 import Store | ✅ 新组件通过 props 接收数据 |
| 展示组件不直接调 API | ✅ 请求在 composable 中触发 |
| 状态收敛在 Store | ✅ 数据来自 useUsageStore/usePlatformStore/useMonitorStore/useAgentsCatalogStore |
| composable 组合 Store | ✅ useOverviewPage 组合多个 Store |

## 3. 组件分层

```
pages/OverviewPage.vue                    ← 页面容器
  ├── components/usage/
  │   ├── CommandCenterHero.vue           ← 新增：Hero 欢迎区
  │   ├── CommandCenterStatusPanels.vue   ← 新增：状态面板行容器
  │   │   ├── StatusPanelAgent.vue        ← 新增：Agent 状态面板
  │   │   ├── StatusPanelSession.vue      ← 新增：会话活跃面板
  │   │   ├── StatusPanelProvider.vue     ← 新增：Provider 健康面板
  │   │   └── StatusPanelSystem.vue       ← 新增：系统负载面板
  │   ├── CommandCenterQuickActions.vue   ← 新增：快捷入口区
  │   ├── OverviewPageHero.vue            ← 保留（降级为可选，新 Hero 替代）
  │   ├── UsageMetricCards.vue            ← 保留
  │   ├── UsageTrendChart.vue             ← 保留
  │   ├── ... 其他现有组件保留
  features/usage/
  │   └── useOverviewPage.ts              ← 升级：新增 agentStats/username/uptime/systemLoad
```

## 4. CSS 设计

### 4.1 新增 CSS 变量

```sass
// 在 css/theme/_css-vars-light.sass 和 _css-vars-dark.sass 中新增
--command-center-hero-bg: ...
--command-center-stat-ring-track: ...
--command-center-stat-ring-fill: ...
--command-center-quick-card-bg: ...
--command-center-quick-card-hover: ...
```

### 4.2 命名规范

所有新增 CSS class 遵循 BEM 命名，前缀 `command-center-`：

```css
.command-center-hero { ... }
.command-center-hero__greeting { ... }
.command-center-hero__stats { ... }
.command-center-hero__stat { ... }
.command-center-stat-panel { ... }
.command-center-stat-panel__ring { ... }
.command-center-quick-actions { ... }
.command-center-quick-action { ... }
```

### 4.3 响应式断点

| 断点 | 状态面板行 | 快捷入口区 |
|------|-----------|-----------|
| ≥ 1024px | 4 列 | 4 列 |
| 600-1023px | 2 列 | 2 列 |
| < 600px | 1 列 | 2 列 |

## 5. 与现有组件的关系

| 现有组件 | 变更 |
|----------|------|
| `OverviewPageHero.vue` | 被 `CommandCenterHero.vue` 替换，原文件保留但不再在 OverviewPage 中引用 |
| `OverviewMonitorQuickLinks.vue` | 移入 `CommandCenterHero.vue` 的 actions 插槽 |
| `OverviewProviderHealth.vue` | 保留在用量分析区，同时 `StatusPanelProvider.vue` 复用其数据源 |
| `OverviewRunnerMetrics.vue` | 保留在用量分析区，同时 `StatusPanelSystem.vue` 复用其数据源 |
| `UsageMetricCards.vue` | 保留不变 |
| 其他现有组件 | 保留不变 |

## 6. 性能考虑

| 场景 | 策略 |
|------|------|
| Agent 列表获取 | 使用 `useAgentsCatalogStore.fetchAgents({ limit: 1000 })`，仅获取摘要字段 |
| 状态面板刷新 | 复用现有 Store 的数据，不新增独立轮询 |
| Sparkline 渲染 | 使用纯 CSS/SVG 迷你图，不引入图表库 |
| 异步组件 | `UsageTrendChart` 和 `UsageBreakdownCharts` 保持 defineAsyncComponent |
