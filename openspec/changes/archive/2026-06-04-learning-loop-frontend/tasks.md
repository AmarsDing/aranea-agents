# 学习闭环前端可视化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Agent 详情页新增"学习闭环"Tab，展示学习闭环运行状态、已识别模式、待审批提议、已注册知识和观察记录。遵循 aranea-frontend-guide 数据流铁律：API → Store → Composable → Page → Component。

**Architecture:** 后端 API 已完成（`LearningLoopService` 6 个 HTTP 端点）。前端新增类型定义 → API 函数 → Store action → Composable → 5 个组件 → Tab 集成。

**Tech Stack:** Vue 3 + Quasar + Pinia + TypeScript

---

## File Structure

| 操作 | 文件 | 职责 |
|------|------|------|
| Create | `web/src/features/agents/useAgentLearningLoopPanel.ts` | 学习闭环 Composable |
| Create | `web/src/components/agents/AgentLearningLoopPanel.vue` | 编排组件 |
| Create | `web/src/components/agents/LearningLoopOverview.vue` | 概览卡片 |
| Create | `web/src/components/agents/LearningPatternList.vue` | 模式列表 |
| Create | `web/src/components/agents/LearningProposalList.vue` | 提议列表 |
| Create | `web/src/components/agents/LearningObservationList.vue` | 观察记录 |
| Modify | `web/src/features/agents/types.ts` | 新增 3 个类型 |
| Modify | `web/src/features/agents/api.ts` | 新增 6 个 API 函数 + 3 个归一化函数 |
| Modify | `web/src/stores/agents/detail.ts` | 新增 6 个 Store action |
| Modify | `web/src/services/index.ts` | 注册 createLearningLoopService |
| Modify | Agent 详情页 Tab 配置 | 新增"学习闭环"Tab |

---

### Task 1: Proto 生成客户端 + 服务注册

**Files:**
- Generate: `web/src/services/kratos/learning_loop/v1/`（由 `make api` 生成）
- Modify: `web/src/services/index.ts`

**DoD:**
- `make api` 成功生成 `learning_loop/v1/` 客户端代码
- `createLearningLoopService()` 在 `services/index.ts` 中注册并导出
- `pnpm build` 通过

- [ ] **Step 1: 运行 make api 生成 Proto 客户端**

Run: `make api`
Expected: 生成 `web/src/services/kratos/learning_loop/v1/index.ts` 等文件

- [ ] **Step 2: 在 services/index.ts 中注册 createLearningLoopService**

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

- [ ] **Step 3: 验证前端编译通过**

Run: `cd web && pnpm build`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add web/src/services/
git commit -m "feat(learning-loop): register LearningLoopService client"
```

---

### Task 2: 类型定义

**Files:**
- Modify: `web/src/features/agents/types.ts`

**DoD:**
- `LearningObservation`、`LearningPattern`、`LearningProposal` 类型定义完整
- 所有枚举值与后端 `learning_loop_types.go` 一致
- TypeScript 编译通过

- [ ] **Step 1: 在 types.ts 末尾新增类型定义**

```typescript
export type ObservationKind = "tool_call" | "feedback" | "memory_hit" | "memory_miss";

export type LearningObservation = {
  id: string;
  agent_id: string;
  session_id: string;
  kind: ObservationKind;
  content: string;
  metadata: string;
  observed_at: string;
};

export type PatternStatus = "detected" | "confirmed" | "dismissed";

export type LearningPattern = {
  id: string;
  agent_id: string;
  kind: string;
  description: string;
  frequency: number;
  confidence: number;
  evidence: string;
  status: PatternStatus;
  detected_at: string;
};

export type ProposalStatus =
  | "draft"
  | "validated"
  | "approved"
  | "rejected"
  | "applied"
  | "conflict"
  | "expired";

export type LearningProposal = {
  id: string;
  agent_id: string;
  pattern_id: string;
  title: string;
  content: string;
  kind: string;
  status: ProposalStatus;
  validated_at: string;
  approved_by: string;
  created_at: string;
  updated_at: string;
};
```

- [ ] **Step 2: 验证 TypeScript 编译通过**

Run: `cd web && pnpm build`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add web/src/features/agents/types.ts
git commit -m "feat(learning-loop): add LearningObservation, LearningPattern, LearningProposal types"
```

---

### Task 3: API 函数 + 归一化

**Files:**
- Modify: `web/src/features/agents/api.ts`

**DoD:**
- 6 个 API 函数实现：getLearningObservations / getLearningPatterns / getLearningProposals / approveLearningProposal / rejectLearningProposal / runLearningLoop
- 3 个归一化函数处理 camelCase → snake_case
- 函数签名与 Proto 生成客户端一致
- TypeScript 编译通过

- [ ] **Step 1: 在 api.ts 中新增 import**

```typescript
import { createLearningLoopService } from "../../services";
```

新增类型 import：
```typescript
import type {
  LearningObservation,
  LearningPattern,
  LearningProposal
} from "./types";
```

- [ ] **Step 2: 新增 3 个归一化函数**

```typescript
function normalizeObservation(raw: Record<string, unknown>): LearningObservation {
  return {
    id: pickStr(raw, "id"),
    agent_id: pickStr(raw, "agentId", "agent_id"),
    session_id: pickStr(raw, "sessionId", "session_id"),
    kind: pickStr(raw, "kind") as ObservationKind,
    content: pickStr(raw, "content"),
    metadata: pickStr(raw, "metadata"),
    observed_at: pickStr(raw, "observedAt", "observed_at")
  };
}

function normalizePattern(raw: Record<string, unknown>): LearningPattern {
  return {
    id: pickStr(raw, "id"),
    agent_id: pickStr(raw, "agentId", "agent_id"),
    kind: pickStr(raw, "kind"),
    description: pickStr(raw, "description"),
    frequency: pickI32(raw, "frequency"),
    confidence: Number(raw["confidence"] ?? 0),
    evidence: pickStr(raw, "evidence"),
    status: pickStr(raw, "status") as PatternStatus,
    detected_at: pickStr(raw, "detectedAt", "detected_at")
  };
}

function normalizeProposal(raw: Record<string, unknown>): LearningProposal {
  return {
    id: pickStr(raw, "id"),
    agent_id: pickStr(raw, "agentId", "agent_id"),
    pattern_id: pickStr(raw, "patternId", "pattern_id"),
    title: pickStr(raw, "title"),
    content: pickStr(raw, "content"),
    kind: pickStr(raw, "kind"),
    status: pickStr(raw, "status") as ProposalStatus,
    validated_at: pickStr(raw, "validatedAt", "validated_at"),
    approved_by: pickStr(raw, "approvedBy", "approved_by"),
    created_at: pickStr(raw, "createdAt", "created_at"),
    updated_at: pickStr(raw, "updatedAt", "updated_at")
  };
}
```

> 注意：归一化函数需使用 `asRecord()` 包装 Proto 返回值后再传入，参考 `getAgentPromptPreview` 的模式。具体实现以 Proto 生成客户端的返回类型为准。

- [ ] **Step 3: 新增 6 个 API 函数**

```typescript
export async function getLearningObservations(
  agentId: string,
  since?: string
): Promise<LearningObservation[]> {
  const svc = createLearningLoopService();
  const res = await svc.ListObservations({ agentId, since });
  return (res.items ?? []).map((item) => normalizeObservation(asRecord(item)));
}

export async function getLearningPatterns(
  agentId: string,
  status?: string
): Promise<LearningPattern[]> {
  const svc = createLearningLoopService();
  const res = await svc.ListPatterns({ agentId, status });
  return (res.items ?? []).map((item) => normalizePattern(asRecord(item)));
}

export async function getLearningProposals(
  agentId: string,
  status?: string
): Promise<LearningProposal[]> {
  const svc = createLearningLoopService();
  const res = await svc.ListProposals({ agentId, status });
  return (res.items ?? []).map((item) => normalizeProposal(asRecord(item)));
}

export async function approveLearningProposal(
  agentId: string,
  proposalId: string
): Promise<LearningProposal> {
  const svc = createLearningLoopService();
  const res = await svc.ApproveProposal({ agentId, id: proposalId });
  return normalizeProposal(asRecord(res));
}

export async function rejectLearningProposal(
  agentId: string,
  proposalId: string
): Promise<LearningProposal> {
  const svc = createLearningLoopService();
  const res = await svc.RejectProposal({ agentId, id: proposalId });
  return normalizeProposal(asRecord(res));
}

export async function runLearningLoop(agentId: string): Promise<void> {
  const svc = createLearningLoopService();
  await svc.RunLoop({ agentId });
}
```

- [ ] **Step 4: 在 api.ts 的 export type 区域新增导出**

```typescript
export type { LearningObservation, LearningPattern, LearningProposal } from "./types";
```

- [ ] **Step 5: 验证 TypeScript 编译通过**

Run: `cd web && pnpm build`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add web/src/features/agents/api.ts
git commit -m "feat(learning-loop): add API functions and wire normalization"
```

---

### Task 4: Store 层扩展

**Files:**
- Modify: `web/src/stores/agents/detail.ts`

**DoD:**
- 6 个 Store action 新增：fetchLearningObservations / fetchLearningPatterns / fetchLearningProposals / approveLearningProposal / rejectLearningProposal / runLearningLoop
- import 正确
- TypeScript 编译通过

- [ ] **Step 1: 新增 import**

在 detail.ts 的 import 区域新增：
```typescript
import {
  getLearningObservations, getLearningPatterns, getLearningProposals,
  approveLearningProposal, rejectLearningProposal, runLearningLoop,
  type LearningObservation, type LearningPattern, type LearningProposal
} from "../../features/agents/api";
```

- [ ] **Step 2: 在 defineStore 内新增 6 个 action**

```typescript
async function fetchLearningObservations(id: string, since?: string): Promise<LearningObservation[]> {
  return getLearningObservations(id, since);
}

async function fetchLearningPatterns(id: string, status?: string): Promise<LearningPattern[]> {
  return getLearningPatterns(id, status);
}

async function fetchLearningProposals(id: string, status?: string): Promise<LearningProposal[]> {
  return getLearningProposals(id, status);
}

async function approveLearning(agentId: string, proposalId: string): Promise<LearningProposal> {
  return approveLearningProposal(agentId, proposalId);
}

async function rejectLearning(agentId: string, proposalId: string): Promise<LearningProposal> {
  return rejectLearningProposal(agentId, proposalId);
}

async function triggerLearningLoop(agentId: string): Promise<void> {
  return runLearningLoop(agentId);
}
```

- [ ] **Step 3: 在 return 中导出新增 action**

```typescript
return {
  // ...existing
  fetchLearningObservations,
  fetchLearningPatterns,
  fetchLearningProposals,
  approveLearning,
  rejectLearning,
  triggerLearningLoop
};
```

- [ ] **Step 4: 验证 TypeScript 编译通过**

Run: `cd web && pnpm build`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/stores/agents/detail.ts
git commit -m "feat(learning-loop): add Store actions for learning loop"
```

---

### Task 5: Composable 实现

**Files:**
- Create: `web/src/features/agents/useAgentLearningLoopPanel.ts`

**DoD:**
- Composable 封装 Store 调用 + 响应式状态
- watch agentId 自动加载
- 审批/拒绝操作带确认 Dialog
- RunLoop 操作带 loading 防重复
- TypeScript 编译通过

- [ ] **Step 1: 创建 useAgentLearningLoopPanel.ts**

```typescript
import { computed, ref, watch } from "vue";
import { useQuasar } from "quasar";
import type { LearningObservation, LearningPattern, LearningProposal } from "./types";
import { useAgentDetailStore } from "../../stores/agents/detail";

export function useAgentLearningLoopPanel(agentId: () => string) {
  const $q = useQuasar();
  const agentDetailStore = useAgentDetailStore();

  const loading = ref(false);
  const patterns = ref<LearningPattern[]>([]);
  const proposals = ref<LearningProposal[]>([]);
  const observations = ref<LearningObservation[]>([]);
  const patternStatusFilter = ref<string>("");
  const proposalStatusFilter = ref<string>("");
  const approvingId = ref<string | null>(null);
  const rejectingId = ref<string | null>(null);
  const runningLoop = ref(false);

  const pendingProposals = computed(() =>
    proposals.value.filter((p) => p.status === "validated")
  );
  const appliedProposals = computed(() =>
    proposals.value.filter((p) => p.status === "applied")
  );
  const detectedPatterns = computed(() =>
    patterns.value.filter((p) => p.status === "detected")
  );

  async function fetchPatterns() {
    const id = agentId();
    if (!id) return;
    try {
      patterns.value = await agentDetailStore.fetchLearningPatterns(id, patternStatusFilter.value || undefined);
    } catch {
      patterns.value = [];
    }
  }

  async function fetchProposals() {
    const id = agentId();
    if (!id) return;
    try {
      proposals.value = await agentDetailStore.fetchLearningProposals(id, proposalStatusFilter.value || undefined);
    } catch {
      proposals.value = [];
    }
  }

  async function fetchObservations() {
    const id = agentId();
    if (!id) return;
    try {
      observations.value = await agentDetailStore.fetchLearningObservations(id);
    } catch {
      observations.value = [];
    }
  }

  async function fetchAll() {
    const id = agentId();
    if (!id) return;
    loading.value = true;
    try {
      await Promise.all([fetchPatterns(), fetchProposals(), fetchObservations()]);
    } finally {
      loading.value = false;
    }
  }

  async function onApprove(id: string) {
    const aid = agentId();
    if (!aid) return;
    $q.dialog({
      title: "审批知识提议",
      message: "确定审批此知识提议？审批后将注册到 Agent 知识库。",
      cancel: true,
      persistent: true
    }).onOk(async () => {
      approvingId.value = id;
      try {
        await agentDetailStore.approveLearning(aid, id);
        await fetchProposals();
        await fetchPatterns();
      } finally {
        approvingId.value = null;
      }
    });
  }

  async function onReject(id: string) {
    const aid = agentId();
    if (!aid) return;
    rejectingId.value = id;
    try {
      await agentDetailStore.rejectLearning(aid, id);
      await fetchProposals();
    } finally {
      rejectingId.value = null;
    }
  }

  async function onRunLoop() {
    const aid = agentId();
    if (!aid) return;
    runningLoop.value = true;
    try {
      await agentDetailStore.triggerLearningLoop(aid);
      await fetchAll();
    } finally {
      runningLoop.value = false;
    }
  }

  watch(() => agentId(), () => void fetchAll(), { immediate: true });

  return {
    loading,
    patterns,
    proposals,
    observations,
    patternStatusFilter,
    proposalStatusFilter,
    approvingId,
    rejectingId,
    runningLoop,
    pendingProposals,
    appliedProposals,
    detectedPatterns,
    fetchPatterns,
    fetchProposals,
    fetchObservations,
    fetchAll,
    onApprove,
    onReject,
    onRunLoop
  };
}
```

- [ ] **Step 2: 验证 TypeScript 编译通过**

Run: `cd web && pnpm build`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add web/src/features/agents/useAgentLearningLoopPanel.ts
git commit -m "feat(learning-loop): add useAgentLearningLoopPanel composable"
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

- [ ] **Step 1: 创建 LearningLoopOverview.vue**

组件 props：
- `patternCount: number`
- `pendingCount: number`
- `appliedCount: number`
- `observationCount: number`
- `runningLoop: boolean`

组件 emits：
- `run-loop`

模板结构：4 个 `<q-card flat bordered class="overview-metric-card">` + 1 个 `<q-btn>` 运行闭环按钮。

图标选择：
- 已识别模式：`pattern` (或 `search`)
- 待审批提议：`pending_actions`
- 已注册知识：`verified`
- 观察记录：`visibility`

- [ ] **Step 2: 验证组件在 AgentLearningLoopPanel 中可渲染**

先创建 AgentLearningLoopPanel 的占位文件，确认 import 无误。

- [ ] **Step 3: Commit**

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

- [ ] **Step 1: 创建 LearningPatternList.vue**

组件 props：
- `patterns: LearningPattern[]`
- `statusFilter: string`
- `loading: boolean`

组件 emits：
- `update:statusFilter`

每项模板：
```html
<q-item class="app-glass-list__item--lg">
  <q-item-section>
    <q-item-label class="text-weight-medium">
      <q-badge :color="patternKindColor(item.kind)" :label="item.kind" class="q-mr-sm" />
      {{ item.description }}
    </q-item-label>
    <q-item-label caption class="q-mt-xs">
      频率: {{ item.frequency }}  置信度: {{ (item.confidence * 100).toFixed(1) }}%
    </q-item-label>
    <q-item-label caption class="q-mt-xs text-grey-5">
      {{ formatDate(item.detected_at) }}
    </q-item-label>
  </q-item-section>
  <q-item-section side>
    <q-badge :color="patternStatusColor(item.status)" :label="patternStatusLabel(item.status)" />
  </q-item-section>
</q-item>
```

- [ ] **Step 2: Commit**

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

- [ ] **Step 1: 创建 LearningProposalList.vue**

组件 props：
- `proposals: LearningProposal[]`
- `statusFilter: string`
- `approvingId: string | null`
- `rejectingId: string | null`
- `loading: boolean`

组件 emits：
- `update:statusFilter`
- `approve: [id: string]`
- `reject: [id: string]`

每项模板：
```html
<q-item class="app-glass-list__item--lg">
  <q-item-section>
    <q-item-label class="text-weight-medium">
      <q-badge :color="proposalKindColor(item.kind)" :label="item.kind" class="q-mr-sm" />
      {{ item.title }}
    </q-item-label>
    <q-item-label caption class="q-mt-xs">{{ item.content }}</q-item-label>
    <q-item-label caption class="q-mt-xs text-grey-5">
      {{ formatDate(item.created_at) }}
      <span v-if="item.approved_by"> · 审批人: {{ item.approved_by }}</span>
    </q-item-label>
  </q-item-section>
  <q-item-section side>
    <div v-if="item.status === 'validated'" class="row q-gutter-xs">
      <q-btn flat round dense icon="check" color="positive" size="sm"
        :loading="approvingId === item.id" @click="emit('approve', item.id)">
        <q-tooltip>审批</q-tooltip>
      </q-btn>
      <q-btn flat round dense icon="close" color="negative" size="sm"
        :loading="rejectingId === item.id" @click="emit('reject', item.id)">
        <q-tooltip>拒绝</q-tooltip>
      </q-btn>
    </div>
    <q-badge v-else :color="proposalStatusColor(item.status)" :label="proposalStatusLabel(item.status)" />
  </q-item-section>
</q-item>
```

- [ ] **Step 2: Commit**

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

- [ ] **Step 1: 创建 LearningObservationList.vue**

组件 props：
- `observations: LearningObservation[]`
- `loading: boolean`

每项模板：
```html
<q-item class="app-glass-list__item--md">
  <q-item-section side>
    <q-icon :name="observationKindIcon(item.kind)" :color="observationKindColor(item.kind)" />
  </q-item-section>
  <q-item-section>
    <q-item-label>{{ item.content || observationKindLabel(item.kind) }}</q-item-label>
    <q-item-label caption>
      Session: {{ item.session_id.slice(0, 12) }}... · {{ formatDate(item.observed_at) }}
    </q-item-label>
  </q-item-section>
</q-item>
```

- [ ] **Step 2: Commit**

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

- [ ] **Step 1: 创建 AgentLearningLoopPanel.vue**

模板结构：
```html
<template>
  <div class="learning-loop-panel settings-grid settings-grid--wide">
    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title"><span class="section-title__text">学习闭环</span></div>
          <p class="settings-section__hint">Observation → Pattern → Proposal → Validation → Registration 完整闭环。</p>
        </div>
      </div>
      <LearningLoopOverview
        :pattern-count="detectedPatterns.length"
        :pending-count="pendingProposals.length"
        :applied-count="appliedProposals.length"
        :observation-count="observations.length"
        :running-loop="runningLoop"
        @run-loop="onRunLoop"
      />
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title"><span class="section-title__text">已识别模式</span></div>
          <p class="settings-section__hint">基于观察数据自动识别的重复行为模式。</p>
        </div>
      </div>
      <LearningPatternList
        v-model:status-filter="patternStatusFilter"
        :patterns="patterns"
        :loading="loading"
      />
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title"><span class="section-title__text">知识提议</span></div>
          <p class="settings-section__hint">基于模式生成的知识提议，可审批或拒绝。</p>
        </div>
      </div>
      <LearningProposalList
        v-model:status-filter="proposalStatusFilter"
        :proposals="proposals"
        :approving-id="approvingId"
        :rejecting-id="rejectingId"
        :loading="loading"
        @approve="onApprove"
        @reject="onReject"
      />
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title"><span class="section-title__text">观察记录</span></div>
          <p class="settings-section__hint">Agent 运行时收集的原始行为数据。</p>
        </div>
      </div>
      <LearningObservationList
        :observations="observations"
        :loading="loading"
      />
    </section>
  </div>
</template>

<script setup lang="ts">
import { watch } from "vue";
import { useAgentLearningLoopPanel } from "../../features/agents/useAgentLearningLoopPanel";
import LearningLoopOverview from "./LearningLoopOverview.vue";
import LearningPatternList from "./LearningPatternList.vue";
import LearningProposalList from "./LearningProposalList.vue";
import LearningObservationList from "./LearningObservationList.vue";

const props = defineProps<{ agentId: string }>();

const {
  loading, patterns, proposals, observations,
  patternStatusFilter, proposalStatusFilter,
  approvingId, rejectingId, runningLoop,
  pendingProposals, appliedProposals, detectedPatterns,
  fetchPatterns, fetchProposals, fetchObservations,
  onApprove, onReject, onRunLoop
} = useAgentLearningLoopPanel(() => props.agentId);

watch(patternStatusFilter, () => void fetchPatterns());
watch(proposalStatusFilter, () => void fetchProposals());
</script>
```

- [ ] **Step 2: 验证组件渲染无报错**

Run: `cd web && pnpm build`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add web/src/components/agents/AgentLearningLoopPanel.vue
git commit -m "feat(learning-loop): add AgentLearningLoopPanel orchestration component"
```

---

### Task 11: Tab 集成

**Files:**
- Modify: Agent 详情页 Tab 配置文件（需确认具体文件路径）

**DoD:**
- "学习闭环"Tab 出现在 Agent 详情页
- 点击 Tab 渲染 `<AgentLearningLoopPanel :agent-id="agentId" />`
- Tab 切换时数据正确加载
- 图标使用 `school`

- [ ] **Step 1: 找到 Agent 详情页 Tab 配置**

搜索 `evolution` Tab 的注册位置，确认文件路径。

- [ ] **Step 2: 在 Tab 配置中新增"学习闭环"Tab**

在 `evolution` Tab 后新增：
```typescript
{ key: "learning", label: "学习闭环", icon: "school" }
```

- [ ] **Step 3: 在 Tab 内容区域新增条件渲染**

```html
<AgentLearningLoopPanel v-if="activeTab === 'learning'" :agent-id="agentId" />
```

- [ ] **Step 4: 验证前端编译通过**

Run: `cd web && pnpm build`
Expected: PASS

- [ ] **Step 5: Commit**

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

- [ ] **Step 1: 前端 lint**

Run: `cd web && pnpm lint`
Expected: PASS

- [ ] **Step 2: 前端 build**

Run: `cd web && pnpm build`
Expected: PASS

- [ ] **Step 3: 手动集成测试**

1. 启动应用
2. 进入 Agent 详情页 → 切换到"学习闭环"Tab
3. 验证概览卡片数据正确
4. 验证模式列表加载和筛选
5. 验证提议列表加载和审批/拒绝
6. 验证观察记录列表加载
7. 验证"运行闭环"按钮

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "feat(learning-loop): complete learning loop frontend visualization"
```
