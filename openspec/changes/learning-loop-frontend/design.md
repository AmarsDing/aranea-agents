# 学习闭环前端可视化设计

> 日期：2026-06-02
> 状态：设计已批准

---

## 1. 需求摘要

| 需求 | 决策 |
|------|------|
| 学习闭环 Tab | 在 Agent 详情页新增"学习闭环"Tab，与"进化"Tab 并列 |
| 模式列表 | 展示 Pattern 列表，支持按状态筛选，显示频率/置信度/描述 |
| 提议管理 | 展示 Proposal 列表，支持审批/拒绝操作，状态颜色区分 |
| 已注册知识 | 展示 applied 状态的 Proposal，查看注册内容 |
| 观察记录 | 展示 Observation 列表，了解原始行为数据 |
| 手动触发 | 提供"运行闭环"按钮，调用 RunLoop API |

---

## 2. 方案选择

**方案 A：独立 Tab + 独立组件**（已批准）

在 Agent 详情页新增"学习闭环"Tab，包含独立的 `AgentLearningLoopPanel.vue` 组件。理由：

1. 学习闭环与进化面板职责不同：进化面板关注指标和建议，学习闭环关注模式识别和知识注册
2. 独立 Tab 避免进化面板膨胀（当前已 280 行）
3. 数据流独立：学习闭环有独立的 API/Store/Composable
4. 符合项目前端规范：功能模块内聚，组件职责单一

---

## 3. 数据流设计

遵循 aranea-frontend-guide 数据流铁律：**API → Store → Composable → Page → Component**

> **实际实现与原始设计有差异**：采用独立 Store 和独立 API 文件，而非扩展现有文件。

```
Proto 生成客户端 (services/kratos/learning_loop/v1/)
    │
    ▼
api.learning.ts — 类型定义 + HTTP 调用 + 归一化（独立文件）
    │
    ▼
Store (stores/learningLoop/index.ts — 独立 Store)
    │  fetchObservations / fetchPatterns / fetchProposals
    │  approveProposal / rejectProposal / runLoop
    │  持有领域状态：observations / patterns / proposals + loading / error
    ▼
Composable (features/agents/useLearningLoopPanel.ts)
    │  封装 Store 调用 + 响应式状态 + watch 自动加载
    ▼
Page (AgentSettingsPage.vue — 新增 Tab)
    │
    ▼
Component (components/agents/AgentLearningLoopPanel.vue)
    │  展示子组件编排
    ├─ LearningLoopOverview.vue    ← 闭环概览卡片
    ├─ LearningPatternList.vue     ← 模式列表
    ├─ LearningProposalList.vue    ← 提议列表（含审批/拒绝）
    └─ LearningObservationList.vue ← 观察记录列表
```

---

## 4. API 层设计

### 4.1 Proto 生成客户端

已运行 `make api` 生成 `web/src/services/kratos/learning_loop/v1/index.ts` 客户端代码。

在 `web/src/services/index.ts` 中注册：

```typescript
import { createLearningLoopServiceClient } from "./kratos/learning_loop/v1/index";

export function createLearningLoopService() {
  return createLearningLoopServiceClient(requestHandler);
}
```

### 4.2 类型定义

> **实际实现**：类型定义在独立文件 `web/src/features/agents/learning.types.ts` 中，`kind` 和 `status` 字段使用 `string` 类型（与 Proto 生成类型一致），而非联合类型。

```typescript
export type LearningObservation = {
  id: string;
  agent_id: string;
  session_id: string;
  kind: string;
  content: string;
  metadata: string;
  observed_at: string;
};

export type LearningPattern = {
  id: string;
  agent_id: string;
  kind: string;
  description: string;
  frequency: number;
  confidence: number;
  evidence: string;
  status: string;
  detected_at: string;
};

export type LearningProposal = {
  id: string;
  agent_id: string;
  pattern_id: string;
  title: string;
  content: string;
  kind: string;
  status: string;
  validated_at: string;
  approved_by: string;
  created_at: string;
  updated_at: string;
};
```

### 4.3 API 函数

> **实际实现**：API 函数在独立文件 `web/src/features/agents/api.learning.ts` 中。归一化函数直接使用 Proto 生成类型（`Observation`、`Pattern`、`KnowledgeProposal`）作为入参，而非 `Record<string, unknown>` + `asRecord()` 模式。函数命名前缀为 `list`（列表查询）而非 `get`。

```typescript
export async function listLearningObservations(
  agentId: string,
  since?: string
): Promise<LearningObservation[]> {
  const svc = createLearningLoopService();
  const res = await svc.ListObservations({ agentId, since });
  return (res.items ?? []).map(normalizeObservation);
}

export async function listLearningPatterns(
  agentId: string,
  status?: string
): Promise<LearningPattern[]> {
  const svc = createLearningLoopService();
  const res = await svc.ListPatterns({ agentId, status });
  return (res.items ?? []).map(normalizePattern);
}

export async function listLearningProposals(
  agentId: string,
  status?: string
): Promise<LearningProposal[]> {
  const svc = createLearningLoopService();
  const res = await svc.ListProposals({ agentId, status });
  return (res.items ?? []).map(normalizeProposal);
}

export async function approveLearningProposal(
  agentId: string,
  proposalId: string
): Promise<LearningProposal> {
  const svc = createLearningLoopService();
  const res = await svc.ApproveProposal({ agentId, id: proposalId });
  return normalizeProposal(res);
}

export async function rejectLearningProposal(
  agentId: string,
  proposalId: string
): Promise<LearningProposal> {
  const svc = createLearningLoopService();
  const res = await svc.RejectProposal({ agentId, id: proposalId });
  return normalizeProposal(res);
}

export async function runLearningLoop(agentId: string): Promise<void> {
  const svc = createLearningLoopService();
  await svc.RunLoop({ agentId });
}
```

归一化函数直接使用 Proto 生成类型，处理 `undefined` → `''` / `0` 的默认值转换和 camelCase → snake_case 字段名转换。

---

## 5. Store 层设计

> **实际实现**：采用独立 Store `web/src/stores/learningLoop/index.ts`，而非扩展 `stores/agents/detail.ts`。Store 持有领域状态（observations / patterns / proposals），与原始设计"不缓存领域状态"不同。Store 已在 `stores/index.ts` 中导出。

```typescript
import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  listLearningObservations, listLearningPatterns, listLearningProposals,
  approveLearningProposal, rejectLearningProposal, runLearningLoop,
} from '../../features/agents/api.learning';
import type { LearningObservation, LearningPattern, LearningProposal } from '../../features/agents/learning.types';

export const useLearningLoopStore = defineStore('learningLoop', () => {
  const observations = ref<LearningObservation[]>([]);
  const patterns = ref<LearningPattern[]>([]);
  const proposals = ref<LearningProposal[]>([]);
  const loading = ref(false);
  const error = ref<string | null>(null);

  async function fetchObservations(agentId: string, since?: string): Promise<LearningObservation[]> {
    loading.value = true;
    error.value = null;
    try {
      const result = await listLearningObservations(agentId, since);
      observations.value = result;
      return result;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    } finally {
      loading.value = false;
    }
  }

  async function fetchPatterns(agentId: string, status?: string): Promise<LearningPattern[]> { ... }
  async function fetchProposals(agentId: string, status?: string): Promise<LearningProposal[]> { ... }

  async function approveProposal(agentId: string, proposalId: string): Promise<LearningProposal> {
    const result = await approveLearningProposal(agentId, proposalId);
    const idx = proposals.value.findIndex((p) => p.id === proposalId);
    if (idx !== -1) proposals.value[idx] = result;
    return result;
  }

  async function rejectProposal(agentId: string, proposalId: string): Promise<LearningProposal> {
    const result = await rejectLearningProposal(agentId, proposalId);
    const idx = proposals.value.findIndex((p) => p.id === proposalId);
    if (idx !== -1) proposals.value[idx] = result;
    return result;
  }

  async function runLoop(agentId: string): Promise<void> {
    return runLearningLoop(agentId);
  }

  return {
    observations, patterns, proposals, loading, error,
    fetchObservations, fetchPatterns, fetchProposals,
    approveProposal, rejectProposal, runLoop,
  };
});
```

> 设计理由：独立 Store 让学习闭环模块完全解耦，不污染 `useAgentDetailStore`。Store 持有领域状态使得 Composable 可以通过 computed 消费，实现响应式数据流。

---

## 6. Composable 层设计

> **实际实现**：文件名为 `useLearningLoopPanel.ts`（非 `useAgentLearningLoopPanel.ts`），使用独立 `useLearningLoopStore`。

新建 `web/src/features/agents/useLearningLoopPanel.ts`：

```typescript
import { computed, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useLearningLoopStore } from '../../stores/learningLoop';
import type { LearningObservation, LearningPattern, LearningProposal } from './api.learning';

export function useLearningLoopPanel(agentId: () => string) {
  const $q = useQuasar();
  const store = useLearningLoopStore();

  const loading = ref(false);
  const runningLoop = ref(false);
  const approvingId = ref<string | null>(null);
  const rejectingId = ref<string | null>(null);
  const patternStatusFilter = ref<string>('');
  const proposalStatusFilter = ref<string>('');

  // 从 Store computed 获取领域数据
  const observations = computed<LearningObservation[]>(() => store.observations);
  const patterns = computed<LearningPattern[]>(() => store.patterns);
  const proposals = computed<LearningProposal[]>(() => store.proposals);

  // 统计 computed
  const pendingProposalsCount = computed(() =>
    proposals.value.filter((p) => p.status === 'validated').length
  );
  const registeredKnowledgeCount = computed(() =>
    proposals.value.filter((p) => p.status === 'applied').length
  );

  async function fetchAll() { ... }
  async function onApprove(proposalId: string) { ... }  // 带确认 Dialog
  async function onReject(proposalId: string) { ... }   // 无确认 Dialog
  async function onRunLoop() { ... }

  // watch agentId + 筛选器变化，自动重载
  watch(
    () => [agentId(), patternStatusFilter.value, proposalStatusFilter.value],
    () => { void fetchAll(); },
    { immediate: true },
  );

  return {
    loading, runningLoop, approvingId, rejectingId,
    patternStatusFilter, proposalStatusFilter,
    observations, patterns, proposals,
    pendingProposalsCount, registeredKnowledgeCount,
    onApprove, onReject, onRunLoop, fetchAll,
  };
}
```

> 与原始设计的差异：
> 1. 数据从 Store 的 computed 获取，而非本地 ref
> 2. watch 同时监听 agentId + 两个筛选器，筛选变化自动重载
> 3. `pendingProposalsCount` 和 `registeredKnowledgeCount` 是 computed number
> 4. `onReject` 没有确认 Dialog（仅 `onApprove` 有）

---

## 7. 组件设计

### 7.1 组件层级

```
AgentLearningLoopPanel.vue          ← 编排层：概览 + 三个子列表
  ├─ LearningLoopOverview.vue       ← 闭环概览卡片（统计数字）
  ├─ LearningPatternList.vue        ← 模式列表（可筛选）
  ├─ LearningProposalList.vue       ← 提议列表（含审批/拒绝按钮）
  └─ LearningObservationList.vue    ← 观察记录列表
```

### 7.2 各组件职责

> **实际实现差异**：子组件各自包含 `<section class="settings-section">` 包裹和 section-heading，而非由编排组件统一包裹。

| 组件 | 职责 | 关键 props | 关键 emits |
|------|------|-----------|-----------|
| **AgentLearningLoopPanel** | 编排：加载状态 + 四个子组件布局 | `agentId: string \| (() => string)` | — |
| **LearningLoopOverview** | 展示统计卡片：观察数/模式数/待审批/已注册 + 运行闭环按钮 | `observationCount`, `patternCount`, `pendingCount`, `registeredCount`, `runningLoop` | `run-loop` |
| **LearningPatternList** | 模式列表 + 状态筛选 + 置信度/频率展示（自带 section 包裹） | `patterns`, `statusFilter`, `loading` | `update:status-filter` |
| **LearningProposalList** | 提议列表 + 状态筛选 + 审批/拒绝操作（自带 section 包裹） | `proposals`, `statusFilter`, `approvingId`, `rejectingId`, `loading` | `update:status-filter`, `approve`, `reject` |
| **LearningObservationList** | 观察记录列表 + kind 图标 + 时间展示（自带 section 包裹） | `observations`, `loading` | — |

### 7.3 AgentLearningLoopPanel 模板结构

> **实际实现**：编排组件仅包裹概览 section，子列表组件各自包含 section 包裹。`agentId` 支持 `string | (() => string)` 类型。

```html
<div class="evolution-panel settings-grid settings-grid--wide">
  <section class="settings-section">
    <div class="section-heading">...</div>
    <q-inner-loading :showing="loading" label="加载学习数据..." />
    <learning-loop-overview
      v-if="!loading"
      :observation-count="observations.length"
      :pattern-count="patterns.length"
      :pending-count="pendingProposalsCount"
      :registered-count="registeredKnowledgeCount"
      :running-loop="runningLoop"
      @run-loop="onRunLoop"
    />
  </section>

  <learning-pattern-list
    :patterns="patterns"
    :loading="loading"
    :status-filter="patternStatusFilter"
    @update:status-filter="patternStatusFilter = $event"
  />

  <learning-proposal-list
    :proposals="proposals"
    :loading="loading"
    :status-filter="proposalStatusFilter"
    :approving-id="approvingId"
    :rejecting-id="rejectingId"
    @update:status-filter="proposalStatusFilter = $event"
    @approve="onApprove"
    @reject="onReject"
  />

  <learning-observation-list :observations="observations" :loading="loading" />
</div>
```

### 7.4 LearningLoopOverview 设计

4 个统计卡片 + 运行闭环按钮，复用 `overview-metric-card` 样式（与 AgentEvolutionPanel 一致）：

> **实际实现**：使用 `app-metrics-grid` 布局。图标选择与设计略有差异。

```
┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐
│ 👁 观察数     │ │ 🔍 模式数     │ │ ⏳ 待审批     │ │ 🎓 已注册     │
│  visibility  │ │  pattern     │ │pending_actions│ │   school     │
│     156      │ │     12       │ │      3       │ │      8       │
│ 累计行为观察  │ │ 已识别行为模式│ │ 待审批知识提议│ │ 已注册知识    │
└──────────────┘ └──────────────┘ └──────────────┘ └──────────────┘
                                              [▶ 运行学习闭环]
```

### 7.5 LearningPatternList 设计

- 顶部：`<q-btn-toggle>` 状态筛选（全部 / detected / confirmed / dismissed）
- 列表：`<q-list separator class="app-glass-list">`
- 每项：Kind badge + 描述 + 频率 + 置信度进度条 + 检测时间

```
┌─────────────────────────────────────────────────────────┐
│ [全部] [detected] [confirmed] [dismissed]               │
├─────────────────────────────────────────────────────────┤
│ 🔧 tool_call  高频工具调用模式: search(15), read(12)    │
│ 频率: 27  置信度: ████████░░ 82%  检测于: 2026-05-28   │
├─────────────────────────────────────────────────────────┤
│ 💬 feedback   用户反馈模式: 8 条中 3 条负面              │
│ 频率: 8   置信度: ██████░░░░ 61%  检测于: 2026-05-30   │
└─────────────────────────────────────────────────────────┘
```

### 7.6 LearningProposalList 设计

- 顶部：`<q-btn-toggle>` 状态筛选（全部 / draft / validated / approved / applied / rejected / conflict）
- 列表：`<q-list separator class="app-glass-list">`
- 每项：Kind badge + 标题 + 内容摘要 + 状态 badge + 操作按钮

```
┌─────────────────────────────────────────────────────────┐
│ [全部] [validated] [approved] [applied] [rejected]      │
├─────────────────────────────────────────────────────────┤
│ 📝 prompt  学习闭环: 高频工具调用模式 (置信度 82%)       │
│ 检测到重复模式（频率 27，置信度 82.0%）。建议: 基于此... │
│ [validated]                        [✓ 审批] [✗ 拒绝]   │
├─────────────────────────────────────────────────────────┤
│ 📝 prompt  学习闭环: 用户反馈模式 (置信度 61%)           │
│ 检测到重复模式（频率 8，置信度 61.0%）。建议: 基于此...  │
│ [applied]  审批人: user:1       2026-05-30              │
└─────────────────────────────────────────────────────────┘
```

### 7.7 LearningObservationList 设计

- 列表：`<q-list separator class="app-glass-list">`
- 每项：Kind 图标 + 内容摘要 + Session ID + 观察时间

```
┌─────────────────────────────────────────────────────────┐
│ 🔧 tool_call  调用 search 工具                          │
│ Session: sess_abc123  观察于: 2026-05-28 14:30         │
├─────────────────────────────────────────────────────────┤
│ 💬 feedback   用户表示不满意                             │
│ Session: sess_def456  观察于: 2026-05-29 09:15         │
└─────────────────────────────────────────────────────────┘
```

---

## 8. Tab 集成

在 Agent 详情页的 Tab 配置中新增"学习闭环"Tab：

> **实际实现**：在 `web/src/pages/AgentSettingsPage.vue` 中直接使用 `<q-tab>` 和 `<q-tab-panels>` 结构，Tab 无图标。

```html
<q-tabs v-model="tab" dense align="left" class="agent-settings-tabs" :breakpoint="0">
  <q-tab name="agent" label="Agent" />
  <q-tab name="memory" label="记忆" />
  <q-tab name="files" label="文件" />
  <q-tab name="permissions" label="权限" />
  <q-tab name="skills" label="Skill / 工具" />
  <q-tab name="evolution" label="进化" />
  <q-tab name="learning" label="学习闭环" />  <!-- 新增 -->
  <q-tab name="hooks" label="钩子" />
  <q-tab name="a2a" label="A2A 协议" />
</q-tabs>

<q-tab-panels v-model="tab" animated class="settings-panels">
  <!-- ...existing panels... -->
  <q-tab-panel name="learning">
    <agent-learning-loop-panel :agent-id="agentId" />
  </q-tab-panel>
  <!-- ... -->
</q-tab-panels>
```

---

## 9. 状态颜色映射

### Pattern 状态

| 状态 | 颜色 | 标签 |
|------|------|------|
| detected | orange | 已检测 |
| confirmed | positive | 已确认 |
| dismissed | grey | 已忽略 |

### Proposal 状态

| 状态 | 颜色 | 标签 |
|------|------|------|
| draft | grey | 草稿 |
| validated | blue | 已验证 |
| approved | teal | 已审批 |
| rejected | negative | 已拒绝 |
| applied | positive | 已应用 |
| conflict | warning | 冲突 |
| expired | grey | 已过期 |

### Observation Kind

| Kind | 图标 | 颜色 |
|------|------|------|
| tool_call | build | blue |
| feedback | chat | purple |
| memory_hit | psychology | teal |
| memory_miss | psychology | grey |

---

## 10. 文件变更清单

> **实际实现与原始设计有显著差异**：类型、API、Store 均为独立文件，而非修改现有文件。

| 操作 | 文件 | 说明 |
|------|------|------|
| **已生成** | `web/src/services/kratos/learning_loop/v1/index.ts` | Proto 生成客户端 |
| **已修改** | `web/src/services/index.ts` | 注册 createLearningLoopService |
| **新建** | `web/src/features/agents/learning.types.ts` | 3 个类型定义（独立文件，非 types.ts） |
| **新建** | `web/src/features/agents/api.learning.ts` | 6 个 API 函数 + 3 个归一化函数（独立文件，非 api.ts） |
| **新建** | `web/src/stores/learningLoop/index.ts` | 独立 Store（非扩展 detail.ts） |
| **已修改** | `web/src/stores/index.ts` | 导出 useLearningLoopStore |
| **新建** | `web/src/features/agents/useLearningLoopPanel.ts` | 学习闭环 Composable |
| **新建** | `web/src/components/agents/AgentLearningLoopPanel.vue` | 学习闭环编排组件 |
| **新建** | `web/src/components/agents/LearningLoopOverview.vue` | 概览卡片组件 |
| **新建** | `web/src/components/agents/LearningPatternList.vue` | 模式列表组件 |
| **新建** | `web/src/components/agents/LearningProposalList.vue` | 提议列表组件 |
| **新建** | `web/src/components/agents/LearningObservationList.vue` | 观察记录组件 |
| **已修改** | `web/src/pages/AgentSettingsPage.vue` | 新增"学习闭环"Tab |

**不需要改动的**：
- 后端（API 已完成）
- Proto 文件（已定义）
- AgentEvolutionPanel.vue（独立 Tab，互不影响）
- `web/src/features/agents/types.ts`（类型定义在独立文件中）
- `web/src/features/agents/api.ts`（API 函数在独立文件中）
- `web/src/stores/agents/detail.ts`（Store 在独立文件中）

---

## 11. 风险与缓解

| 风险 | 缓解措施 |
|------|----------|
| Proto 生成客户端可能尚未生成 | 先运行 `make api` 确认生成，api.ts 函数签名以生成结果为准 |
| 观察记录数据量大导致渲染卡顿 | 前端限制展示最近 100 条，后端 API 已有 since 参数 |
| RunLoop 是异步操作，前端无法即时反馈 | 运行后自动刷新列表数据，按钮加 loading 防重复点击 |
| 提议审批/拒绝后列表状态不一致 | 操作成功后重新 fetchProposals + fetchPatterns |
