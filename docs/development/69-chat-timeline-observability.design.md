# M69: Chat 时间线展示与团队列表修复 — 实现设计

> **版本**：2026-06-09 | **状态**：Implemented
> **需求规格**：[69-chat-timeline-observability.md](./69-chat-timeline-observability.md)
> **开发计划**：[69-chat-timeline-observability.development.md](./69-chat-timeline-observability.development.md)

---

## 1. 设计概览

本设计解决三个核心问题：
1. 团队列表数据加载缺失（致命 Bug）
2. 消息展示缺乏时间线脉络
3. 已有组件未集成到执行面板

---

## 2. D1: 修复团队列表数据加载

### 2.1 问题根因

`spiritStore.loadSpiritTeams(spiritSessionId)` 已定义但从未被调用。团队数据只能通过 WS 事件被动添加，刷新页面后历史团队全部丢失。

### 2.2 修复方案

**变更文件**：`web/src/features/chat/composables/useChatWorkspace.ts`

在 `useChatWorkspace` 中添加 watch，监听 Spirit session 选择变化：

```typescript
// 当选中 Spirit Agent 的 session 时，加载团队列表
watch(
  () => ({
    sessionId: sessionStore.selectedSession?.id,
    agentKey: appStore.selectedAgent?.agent_key,
  }),
  ({ sessionId, agentKey }) => {
    if (!sessionId) return;
    if (agentKey === '__spirit__') {
      spiritStore.loadSpiritTeams(sessionId);
    } else {
      // 非 Spirit session 时清理团队数据
      if (spiritStore.teams.length > 0) {
        spiritStore.reset();
      }
    }
  },
  { immediate: true },
);
```

### 2.3 WS 重连恢复

**变更文件**：`web/src/features/chat/composables/useChatWorkspace.ts`

监听 `streamManager.wsReplaying` 从 `true` 变为 `false`，在 WS replay 完成后恢复团队数据：

```typescript
watch(streamManager.wsReplaying, (replaying, wasReplaying) => {
  if (wasReplaying && !replaying && spiritStore.currentSpiritSessionId) {
    spiritStore.reloadTeams();
  }
});
```

> **评审修正 D-6**：原设计将 WS 重连恢复放在 `useChatInboundSync.ts`，实际实现改为在 `useChatWorkspace.ts` 中监听 `wsReplaying` 信号。原因是 `useChatWorkspace` 已持有 `streamManager` 和 `spiritStore` 引用，且 `wsReplaying` 提供了更精确的"replay 完成"时机，避免了在 inbound sync 回调中猜测重连完成时间点的问题。

### 2.4 数据流验证

```
用户选择 Spirit Agent → useChatWorkspace watch 触发
  → spiritStore.loadSpiritTeams(sessionId)
  → listSpiritTeams API (GET /v1/spirit/{sessionId}/teams)
  → teams.value 更新
  → ChatEntitySidebar 接收 spiritStore.sortedTeams
  → activeTeamList / completedTeamList 计算属性更新
  → TeamTaskCard 渲染

WS 事件 spirit_team_assembled → handleSpiritEnvelope → addTeam()
WS 重连 → reloadTeams() → 重新加载完整列表
```

---

## 3. D2: 时间线展示模型重构

### 3.1 类型定义

**新增文件**：`web/src/features/chat/timelineTypes.ts`

```typescript
/** 时间线元素类型 */
export type TimelineElementKind = 'user' | 'thinking' | 'action' | 'summary' | 'end' | 'error';

/** 时间线元素 */
export interface TimelineElement {
  kind: TimelineElementKind;
  id: string;
  timestamp: string;
  /** thinking: reasoning 内容 */
  reasoning?: string;
  /** action: 工具调用信息 */
  toolName?: string;
  toolStatus?: string;
  toolDuration?: number;
  toolCallId?: string;
  /** user/summary: 用户消息或最终回复内容 */
  content?: string;
  /** end: turn 完成状态 */
  turnStatus?: string;
  /** error: 错误信息 */
  errorMessage?: string;
  /** 折叠状态：thinking 和 action 完成后自动折叠 */
  collapsed: boolean;
}
```

### 3.2 时间线计算逻辑

**变更文件**：`web/src/features/chat/composables/useChatTimeline.ts`

扩展 `useChatTimeline` composable，新增 `timelineElements` 计算属性：

> **评审修正 D-1**：原设计直接访问 `block.assistant.reactSteps`，但 Message 类型上不存在此字段。实际实现通过 `resolveAssistantPresentation(plannerKind, message)` 从 `content_markdown` 解析获得 ReAct 步骤。`resolveAssistantPresentation` 是已有的工具函数（定义在 `messagePlannerPresentation.ts`），根据 `plannerKind` 选择正确的解析策略。

> **评审修正 D-3**：`timelineElements` 作为 `useChatTimeline` composable 的返回值暴露，由 `ChatMessagePanel` 调用后通过 props 传递给 `ChatMessageList`，数据流清晰：`useChatTimeline → ChatMessagePanel → ChatMessageList (props)`。

```typescript
const timelineElements = computed<TimelineElement[]>(() => {
  if (!useTurnBlockMode.value) return [];
  const plannerKind = deps.plannerKind?.value ?? '';
  const elements: TimelineElement[] = [];

  for (const block of turnBlocks.value) {
    const reactLinkedToolIds = new Set<string>();

    // 0. User message at the start of each turn
    if (block.user) {
      elements.push({
        kind: 'user',
        id: `${block.key}-user`,
        timestamp: block.user.created_at || '',
        content: block.user.content_markdown || '',
        collapsed: false,
      });
    }

    if (block.assistant) {
      // 通过 resolveAssistantPresentation 解析 ReAct 步骤
      const presentation = resolveAssistantPresentation(plannerKind, block.assistant);
      const steps = presentation.reactSteps?.steps;

      if (steps?.length) {
        for (const step of steps) {
          const isThinking = step.kind === 'planning' || step.kind === 'reasoning' || step.kind === 'replanning';
          const isAction = step.kind === 'action';

          if (isThinking) {
            elements.push({
              kind: 'thinking',
              id: `${block.key}-think-${elements.length}`,
              timestamp: block.assistant.created_at || '',
              reasoning: step.body,
              collapsed: true,
            });
          } else if (isAction) {
            // Find matching tool messages for this action step
            let matchedTool: Message | undefined;
            for (const toolMsg of block.tools) {
              const toolEv = toolEventFromMessage(toolMsg);
              if (toolEv?.id && !reactLinkedToolIds.has(toolEv.id)) {
                matchedTool = toolMsg;
                reactLinkedToolIds.add(toolEv.id);
                break;
              }
            }
            const toolEv = matchedTool ? toolEventFromMessage(matchedTool) : null;
            elements.push({
              kind: 'action',
              id: `${block.key}-action-${elements.length}`,
              timestamp: matchedTool?.created_at || block.assistant.created_at || '',
              reasoning: step.body,
              toolName: toolEv?.tool_name,
              toolStatus: toolEv?.status,
              toolDuration: toolEv?.duration_ms,
              toolCallId: toolEv?.id,
              collapsed: true,
            });
          }
        }
      }

      // Standalone tool messages not covered by ReAct steps
      for (const toolMsg of block.tools) {
        const toolEv = toolEventFromMessage(toolMsg);
        if (toolEv?.id && reactLinkedToolIds.has(toolEv.id)) continue;
        elements.push({
          kind: 'action',
          id: `${block.key}-tool-${toolMsg.id}`,
          timestamp: toolMsg.created_at || '',
          toolName: toolEv?.tool_name,
          toolStatus: toolEv?.status,
          toolDuration: toolEv?.duration_ms,
          toolCallId: toolEv?.id,
          collapsed: true,
        });
      }

      // Summary: assistant body content (via presentation.bodyMarkdown)
      const body = presentation.bodyMarkdown?.trim();
      if (body) {
        elements.push({
          kind: 'summary',
          id: `${block.key}-summary`,
          timestamp: block.assistant.created_at || '',
          content: body,
          collapsed: false,
        });
      }
    } else {
      // No assistant message — emit standalone tool messages
      for (const toolMsg of block.tools) {
        const toolEv = toolEventFromMessage(toolMsg);
        elements.push({
          kind: 'action',
          id: `${block.key}-tool-${toolMsg.id}`,
          timestamp: toolMsg.created_at || '',
          toolName: toolEv?.tool_name,
          toolStatus: toolEv?.status,
          toolDuration: toolEv?.duration_ms,
          toolCallId: toolEv?.id,
          collapsed: true,
        });
      }
    }

    // End element when block is completed
    if (block.isCompleted) {
      elements.push({
        kind: 'end',
        id: `${block.key}-end`,
        timestamp: block.assistant?.created_at || block.tools.at(-1)?.created_at || '',
        turnStatus: 'completed',
        collapsed: false,
      });
    }
  }

  return elements;
});
```

**composable 签名扩展**：

```typescript
export function useChatTimeline(deps: {
  messages: ComputedRef<Message[]>;
  isTeamSession?: boolean;
  plannerKind?: ComputedRef<string>;  // 新增：用于 resolveAssistantPresentation
}) {
  // ...
  return { turnBlocks, useTurnBlockMode, timelineElements };
}
```

### 3.3 时间线渲染组件

**新增文件**：`web/src/components/chat/TimelineNode.vue`

纯展示组件（仅 props/emits），渲染单个时间线节点。支持 6 种元素类型：

| kind | 图标 | 折叠 | 说明 |
|------|------|------|------|
| `user` | `person` | 不折叠 | 用户消息，markdown 渲染 |
| `thinking` | `psychology` | 默认折叠 | reasoning 内容，折叠态显示最后 2 行预览 |
| `action` | `bolt` | 默认折叠 | 工具名+状态+耗时，展开显示 `reasoning` 内容 |
| `summary` | `article` | 不折叠 | 最终回复，markdown 渲染 |
| `end` | `check_circle` | 不折叠 | Turn 完成标记 |
| `error` | `error` | 默认折叠 | 错误摘要，展开显示完整信息 |

**Props**：
- `element: TimelineElement` — 时间线元素数据
- `isLast: boolean` — 是否最后一个节点（控制连接线）

**Emits**：
- `toggle` — 切换折叠/展开

**样式特点**：
- 左侧时间线轨道：竖线 + 节点圆点，颜色按 kind 映射
- 用户消息 body 使用 `--color-accent` 淡色背景区分
- 折叠/展开动画（`timeline-expand` keyframe）
- 所有颜色使用 CSS 变量（`var(--*)`），无硬编码 hex

### 3.4 ChatMessageList 集成

**变更文件**：`web/src/components/chat/ChatMessageList.vue`

三路条件渲染（非虚拟滚动模式）：

```vue
<!-- 1. 时间线模式：timelineElements 优先 -->
<template v-if="useTurnBlockMode && timelineElements.length > 0">
  <TimelineNode v-for="(el, idx) in resolvedTimelineElements" ... />
</template>

<!-- 2. TurnBlock 模式：无 timelineElements 时回退 -->
<template v-else-if="useTurnBlockMode">
  <TurnBlock v-for="block in turnBlocks" ... />
</template>

<!-- 3. 逐条消息模式 -->
<ChatMessageRow v-else ... />
</template>
```

虚拟滚动模式通过 `virtualItems` 计算属性自动切换数据源：

```typescript
const virtualItems = computed(() => {
  if (props.useTurnBlockMode && props.timelineElements.length > 0) {
    return resolvedTimelineElements.value;  // TimelineElement[]
  }
  return props.timelineItems;  // TimelineItem[]
});
```

**折叠状态管理**：`collapseState` reactive Map + watch 清理 stale entries + `defaultCollapsedByKind` 函数。

### 3.5 Team 会话启用时间线

**变更文件**：`web/src/features/chat/composables/useChatTimeline.ts`

移除 Team 会话禁用 TurnBlock 的逻辑：

```typescript
// 之前：Team 会话禁用 TurnBlock
// const useTurnBlockMode = computed(() => useTurnBlockEnabled() && !deps.isTeamSession);

// 之后：所有模式均启用
const useTurnBlockMode = computed(() => useTurnBlockEnabled());
```

---

## 4. D3: TaskExecutionPanel 集成三区布局

### 4.1 当前问题

`TeamProgressCard` 和 `SynthesisResultCard` 已编写且功能完整，但 `TaskExecutionPanel` 未引用它们。`ParallelTeamOverview` 和 `InterruptedTeamCard` 已在 TaskExecutionPanel 中集成。

> **评审修正 D-4**：原设计描述"三个组件均未引用"，实际 `ParallelTeamOverview` 和 `InterruptedTeamCard` 已在 TaskExecutionPanel 中集成。仅需补充集成 `TeamProgressCard` 和 `SynthesisResultCard`。

### 4.2 集成方案

**变更文件**：`web/src/components/spirit/TaskExecutionPanel.vue`

将 TaskExecutionPanel 的布局从简化版升级为三区布局：

```
TaskExecutionPanel
  ├── 顶部导航栏（返回精灵 + 团队名称 + 状态 Badge + 编排模式标签）
  ├── ParallelTeamOverview     ← 已有组件，直接引用
  │     ├── DAGDiagramCard
  │     └── 并行配额进度条
  ├── TeamProgressCard (×N)    ← 已有组件，直接引用
  ├── InterruptedTeamCard      ← 已有组件，条件显示
  ├── 执行时间线（TimelineElement 序列）
  └── SynthesisResultCard      ← 已有组件，条件显示
```

### 4.3 数据流

```
spiritStore.activeTeam → TaskExecutionPanel props
  ├── spiritStore.teams → ParallelTeamOverview
  ├── spiritStore.teams → TeamProgressCard (×N)
  ├── spiritStore.activeTeam (interrupted) → InterruptedTeamCard
  ├── session.displayMessages → useChatTimeline → TimelineNode (×N)
  └── spiritStore.synthesisResult → SynthesisResultCard
```

---

## 5. D4: 左侧面板简化

### 5.1 变更方案

**变更文件**：`web/src/components/chat/ChatEntitySidebar.vue`

- 隐藏 Agent 分组列表（`ChatSectionHeader` + `ChatEntityGroup`）
- 保留 SpiritEntry
- 仅显示：精灵入口 + 进行中团队 + 已完成团队
- 搜索框搜索范围限定为团队名称

### 5.2 实现方式

通过 `spiritMode` prop 控制是否显示 Agent 列表：

> **评审修正 D-5**：原设计使用 `selectedKind === 'spirit'` 判断，实际实现使用 `spiritStore.activePanelMode === 'spirit'` 作为 `spiritMode` prop 传入。由 `ChatPage.vue` 计算后传入 `ChatEntitySidebar`，避免展示组件直接依赖 store 判断逻辑。

```vue
<!-- ChatPage.vue -->
<ChatEntitySidebar :spirit-mode="spiritStore.activePanelMode === 'spirit'" />

<!-- ChatEntitySidebar.vue -->
<template v-if="!spiritMode">
  <ChatSectionHeader icon="smart_toy" ... />
  <ChatEntityGroup v-for="group in agentGroups" ... />
</template>
```

当 `spiritMode` 为 `true` 时，左侧面板仅显示精灵入口和团队列表。

---

## 6. 影响域

| 变更 | 影响文件 | 影响范围 |
|------|----------|----------|
| D1 团队数据加载 | `useChatWorkspace.ts` | composable 层 |
| D2 时间线模型 | `useChatTimeline.ts`, `timelineTypes.ts`(新), `TimelineNode.vue`(新), `ChatMessageList.vue`, `ChatMessagePanel.vue` | composable + 组件层 |
| D3 三区布局 | `TaskExecutionPanel.vue` | 组件层 |
| D4 左侧面板 | `ChatEntitySidebar.vue`, `ChatPage.vue` | 组件层 |

> **评审修正 D-2**：原设计将 Team 会话启用时间线的变更文件列为 `ChatMessagePanel.vue`，实际实现中 `useTurnBlockMode` 的条件修改在 `useChatTimeline.ts` 中完成（移除 `!deps.isTeamSession` 条件），`ChatMessagePanel.vue` 仅负责传入 `plannerKind` 参数和传递 `timelineElements` prop。

### 6.1 不影响的文件

- 后端代码：无变更
- Store 层：仅 `spirit/index.ts` 已有方法被调用，无新增
- API 层：无变更
- Proto 层：无变更

---

## 7. 风险与缓解

| 风险 | 缓解措施 |
|------|----------|
| 时间线模式可能影响虚拟滚动性能 | TimelineElement 数量通常 < 20/turn，性能可控 |
| Team 会话启用时间线可能导致消息闪烁 | 使用 `groupMessagesByTurn` 堆栈模型，不使用 turn_index |
| 左侧面板隐藏 Agent 列表可能影响非 Spirit 用户 | 通过 `selectedKind` 条件控制，非 Spirit 模式仍显示 Agent |

---

## 8. 第三轮审查修复记录

> **日期**：2026-06-09 | **触发**：用户反馈 UI 实际显示问题

| ID | 问题 | 根因 | 修复 | 影响文件 |
|----|------|------|------|----------|
| F-1 | 左侧面板团队列表始终无数据 | API 响应字段名不匹配：前端读 `data.items`，后端返回 `data.teams` | `data?.items` → `data?.teams` | `features/spirit/api.ts` |
| F-2 | 工具名显示一长串原始名称（如 `subagents_spawn`） | TimelineNode 使用原始 `tool_name`，未使用 `resolveDisplayLabel` | 三处 `toolEv?.tool_name` → `toolEv ? resolveDisplayLabel(toolEv) : undefined` | `useChatTimeline.ts` |
| F-3 | 工具名过长溢出 | TimelineNode action 元素的 `toolName` 无截断逻辑 | 添加 `.timeline-node__tool-name` 样式（ellipsis + max-width: 200px） | `TimelineNode.vue` |
| F-4 | `subagents_spawn` 不被识别为子代理 | `classifyActivityKind` 只匹配 `spawn_subagent`，与后端 `subagents_spawn` 不一致 | 添加 `subagents_spawn` 到 subagent 分类列表 | `activityPresentation.ts` |
| F-5 | `cli_admin_agent_get`/`file_read_file`/`todo_write` 显示原始名称 | `builtinLabels` 缺少这些工具的友好名 | 添加 4 个工具的中文友好名 | `activityPresentation.ts` |
| F-6 | 非 ReAct 模式下 thinking 不显示 | `timelineElements` 只在 `reactSteps` 有内容时生成 thinking 元素 | 添加回退逻辑：当 `reactSteps` 为空但 `presentation.reasoning` 有内容时生成 thinking 元素 | `useChatTimeline.ts` |

---

## 9. 第四轮审查修复记录

> **日期**：2026-06-09 | **触发**：深度 UI 需求核对

| ID | 问题 | 根因 | 修复 | 影响文件 |
|----|------|------|------|----------|
| F-7 | TeamTaskCard 折叠态缺少进度条和成员头像 | 进度条和头像仅在 `expanded` 状态下渲染 | 添加迷你进度条（3px）和迷你头像（18px）到折叠态行 | `TeamTaskCard.vue` |
| F-8 | taskSummary 被 durationText 覆盖 | `v-else-if` 导致有 duration 时 taskSummary 不显示 | 改为两个独立的 `v-if`，两者都显示 | `TeamTaskCard.vue` |
| F-9 | action 展开态缺少工具参数和结果 | `TimelineElement` 无 `toolArguments`/`toolResult` 字段 | 添加字段，`useChatTimeline` 从 `ToolUseEvent` 提取，`TimelineNode` 渲染 | `timelineTypes.ts` + `useChatTimeline.ts` + `TimelineNode.vue` |
| F-10 | error 元素从未被生成 | `useChatTimeline` 中无 `kind: 'error'` 生成逻辑 | 添加：工具 `failed` 且有 `error` 字段时生成 error 元素 | `useChatTimeline.ts` |
| F-11 | InterruptedTeamCard 未导入 | `TaskExecutionPanel.vue` 模板使用了但未 import | 添加 `import InterruptedTeamCard` | `TaskExecutionPanel.vue` |
| F-12 | 搜索框未对团队列表过滤 | `activeTeamList`/`completedTeamList` 未引用 `search` prop | 添加 `teamName` + `taskSummary` 的搜索过滤 | `ChatEntitySidebar.vue` |
