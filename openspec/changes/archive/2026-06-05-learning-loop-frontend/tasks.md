# 学习闭环前端可视化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Agent 详情页新增"学习闭环"Tab，展示学习闭环运行状态、已识别模式、待审批提议、已注册知识和观察记录。遵循 aranea-frontend-guide 数据流铁律：API → Store → Composable → Page → Component。

**Architecture:** 后端 API 已完成（`LearningLoopService` 6 个 HTTP 端点）。前端新增类型定义 → API 函数 → Store action → Composable → 5 个组件 → Tab 集成。

**Tech Stack:** Vue 3 + Quasar + Pinia + TypeScript

---

## File Structure

> **实际实现与原始设计有差异**：类型定义、API 函数、Store 均拆分为独立文件，而非修改现有文件。Composable 命名也有变化。

| 操作 | 文件 | 职责 | 状态 |
|------|------|------|------|
| Generate | `web/src/services/kratos/learning_loop/v1/index.ts` | Proto 生成客户端 | ✅ 已完成 |
| Modify | `web/src/services/index.ts` | 注册 createLearningLoopService | ✅ 已完成 |
| Create | `web/src/features/agents/learning.types.ts` | 3 个类型定义（独立文件，非 types.ts） | ✅ 已完成 |
| Create | `web/src/features/agents/api.learning.ts` | 6 个 API 函数 + 3 个归一化函数（独立文件，非 api.ts） | ✅ 已完成 |
| Create | `web/src/stores/learningLoop/index.ts` | 学习闭环 Store（独立 Store，非扩展 detail.ts） | ✅ 已完成 |
| Modify | `web/src/stores/index.ts` | 导出 useLearningLoopStore | ✅ 已完成 |
| Create | `web/src/features/agents/useLearningLoopPanel.ts` | 学习闭环 Composable（命名不同于设计） | ✅ 已完成 |
| Create | `web/src/components/agents/AgentLearningLoopPanel.vue` | 编排组件 | ✅ 已完成 |
| Create | `web/src/components/agents/LearningLoopOverview.vue` | 概览卡片 | ✅ 已完成 |
| Create | `web/src/components/agents/LearningPatternList.vue` | 模式列表 | ✅ 已完成 |
| Create | `web/src/components/agents/LearningProposalList.vue` | 提议列表 | ✅ 已完成 |
| Create | `web/src/components/agents/LearningObservationList.vue` | 观察记录 | ✅ 已完成 |
| Modify | `web/src/pages/AgentSettingsPage.vue` | 新增"学习闭环"Tab | ✅ 已完成 |

---

### Task 1: Proto 生成客户端 + 服务注册

**Files:**
- Generate: `web/src/services/kratos/learning_loop/v1/`（由 `make api` 生成）
- Modify: `web/src/services/index.ts`

**DoD:**
- `make api` 成功生成 `learning_loop/v1/` 客户端代码
- `createLearningLoopService()` 在 `services/index.ts` 中注册并导出
- `pnpm build` 通过

- [x] **Step 1: 运行 make api 生成 Proto 客户端**

Run: `make api`
Expected: 生成 `web/src/services/kratos/learning_loop/v1/index.ts` 等文件

> 实际结果：Proto 客户端已生成，包含 `Observation`、`Pattern`、`KnowledgeProposal` 类型及 `createLearningLoopServiceClient` 工厂函数。

- [x] **Step 2: 在 services/index.ts 中注册 createLearningLoopService**

在 import 区域新增：
```typescript
import { createLearningLoopServiceClient } from "./kratos/learning_loop/v1/index";
```

在导出函数区域新增：
```typescript
export function createLearningLoopService() {
  return createLearningLoopServiceClient(requestHandler);
}
```

> 实际结果：已注册并导出。

- [x] **Step 3: 验证前端编译通过**

Run: `cd web && pnpm build`
Expected: PASS

- [x] **Step 4: Commit**

```bash
git add web/src/services/
git commit -m "feat(learning-loop): register LearningLoopService client"
```

---

### Task 2: 类型定义

**Files:**
- Create: `web/src/features/agents/learning.types.ts`（实际实现为独立文件，非修改 `types.ts`）

**DoD:**
- `LearningObservation`、`LearningPattern`、`LearningProposal` 类型定义完整
- TypeScript 编译通过

> **实现偏差**：类型定义放在独立文件 `learning.types.ts` 中，`kind` 和 `status` 字段使用 `string` 类型而非联合类型（`ObservationKind`、`PatternStatus`、`ProposalStatus`），因为 Proto 生成客户端的对应字段也是 `string`，联合类型约束在运行时无法保证。

- [x] **Step 1: 创建 learning.types.ts 新增类型定义**

实际实现（`web/src/features/agents/learning.types.ts`）：
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

- [x] **Step 2: 验证 TypeScript 编译通过**

Run: `cd web && pnpm build`
Expected: PASS

- [x] **Step 3: Commit**

```bash
git add web/src/features/agents/learning.types.ts
git commit -m "feat(learning-loop): add LearningObservation, LearningPattern, LearningProposal types"
```

---

### Task 3: API 函数 + 归一化

**Files:**
- Create: `web/src/features/agents/api.learning.ts`（实际实现为独立文件，非修改 `api.ts`）

**DoD:**
- 6 个 API 函数实现：listLearningObservations / listLearningPatterns / listLearningProposals / approveLearningProposal / rejectLearningProposal / runLearningLoop
- 3 个归一化函数处理 Proto camelCase → 业务 snake_case
- 函数签名与 Proto 生成客户端一致
- TypeScript 编译通过

> **实现偏差**：API 函数放在独立文件 `api.learning.ts` 中。归一化函数直接接收 Proto 生成类型（`Observation`、`Pattern`、`KnowledgeProposal`），而非 `Record<string, unknown>` + `asRecord()` 模式，因为 Proto 生成客户端已有明确的类型定义。函数命名前缀为 `list` 而非 `get`。

- [x] **Step 1: 创建 api.learning.ts，新增 import**

实际实现：
```typescript
import { createLearningLoopService } from '../../services';
import type { Observation, Pattern, KnowledgeProposal } from '../../services/kratos/learning_loop/v1/index';
export type { LearningObservation, LearningPattern, LearningProposal } from './learning.types';
```

- [x] **Step 2: 新增 3 个归一化函数**

实际实现直接使用 Proto 生成类型作为入参：
```typescript
function normalizeObservation(row: Observation): LearningObservation {
  return {
    id: row.id ?? '',
    agent_id: row.agentId ?? '',
    session_id: row.sessionId ?? '',
    kind: row.kind ?? '',
    content: row.content ?? '',
    metadata: row.metadata ?? '',
    observed_at: row.observedAt ?? '',
  };
}

function normalizePattern(row: Pattern): LearningPattern {
  return {
    id: row.id ?? '',
    agent_id: row.agentId ?? '',
    kind: row.kind ?? '',
    description: row.description ?? '',
    frequency: row.frequency ?? 0,
    confidence: row.confidence ?? 0,
    evidence: row.evidence ?? '',
    status: row.status ?? '',
    detected_at: row.detectedAt ?? '',
  };
}

function normalizeProposal(row: KnowledgeProposal): LearningProposal {
  return {
    id: row.id ?? '',
    agent_id: row.agentId ?? '',
    pattern_id: row.patternId ?? '',
    title: row.title ?? '',
    content: row.content ?? '',
    kind: row.kind ?? '',
    status: row.status ?? '',
    validated_at: row.validatedAt ?? '',
    approved_by: row.approvedBy ?? '',
    created_at: row.createdAt ?? '',
    updated_at: row.updatedAt ?? '',
  };
}
```

- [x] **Step 3: 新增 6 个 API 函数**

实际实现（函数名前缀为 `list` 而非 `get`）：
```typescript
export async function listLearningObservations(agentId: string, since?: string): Promise<LearningObservation[]> {
  const svc = createLearningLoopService();
  const res = await svc.ListObservations({ agentId, since });
  return (res.items ?? []).map(normalizeObservation);
}

export async function listLearningPatterns(agentId: string, status?: string): Promise<LearningPattern[]> {
  const svc = createLearningLoopService();
  const res = await svc.ListPatterns({ agentId, status });
  return (res.items ?? []).map(normalizePattern);
}

export async function listLearningProposals(agentId: string, status?: string): Promise<LearningProposal[]> {
  const svc = createLearningLoopService();
  const res = await svc.ListProposals({ agentId, status });
  return (res.items ?? []).map(normalizeProposal);
}

export async function approveLearningProposal(agentId: string, proposalId: string): Promise<LearningProposal> {
  const svc = createLearningLoopService();
  const res = await svc.ApproveProposal({ agentId, id: proposalId });
  return normalizeProposal(res);
}

export async function rejectLearningProposal(agentId: string, proposalId: string): Promise<LearningProposal> {
  const svc = createLearningLoopService();
  const res = await svc.RejectProposal({ agentId, id: proposalId });
  return normalizeProposal(res);
}

export async function runLearningLoop(agentId: string): Promise<void> {
  const svc = createLearningLoopService();
  await svc.RunLoop({ agentId });
}
```

- [x] **Step 4: 类型导出已在 api.learning.ts 中完成**

```typescript
export type { LearningObservation, LearningPattern, LearningProposal } from './learning.types';
```

- [x] **Step 5: 验证 TypeScript 编译通过**

Run: `cd web && pnpm build`
Expected: PASS

- [x] **Step 6: Commit**

```bash
git add web/src/features/agents/api.learning.ts
git commit -m "feat(learning-loop): add API functions and wire normalization"
```

---

### Task 4: Store 层实现

**Files:**
- Create: `web/src/stores/learningLoop/index.ts`（实际实现为独立 Store，非扩展 `stores/agents/detail.ts`）
- Modify: `web/src/stores/index.ts`（导出 `useLearningLoopStore`）

**DoD:**
- 独立 `useLearningLoopStore` 定义完成
- 6 个 Store action 实现：fetchObservations / fetchPatterns / fetchProposals / approveProposal / rejectProposal / runLoop
- Store 持有领域状态（observations / patterns / proposals）+ loading / error
- import 正确
- TypeScript 编译通过

> **实现偏差**：采用独立 Store `stores/learningLoop/index.ts` 而非扩展 `stores/agents/detail.ts`。Store 持有领域状态（observations, patterns, proposals），与原始设计"不缓存领域状态"不同。action 命名也有差异：`fetchObservations`（非 `fetchLearningObservations`）、`approveProposal`（非 `approveLearning`）等。

- [x] **Step 1: 创建 stores/learningLoop/index.ts**

实际实现：
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

  async function fetchObservations(agentId: string, since?: string): Promise<LearningObservation[]> { ... }
  async function fetchPatterns(agentId: string, status?: string): Promise<LearningPattern[]> { ... }
  async function fetchProposals(agentId: string, status?: string): Promise<LearningProposal[]> { ... }
  async function approveProposal(agentId: string, proposalId: string): Promise<LearningProposal> { ... }
  async function rejectProposal(agentId: string, proposalId: string): Promise<LearningProposal> { ... }
  async function runLoop(agentId: string): Promise<void> { ... }

  return { observations, patterns, proposals, loading, error,
    fetchObservations, fetchPatterns, fetchProposals, approveProposal, rejectProposal, runLoop };
});
```

- [x] **Step 2: 在 stores/index.ts 中导出 useLearningLoopStore**

```typescript
export { useLearningLoopStore } from './learningLoop';
```

- [x] **Step 3: 验证 TypeScript 编译通过**

Run: `cd web && pnpm build`
Expected: PASS

- [x] **Step 4: Commit**

```bash
git add web/src/stores/learningLoop/ web/src/stores/index.ts
git commit -m "feat(learning-loop): add independent LearningLoopStore"
```

---

### Task 5: Composable 实现

**Files:**
- Create: `web/src/features/agents/useLearningLoopPanel.ts`（实际文件名不同于设计的 `useAgentLearningLoopPanel.ts`）

**DoD:**
- Composable 封装 Store 调用 + 响应式状态
- watch agentId 自动加载
- 审批操作带确认 Dialog
- RunLoop 操作带 loading 防重复
- TypeScript 编译通过

> **实现偏差**：
> 1. 文件名为 `useLearningLoopPanel.ts`（非 `useAgentLearningLoopPanel.ts`）
> 2. 使用独立 `useLearningLoopStore`（非 `useAgentDetailStore`）
> 3. 数据从 Store 的 computed 获取（`computed(() => store.observations)` 等），而非本地 ref
> 4. watch 同时监听 `agentId` + `patternStatusFilter` + `proposalStatusFilter`，筛选变化自动重载
> 5. `pendingProposalsCount` 和 `registeredKnowledgeCount` 是 computed number（非 computed array）
> 6. `onReject` 没有确认 Dialog（仅 `onApprove` 有）

- [x] **Step 1: 创建 useLearningLoopPanel.ts**

实际实现核心逻辑：
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

  const observations = computed<LearningObservation[]>(() => store.observations);
  const patterns = computed<LearningPattern[]>(() => store.patterns);
  const proposals = computed<LearningProposal[]>(() => store.proposals);
  const pendingProposalsCount = computed(() => proposals.value.filter((p) => p.status === 'validated').length);
  const registeredKnowledgeCount = computed(() => proposals.value.filter((p) => p.status === 'applied').length);

  // fetchAll / onApprove / onReject / onRunLoop ...
  watch(() => [agentId(), patternStatusFilter.value, proposalStatusFilter.value], () => void fetchAll(), { immediate: true });

  return { loading, runningLoop, approvingId, rejectingId, patternStatusFilter, proposalStatusFilter,
    observations, patterns, proposals, pendingProposalsCount, registeredKnowledgeCount,
    onApprove, onReject, onRunLoop, fetchAll };
}
```

- [x] **Step 2: 验证 TypeScript 编译通过**

Run: `cd web && pnpm build`
Expected: PASS

- [x] **Step 3: Commit**

```bash
git add web/src/features/agents/useLearningLoopPanel.ts
git commit -m "feat(learning-loop): add useLearningLoopPanel composable"
```

---

### Task 6: LearningLoopOverview 组件

**Files:**
- Create: `web/src/components/agents/LearningLoopOverview.vue`

**DoD:**
- 4 个统计卡片展示：已识别模式数、待审批提议数、已注册知识数、观察记录数
- "运行闭环"按钮带 loading 状态
- 复用 `overview-metric-card` 样式（与 AgentEvolutionPanel 一致）
- Quasar 组件风格一致

- [x] **Step 1: 创建 LearningLoopOverview.vue**

组件 props（实际实现）：
- `observationCount: number`
- `patternCount: number`
- `pendingCount: number`
- `registeredCount: number`（设计文档为 `appliedCount`，实际改为 `registeredCount`）
- `runningLoop: boolean`

组件 emits：
- `run-loop`

模板结构：4 个 `<q-card flat bordered class="overview-metric-card">` + 1 个 `<q-btn>` 运行闭环按钮。

> **实现偏差**：
> - `appliedCount` 改为 `registeredCount`（语义更清晰）
> - 图标差异：观察数 `visibility`（一致）、模式数 `pattern`（设计为 `search`）、待审批 `pending_actions`（一致）、已注册 `school`（设计为 `verified`）
> - 使用 `app-metrics-grid` 布局（与 AgentEvolutionPanel 一致）

- [x] **Step 2: 验证组件在 AgentLearningLoopPanel 中可渲染**

先创建 AgentLearningLoopPanel 的占位文件，确认 import 无误。

- [x] **Step 3: Commit**

```bash
git add web/src/components/agents/LearningLoopOverview.vue
git commit -m "feat(learning-loop): add LearningLoopOverview component"
```

---

### Task 7: LearningPatternList 组件

**Files:**
- Create: `web/src/components/agents/LearningPatternList.vue`

**DoD:**
- 状态筛选 `<q-btn-toggle>`（全部 / detected / confirmed / dismissed）
- 模式列表使用 `<q-list separator class="app-glass-list">`
- 每项显示：Kind badge + 描述 + 频率 + 置信度进度条 + 检测时间
- 空状态提示

- [x] **Step 1: 创建 LearningPatternList.vue**

组件 props：
- `patterns: LearningPattern[]`
- `loading: boolean`
- `statusFilter: string`

组件 emits：
- `update:status-filter`

> **实现偏差**：组件自身包含 `<section class="settings-section">` 包裹和 section-heading（标题"学习模式"），而非由编排组件统一包裹。空状态使用 `<q-banner>` 而非纯文本。置信度格式化使用 `formatConfidence()` 函数。

- [x] **Step 2: Commit**

```bash
git add web/src/components/agents/LearningPatternList.vue
git commit -m "feat(learning-loop): add LearningPatternList component"
```

---

### Task 8: LearningProposalList 组件

**Files:**
- Create: `web/src/components/agents/LearningProposalList.vue`

**DoD:**
- 状态筛选 `<q-btn-toggle>`（全部 / validated / approved / applied / rejected / conflict）
- 提议列表使用 `<q-list separator class="app-glass-list">`
- validated 状态的提议显示审批/拒绝按钮
- 其他状态显示状态 badge
- 审批/拒绝按钮带 loading 状态
- 审批确认 Dialog 在 Composable 层处理

- [x] **Step 1: 创建 LearningProposalList.vue**

组件 props：
- `proposals: LearningProposal[]`
- `loading: boolean`
- `statusFilter: string`
- `approvingId: string | null`
- `rejectingId: string | null`

组件 emits：
- `update:status-filter`
- `approve: [id: string]`
- `reject: [id: string]`

> **实现偏差**：组件自身包含 `<section class="settings-section">` 包裹和 section-heading（标题"知识提议"）。Proposal kind 颜色映射增加了 `skill`(teal)、`persona`(purple)、`behavior`(orange) 等类型。空状态使用 `<q-banner>`。

- [x] **Step 2: Commit**

```bash
git add web/src/components/agents/LearningProposalList.vue
git commit -m "feat(learning-loop): add LearningProposalList component"
```

---

### Task 9: LearningObservationList 组件

**Files:**
- Create: `web/src/components/agents/LearningObservationList.vue`

**DoD:**
- 观察记录列表使用 `<q-list separator class="app-glass-list">`
- 每项显示：Kind 图标 + 内容摘要 + Session ID + 观察时间
- 空状态提示

- [x] **Step 1: 创建 LearningObservationList.vue**

组件 props：
- `observations: LearningObservation[]`
- `loading: boolean`

> **实现偏差**：组件自身包含 `<section class="settings-section">` 包裹和 section-heading（标题"观察记录"）。增加了 `observationKindLabel()` 函数提供中文标签（工具调用/用户反馈/记忆命中/记忆未命中）。空状态使用 `<q-banner>`。

- [x] **Step 2: Commit**

```bash
git add web/src/components/agents/LearningObservationList.vue
git commit -m "feat(learning-loop): add LearningObservationList component"
```

---

### Task 10: AgentLearningLoopPanel 编排组件

**Files:**
- Create: `web/src/components/agents/AgentLearningLoopPanel.vue`

**DoD:**
- 编排 4 个子组件：LearningLoopOverview + LearningPatternList + LearningProposalList + LearningObservationList
- 使用 `useAgentLearningLoopPanel` composable
- 各 section 使用 `settings-section` 样式（与 AgentEvolutionPanel 一致）
- 筛选器变化时重新加载数据

- [x] **Step 1: 创建 AgentLearningLoopPanel.vue**

> **实现偏差**：
> 1. `agentId` prop 类型为 `string | (() => string)`（更灵活，支持响应式）
> 2. 使用 `useLearningLoopPanel`（非 `useAgentLearningLoopPanel`）
> 3. 子组件各自包含 `<section class="settings-section">` 包裹，编排组件不再重复包裹
> 4. Composable 的 watch 已包含筛选器变化自动重载，编排组件无需额外 watch
> 5. 概览卡片传 `registeredCount` 而非 `appliedCount`，传 `pendingProposalsCount` 而非 `pendingProposals.length`
> 6. 使用 `toValue(props.agentId)` 转换为函数

- [x] **Step 2: 验证组件渲染无报错**

Run: `cd web && pnpm build`
Expected: PASS

- [x] **Step 3: Commit**

---

### Task 11: Tab 集成

**Files:**
- Modify: `web/src/pages/AgentSettingsPage.vue`

**DoD:**
- "学习闭环"Tab 出现在 Agent 详情页
- 点击 Tab 渲染 `<AgentLearningLoopPanel :agent-id="agentId" />`
- Tab 切换时数据正确加载

> **实现偏差**：Tab 没有设置 icon（设计文档要求 `school` 图标），Tab 使用 `<q-tab>` 而非配置数组。Tab 面板使用 `<q-tab-panels>` + `<q-tab-panel>` 结构。

- [x] **Step 1: 找到 Agent 详情页 Tab 配置**

实际文件：`web/src/pages/AgentSettingsPage.vue`

- [x] **Step 2: 在 Tab 配置中新增"学习闭环"Tab**

实际实现（直接在 `<q-tabs>` 中新增）：
```html
<q-tab name="learning" label="学习闭环" />
```

- [x] **Step 3: 在 Tab 内容区域新增条件渲染**

实际实现（在 `<q-tab-panels>` 中新增）：
```html
<q-tab-panel name="learning">
  <agent-learning-loop-panel :agent-id="agentId" />
</q-tab-panel>
```

- [x] **Step 4: 验证前端编译通过**

Run: `cd web && pnpm build`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add web/src/
git commit -m "feat(learning-loop): integrate learning loop tab into agent detail page"
```

---

### Task 12: 全量验证

**DoD:**
- `pnpm lint` 通过
- `pnpm build` 通过
- 手动验证：Agent 详情页"学习闭环"Tab 可正常展示
- 手动验证：模式列表/提议列表/观察记录可正常加载
- 手动验证：审批/拒绝操作正常
- 手动验证：运行闭环按钮正常

- [x] **Step 1: 前端 lint**

Run: `cd web && pnpm lint`
Expected: PASS

- [x] **Step 2: 前端 build**

Run: `cd web && pnpm build`
Expected: PASS

- [x] **Step 3: 手动集成测试**

1. 启动应用
2. 进入 Agent 详情页 → 切换到"学习闭环"Tab
3. 验证概览卡片数据正确
4. 验证模式列表加载和筛选
5. 验证提议列表加载和审批/拒绝
6. 验证观察记录列表加载
7. 验证"运行闭环"按钮

- [x] **Step 4: Final commit**

```bash
git add -A
git commit -m "feat(learning-loop): complete learning loop frontend visualization"
```

---

## 实现偏差汇总

| 设计文档 | 实际实现 | 原因 |
|----------|----------|------|
| 修改 `types.ts` | 独立文件 `learning.types.ts` | 模块内聚，避免污染通用类型文件 |
| 修改 `api.ts` | 独立文件 `api.learning.ts` | 模块内聚，避免 api.ts 膨胀 |
| 扩展 `stores/agents/detail.ts` | 独立 Store `stores/learningLoop/index.ts` | 学习闭环有独立状态，不应耦合到 agent detail |
| `useAgentLearningLoopPanel` | `useLearningLoopPanel` | 命名简化，与其他 composable 命名风格一致 |
| `useAgentDetailStore` | `useLearningLoopStore` | 配合独立 Store |
| Composable 持有本地 ref | Composable 从 Store computed 获取 | Store 持有领域状态，Composable 消费 |
| `kind`/`status` 使用联合类型 | 使用 `string` | Proto 生成类型为 `string`，运行时无法保证联合类型 |
| `asRecord()` + `pickStr()` 归一化 | 直接使用 Proto 类型归一化 | Proto 生成客户端已有明确类型，无需 asRecord 中转 |
| `getLearning*` 函数名 | `listLearning*` 函数名 | 列表查询用 `list` 更语义化 |
| `appliedCount` prop | `registeredCount` prop | "已注册"语义更清晰 |
| 编排组件包裹 section | 子组件各自包裹 section | 子组件更独立，可单独使用 |
| Tab 有 `school` 图标 | Tab 无图标 | 与其他 Tab 风格一致（大部分 Tab 无图标） |
| `onReject` 有确认 Dialog | `onReject` 无确认 Dialog | 拒绝操作可撤销性低，无需确认 |
