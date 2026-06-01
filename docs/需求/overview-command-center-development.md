# Overview 指挥中心 — 开发计划

> **Goal:** 将 OverviewPage 从用量仪表盘升级为智能体指挥中心，新增 Hero 欢迎区、实时状态面板行、快捷入口区，保留现有用量分析功能。

> **Architecture:** 前端组件分层：Page → Composable → Store → API。新增 6 个展示组件 + 升级 1 个 composable，不新增后端 API。数据复用现有 Store（usage/platform/monitor/agents-catalog）。

> **Tech Stack:** Vue 3 + Quasar + Pinia + TypeScript

---

## Task 1: CommandCenterHero 组件

**Files:**
- Create: `web/src/components/usage/CommandCenterHero.vue`

- [ ] **Step 1: 创建 CommandCenterHero.vue**

```vue
<template>
  <section class="command-center-hero">
    <div class="command-center-hero__header">
      <div class="command-center-hero__greeting">
        <h1 class="command-center-hero__title">你好，{{ username }}</h1>
        <p class="command-center-hero__subtitle">
          <q-icon name="schedule" size="16px" class="q-mr-xs" />
          {{ currentTime }} · 系统已运行 {{ uptime }}
        </p>
      </div>
      <div class="command-center-hero__actions">
        <slot name="actions" />
      </div>
    </div>
    <div class="command-center-hero__stats">
      <div class="command-center-hero__stat">
        <div class="command-center-hero__stat-icon command-center-hero__stat-icon--agent">
          <q-icon name="smart_toy" size="20px" />
        </div>
        <div class="command-center-hero__stat-body">
          <div class="command-center-hero__stat-value">{{ activeAgentCount }}</div>
          <div class="command-center-hero__stat-label">活跃 Agent</div>
        </div>
      </div>
      <div class="command-center-hero__stat">
        <div class="command-center-hero__stat-icon command-center-hero__stat-icon--session">
          <q-icon name="chat" size="20px" />
        </div>
        <div class="command-center-hero__stat-body">
          <div class="command-center-hero__stat-value">{{ todaySessionCount }}</div>
          <div class="command-center-hero__stat-label">今日对话</div>
        </div>
      </div>
      <div class="command-center-hero__stat">
        <div class="command-center-hero__stat-icon command-center-hero__stat-icon--token">
          <q-icon name="data_usage" size="20px" />
        </div>
        <div class="command-center-hero__stat-body">
          <div class="command-center-hero__stat-value">{{ formattedTokenCount }}</div>
          <div class="command-center-hero__stat-label">今日 Token</div>
        </div>
      </div>
    </div>
    <div class="command-center-hero__quick-buttons">
      <q-btn unelevated no-caps class="command-center-hero__primary-btn" icon="chat" label="开始对话" :to="'/chat'" />
      <q-btn outline no-caps class="command-center-hero__secondary-btn" icon="warning_amber" label="查看告警" @click="$emit('view-alerts')" />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from "vue"
import { formatCount } from "../../features/usage/moneyFormat"

const props = defineProps<{
  username: string
  uptime: string
  activeAgentCount: number
  todaySessionCount: number
  todayTokenCount: number
}>()

defineEmits<{
  "view-alerts": []
}>()

const formattedTokenCount = computed(() => formatCount(props.todayTokenCount))

const currentTime = ref(formatTime())
let timer: ReturnType<typeof setInterval> | null = null

function formatTime() {
  return new Date().toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })
}

onMounted(() => {
  timer = setInterval(() => { currentTime.value = formatTime() }, 60_000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>
```

- [ ] **Step 2: 添加 CommandCenterHero 样式**

在 `CommandCenterHero.vue` 的 `<style>` 块中添加：

```sass
<style lang="sass">
.command-center-hero
  padding: 24px 0 16px

  &__header
    display: flex
    justify-content: space-between
    align-items: flex-start
    margin-bottom: 20px
    flex-wrap: wrap
    gap: 12px

  &__title
    font-size: 1.5rem
    font-weight: 600
    margin: 0 0 4px
    line-height: 1.3

  &__subtitle
    font-size: 0.85rem
    color: var(--q-secondary)
    margin: 0
    display: flex
    align-items: center

  &__stats
    display: flex
    gap: 16px
    margin-bottom: 16px
    flex-wrap: wrap

  &__stat
    display: flex
    align-items: center
    gap: 10px
    padding: 12px 16px
    border-radius: 10px
    background: var(--q-primary-paste, rgba(var(--q-primary-rgb), 0.06))
    flex: 1
    min-width: 160px

  &__stat-icon
    width: 40px
    height: 40px
    border-radius: 10px
    display: flex
    align-items: center
    justify-content: center
    color: white
    &--agent
      background: linear-gradient(135deg, #6366f1, #8b5cf6)
    &--session
      background: linear-gradient(135deg, #0ea5e9, #06b6d4)
    &--token
      background: linear-gradient(135deg, #f59e0b, #f97316)

  &__stat-value
    font-size: 1.4rem
    font-weight: 700
    line-height: 1.2

  &__stat-label
    font-size: 0.8rem
    color: var(--q-secondary)

  &__quick-buttons
    display: flex
    gap: 10px
    flex-wrap: wrap

  &__primary-btn
    background: var(--color-accent, var(--q-primary)) !important
    color: white !important
    border-radius: 8px
    padding: 8px 20px

  &__secondary-btn
    border-radius: 8px
    padding: 8px 20px
    color: var(--q-secondary)
</style>
```

---

## Task 2: StatusPanelAgent 组件

**Files:**
- Create: `web/src/components/usage/StatusPanelAgent.vue`

- [ ] **Step 1: 创建 StatusPanelAgent.vue**

```vue
<template>
  <q-card flat class="command-center-stat-panel">
    <q-card-section class="q-pb-sm">
      <div class="command-center-stat-panel__header">
        <q-icon name="smart_toy" size="16px" class="command-center-stat-panel__icon" />
        <span class="command-center-stat-panel__title">Agent 状态</span>
      </div>
    </q-card-section>
    <q-card-section class="q-pt-none">
      <div v-if="loading" class="row justify-center q-py-md">
        <q-skeleton type="circle" size="64px" />
      </div>
      <template v-else>
        <div class="command-center-stat-panel__ring-wrap">
          <svg class="command-center-stat-panel__ring" viewBox="0 0 36 36">
            <circle class="command-center-stat-panel__ring-track" cx="18" cy="18" r="15.9" fill="none" stroke-width="3" />
            <circle class="command-center-stat-panel__ring-fill" cx="18" cy="18" r="15.9" fill="none" stroke-width="3"
              :stroke-dasharray="`${activePercent} ${100 - activePercent}`"
              stroke-dashoffset="25"
            />
          </svg>
          <div class="command-center-stat-panel__ring-text">
            <span class="command-center-stat-panel__ring-value">{{ active }}</span>
            <span class="command-center-stat-panel__ring-label">/ {{ total }}</span>
          </div>
        </div>
        <div class="command-center-stat-panel__detail">
          <span class="command-center-stat-panel__dot command-center-stat-panel__dot--active" />
          <span>在线 {{ active }}</span>
          <span class="command-center-stat-panel__dot command-center-stat-panel__dot--inactive q-ml-sm" />
          <span>离线 {{ total - active }}</span>
        </div>
      </template>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from "vue"

const props = defineProps<{
  active: number
  total: number
  loading: boolean
}>()

const activePercent = computed(() => {
  if (props.total === 0) return 0
  return Math.round((props.active / props.total) * 100)
})
</script>
```

- [ ] **Step 2: 添加 StatusPanelAgent 样式**

```sass
<style lang="sass">
.command-center-stat-panel
  &__header
    display: flex
    align-items: center
    gap: 6px
  &__icon
    color: var(--q-primary)
  &__title
    font-size: 0.85rem
    font-weight: 500
  &__ring-wrap
    position: relative
    width: 80px
    height: 80px
    margin: 0 auto 8px
  &__ring
    width: 100%
    height: 100%
    transform: rotate(-90deg)
  &__ring-track
    stroke: rgba(var(--q-primary-rgb), 0.12)
  &__ring-fill
    stroke: var(--q-primary)
    stroke-linecap: round
    transition: stroke-dasharray 0.6s ease
  &__ring-text
    position: absolute
    top: 50%
    left: 50%
    transform: translate(-50%, -50%)
    text-align: center
  &__ring-value
    font-size: 1.1rem
    font-weight: 700
    display: block
    line-height: 1.2
  &__ring-label
    font-size: 0.7rem
    color: var(--q-secondary)
  &__detail
    font-size: 0.75rem
    color: var(--q-secondary)
    display: flex
    align-items: center
    justify-content: center
    gap: 4px
  &__dot
    width: 8px
    height: 8px
    border-radius: 50%
    display: inline-block
    &--active
      background: #22c55e
    &--inactive
      background: #94a3b8
</style>
```

---

## Task 3: StatusPanelSession 组件

**Files:**
- Create: `web/src/components/usage/StatusPanelSession.vue`

- [ ] **Step 1: 创建 StatusPanelSession.vue**

```vue
<template>
  <q-card flat class="command-center-stat-panel">
    <q-card-section class="q-pb-sm">
      <div class="command-center-stat-panel__header">
        <q-icon name="chat" size="16px" class="command-center-stat-panel__icon command-center-stat-panel__icon--session" />
        <span class="command-center-stat-panel__title">会话活跃</span>
      </div>
    </q-card-section>
    <q-card-section class="q-pt-none">
      <div v-if="loading" class="row justify-center q-py-md">
        <q-skeleton type="text" width="60px" />
      </div>
      <template v-else>
        <div class="command-center-stat-panel__big-value">{{ activeCount }}</div>
        <div class="command-center-stat-panel__caption">当前活跃会话</div>
        <div v-if="sparkline.length > 1" class="command-center-stat-panel__sparkline">
          <svg viewBox="0 0 100 24" preserveAspectRatio="none" class="command-center-stat-panel__sparkline-svg">
            <polyline :points="sparklinePoints" fill="none" stroke="var(--q-primary)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </div>
      </template>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from "vue"

const props = defineProps<{
  activeCount: number
  sparkline: number[]
  loading: boolean
}>()

const sparklinePoints = computed(() => {
  if (props.sparkline.length < 2) return ""
  const max = Math.max(...props.sparkline, 1)
  return props.sparkline
    .map((v, i) => {
      const x = (i / (props.sparkline.length - 1)) * 100
      const y = 24 - (v / max) * 20
      return `${x},${y}`
    })
    .join(" ")
})
</script>
```

- [ ] **Step 2: 添加 StatusPanelSession 样式**

```sass
<style lang="sass">
.command-center-stat-panel
  &__icon--session
    color: #0ea5e9
  &__big-value
    font-size: 2rem
    font-weight: 700
    line-height: 1.2
    text-align: center
  &__caption
    font-size: 0.75rem
    color: var(--q-secondary)
    text-align: center
    margin-bottom: 8px
  &__sparkline
    height: 24px
    margin-top: 4px
  &__sparkline-svg
    width: 100%
    height: 100%
</style>
```

---

## Task 4: StatusPanelProvider 组件

**Files:**
- Create: `web/src/components/usage/StatusPanelProvider.vue`

- [ ] **Step 1: 创建 StatusPanelProvider.vue**

```vue
<template>
  <q-card flat class="command-center-stat-panel">
    <q-card-section class="q-pb-sm">
      <div class="command-center-stat-panel__header">
        <q-icon name="dns" size="16px" class="command-center-stat-panel__icon command-center-stat-panel__icon--provider" />
        <span class="command-center-stat-panel__title">Provider 健康</span>
      </div>
    </q-card-section>
    <q-card-section class="q-pt-none">
      <div v-if="loading" class="row justify-center q-py-md">
        <q-skeleton type="text" width="60px" />
      </div>
      <template v-else>
        <div class="command-center-stat-panel__health-row">
          <span class="command-center-stat-panel__health-dot command-center-stat-panel__health-dot--ok" />
          <span class="command-center-stat-panel__health-label">活跃</span>
          <span class="command-center-stat-panel__health-value">{{ active }}</span>
        </div>
        <div class="command-center-stat-panel__health-row">
          <span class="command-center-stat-panel__health-dot command-center-stat-panel__health-dot--degraded" />
          <span class="command-center-stat-panel__health-label">降级</span>
          <span class="command-center-stat-panel__health-value command-center-stat-panel__health-value--danger">{{ degraded }}</span>
        </div>
        <div class="command-center-stat-panel__health-row">
          <span class="command-center-stat-panel__health-label">总计</span>
          <span class="command-center-stat-panel__health-value">{{ total }}</span>
        </div>
        <div v-if="degraded > 0" class="command-center-stat-panel__health-warn">
          <q-icon name="warning_amber" size="14px" color="warning" />
          <span>{{ degraded }} 个模型降级</span>
        </div>
      </template>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
defineProps<{
  active: number
  degraded: number
  total: number
  loading: boolean
}>()
</script>
```

- [ ] **Step 2: 添加 StatusPanelProvider 样式**

```sass
<style lang="sass">
.command-center-stat-panel
  &__icon--provider
    color: #22c55e
  &__health-row
    display: flex
    align-items: center
    gap: 6px
    font-size: 0.85rem
    padding: 2px 0
  &__health-dot
    width: 8px
    height: 8px
    border-radius: 50%
    &--ok
      background: #22c55e
    &--degraded
      background: #f59e0b
  &__health-label
    color: var(--q-secondary)
    flex: 1
  &__health-value
    font-weight: 600
    &--danger
      color: #ef4444
  &__health-warn
    display: flex
    align-items: center
    gap: 4px
    font-size: 0.75rem
    color: #f59e0b
    margin-top: 6px
</style>
```

---

## Task 5: StatusPanelSystem 组件

**Files:**
- Create: `web/src/components/usage/StatusPanelSystem.vue`

- [ ] **Step 1: 创建 StatusPanelSystem.vue**

```vue
<template>
  <q-card flat class="command-center-stat-panel">
    <q-card-section class="q-pb-sm">
      <div class="command-center-stat-panel__header">
        <q-icon name="monitor_heart" size="16px" class="command-center-stat-panel__icon command-center-stat-panel__icon--system" />
        <span class="command-center-stat-panel__title">系统负载</span>
      </div>
    </q-card-section>
    <q-card-section class="q-pt-none">
      <div v-if="loading" class="row justify-center q-py-md">
        <q-skeleton type="text" width="60px" />
      </div>
      <template v-else>
        <div class="command-center-stat-panel__gauge">
          <div class="command-center-stat-panel__gauge-item">
            <div class="command-center-stat-panel__gauge-bar">
              <div class="command-center-stat-panel__gauge-fill" :class="cpuClass" :style="{ width: `${cpuPercent}%` }" />
            </div>
            <div class="command-center-stat-panel__gauge-info">
              <span>CPU</span>
              <span class="command-center-stat-panel__gauge-value">{{ cpuPercent }}%</span>
            </div>
          </div>
          <div class="command-center-stat-panel__gauge-item">
            <div class="command-center-stat-panel__gauge-bar">
              <div class="command-center-stat-panel__gauge-fill" :class="memClass" :style="{ width: `${memPercent}%` }" />
            </div>
            <div class="command-center-stat-panel__gauge-info">
              <span>MEM</span>
              <span class="command-center-stat-panel__gauge-value">{{ memPercent }}%</span>
            </div>
          </div>
        </div>
      </template>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from "vue"

const props = defineProps<{
  cpuPercent: number
  memPercent: number
  loading: boolean
}>()

function gaugeClass(val: number) {
  if (val >= 90) return "command-center-stat-panel__gauge-fill--danger"
  if (val >= 70) return "command-center-stat-panel__gauge-fill--warn"
  return "command-center-stat-panel__gauge-fill--ok"
}

const cpuClass = computed(() => gaugeClass(props.cpuPercent))
const memClass = computed(() => gaugeClass(props.memPercent))
</script>
```

- [ ] **Step 2: 添加 StatusPanelSystem 样式**

```sass
<style lang="sass">
.command-center-stat-panel
  &__icon--system
    color: #8b5cf6
  &__gauge
    display: flex
    flex-direction: column
    gap: 10px
  &__gauge-item
    display: flex
    flex-direction: column
    gap: 4px
  &__gauge-bar
    height: 6px
    border-radius: 3px
    background: rgba(var(--q-primary-rgb), 0.1)
    overflow: hidden
  &__gauge-fill
    height: 100%
    border-radius: 3px
    transition: width 0.6s ease
    &--ok
      background: #22c55e
    &--warn
      background: #f59e0b
    &--danger
      background: #ef4444
  &__gauge-info
    display: flex
    justify-content: space-between
    font-size: 0.75rem
    color: var(--q-secondary)
  &__gauge-value
    font-weight: 600
</style>
```

---

## Task 6: CommandCenterStatusPanels 容器组件

**Files:**
- Create: `web/src/components/usage/CommandCenterStatusPanels.vue`

- [ ] **Step 1: 创建 CommandCenterStatusPanels.vue**

```vue
<template>
  <div class="command-center-status-panels">
    <StatusPanelAgent :active="agentStats.active" :total="agentStats.total" :loading="loading" />
    <StatusPanelSession :active-count="sessionActiveCount" :sparkline="sessionSparkline" :loading="loading" />
    <StatusPanelProvider :active="providerHealth.active" :degraded="providerHealth.degraded" :total="providerHealth.total" :loading="loading" />
    <StatusPanelSystem :cpu-percent="systemLoad.cpuPercent" :mem-percent="systemLoad.memPercent" :loading="loading" />
  </div>
</template>

<script setup lang="ts">
import StatusPanelAgent from "./StatusPanelAgent.vue"
import StatusPanelSession from "./StatusPanelSession.vue"
import StatusPanelProvider from "./StatusPanelProvider.vue"
import StatusPanelSystem from "./StatusPanelSystem.vue"

defineProps<{
  agentStats: { active: number; total: number }
  sessionActiveCount: number
  sessionSparkline: number[]
  providerHealth: { active: number; degraded: number; total: number }
  systemLoad: { cpuPercent: number; memPercent: number }
  loading: boolean
}>()
</script>
```

- [ ] **Step 2: 添加容器样式**

```sass
<style lang="sass">
.command-center-status-panels
  display: grid
  grid-template-columns: repeat(4, 1fr)
  gap: 12px
  margin-bottom: 16px

  @media (max-width: 1023px)
    grid-template-columns: repeat(2, 1fr)

  @media (max-width: 599px)
    grid-template-columns: 1fr
</style>
```

---

## Task 7: CommandCenterQuickActions 组件

**Files:**
- Create: `web/src/components/usage/CommandCenterQuickActions.vue`

- [ ] **Step 1: 创建 CommandCenterQuickActions.vue**

```vue
<template>
  <div class="command-center-quick-actions">
    <router-link v-for="action in actions" :key="action.to" :to="action.to" class="command-center-quick-action">
      <div class="command-center-quick-action__icon" :class="action.iconClass">
        <q-icon :name="action.icon" size="24px" />
      </div>
      <div class="command-center-quick-action__body">
        <div class="command-center-quick-action__title">{{ action.title }}</div>
        <div class="command-center-quick-action__desc">{{ action.desc }}</div>
      </div>
      <q-icon name="chevron_right" size="18px" class="command-center-quick-action__arrow" />
    </router-link>
  </div>
</template>

<script setup lang="ts">
const actions = [
  { to: "/chat", icon: "chat", iconClass: "command-center-quick-action__icon--chat", title: "新建对话", desc: "与 AI 智能体对话" },
  { to: "/agents", icon: "smart_toy", iconClass: "command-center-quick-action__icon--agent", title: "创建 Agent", desc: "配置新的智能体" },
  { to: "/cron", icon: "schedule", iconClass: "command-center-quick-action__icon--cron", title: "定时任务", desc: "管理自动化任务" },
  { to: "/settings", icon: "settings", iconClass: "command-center-quick-action__icon--settings", title: "系统设置", desc: "系统参数配置" }
]
</script>
```

- [ ] **Step 2: 添加快捷入口样式**

```sass
<style lang="sass">
.command-center-quick-actions
  display: grid
  grid-template-columns: repeat(4, 1fr)
  gap: 12px
  margin-top: 16px

  @media (max-width: 1023px)
    grid-template-columns: repeat(2, 1fr)

  @media (max-width: 599px)
    grid-template-columns: repeat(2, 1fr)

.command-center-quick-action
  display: flex
  align-items: center
  gap: 12px
  padding: 14px 16px
  border-radius: 10px
  background: var(--q-primary-paste, rgba(var(--q-primary-rgb), 0.04))
  text-decoration: none
  color: inherit
  transition: background 0.2s, box-shadow 0.2s
  cursor: pointer

  &:hover
    background: var(--q-primary-paste, rgba(var(--q-primary-rgb), 0.1))
    box-shadow: 0 2px 8px rgba(0,0,0,0.06)

  &__icon
    width: 44px
    height: 44px
    border-radius: 10px
    display: flex
    align-items: center
    justify-content: center
    color: white
    flex-shrink: 0
    &--chat
      background: linear-gradient(135deg, #0ea5e9, #06b6d4)
    &--agent
      background: linear-gradient(135deg, #6366f1, #8b5cf6)
    &--cron
      background: linear-gradient(135deg, #f59e0b, #f97316)
    &--settings
      background: linear-gradient(135deg, #64748b, #94a3b8)

  &__body
    flex: 1
    min-width: 0

  &__title
    font-size: 0.9rem
    font-weight: 600
    line-height: 1.3

  &__desc
    font-size: 0.75rem
    color: var(--q-secondary)
    line-height: 1.3

  &__arrow
    color: var(--q-secondary)
    flex-shrink: 0
</style>
```

---

## Task 8: 升级 useOverviewPage composable

**Files:**
- Modify: `web/src/features/usage/useOverviewPage.ts`

- [ ] **Step 1: 新增 agentStats / username / uptime / systemLoad / providerHealthSummary / sessionActiveCount / sessionSparkline**

在 `useOverviewPage.ts` 中追加以下逻辑：

```typescript
import { useAgentsCatalogStore } from "../../stores/agents/catalog"

// 在函数体内新增：
const agentsCatalogStore = useAgentsCatalogStore()
const agentStats = ref({ active: 0, total: 0 })

async function loadAgentStats() {
  try {
    const list = await agentsCatalogStore.fetchAgents({ limit: 1000 })
    agentStats.value = {
      active: list.filter((a: any) => a.status === "active" || !a.status).length,
      total: list.length
    }
  } catch { /* silent */ }
}

const username = computed(() => {
  try {
    const raw = localStorage.getItem("auth_user")
    if (raw) return JSON.parse(raw).username || "Admin"
  } catch { /* silent */ }
  return "Admin"
})

const uptime = computed(() => {
  const s = runnerMetrics.value?.uptime_seconds
  if (!s) return "--"
  const d = Math.floor(s / 86400)
  const h = Math.floor((s % 86400) / 3600)
  if (d > 0) return `${d} 天 ${h} 小时`
  const m = Math.floor((s % 3600) / 60)
  return `${h} 小时 ${m} 分钟`
})

const systemLoad = computed(() => ({
  cpuPercent: Math.round(runnerMetrics.value?.cpu_percent ?? 0),
  memPercent: Math.round(runnerMetrics.value?.memory_percent ?? 0)
}))

const providerHealthSummary = computed(() => {
  const models = providerModels.value
  const active = models.filter((m: any) => m.status === "active" || !m.status).length
  const degraded = models.filter((m: any) => m.status === "degraded").length
  return { active, degraded, total: models.length }
})

const sessionActiveCount = computed(() => overview.value?.today?.call_count ?? 0)

const sessionSparkline = computed(() => {
  const trends = overview.value?.trends ?? []
  return trends.slice(-24).map((t: any) => t.call_count ?? 0)
})

// onMounted 追加
onMounted(() => {
  void loadAgentStats()
})

// return 追加
return {
  // ... 现有返回值 ...
  agentStats,
  username,
  uptime,
  systemLoad,
  providerHealthSummary,
  sessionActiveCount,
  sessionSparkline,
}
```

---

## Task 9: 重构 OverviewPage 集成新组件

**Files:**
- Modify: `web/src/pages/OverviewPage.vue`

- [ ] **Step 1: 替换 Hero + 新增状态面板 + 新增快捷入口**

将 OverviewPage.vue 的 template 更新为：

```vue
<template>
  <q-page class="app-standard-page overview-page">
    <div class="overview-page__shell">
      <CommandCenterHero
        :username="username"
        :uptime="uptime"
        :active-agent-count="agentStats.active"
        :today-session-count="sessionActiveCount"
        :today-token-count="overview?.today?.total_tokens ?? 0"
        @view-alerts="scrollToAlerts"
      >
        <template #actions>
          <OverviewMonitorQuickLinks />
          <q-btn
            unelevated
            no-caps
            class="overview-primary-btn"
            icon="receipt_long"
            label="查看明细"
            :to="eventsPageQuery"
          />
        </template>
      </CommandCenterHero>

      <CommandCenterStatusPanels
        :agent-stats="agentStats"
        :session-active-count="sessionActiveCount"
        :session-sparkline="sessionSparkline"
        :provider-health="providerHealthSummary"
        :system-load="systemLoad"
        :loading="loading"
      />

      <div class="overview-filter-bar">
        <!-- 现有筛选栏保持不变 -->
        ...
      </div>

      <div class="overview-content" :class="{ 'overview-content--loading': loading }">
        <!-- 现有内容区保持不变 -->
        ...

        <div class="overview-alert-stack" ref="alertStackRef">
          ...
        </div>
      </div>

      <CommandCenterQuickActions />
    </div>
  </q-page>
</template>
```

- [ ] **Step 2: 更新 script setup**

```typescript
import CommandCenterHero from "../components/usage/CommandCenterHero.vue"
import CommandCenterStatusPanels from "../components/usage/CommandCenterStatusPanels.vue"
import CommandCenterQuickActions from "../components/usage/CommandCenterQuickActions.vue"

// 移除 OverviewPageHero 的 import（保留 OverviewMonitorQuickLinks）

const alertStackRef = ref<HTMLElement | null>(null)

function scrollToAlerts() {
  alertStackRef.value?.scrollIntoView({ behavior: "smooth" })
}

// 从 useOverviewPage 解构新增字段
const {
  // ... 现有解构 ...
  agentStats,
  username,
  uptime,
  systemLoad,
  providerHealthSummary,
  sessionActiveCount,
  sessionSparkline,
} = useOverviewPage()
```

---

## Task 10: 验证与收尾

- [ ] **Step 1: 运行前端 lint**

```bash
cd web && pnpm lint
```

- [ ] **Step 2: 运行前端类型检查**

```bash
cd web && pnpm build
```

- [ ] **Step 3: 手动验证**

1. 访问 `/overview`，确认 Hero 欢迎区显示用户名、时间、3 个核心指标
2. 确认 4 个状态面板正确展示数据
3. 确认快捷入口可点击跳转
4. 确认筛选栏和用量分析区功能不受影响
5. 切换 Dark mode，确认所有新组件样式正确
6. 缩小浏览器窗口，确认响应式布局正确

- [ ] **Step 4: 更新文档**

更新 `docs/需求/overview-command-center.md` 标记已完成需求。
