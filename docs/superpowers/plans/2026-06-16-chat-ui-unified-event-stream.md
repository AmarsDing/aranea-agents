# Chat UI 统一事件流重构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 删除前端消息推理路径，统一为 Activity-First 单路径，合并双渲染模型为 EventStream

**Architecture:** 后端 ActivityProjector 已是 WS 唯一发布路径，前端 AF 路径已激活为默认。本次重构删除推理路径 fallback 和双渲染模型（TaskBoard/Timeline 互斥），统一类型为 StreamEvent，统一渲染为 EventStream。confirm/notice 不作为 Activity Kind——confirm 走 await-reply 通道，notice 走 AlertNotify 信封。

**Tech Stack:** Vue 3 + Composition API + Pinia + TypeScript + Quasar

---

## Task 1: 删除叶节点死代码（无外部依赖的推理路径文件）

**Files:**
- Delete: `web/src/components/chat/CompactTimeline.vue`
- Delete: `web/src/components/chat/TimelineNode.vue`
- Delete: `web/src/components/chat/AgentTreeTimeline.vue`
- Delete: `web/src/components/chat/AgentToolSection.vue` (仅 AgentBlock 引用，AgentBlock 将在 Task 2 删除)
- Delete: `web/src/components/chat/PlanCard.vue` (仅 AgentBlock 引用)
- Delete: `web/src/features/chat/composables/useToolDisplayMode.ts`
- Delete: `web/src/features/chat/composables/useAgentBlocks.ts`
- Delete: `web/src/features/chat/useTurnBlock.ts`
- Delete: `web/src/features/chat/useChatMessageRow.ts`
- Delete: `web/src/features/chat/reactPlannerParse.ts`
- Delete: `web/src/features/chat/reactPlannerToolLink.ts`
- Delete: `web/src/features/chat/composables/useToolCallTimeline.ts`
- Delete: `web/src/features/chat/timelineTypes.ts`
- Delete: `web/src/features/chat/compactTimeline.ts`

- [ ] **Step 1: 删除叶节点 Vue 组件**

删除以下文件（均无外部导入者或仅被其他待删文件导入）：
- `web/src/components/chat/CompactTimeline.vue`
- `web/src/components/chat/TimelineNode.vue`
- `web/src/components/chat/AgentTreeTimeline.vue`
- `web/src/components/chat/AgentToolSection.vue`
- `web/src/components/chat/PlanCard.vue`

- [ ] **Step 2: 删除叶节点 TS 文件**

删除以下文件（均无生产代码导入者或仅被其他待删文件导入）：
- `web/src/features/chat/composables/useToolDisplayMode.ts`
- `web/src/features/chat/composables/useAgentBlocks.ts`
- `web/src/features/chat/useTurnBlock.ts`
- `web/src/features/chat/useChatMessageRow.ts`
- `web/src/features/chat/reactPlannerParse.ts`
- `web/src/features/chat/reactPlannerToolLink.ts`
- `web/src/features/chat/composables/useToolCallTimeline.ts`
- `web/src/features/chat/timelineTypes.ts`
- `web/src/features/chat/compactTimeline.ts`

- [ ] **Step 3: 删除推理路径测试文件**

删除以下测试文件：
- `web/src/features/chat/composables/__tests__/useToolDisplayMode.spec.ts`
- `web/src/features/chat/composables/__tests__/useAgentBlocks.spec.ts`
- `web/src/features/chat/__tests__/useTurnBlock.spec.ts` (如存在)
- `web/src/features/chat/__tests__/reactPlannerParse.spec.ts`
- `web/src/features/chat/__tests__/reactToolLinkIndex.spec.ts`
- `web/src/features/chat/__tests__/reactPlannerToolLink.spec.ts`
- `web/src/features/chat/composables/__tests__/useToolCallTimeline.spec.ts`
- `web/src/features/chat/__tests__/compactTimeline.spec.ts`
- `web/src/features/chat/__tests__/messagePlannerPresentation.spec.ts`
- `web/src/features/chat/composables/__tests__/useAutoCollapse.spec.ts`

注意：先确认测试文件路径存在再删除，部分可能在 `__tests__/` 或同级目录。

- [ ] **Step 4: 验证构建**

Run: `cd web && pnpm build`
Expected: 可能有编译错误（因为后续 Task 的文件仍引用这些已删文件），先记录错误，Task 2 会修复

---

## Task 2: 删除推理路径核心文件 + 更新调用方

**Files:**
- Delete: `web/src/components/chat/TurnBlock.vue`
- Delete: `web/src/components/chat/ChatMessageRow.vue`
- Delete: `web/src/components/chat/ToolStrip.vue`
- Delete: `web/src/components/chat/ChatReactSteps.vue`
- Delete: `web/src/components/chat/ToolCallTimeline.vue`
- Delete: `web/src/components/chat/ToolCallTimelineItem.vue`
- Delete: `web/src/components/chat/AgentBlock.vue`
- Delete: `web/src/features/chat/groupMessagesByTurn.ts`
- Delete: `web/src/features/chat/composables/useChatTimeline.ts`
- Delete: `web/src/features/chat/reactPlannerTypes.ts`
- Delete: `web/src/features/chat/reactToolLinkIndex.ts`
- Delete: `web/src/features/chat/messagePlannerPresentation.ts`
- Delete: `web/src/features/chat/__tests__/groupMessagesByTurn.spec.ts`
- Modify: `web/src/components/chat/ChatMessageList.vue`
- Modify: `web/src/components/chat/ChatMessagePanel.vue`
- Modify: `web/src/features/chat/composables/useChatWorkspace.ts`
- Modify: `web/src/domain/types.ts` 或 `web/src/features/chat/types.ts`
- Modify: `web/src/features/chat/composables/useConversationTimeline.ts`
- Modify: `web/src/features/chat/composables/useChatMessageScroll.ts`
- Modify: `web/src/features/chat/composables/useAutoCollapse.ts`

- [ ] **Step 1: 删除推理路径 Vue 组件**

删除以下文件：
- `web/src/components/chat/TurnBlock.vue`
- `web/src/components/chat/ChatMessageRow.vue`
- `web/src/components/chat/ToolStrip.vue`
- `web/src/components/chat/ChatReactSteps.vue`
- `web/src/components/chat/ToolCallTimeline.vue`
- `web/src/components/chat/ToolCallTimelineItem.vue`
- `web/src/components/chat/AgentBlock.vue`

- [ ] **Step 2: 删除推理路径 TS 文件**

删除以下文件：
- `web/src/features/chat/groupMessagesByTurn.ts`
- `web/src/features/chat/composables/useChatTimeline.ts`
- `web/src/features/chat/reactPlannerTypes.ts`
- `web/src/features/chat/reactToolLinkIndex.ts`
- `web/src/features/chat/messagePlannerPresentation.ts`

- [ ] **Step 3: 更新 ChatMessageList.vue**

移除：
- `TurnBlock` / `ChatMessageRow` 的 import 和模板分支
- `TurnBlockGroup` / `TimelineItem` 类型导入
- `reactToolLinkIndex` prop
- `useTurnBlockMode` 分支逻辑
- 虚拟滚动路径中对 TurnBlock/ChatMessageRow 的引用

保留：
- ConversationTurn 渲染路径（AF 路径）
- 空状态渲染
- ChatPendingQueue
- 滚动到底部按钮

- [ ] **Step 4: 更新 ChatMessagePanel.vue**

移除：
- `useChatTimeline` 导入及相关变量（turnBlocks, timelineItems 等）
- `useAutoCollapse` 导入（TurnBlock 模式专用）
- `useChatMessageScroll` 中对 TurnBlock/groupMessagesByTurn 的依赖
- `reactToolLinkIndex` prop 传递
- `chatListVirtual` 导入（虚拟滚动仅用于 TurnBlock/ChatMessageRow）
- `useTurnBlockMode` 相关逻辑

- [ ] **Step 5: 更新 useChatWorkspace.ts**

移除：
- `buildReactToolLinkIndex` 计算属性
- `reactToolLinkIndex` shallowRef
- 下游 prop 传递链中的 reactToolLinkIndex

- [ ] **Step 6: 更新 types.ts**

移除：
- `import type { ReactStep } from './reactPlannerTypes'`
- `ReactStepWithTools` / `ReactToolLinkIndex` 类型定义（如存在）

- [ ] **Step 7: 更新 useConversationTimeline.ts**

移除：
- `import { resolveAssistantPresentation } from '../messagePlannerPresentation'`
- `buildConversationTurn` 函数（消息推理路径核心，~190 行）
- `buildLegacyConversationTurn` 函数（~45 行）
- `findUserTurns` 函数（~30 行）
- 所有 `TECH-DEBT` 标注的推理路径代码

保留：
- `buildAllConversationTurnsFromActivities`（AF 路径）
- `buildSingleTurnFromActivities`（AF 路径）
- `mergeAdjacentThinkActivities`
- `buildTreeFromRecords`
- `buildTeamPanelFromActivityTree`
- `buildTaskBoardNodesFromActivityTree`
- 辅助函数

- [ ] **Step 8: 更新 useChatMessageScroll.ts**

移除：
- `groupMessagesByTurn` / `TurnBlockGroup` 导入
- `lastAssistantTurnBlockIndex` 相关逻辑

- [ ] **Step 9: 删除 useAutoCollapse.ts**

该文件仅用于 TurnBlock 模式，且依赖 groupMessagesByTurn.ts（已删）。

- [ ] **Step 10: 验证构建**

Run: `cd web && pnpm build`
Expected: BUILD SUCCESS

- [ ] **Step 11: 验证 lint**

Run: `cd web && pnpm lint`
Expected: LINT PASS（可能有 unused import 警告，修复之）

- [ ] **Step 12: 验证测试**

Run: `cd web && pnpm test`
Expected: TEST PASS（删除的测试文件对应的测试不再运行）

---

## Task 3: 删除 useActivityFirstFlag + 简化 AF 条件分支

**Files:**
- Delete: `web/src/features/chat/useActivityFirstFlag.ts`
- Modify: `web/src/features/chat/composables/useChatWorkspace.ts`
- Modify: `web/src/components/chat/ChatMessagePanel.vue`
- Modify: `web/src/components/chat/ChatMessageList.vue`

- [ ] **Step 1: 删除 useActivityFirstFlag.ts**

- [ ] **Step 2: 更新所有调用方**

将 `useActivityFirstEnabled()` 调用替换为 `true`（硬编码），然后移除相关条件分支：
- `useChatWorkspace.ts` — 移除 `activityFirstEnabled` 变量，AF 逻辑始终执行
- `ChatMessagePanel.vue` — 移除 AF 条件判断
- `ChatMessageList.vue` — 移除 `useActivityTimeline` 条件，始终使用 ConversationTurn

- [ ] **Step 3: 验证构建 + lint + test**

Run: `cd web && pnpm build && pnpm lint && pnpm test`
Expected: ALL PASS

---

## Task 4: 重构类型系统 — activityTimelineTypes → streamEventTypes

**Files:**
- Create: `web/src/features/chat/streamEventTypes.ts`
- Modify: `web/src/features/chat/activityTimelineTypes.ts` → 保留为 re-export 过渡
- Modify: `web/src/features/chat/composables/useActivityTimeline.ts`
- Modify: `web/src/features/chat/composables/useConversationTimeline.ts`
- Modify: `web/src/components/chat/ActivityTimeline.vue`
- Modify: `web/src/components/chat/AgentWorkPanel.vue`
- Modify: 所有导入 activityTimelineTypes 的文件

- [ ] **Step 1: 创建 streamEventTypes.ts**

基于修正后的 5 种 Activity Kind 体系（thinking/action/reply/error/plan），创建统一类型定义：

```typescript
// streamEventTypes.ts — 统一事件流类型定义

export type StreamEventKind = 'thinking' | 'action' | 'reply' | 'error' | 'plan';
export type StreamEventStatus =
  | 'pending' | 'running' | 'tool_running' | 'tool_blocked'
  | 'completed' | 'failed' | 'partial_failure' | 'cancelled';

interface StreamEventBase {
  id: string;
  kind: StreamEventKind;
  status: StreamEventStatus;
  durationMs?: number;
  agentKey?: string;
  agentName?: string;
  timestamp: string;
}

export interface ThinkingEvent extends StreamEventBase {
  kind: 'thinking';
  content: string;
  label?: string;
  collapsed: boolean;
  streaming: boolean;
  subSteps?: ThinkingEvent[];
}

export interface ActionEvent extends StreamEventBase {
  kind: 'action';
  toolName: string;
  toolLabel?: string;
  toolArguments?: string;
  toolResult?: string;
  toolErrorCode?: string;
  toolDurationMs?: number;
  iconKey?: string;
  isLongRunning?: boolean;
}

export interface ReplyEvent extends StreamEventBase {
  kind: 'reply';
  content: string;
  isFinal: boolean;
  streaming: boolean;
  variant: 'default' | 'a2ui';
}

export interface ErrorEvent extends StreamEventBase {
  kind: 'error';
  content: string;
  errorCode?: string;
}

export interface PlanEvent extends StreamEventBase {
  kind: 'plan';
  title: string;
  steps: PlanStep[];
}

export interface PlanStep {
  id: string;
  label: string;
  status: StreamEventStatus;
  agentName?: string;
  dagNodeId?: string;
  dependsOn?: string[];
  children?: StreamEvent[];
}

export type StreamEvent = ThinkingEvent | ActionEvent | ReplyEvent | ErrorEvent | PlanEvent;
```

- [ ] **Step 2: 更新 activityTimelineTypes.ts 为 re-export 过渡层**

将 `Activity` 联合类型改为 `StreamEvent` 的别名，保持向后兼容：
```typescript
export type { StreamEvent as Activity, StreamEventKind, StreamEventStatus } from './streamEventTypes';
export type { ThinkingEvent as ThinkActivity } from './streamEventTypes';
// ... 其他 re-exports
```

- [ ] **Step 3: 逐步迁移导入**

将所有 `import ... from './activityTimelineTypes'` 改为 `import ... from './streamEventTypes'`。
涉及文件：ActivityTimeline.vue, AgentWorkPanel.vue, useConversationTimeline.ts, useActivityTimeline.ts, ConversationTurn.vue, TaskBoardSection.vue, SayActivity.vue, ActActivity.vue, NoticeActivity.vue, DelegateActivity.vue, DagSection.vue, TeamPanel.vue, TeamProgressSection.vue

- [ ] **Step 4: 验证构建 + lint + test**

Run: `cd web && pnpm build && pnpm lint && pnpm test`
Expected: ALL PASS

---

## Task 5: 创建 ErrorBlock.vue + 统一组件命名

**Files:**
- Create: `web/src/components/chat/ErrorBlock.vue`
- Rename: `web/src/components/chat/ActActivity.vue` → `ActionBlock.vue`
- Rename: `web/src/components/chat/SayActivity.vue` → `ReplyBlock.vue`
- Rename: `web/src/components/chat/NoticeActivity.vue` → `NoticeBlock.vue`
- Modify: 所有导入上述组件的文件

- [ ] **Step 1: 创建 ErrorBlock.vue**

从 NoticeActivity 中拆分 error 渲染逻辑，创建独立的 ErrorBlock 组件。
ErrorBlock 始终展开，danger 色左边框，显示错误描述和错误码。

- [ ] **Step 2: 重命名组件**

- `ActActivity.vue` → `ActionBlock.vue`（git mv）
- `SayActivity.vue` → `ReplyBlock.vue`（git mv）
- `NoticeActivity.vue` → `NoticeBlock.vue`（git mv）

- [ ] **Step 3: 更新所有导入**

全局替换组件名引用：
- `ActActivity` → `ActionBlock`
- `SayActivity` → `ReplyBlock`
- `NoticeActivity` → `NoticeBlock`

- [ ] **Step 4: 更新 ActivityTimeline.vue**

在 ActivityTimeline.vue 中：
- 将 `NoticeActivity(kind=error)` 分支替换为 `ErrorBlock`
- 更新组件导入

- [ ] **Step 5: 验证构建 + lint + test**

Run: `cd web && pnpm build && pnpm lint && pnpm test`
Expected: ALL PASS

---

## Task 6: 统一渲染模型 — EventStream.vue + AgentWorkPanel 重构

**Files:**
- Create: `web/src/components/chat/EventStream.vue`
- Modify: `web/src/components/chat/AgentWorkPanel.vue`
- Modify: `web/src/components/chat/ActivityTimeline.vue` → 标记 deprecated

- [ ] **Step 1: 创建 EventStream.vue**

统一事件流渲染组件，替代 ActivityTimeline + TaskBoard 互斥逻辑：

```vue
<template>
  <div class="event-stream">
    <template v-for="event in events" :key="event.id">
      <ThinkingBlock v-if="event.kind === 'thinking'" :event="event" />
      <ActionBlock v-else-if="event.kind === 'action'" :event="event" />
      <ReplyBlock v-else-if="event.kind === 'reply'" :event="event" />
      <ErrorBlock v-else-if="event.kind === 'error'" :event="event" />
      <PlanBlock v-else-if="event.kind === 'plan'" :event="event" />
    </template>
  </div>
</template>
```

- [ ] **Step 2: 重构 AgentWorkPanel.vue**

移除 TaskBoard/ActivityTimeline 互斥分支，统一使用 EventStream：
- 移除 `taskBoardNodes` 分支
- 移除 `ActivityTimeline` 回退分支
- 移除 `TeamPanel + ActivityTimeline` 分支
- 统一为：EventStream（渲染 activities）+ TeamPanel（如有 panel 数据）

- [ ] **Step 3: 验证构建 + lint + test**

Run: `cd web && pnpm build && pnpm lint && pnpm test`
Expected: ALL PASS

---

## Task 7: 后端 Activity Kind 清理

**Files:**
- Modify: `internal/biz/activity.go` — 添加 ActivityKindPlan 常量
- Modify: `internal/agent/activity_projector.go` — 添加 OnPlanStart/OnPlanStepUpdate 方法（可选）
- Modify: `internal/agent/activity_projector.go` — 清理未使用的 Kind（task/end/sub_task_board/delegate）

- [ ] **Step 1: 评估 plan Kind 的必要性**

检查 Spirit→Team 委派场景是否需要 plan Activity Kind。
如果 TaskBoard 已能覆盖此需求，则暂不新增。

- [ ] **Step 2: 清理未使用的 ActivityKind 常量（可选）**

评估是否可以删除 `ActivityKindEnd`（无 Projector 方法，无前端消费）。
注意：`ActivityKindTask`/`ActivityKindSubTaskBoard`/`ActivityKindDelegate` 仍有 Projector 方法在使用，暂不删除。

- [ ] **Step 3: 验证后端构建 + 测试**

Run: `go build ./... && go test ./internal/agent/... ./internal/biz/... -count=1`
Expected: ALL PASS

---

## Task 8: 最终验证 + 清理

**Files:**
- 全项目

- [ ] **Step 1: 前端全量验证**

Run: `cd web && pnpm lint && pnpm test && pnpm build`
Expected: ALL PASS

- [ ] **Step 2: 后端全量验证**

Run: `make api && make wire && make build && make test && make lint`
Expected: ALL PASS

- [ ] **Step 3: 清理 i18n 无用翻译键**

移除 compactTimeline / turnBlock / chatMessageRow 等已删组件对应的翻译键。

- [ ] **Step 4: 清理 CSS 无用样式**

移除已删组件对应的样式定义。

---

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| 旧会话无 Activity 数据，推理路径删除后显示空白 | `buildSingleTurnFromActivities` 中已有 fallback：无 Activity 的 Turn 标记 `isLegacy: true`，AgentWorkPanel 显示"历史对话"提示 |
| 删除文件后编译错误 | 每个 Task 结束后验证构建，逐步修复 |
| TaskBoard 用户依赖 | TaskBoard 仍保留（AgentWorkPanel 使用），EventStream 作为替代方案渐进引入 |
| a2uiParse.ts 不能删除 | 保留，仅删除推理路径专用文件 |
