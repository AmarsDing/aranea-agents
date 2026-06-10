# Proposal: ChatExecutionCard 独立折叠设计

> **日期**：2026-06-09 | **版本**：v1.1 | **作者**：AI 设计
> **关联**：[59-chat-ui-optimization.md](../development/59-chat-ui-optimization.md) · [59-chat-ui-optimization.design.md](../development/59-chat-ui-optimization.design.md)（合并自原 M59 + M69）
> **竞品参考**：Fazm (Claude Code Desktop) · Harness Chat Mode · Gradio Agent UI · Cursor Thought Bubbles
>
> **v1.1 修订说明**：基于深度代码审核，发现 3 个方案缺陷并修正：(1) provide/inject 在 chat 组件目录中无先例，改用更轻量的 provide key 常量 + 类型安全 inject；(2) ToolUseEvent.expanded 字段为死代码需清理；(3) elapsed timer 的 `started_at` 可能为空需降级处理。新增附录 C（审核发现与修正记录）。

---

## 摘要

本报告提出 ChatExecutionCard 独立折叠的最终设计方案。经过三轮迭代评审（初始方案 → 优化方案 → Provide/Inject 方案），最终采用 **Provide/Inject + Signal** 架构，在不提升折叠状态、不新增组件/composable 的前提下，实现工具卡片的独立折叠控制、5 秒耗时守卫和摘要增强。

---

## 一、现状问题

### 1.1 两套独立折叠机制并存

| 层级 | 管理方 | 粒度 | 交互 |
|------|--------|------|------|
| TurnBlock 级 | `useAutoCollapse` composable | 整个 turn | 自动折叠 + 手动 toggle + 全局展开/折叠 |
| ChatExecutionCard 级 | 组件内部 `expanded` ref | 单个工具事件 | 自动折叠（running→终态）+ 手动展开 |

**问题**：两层互不感知。TurnBlock 展开后，内部 ChatExecutionCard 可能已自动折叠，用户需要两层展开才能看到内容。

### 1.2 运行时长不可见

ChatExecutionCard 在工具运行时只显示 spinner，无实时耗时计数器。长任务（>30s）无法判断是否卡死。

### 1.3 折叠态摘要信息不足

- ChatExecutionCard 折叠态摘要依赖 `event.summary`（后端字段），后端不总是提供
- ToolStrip 折叠态只显示 `"N tools · Xs"`，不区分工具类型

### 1.4 Spirit 模式折叠体验割裂

TaskExecutionPanel 直接使用 ChatExecutionCard 列表，不走 TurnBlock/useAutoCollapse 路径，无法响应全局展开/折叠操作。

---

## 二、竞品调研

### 2.1 Fazm (Claude Code Desktop) — 7 Block Type + 5s Elapsed Guard

**核心设计**：
- 7 种内容块类型（Text / Tool Call / Thinking / Discovery / Observer / System Event / Browser），每种有独立渲染通道
- **5 秒耗时守卫**：工具运行 <5s 只显示 spinner，≥5s 显示实时计时器 `5s → 6s → ... → 1m 12s`
- **连续同类合并**：40 个 file_read 变成 1 行 "40 actions"，展开后显示子列表

**可借鉴**：5s elapsed timer 是最高价值特性，直接提升长任务可观测性。

### 2.2 Harness Chat Mode — Collapsible Cards + Inline Approval

**核心设计**：
- 工具调用渲染为可折叠卡片（collapsible card）
- Bash/Write 操作显示为卡片 + 命令行 + 命名按钮
- 审批操作为内联按钮（inline approval）

**可借鉴**：卡片化展示 + 内联操作按钮的模式。

### 2.3 Gradio Agent UI — ChatMessage Metadata

**核心设计**：
- `ChatMessage.metadata` 包含 `title`、`duration`、`status`（pending/done）
- `status: "pending"` 时显示 spinner + 初始展开
- `status: "done"` 时自动折叠
- `id`/`parent_id` 支持嵌套折叠

**可借鉴**：metadata 驱动折叠状态的简洁模式。

### 2.4 Cursor Thought Bubbles — 用户意图保护

**核心设计**：
- 思考气泡在 AI 生成时展开，完成后自动折叠
- 社区强烈要求：用户手动展开后不应被自动折叠覆盖
- 请求增加 Pin 功能和全局 toggle

**可借鉴**：用户意图保护（`userManuallyExpanded`）是关键 UX 规则。

---

## 三、方案演进

### 3.1 初始方案（已否决）

引入 `ExecutionBlockGroup` + `ConsecutiveToolMerge` 两个新组件 + `useExecutionCollapse` 新 composable + 5 种 Block Type 分流。

**否决原因**：
- 2 个新组件 + 1 个新 composable，改动范围大
- 5 种 Block Type 与现有 ChatMessageRow 三路分支冲突
- 同类工具聚合打乱消息时序，与 ReAct 步骤展示冲突
- Pin 按钮过度设计（YAGNI）

### 3.2 优化方案（已否决）

将 ChatExecutionCard 折叠状态提升到 `useAutoCollapse`，统一管理两层折叠。

**否决原因**：
- Props 透传链 4 层（ChatMessagePanel → ChatMessageList → TurnBlock → ChatMessageRow → ChatExecutionCard），违反项目前端规范
- `isEventCollapsed` 函数类型 prop 每次渲染创建新引用，导致全链路重渲染
- autoCollapse watch 需要在 deep watch 内遍历所有 event 的 status 变化，复杂度 O(T×K)
- 两层折叠状态的协调逻辑复杂且脆弱

### 3.3 最终方案：Provide/Inject + Signal（采纳）

**核心思路**：不提升 ChatExecutionCard 的折叠状态，保持其内部 `expanded` ref 不变。通过 `provide/inject` 传递全局控制信号，ChatExecutionCard 自行决定是否响应。

---

## 四、最终方案详细设计

### 4.1 架构

```
ChatMessagePanel
├── provide('executionCollapseControl', {
│     expandAllSignal: Ref<number>,     // 递增计数器
│     collapseAllSignal: Ref<number>,   // 递增计数器
│   })
│
├── TurnBlock (useAutoCollapse 管理 block 级折叠，不变)
│   ├── ToolStrip
│   └── ChatMessageRow
│       └── ChatExecutionCard
│           ├── inject('executionCollapseControl')
│           ├── 内部 expanded ref（不变）
│           ├── autoCollapse watch（不变）
│           ├── 新增：响应 expandAll/collapseAll signal
│           └── 新增：5s elapsed timer
│
└── TaskExecutionPanel (Spirit 模式)
    └── ChatExecutionCard (同样 inject，自动生效)
```

### 4.2 Provide/Inject 控制信号

**类型定义**（新增到 `features/chat/types.ts`）：

```typescript
// Provide key 常量，避免魔术字符串
export const EXECUTION_COLLAPSE_CONTROL_KEY: InjectionKey<ExecutionCollapseControl> =
  Symbol('execution-collapse-control');

export interface ExecutionCollapseControl {
  expandAllSignal: Readonly<Ref<number>>;
  collapseAllSignal: Readonly<Ref<number>>;
}
```

**Provider**（`ChatMessagePanel.vue`）：

```typescript
import { EXECUTION_COLLAPSE_CONTROL_KEY } from '../../features/chat/types';

const expandAllSignal = ref(0);
const collapseAllSignal = ref(0);

provide(EXECUTION_COLLAPSE_CONTROL_KEY, {
  expandAllSignal: readonly(expandAllSignal),
  collapseAllSignal: readonly(collapseAllSignal),
});

function handleExpandAll() {
  expandAllBlocks();        // TurnBlock 级（现有逻辑）
  expandAllSignal.value++;  // ChatExecutionCard 级
}

function handleCollapseAll() {
  collapseAllBlocks();        // TurnBlock 级（现有逻辑）
  collapseAllSignal.value++;  // ChatExecutionCard 级
}
```

**Consumer**（`ChatExecutionCard.vue`）：

```typescript
import { EXECUTION_COLLAPSE_CONTROL_KEY } from '../../features/chat/types';

const control = inject(EXECUTION_COLLAPSE_CONTROL_KEY, null);

// 响应全局展开
watch(() => control?.expandAllSignal.value, () => {
  if (!control) return;
  expanded.value = true;
  userManuallyExpanded.value = true;  // 阻止后续自动折叠
});

// 响应全局折叠（运行中的工具不折叠）
watch(() => control?.collapseAllSignal.value, () => {
  if (!control) return;
  if (props.event.status !== 'running') {
    expanded.value = false;
    userManuallyExpanded.value = false;
  }
});
```

### 4.3 五秒耗时守卫

**实现**（`ChatExecutionCard.vue` 内部）：

> **审核修正**：`started_at` 为可选字段（`started_at?: string`），后端可能不提供。降级策略：无 `started_at` 时使用组件挂载时间作为起点（精度略低但可接受），而非不显示计时器。

```typescript
const effectiveStartTime = computed(() => {
  if (props.event.started_at) return new Date(props.event.started_at).getTime();
  // 降级：使用 occurred_at 或组件挂载时间
  if (props.event.occurred_at) return new Date(props.event.occurred_at).getTime();
  return Date.now(); // 最终降级：组件创建时间
});
const elapsedSeconds = ref(0);
let elapsedTimer: ReturnType<typeof setInterval> | null = null;

watch(() => props.event.status, (status) => {
  if (status === 'running') {
    // 初始化 elapsed（可能已经运行了一段时间）
    elapsedSeconds.value = Math.max(0, Math.floor((Date.now() - effectiveStartTime.value) / 1000));
    elapsedTimer = setInterval(() => {
      elapsedSeconds.value = Math.floor((Date.now() - effectiveStartTime.value) / 1000);
    }, 1000);
  } else {
    if (elapsedTimer) { clearInterval(elapsedTimer); elapsedTimer = null; }
    if (status !== 'running') elapsedSeconds.value = 0;
  }
}, { immediate: true });

onBeforeUnmount(() => {
  if (elapsedTimer) clearInterval(elapsedTimer);
});

const showElapsed = computed(() =>
  props.event.status === 'running' && elapsedSeconds.value >= 5
);

const elapsedLabel = computed(() => {
  const s = elapsedSeconds.value;
  if (s < 60) return `${s}s`;
  const min = Math.floor(s / 60);
  const sec = s % 60;
  return `${min}m ${sec}s`;
});

const elapsedColor = computed(() =>
  elapsedSeconds.value >= 60 ? 'var(--color-warning)' : 'var(--color-text-tertiary)'
);
```

**视觉规范**：

| 运行时长 | 显示 | 颜色 |
|----------|------|------|
| < 5s | 仅 spinner | — |
| 5s ~ 59s | `5s` `6s` ... | `var(--color-text-tertiary)` |
| ≥ 60s | `1m 12s` | `var(--color-warning)` |

### 4.4 折叠态摘要兜底

**实现**（`ChatExecutionCard.vue` 内部）：

```typescript
const displaySummary = computed(() => {
  if (props.event.summary) return props.event.summary;
  const args = props.event.arguments;
  if (!args) return '';
  switch (props.event.tool_name) {
    case 'file_edit':
    case 'file_write':
      return args.path ? `修改 ${String(args.path).split('/').pop()}` : '';
    case 'file_read':
      return args.path ? `读取 ${String(args.path).split('/').pop()}` : '';
    case 'grep':
    case 'search_files':
      return args.pattern ? `搜索 "${truncate(String(args.pattern), 30)}"` : '';
    case 'bash':
      return args.command ? `> ${truncate(String(args.command), 40)}` : '';
    default:
      return '';
  }
});

function truncate(str: string, max: number): string {
  return str.length > max ? str.slice(0, max) + '...' : str;
}
```

### 4.5 ToolStrip 摘要增强

**实现**（`ToolStrip.vue` 内部）：

```typescript
const toolBreakdown = computed(() => {
  const groups = new Map<string, number>();
  for (const t of props.tools) {
    const ev = toolEventFromMessage(t);
    const name = ev?.tool_name ?? 'unknown';
    groups.set(name, (groups.get(name) ?? 0) + 1);
  }
  return [...groups.entries()]
    .sort((a, b) => b[1] - a[1])  // 按数量降序
    .map(([name, count]) => count > 1 ? `${count} ${name}` : name)
    .join(' + ');
});
```

**效果对比**：

```
修改前: "3 tools · 2.5s"
修改后: "3 file_read · 2.5s"  或  "2 grep + 1 bash · 2.5s · 1 failed"
```

---

## 五、改动清单

| 文件 | 改动类型 | 改动内容 | Phase |
|------|----------|----------|-------|
| `features/chat/types.ts` | 修改 | 新增 `ExecutionCollapseControl` 接口 + `EXECUTION_COLLAPSE_CONTROL_KEY` InjectionKey；清理 `ToolUseEvent.expanded` 死代码字段 | P2 |
| `components/chat/ChatExecutionCard.vue` | 修改 | 5s elapsed timer + 摘要兜底 + inject signal 响应 + `onBeforeUnmount` timer 清理 | P1+P2 |
| `components/chat/ToolStrip.vue` | 修改 | 折叠态摘要增强（tool breakdown），需 import `toolEventFromMessage` | P1 |
| `components/chat/ChatMessagePanel.vue` | 修改 | provide 控制信号 + 全局按钮增强 | P2 |
| `components/spirit/TaskExecutionPanel.vue` | 无改动 | ChatExecutionCard 自动 inject，无需修改 | P3 |

**新增组件数**：0
**新增 composable 数**：0
**新增文件数**：0

---

## 六、边界场景处理

| # | 场景 | 处理方式 |
|---|------|----------|
| 1 | TurnBlock 折叠时 ChatExecutionCard 不渲染，signal 无法到达 | 无需处理——TurnBlock 展开后 ChatExecutionCard 重新渲染，按自身 `initialCollapsed` + `autoCollapse` 逻辑决定状态 |
| 2 | 运行中的工具收到 collapseAll signal | ChatExecutionCard 忽略（`status === 'running'` 时不折叠） |
| 3 | 用户手动展开后收到 collapseAll signal | 允许折叠（collapseAll 是显式用户操作，覆盖手动展开意图） |
| 4 | 多个 ChatExecutionCard 同时响应 signal | 各自独立响应，无竞态（signal 是只读 ref，无副作用） |
| 5 | TaskExecutionPanel 中 ChatExecutionCard 不在 TurnBlock 内 | 自动 inject 同一 provide，行为一致 |
| 6 | 会话切换 | `reset()` 重置 signal 计数器，ChatExecutionCard 因 v-if 销毁重建，状态自然重置 |
| 7 | 虚拟滚动下 ChatExecutionCard 被回收 | 回收后 timer 清理（`onBeforeUnmount`），重新渲染时按 `startTime` 重新计算 elapsed |
| 8 | `event.started_at` 为空 | 降级链：`started_at` → `occurred_at` → `Date.now()`（组件创建时间），始终启动 timer |

---

## 七、实施路线

### Phase 1：5s Elapsed Timer + 摘要增强（独立交付）

- ChatExecutionCard：5s elapsed timer
- ChatExecutionCard：摘要兜底生成
- ToolStrip：tool breakdown 摘要

**验收标准**：
- 工具运行 ≥5s 时显示实时计时器
- 折叠态摘要不再为空（有兜底生成）
- ToolStrip 折叠态显示工具类型分布

### Phase 2：Provide/Inject 全局控制

- types.ts：新增 `ExecutionCollapseControl` 接口
- ChatMessagePanel：provide 控制信号
- ChatExecutionCard：inject + watch signal
- 全局按钮：expandAll/collapseAll 同时操作两层

**验收标准**：
- 点击"展开全部"：TurnBlock 展开 + 内部 ChatExecutionCard 展开
- 点击"折叠全部"：TurnBlock 折叠 + 内部 ChatExecutionCard 折叠（运行中除外）
- TaskExecutionPanel 中的 ChatExecutionCard 同样响应

### Phase 3：Spirit 模式统一（自动生效）

- 无代码改动
- TaskExecutionPanel 中的 ChatExecutionCard 自动 inject 控制信号

### Phase 4：体验打磨（✅ 已完成）

- ToolStrip：`<details>` → `q-expansion-item`（统一折叠动画）✅
- ChatExecutionCard：`aria-expanded` / `aria-controls`（无障碍）✅
- 虚拟滚动兼容验证 ✅（已隐式实现）
- Provide `readonly()` 运行时包装 ✅
- Summary fallback 语言改中文 ✅

---

## 八、风险评估

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|----------|
| provide/inject 在非 ChatMessagePanel 上下文中失效 | 低 | 中 | inject 有 fallback 默认值，无 signal 时行为不变 |
| elapsed timer 在虚拟滚动回收后泄漏 | 低 | 中 | `onBeforeUnmount` 清理 timer |
| collapseAll signal 与 autoCollapse watch 冲突 | 低 | 低 | collapseAll 显式设置 `userManuallyExpanded = false`，与 autoCollapse 逻辑一致 |
| 全局展开/折叠按钮语义变化（从 TurnBlock 级变为两层） | 中 | 低 | 按钮行为更符合用户预期（"展开全部"应该展开所有内容） |

---

## 附录 A：与原方案对比

| 维度 | 初始方案 | 优化方案（已否决） | 最终方案 |
|------|----------|-------------------|----------|
| 新增组件 | 2 | 0 | 0 |
| 新增 composable | 1 | 0 | 0 |
| Props 透传层级 | — | 4 层 | 0 层 |
| 消息流结构变更 | 聚合打乱时序 | 不变 | 不变 |
| 渲染管线变更 | 5 种 Block Type | 不变 | 不变 |
| 折叠状态一致性 | 两套 composable | 统一到 useAutoCollapse | 各自独立 + signal 协调 |
| 实施风险 | 高 | 中 | 低 |
| 5s Elapsed Timer | 有 | 有 | 有 |
| 同类聚合 | 组件级 | 摘要级 | 摘要级 |
| Pin 按钮 | 有 | 无 | 无 |

## 附录 B：关键代码路径

### B.1 当前 ChatExecutionCard 折叠状态管理

```typescript
// ChatExecutionCard.vue (当前)
const expanded = ref(!props.initialCollapsed);
const userManuallyExpanded = ref(false);

watch(() => props.event.status, (newStatus) => {
  if (!props.autoCollapse) return;
  if (newStatus === 'running') {
    expanded.value = true;
    userManuallyExpanded.value = false;
  } else if (['success', 'failed', 'cancelled'].includes(newStatus) && !userManuallyExpanded.value) {
    expanded.value = false;
  }
});

function onExpanded(value: boolean) {
  if (value) userManuallyExpanded.value = true;
}
```

### B.2 当前 useAutoCollapse 核心逻辑

```typescript
// useAutoCollapse.ts (当前)
watch(turnBlocks, (blocks, prevBlocks) => {
  if (expandAllActive.value) return;
  const prevMap = new Map(prevBlocks?.map(b => [b.key, b]) ?? []);
  for (const block of blocks) {
    if (!block.isCompleted) continue;
    if (block.tools.length === 0 && block.members.length === 0) continue; // OBS-01
    const prev = prevMap.get(block.key);
    if (!prev || !prev.isCompleted) {
      collapsedBlockKeys.value.add(block.key);
    }
  }
}, { deep: true });
```

### B.3 当前全局按钮逻辑

```typescript
// ChatMessagePanel.vue (当前)
function handleExpandAll() {
  expandAll();  // useAutoCollapse.expandAll()
}
function handleCollapseAll() {
  collapseAll();  // useAutoCollapse.collapseAll()
}
```

---

## 附录 C：审核发现与修正记录

### C.1 审核发现

| # | 发现 | 严重度 | 修正措施 |
|---|------|--------|----------|
| A-01 | provide/inject 使用字符串 key（`'executionCollapseControl'`），无类型安全 | 中 | 改用 `InjectionKey<ExecutionCollapseControl>` 类型化 key 常量 `EXECUTION_COLLAPSE_CONTROL_KEY` |
| A-02 | `ToolUseEvent.expanded` 字段存在于类型定义中但从未被消费，是死代码 | 低 | 在 P1.5 阶段与折叠增强一起清理（SP-FE-31），避免与新增的 inject 控制混淆 |
| A-03 | `started_at` 为可选字段，后端可能不提供，原方案在 `started_at` 为空时不启动 timer | 高 | 增加降级链：`started_at` → `occurred_at` → `Date.now()`，始终启动 timer |
| A-04 | chat 组件目录下无 provide/inject 先例，引入新模式需谨慎 | 中 | 使用 `InjectionKey` + `Symbol` 确保类型安全和隔离性；inject fallback 为 `null`，无 provide 时行为完全不变 |
| A-05 | ToolStrip 未 import `toolEventFromMessage`，摘要增强需要新增 import | 低 | P1 阶段在 ToolStrip 中新增 import |
| A-06 | ChatExecutionCard 无 `onBeforeUnmount`，elapsed timer 需要清理 | 中 | 新增 `onBeforeUnmount` 清理 `setInterval` |
| A-07 | 全局按钮当前直接调用 `expandAll()`/`collapseAll()`（useAutoCollapse 方法），无 `handleExpandAll`/`handleCollapseAll` 包装函数 | 低 | P2 阶段新增包装函数，同时操作 TurnBlock 级 + ChatExecutionCard 级 |

### C.2 遗留风险

| # | 风险 | 说明 | 处理建议 |
|---|------|------|----------|
| R-01 | `effectiveStartTime` 使用 `Date.now()` 降级时，如果组件在工具运行一段时间后才挂载（虚拟滚动），elapsed 会从 0 开始计时 | 精度损失可接受：用户感知的是"这个工具已经运行了多久"，而非精确到毫秒的计时 | 可在 Phase 4 考虑从 Store 获取工具开始时间 |
| R-02 | `expandAllSignal`/`collapseAllSignal` 递增计数器在极端情况下可能溢出 | JavaScript number 安全整数范围 2^53，每秒递增一次需要 2.85 亿年才会溢出 | 无需处理 |
| R-03 | 多个 ChatExecutionCard 同时 watch 同一个 signal，可能产生批量 DOM 更新 | Vue 的响应式系统会批量处理同一 tick 内的更新，实际只触发一次 DOM 更新 | 无需处理 |
