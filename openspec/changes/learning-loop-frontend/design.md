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

```
Proto 生成客户端 (services/kratos/learning_loop/v1/)
    │
    ▼
api.ts — 类型定义 + HTTP 调用 + wire 归一化
    │
    ▼
Store (stores/agents/detail.ts 扩展)
    │  fetchLearningPatterns / fetchLearningProposals / fetchLearningObservations
    │  approveLearningProposal / rejectLearningProposal / runLearningLoop
    ▼
Composable (features/agents/useAgentLearningLoopPanel.ts)
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

需要先运行 `make api` 生成 `web/src/services/kratos/learning_loop/v1/` 客户端代码。

然后在 `web/src/services/index.ts` 中注册：

```typescript
import { createLearningLoopServiceClient } from "./kratos/learning_loop/v1/index";

export function createLearningLoopService() {
  return createLearningLoopServiceClient(requestHandler);
}
```

### 4.2 类型定义

在 `web/src/features/agents/types.ts` 中新增：

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

### 4.3 API 函数

在 `web/src/features/agents/api.ts` 中新增：

```typescript
export async function getLearningObservations(
  agentId: string,
  since?: string
): Promise<LearningObservation[]> {
  const svc = createLearningLoopService();
  const res = await svc.ListObservations({ agentId, since });
  return (res.items ?? []).map(normalizeObservation);
}

export async function getLearningPatterns(
  agentId: string,
  status?: string
): Promise<LearningPattern[]> {
  const svc = createLearningLoopService();
  const res = await svc.ListPatterns({ agentId, status });
  return (res.items ?? []).map(normalizePattern);
}

export async function getLearningProposals(
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

归一化函数遵循项目 `wireNormalize.ts` 模式，处理 camelCase → snake_case 转换。

---

## 5. Store 层设计

在 `web/src/stores/agents/detail.ts` 中扩展 `useAgentDetailStore`：

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

async function approveLearningProposal(agentId: string, proposalId: string): Promise<LearningProposal> {
  return approveLearningProposal(agentId, proposalId);
}

async function rejectLearningProposal(agentId: string, proposalId: string): Promise<LearningProposal> {
  return rejectLearningProposal(agentId, proposalId);
}

async function runLearningLoop(agentId: string): Promise<void> {
  return runLearningLoop(agentId);
}
```

> 遵循现有模式：Store 仅做 loading/error 包装和透传，不缓存领域状态。

---

## 6. Composable 层设计

新建 `web/src/features/agents/useAgentLearningLoopPanel.ts`：

```typescript
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

  async function fetchPatterns() { ... }
  async function fetchProposals() { ... }
  async function fetchObservations() { ... }
  async function fetchAll() { ... }
  async function onApprove(id: string) { ... }
  async function onReject(id: string) { ... }
  async function onRunLoop() { ... }

  watch(() => agentId(), () => void fetchAll(), { immediate: true });

  return {
    loading, patterns, proposals, observations,
    patternStatusFilter, proposalStatusFilter,
    approvingId, rejectingId, runningLoop,
    pendingProposals, appliedProposals, detectedPatterns,
    fetchPatterns, fetchProposals, fetchObservations, fetchAll,
    onApprove, onReject, onRunLoop
  };
}
```

> 遵循 `useAgentEvolutionPanel.ts` 的模式：composable 持有响应式状态，watch agentId 自动加载，通过 Store 调用 API。

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

| 组件 | 职责 | 关键 props | 关键 emits |
|------|------|-----------|-----------|
| **AgentLearningLoopPanel** | 编排：加载状态 + 四个子组件布局 | `agentId` | — |
| **LearningLoopOverview** | 展示统计卡片：模式数/待审批/已注册/观察数 | `patternCount`, `pendingCount`, `appliedCount`, `observationCount`, `runningLoop` | `run-loop` |
| **LearningPatternList** | 模式列表 + 状态筛选 + 置信度/频率展示 | `patterns`, `statusFilter`, `loading` | `update:statusFilter` |
| **LearningProposalList** | 提议列表 + 状态筛选 + 审批/拒绝操作 | `proposals`, `statusFilter`, `approvingId`, `rejectingId`, `loading` | `update:statusFilter`, `approve`, `reject` |
| **LearningObservationList** | 观察记录列表 + kind 图标 + 时间展示 | `observations`, `loading` | — |

### 7.3 AgentLearningLoopPanel 模板结构

```html
<div class="learning-loop-panel settings-grid settings-grid--wide">
  <section class="settings-section">
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
    <LearningPatternList
      v-model:status-filter="patternStatusFilter"
      :patterns="patterns"
      :loading="loading"
    />
  </section>

  <section class="settings-section">
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
    <LearningObservationList
      :observations="observations"
      :loading="loading"
    />
  </section>
</div>
```

### 7.4 LearningLoopOverview 设计

4 个统计卡片 + 运行闭环按钮，复用 `overview-metric-card` 样式（与 AgentEvolutionPanel 一致）：

```
┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐
│ 🔍 已识别模式 │ │ ⏳ 待审批提议 │ │ ✅ 已注册知识 │ │ 👁 观察记录   │
│     12       │ │      3       │ │      8       │ │     156      │
│ detected     │ │ validated    │ │ applied      │ │ 30 天内      │
└──────────────┘ └──────────────┘ └──────────────┘ └──────────────┘
                                              [▶ 运行闭环]
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

```typescript
const tabs = [
  { key: "basic", label: "基本", icon: "info" },
  { key: "prompt", label: "提示词", icon: "description" },
  { key: "tools", label: "工具", icon: "build" },
  { key: "memory", label: "记忆", icon: "psychology" },
  { key: "evolution", label: "进化", icon: "auto_fix_high" },
  { key: "learning", label: "学习闭环", icon: "school" },  // 新增
  // ...
];
```

当 `tab === "learning"` 时渲染 `<AgentLearningLoopPanel :agent-id="agentId" />`。

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

| 操作 | 文件 | 说明 |
|------|------|------|
| **新建** | `web/src/features/agents/useAgentLearningLoopPanel.ts` | 学习闭环 Composable |
| **新建** | `web/src/components/agents/AgentLearningLoopPanel.vue` | 学习闭环编排组件 |
| **新建** | `web/src/components/agents/LearningLoopOverview.vue` | 概览卡片组件 |
| **新建** | `web/src/components/agents/LearningPatternList.vue` | 模式列表组件 |
| **新建** | `web/src/components/agents/LearningProposalList.vue` | 提议列表组件 |
| **新建** | `web/src/components/agents/LearningObservationList.vue` | 观察记录组件 |
| **修改** | `web/src/features/agents/types.ts` | 新增 LearningObservation / LearningPattern / LearningProposal 类型 |
| **修改** | `web/src/features/agents/api.ts` | 新增 6 个 API 函数 + 归一化函数 |
| **修改** | `web/src/stores/agents/detail.ts` | 新增 6 个 Store action |
| **修改** | `web/src/services/index.ts` | 注册 createLearningLoopService |
| **修改** | Agent 详情页 Tab 配置 | 新增"学习闭环"Tab |

**不需要改动的**：
- 后端（API 已完成）
- Proto 文件（已定义）
- AgentEvolutionPanel.vue（独立 Tab，互不影响）

---

## 11. 风险与缓解

| 风险 | 缓解措施 |
|------|----------|
| Proto 生成客户端可能尚未生成 | 先运行 `make api` 确认生成，api.ts 函数签名以生成结果为准 |
| 观察记录数据量大导致渲染卡顿 | 前端限制展示最近 100 条，后端 API 已有 since 参数 |
| RunLoop 是异步操作，前端无法即时反馈 | 运行后自动刷新列表数据，按钮加 loading 防重复点击 |
| 提议审批/拒绝后列表状态不一致 | 操作成功后重新 fetchProposals + fetchPatterns |
