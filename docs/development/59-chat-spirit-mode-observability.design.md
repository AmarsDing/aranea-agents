# M59-OBS: Chat 精灵模式可观测性 UX — 实现设计

> 对应需求：[59-chat-spirit-mode-observability.md](./59-chat-spirit-mode-observability.md)
> 调研基础：[2026-06-08-research-chat-observability-ux.md](../reports/2026-06-08-research-chat-observability-ux.md)
> 遵循：[aranea-frontend-guide](../../.trae/skills/aranea-frontend-guide.md) · [59-chat-spirit-mode.design.md](./59-chat-spirit-mode.design.md)

---

## 一、模块概述

### 1.1 设计定位

在 M59 精灵模式现有骨架上，增加 7 项可观测性 UX 增强，遵循"可观测性强但不影响主要内容显示"原则。所有方案基于三层可观测性架构（L1 环境层 → L2 结构层 → L3 证据层），核心模式为"完成即折叠"。

### 1.2 分层与依赖

```
web/src/
  features/chat/
    groupMessagesByTurn.ts           ← 扩展 TurnBlockGroup 增加 isCompleted
    streamHandlers.ts                ← 扩展 tool_call/tool_result 处理逻辑
  features/spirit/
    types.ts                         ← 新增 AgentNodeStatusLabel 联合类型
    spiritUi.ts                      ← 新增 agentNodeStatusToLabel() 聚合映射
    observabilityConstants.ts        ← 新增：语境消息映射表、脉冲颜色配置
  composables/chat/
    useContextualLoadingMessage.ts   ← 新增：OBS-02 语境加载消息 composable
    useAutoCollapse.ts               ← 新增：OBS-01/OBS-06 自动折叠 composable
    useStatusPulse.ts                ← 新增：OBS-05 侧边栏脉冲 composable
  components/spirit/
    AgentStatusLabel.vue             ← 新增：OBS-03 Agent 状态标签组件
    SpiritStatusBar.vue              ← 新增：OBS-04 底部状态栏组件
    InterruptedTeamCard.vue          ← 新增：OBS-07 中断恢复提示卡片
    TeamTaskCard.vue                 ← 修改：增加 AgentStatusLabel + 脉冲动画
    TaskExecutionPanel.vue           ← 修改：集成 ParallelTeamOverview + AgentStatusLabel + InterruptedTeamCard
    ParallelTeamOverview.vue         ← 修改：集成到 TaskExecutionPanel
  components/chat/
    ChatExecutionCard.vue            ← 修改：增加自动折叠逻辑
    ChatMessagePanel.vue             ← 修改：集成 SpiritStatusBar + 语境加载消息
    ChatEntitySidebar.vue            ← 修改：集成 useStatusPulse
```

**红线**：
- 展示组件不 import Store（props/emits 通信）
- 展示组件不直接调 API（Store action 中调用）
- Composable 不持有 UI 状态（只提供逻辑和响应式数据）

---

## 二、核心设计

### 2.1 OBS-01 对话流自动折叠

#### 数据结构扩展

`TurnBlockGroup` 需增加 `isCompleted` 计算属性：

```typescript
// web/src/features/chat/groupMessagesByTurn.ts
export type TurnBlockGroup = {
  key: number;
  turnId: string;
  user: Message | null;
  assistant: Message | null;
  tools: Message[];
  members: Message[];
  // 新增
  isCompleted: boolean;  // true = block 内所有工具调用均 completed/failed/interrupted，且 assistant 消息已到达
};
```

**`isCompleted` 判断逻辑**：

```typescript
function computeBlockCompleted(block: TurnBlockGroup): boolean {
  // 1. assistant 消息必须已到达（非 in-flight）
  if (!block.assistant || block.assistant.status === 'streaming') return false;
  // 2. 所有工具调用必须已完成
  return block.tools.every(t => {
    const toolEvent = t.optionsJSON?.tool_event;
    return toolEvent?.status === 'success' || toolEvent?.status === 'failed' || toolEvent?.status === 'cancelled';
  });
}
```

#### 折叠渲染策略

```
TurnBlockGroup.isCompleted === true  → 渲染为折叠态（单行摘要）
TurnBlockGroup.isCompleted === false → 渲染为展开态（完整内容）
```

折叠态摘要行内容：

| block 类型 | 折叠态摘要 |
|-----------|-----------|
| 纯工具调用 block | "🔧 {tool_name} → ✓ 1.5s" 或多工具 "🔧 3 tools → ✓ 4.2s" |
| 团队组建 block | "🏗️ 组建团队 → {team_name} ✓" |
| 团队完成 block | "✅ 任务完成 → {team_name} 3m 20s" |
| interrupted block | "⏸ 已中断 → {team_name} 3/5 步骤" |

#### Composable: useAutoCollapse

```typescript
// web/src/composables/chat/useAutoCollapse.ts
export function useAutoCollapse() {
  const collapsedBlockKeys = ref<Set<number>>(new Set());

  // 当新 block 变为活跃时，折叠前一个 block
  function onBlockActivated(blockKey: number) {
    // 前一个活跃 block 自动折叠
    for (const key of collapsedBlockKeys.value) {
      // 不操作，保持已折叠
    }
  }

  // 当 block 完成时，自动折叠
  function onBlockCompleted(blockKey: number) {
    collapsedBlockKeys.value.add(blockKey);
  }

  // 展开全部
  function expandAll() {
    collapsedBlockKeys.value.clear();
  }

  // 切换单个 block
  function toggleBlock(blockKey: number) {
    if (collapsedBlockKeys.value.has(blockKey)) {
      collapsedBlockKeys.value.delete(blockKey);
    } else {
      collapsedBlockKeys.value.add(blockKey);
    }
  }

  return { collapsedBlockKeys, onBlockCompleted, expandAll, toggleBlock };
}
```

---

### 2.2 OBS-02 语境加载消息

#### 事件到消息映射

```typescript
// web/src/features/spirit/observabilityConstants.ts

export type ContextualLoadingConfig = {
  eventPattern: string;       // 匹配的事件类型
  icon: string;               // Quasar 图标名
  color: string;              // 左侧竖线颜色
  messageTemplate: string;    // 消息模板，{var} 为占位符
};

export const ORCHESTRATION_LOADING_MAP: ContextualLoadingConfig[] = [
  {
    eventPattern: 'butler.orchestration.started',
    icon: 'sync',
    color: 'grey',
    messageTemplate: '正在处理任务…',
  },
  {
    eventPattern: 'spirit_plan_created',
    icon: 'search',
    color: 'blue',
    messageTemplate: '正在分析任务复杂度…',
  },
  {
    eventPattern: 'spirit_allocation_created',
    icon: 'people',
    color: 'purple',
    messageTemplate: '正在分配 Agent 角色…',
  },
  {
    eventPattern: 'spirit_orchestration_started',
    icon: 'construction',
    color: 'orange',
    messageTemplate: '正在编排执行流程…',
  },
];

export const AGENT_LOADING_MAP: ContextualLoadingConfig[] = [
  {
    eventPattern: 'tool_call',
    icon: 'bolt',
    color: 'blue',
    messageTemplate: '{agentName} 正在{displayLabel}…',
    // 从 EnvelopeToolCall 提取: agentName = AgentName, displayLabel = DisplayLabel
  },
  {
    eventPattern: 'tool_result',
    icon: 'check_circle',
    color: 'green',
    messageTemplate: '{agentName} 完成，耗时 {durationSec}s',
    // 从 EnvelopeToolCall 提取: agentName = AgentName, durationSec = DurationMS / 1000
  },
];
```

#### Composable: useContextualLoadingMessage

```typescript
// web/src/composables/chat/useContextualLoadingMessage.ts
export function useContextualLoadingMessage() {
  const loadingMessage = ref<{ text: string; icon: string; color: string } | null>(null);
  const isReplaying = ref(false);  // WS 回放期间静默

  function onSpiritEnvelope(envelope: SpiritEnvelope) {
    if (isReplaying.value) return;  // 回放期间静默

    // 1. 检查编排阶段事件
    const orchestrationConfig = ORCHESTRATION_LOADING_MAP.find(
      c => c.eventPattern === envelope.type
    );
    if (orchestrationConfig) {
      loadingMessage.value = {
        text: orchestrationConfig.messageTemplate,
        icon: orchestrationConfig.icon,
        color: orchestrationConfig.color,
      };
      return;
    }

    // 2. 检查 tool_call/tool_result 事件（Agent 级）
    if (envelope.type === 'tool_call' || envelope.type === 'tool_result') {
      const md = envelope.metadata as EnvelopeToolCall;
      const agentName = md.agent_name || md.agent_key || 'Agent';
      const config = AGENT_LOADING_MAP.find(c => c.eventPattern === envelope.type);
      if (config) {
        loadingMessage.value = {
          text: config.messageTemplate
            .replace('{agentName}', agentName)
            .replace('{displayLabel}', md.display_label || '执行操作')
            .replace('{durationSec}', String(Math.round((md.duration_ms || 0) / 1000))),
          icon: config.icon,
          color: config.color,
        };
      }
    }

    // 3. 团队完成/失败时清除加载消息
    if (envelope.type === 'spirit_team_completed' || envelope.type === 'spirit_team_failed') {
      loadingMessage.value = null;
    }
  }

  return { loadingMessage, isReplaying, onSpiritEnvelope };
}
```

---

### 2.3 OBS-03 Agent 状态标签

#### 状态聚合映射

```typescript
// web/src/features/spirit/spiritUi.ts（扩展）

export type AgentNodeStatusLabel = 'queued' | 'active' | 'suspended' | 'done' | 'failed' | 'skipped' | 'cancelled';

export const AGENT_NODE_STATUS_MAP: Record<string, AgentNodeStatusLabel> = {
  // Queued
  idle: 'queued',
  queued: 'queued',
  scheduled: 'queued',
  // Active
  running: 'active',
  thinking: 'active',
  tool_running: 'active',
  transferring: 'active',
  retrying: 'active',
  // Suspended
  waiting_input: 'suspended',
  waiting_review: 'suspended',
  waiting_assign: 'suspended',
  blocked: 'suspended',
  // Done
  success: 'done',
  // Failed
  failed: 'failed',
  timed_out: 'failed',
  // Skipped
  skipped: 'skipped',
  // Cancelled
  cancelled: 'cancelled',
};

export const STATUS_LABEL_CONFIG: Record<AgentNodeStatusLabel, { text: string; color: string; icon: string; animated: boolean }> = {
  queued:    { text: '排队中', color: 'grey',    icon: 'circle',      animated: false },
  active:    { text: '执行中', color: 'blue',    icon: 'bolt',        animated: true },
  suspended: { text: '等待中', color: 'orange',  icon: 'pause',       animated: false },
  done:      { text: '已完成', color: 'green',   icon: 'check_circle', animated: false },
  failed:    { text: '失败',   color: 'red',     icon: 'error',       animated: false },
  skipped:   { text: '已跳过', color: 'grey',    icon: 'remove_circle', animated: false },
  cancelled: { text: '已取消', color: 'grey',    icon: 'cancel',      animated: false },
};
```

#### 组件: AgentStatusLabel.vue

```vue
<!-- web/src/components/spirit/AgentStatusLabel.vue -->
<template>
  <q-badge
    :color="config.color"
    :label="config.text"
    :class="{ 'agent-status--animated': config.animated }"
    class="agent-status-label"
  >
    <q-icon :name="config.icon" size="12px" class="q-mr-xs" />
  </q-badge>
</template>
```

#### 双状态源策略

| 使用场景 | 状态源 | 映射 |
|---------|--------|------|
| 侧边栏 `TeamTaskCard` 折叠态 | `SpiritMember.status`（idle/running/error） | idle→queued, running→active, error→failed |
| 任务执行面板 `TaskExecutionPanel` | `AgentNodeStatus`（17 值） | 通过 `AGENT_NODE_STATUS_MAP` 聚合为 7 种标签 |

---

### 2.4 OBS-04 底部状态栏

#### 组件: SpiritStatusBar.vue

```
┌──────────────────────────────────────────────────────────────┐
│ ⚡ 2 running │ ⏸ 1 interrupted │ 📊 2/3 quota │ ✅ Team A  │
└──────────────────────────────────────────────────────────────┘
```

**Props**：

```typescript
defineProps<{
  runningTeamCount: number;
  interruptedTeamCount: number;
  quotaUsed: number;
  quotaMax: number;
  tokenUsage?: { in: number; out: number };
  lastEvent?: { type: 'completed' | 'failed'; teamName: string };
}>();
```

**后端扩展需求**：`spirit_team_completed` / `spirit_teams_all_completed` 事件需增加 `total_token_in` / `total_token_out` 字段。

---

### 2.5 OBS-05 侧边栏状态脉冲

#### Composable: useStatusPulse

```typescript
// web/src/composables/chat/useStatusPulse.ts
export function useStatusPulse() {
  const pulseStates = ref<Map<string, { color: string; active: boolean }>>(new Map());
  const isReplaying = ref(false);

  function onTeamStatusChanged(teamId: string, newStatus: string) {
    if (isReplaying.value) return;

    const pulseColor = PULSE_COLOR_MAP[newStatus];
    if (!pulseColor) return;

    pulseStates.value.set(teamId, { color: pulseColor, active: true });
    setTimeout(() => {
      pulseStates.value.delete(teamId);
    }, PULSE_DURATION_MAP[newStatus] ?? 1500);
  }

  return { pulseStates, isReplaying, onTeamStatusChanged };
}
```

**CSS 动画**：

```css
.team-card--pulse {
  animation: status-pulse 1.5s ease-out;
  border-left: 2px solid var(--pulse-color);
}
@keyframes status-pulse {
  0% { background-color: var(--pulse-color-alpha); }
  100% { background-color: transparent; }
}
```

---

### 2.6 OBS-06 可折叠工具输出

#### ChatExecutionCard 修改

`ChatExecutionCard` 已使用 `<q-expansion-item>` 实现折叠/展开，需增加：

1. **自动折叠逻辑**：监听 `event.status` 变化，completed/failed/cancelled 时设置 `expanded = false`
2. **折叠态 header 优化**：当前 header 已包含标题+摘要+状态图标+时长，信息密度足够
3. **历史消息恢复**：加载历史消息时，从 `OptionsJSON.tool_event.status` 判断是否应折叠

```typescript
// ChatExecutionCard.vue 修改
watch(() => props.event.status, (newStatus) => {
  if (newStatus === 'success' || newStatus === 'failed' || newStatus === 'cancelled') {
    expanded.value = false;  // 自动折叠
  }
}, { immediate: false });
```

---

### 2.7 OBS-07 中断恢复提示

#### 组件: InterruptedTeamCard.vue

```vue
<!-- web/src/components/spirit/InterruptedTeamCard.vue -->
<template>
  <q-card v-if="team.status === 'interrupted'" flat bordered class="interrupted-team-card">
    <q-card-section>
      <div class="row items-center q-mb-sm">
        <q-icon name="pause_circle" color="warning" size="24px" class="q-mr-sm" />
        <span class="text-subtitle2">团队已中断</span>
      </div>
      <div class="text-body2 q-mb-xs">{{ team.name }} 因{{ interruptReason }}而中断</div>
      <div class="text-caption text-grey">已完成 {{ team.completedSteps }}/{{ team.totalSteps }} 步骤</div>
    </q-card-section>
    <q-card-actions align="right">
      <q-btn v-if="canResume" label="恢复执行" color="primary" flat @click="$emit('resume', team.id)" />
      <q-btn v-else label="不支持断点恢复" color="grey" flat disable />
      <q-btn label="取消团队" color="negative" flat @click="$emit('cancel', team.id)" />
    </q-card-actions>
  </q-card>
</template>
```

**Props**：

```typescript
defineProps<{
  team: SpiritTeam;
  canResume: boolean;  // team.graphExecutionId 是否存在
  interruptReason: string;
}>();

defineEmits<{
  resume: [teamId: string];
  cancel: [teamId: string];
}>();
```

---

## 三、集成方案

### 3.1 TaskExecutionPanel 重构

将现有简化版 `TaskExecutionPanel` 升级为集成 `ParallelTeamOverview` 的完整版：

```
当前：
┌─ Overview ───────────────────────────┐
├─ ChatExecutionCard 列表 ─────────────┤
├─ 对话输出（折叠）─────────────────────┤

升级后：
┌─ Overview + AgentStatusLabel 列表 ───┐
├─ InterruptedTeamCard（条件显示）──────┤
├─ ParallelTeamOverview ────────────────┤
│  ├─ DAGDiagramCard                   │
│  ├─ TeamProgressCard × N             │
│  └─ SynthesisResultCard              │
└──────────────────────────────────────┘
```

### 3.2 ChatMessagePanel 修改

在精灵对话面板中集成语境加载消息和底部状态栏：

```
当前：
┌─ ChatHeader ─────────────────────────┐
├─ ChatMessageList ────────────────────┤
├─ SynthesisResultCard（条件显示）──────┤
├─ ChatComposer ───────────────────────┤

升级后：
┌─ ChatHeader ─────────────────────────┤
├─ ContextualLoadingMessage（条件显示）┤  ← 新增
├─ ChatMessageList（自动折叠）─────────┤  ← 修改
├─ SynthesisResultCard（条件显示）──────┤
├─ ChatComposer ───────────────────────┤
├─ SpiritStatusBar ────────────────────┤  ← 新增（底部固定）
```

### 3.3 WS 回放兼容

所有 L1 环境层方案（OBS-02 语境消息、OBS-05 脉冲）需在 WS 回放期间静默：

```typescript
// 统一回放状态管理
const isReplaying = ref(false);

// 在 useChatStreamManager 中
watch(wsReplaying, (val) => {
  isReplaying.value = val;
});
```

---

## 四、影响域

| 包 | 变更类型 | 说明 |
|----|----------|------|
| `web/src/features/chat/groupMessagesByTurn.ts` | 修改 | 扩展 `TurnBlockGroup` 增加 `isCompleted` |
| `web/src/features/spirit/types.ts` | 修改 | 新增 `AgentNodeStatusLabel` 联合类型 |
| `web/src/features/spirit/spiritUi.ts` | 修改 | 新增 `AGENT_NODE_STATUS_MAP` + `STATUS_LABEL_CONFIG` |
| `web/src/features/spirit/observabilityConstants.ts` | 新增 | 语境消息映射表、脉冲颜色配置 |
| `web/src/composables/chat/useAutoCollapse.ts` | 新增 | 自动折叠 composable |
| `web/src/composables/chat/useContextualLoadingMessage.ts` | 新增 | 语境加载消息 composable |
| `web/src/composables/chat/useStatusPulse.ts` | 新增 | 侧边栏脉冲 composable |
| `web/src/components/spirit/AgentStatusLabel.vue` | 新增 | Agent 状态标签组件 |
| `web/src/components/spirit/SpiritStatusBar.vue` | 新增 | 底部状态栏组件 |
| `web/src/components/spirit/InterruptedTeamCard.vue` | 新增 | 中断恢复提示卡片 |
| `web/src/components/spirit/TeamTaskCard.vue` | 修改 | 增加 AgentStatusLabel + 脉冲动画 |
| `web/src/components/spirit/TaskExecutionPanel.vue` | 修改 | 集成 ParallelTeamOverview + AgentStatusLabel + InterruptedTeamCard |
| `web/src/components/chat/ChatExecutionCard.vue` | 修改 | 增加自动折叠逻辑 |
| `web/src/components/chat/ChatMessagePanel.vue` | 修改 | 集成 SpiritStatusBar + 语境加载消息 |
| `web/src/components/chat/ChatEntitySidebar.vue` | 修改 | 集成 useStatusPulse |
| `internal/service/spirit_team.go` | 修改 | `spirit_team_completed` 事件增加 token 字段 |

**不改动**：后端核心编排逻辑、Proto 定义（除 token 字段扩展外）、数据库 Schema。
