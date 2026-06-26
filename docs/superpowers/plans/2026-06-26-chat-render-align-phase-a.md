# Chat 渲染对齐 Phase A 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让前端 chat UI 严格对齐 `docs/reports/2026-06-25-analysis-chat-module-refactor.md` §7 设计，统一用 ActivityStream 按 kind 分发渲染所有 Activity（含 task → UserMessageBubble），删除 ConversationTurn/useConversationTimeline/placeholder 绕道方案，解决"思考和回复 UI 不显示"的 bug。

**Architecture:** 单一渲染管线：WS ActivityEvent → useActivityTimeline（per-session Map）→ sortedActivities computed（扁平含 task，按 timestamp 排序）→ ActivityStream.vue（按 kind 分发，task → UserMessageBubble / thinking → ThinkingBlock / reply → ReplyBlock / ...）。删除 ConversationTurn 双源合并层。

**Tech Stack:** Vue 3 + Composition API + Pinia + TypeScript + vue-virtual-scroller

**设计依据:** `docs/reports/2026-06-25-analysis-chat-module-refactor.md` §7.2 ActivityStream 组件设计

**关键约束:**
- 保留 messageStore（文档 Phase 1c 才删），但渲染只走 Activity 路径
- 保留 Envelope 概念（文档 Phase 1c 才删），不影响 chat 渲染
- 不改后端代码（后端已正确推送 task activity）
- TDD：每个任务先写失败测试，再实现

---

## File Structure

| 文件 | 操作 | 职责 |
|------|------|------|
| `web/src/features/chat/composables/useActivityTimeline.ts` | 修改 | 增加 sortedActivities computed；删除 streamEvents computed |
| `web/src/components/chat/ActivityStream.vue` | 修改 | 改接收 `activities: Activity[]`；增加 task → UserMessageBubble 分支 |
| `web/src/components/chat/ChatMessageList.vue` | 修改 | 改用 ActivityStream 渲染 sortedActivities；删除 useConversationTimeline 调用 |
| `web/src/pages/ChatPage.vue` | 修改 | 简化 props（移除 ConversationTurn 相关） |
| `web/src/components/chat/ChatMessagePanel.vue` | 修改 | 清理透传 props |
| `web/src/components/chat/ConversationTurn.vue` | 删除 | 不再需要 |
| `web/src/features/chat/composables/useConversationTimeline.ts` | 删除 | 不再需要 |
| `web/src/features/chat/activityTimelineTypes.ts` | 修改 | 删除 ConversationTurn 类型（如不再被引用） |
| `web/src/features/chat/streamHandlers.ts` | 修改 | 删除 createPlaceholderMessage |
| `web/src/features/chat/mergeSessionMessages.ts` | 删除 | 不再需要 placeholder 匹配 |
| `web/src/features/chat/composables/useChatSender.ts` | 修改 | 移除 placeholder 创建逻辑 |
| `web/src/features/chat/composables/useChatWorkspace.ts` | 修改 | 移除 displayMessages / ConversationTurn 相关绑定 |

---

## Task 1: useActivityTimeline 增加 sortedActivities + 删除 streamEvents

**Files:**
- Modify: `web/src/features/chat/composables/useActivityTimeline.ts`
- Test: `web/src/features/chat/composables/__tests__/useActivityTimeline.spec.ts`

**目标:** 增加一个 `sortedActivities` computed，返回当前 session 的所有 Activity（含 task）的扁平列表，按 timestamp + seq 排序。删除 `streamEvents` computed（死代码，从 roots 过滤 task 永远为空）。

- [ ] **Step 1: 写失败测试**

在 `useActivityTimeline.spec.ts` 增加测试：
```typescript
describe('sortedActivities', () => {
  it('returns flat list including task, sorted by timestamp', () => {
    const { activitiesBySession, sortedActivities, handleActivityEvent, setCurrentSessionId } = createTimeline();
    setCurrentSessionId('s1');
    // 创建 task + thinking + reply
    handleActivityEvent({ event: 'created', activity: { id: 't1', kind: 'task', status: 'running', sessionId: 's1', timestamp: '2026-01-01T00:00:00Z', content: 'hi' } });
    handleActivityEvent({ event: 'created', activity: { id: 'th1', kind: 'thinking', status: 'running', sessionId: 's1', timestamp: '2026-01-01T00:00:01Z', parentActivityId: 't1' } });
    handleActivityEvent({ event: 'created', activity: { id: 'r1', kind: 'reply', status: 'running', sessionId: 's1', timestamp: '2026-01-01T00:00:02Z', parentActivityId: 't1' } });

    const ids = sortedActivities.value.map(a => a.id);
    expect(ids).toEqual(['t1', 'th1', 'r1']); // 含 task，按 timestamp 排序
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd web && pnpm test useActivityTimeline.spec -- -t "sortedActivities"`
Expected: FAIL（sortedActivities 未定义）

- [ ] **Step 3: 实现 sortedActivities**

在 `useActivityTimeline.ts` 中：
1. 增加 `sortedActivities` computed：
```typescript
const sortedActivities = computed<Activity[]>(() => {
  const sid = currentSessionId.value;
  if (!sid) return [];
  const map = activitiesBySession.value.get(sid);
  if (!map) return [];
  return Array.from(map.values()).sort(compareActivities);
});
```
2. 删除 `streamEvents` computed 及其相关 `activityToStreamEvent` 调用（如果只被 streamEvents 使用）
3. 在返回值中暴露 `sortedActivities`，移除 `streamEvents`

- [ ] **Step 4: 运行测试确认通过**

Run: `cd web && pnpm test useActivityTimeline.spec -- -t "sortedActivities"`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
cd web && git add src/features/chat/composables/useActivityTimeline.ts src/features/chat/composables/__tests__/useActivityTimeline.spec.ts
git commit -m "refactor(chat): add sortedActivities computed, remove streamEvents"
```

---

## Task 2: ActivityStream.vue 增加 task 分支 + 改接收 activities

**Files:**
- Modify: `web/src/components/chat/ActivityStream.vue`
- Test: `web/src/components/chat/__tests__/ActivityStream.spec.ts`（如不存在则创建）

**目标:** ActivityStream 改接收 `activities: Activity[]`（而非 `events: TimelineActivity[]`），增加 `task → UserMessageBubble` 分支。UserMessageBubble 已存在，接收 `message` prop（Message 类型）—— 需要把 task activity 适配为 message 格式。

- [ ] **Step 1: 写失败测试**

测试 ActivityStream 渲染 task activity 为 UserMessageBubble：
```typescript
it('renders task activity as UserMessageBubble', () => {
  const activities = [
    { id: 't1', kind: 'task', status: 'completed', sessionId: 's1', timestamp: '...', content: 'hello world' },
  ];
  const wrapper = mount(ActivityStream, { props: { activities } });
  expect(wrapper.findComponent(UserMessageBubble).exists()).toBe(true);
  expect(wrapper.text()).toContain('hello world');
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd web && pnpm test ActivityStream.spec`
Expected: FAIL（task 不渲染）

- [ ] **Step 3: 实现 task 分支**

在 `ActivityStream.vue` 中：
1. 改 props：`activities: Activity[]`（从 `activityTypes` 导入）
2. 在模板中增加 task 分支：
```vue
<UserMessageBubble
  v-if="activity.kind === 'task' && activity.status !== 'failed'"
  :message="taskToMessage(activity)"
/>
```
3. 增加 `taskToMessage` 适配函数（把 Activity 适配为 UserMessageBubble 需要的 Message 格式）：
```typescript
function taskToMessage(activity: Activity): Message {
  return {
    id: activity.id,
    role: 'user',
    content: activity.content ?? '',
    turn_id: activity.turnId ?? '',
    session_id: activity.sessionId ?? '',
    status: activity.status === 'failed' ? 'failed' : 'completed',
    created_at: activity.timestamp,
  };
}
```
4. task.failed 仍走 ErrorBlock（保留现有 error 分支）

- [ ] **Step 4: 运行测试确认通过**

Run: `cd web && pnpm test ActivityStream.spec`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
cd web && git add src/components/chat/ActivityStream.vue src/components/chat/__tests__/ActivityStream.spec.ts
git commit -m "feat(chat): ActivityStream renders task as UserMessageBubble"
```

---

## Task 3: ChatMessageList.vue 改用 ActivityStream 渲染

**Files:**
- Modify: `web/src/components/chat/ChatMessageList.vue`

**目标:** ChatMessageList 直接消费 `sortedActivities`，用 ActivityStream 替换 ConversationTurn + DynamicScroller。保留 DynamicScroller 用于虚拟滚动，但 items 改为 sortedActivities，每项渲染为 ActivityStream 内的单个 activity。

**注意:** DynamicScroller 需要扁平 items。sortedActivities 已经是扁平的。但每个 item 需要是单个 Activity，由 ActivityStream 内部分发渲染。所以 ChatMessageList 直接用 ActivityStream 渲染整个 sortedActivities（ActivityStream 内部遍历），不用 DynamicScroller 包裹单个 activity。

**简化方案:** ChatMessageList 直接渲染 `<ActivityStream :activities="sortedActivities" />`，移除 DynamicScroller + ConversationTurn。如果性能问题，后续再用 DynamicScroller 优化（YAGNI）。

- [ ] **Step 1: 改造 ChatMessageList**

1. 移除 `useConversationTimeline` 导入和调用
2. 移除 `ConversationTurn` 导入
3. 移除 `DynamicScroller` / `DynamicScrollerItem`（暂时，YAGNI）—— **保留**，因为虚拟滚动对长列表必要
4. 改 props：移除 `activityTimelineActivities` / `activityAgentKey` / `activityTaskContent` / `activityTree` / `activityRawRecords`，改为单一 `activities: Activity[]`
5. 改模板：用 DynamicScroller 包裹 activities，每项用 resolveBlockComponent 渲染（或直接用 ActivityStream 渲染整个列表）

**关键决策:** 保留 DynamicScroller，但 items 改为 `activities`（每个 activity 是一个 item），item 渲染改为按 kind 分发（与 ActivityStream 逻辑相同）。或者：DynamicScroller items 为 activities，每个 item 用 `<ActivityStreamItem :activity="activity" />` 渲染（提取 ActivityStream 的单 item 渲染逻辑）。

**最简方案（推荐）:** ChatMessageList 直接 `<ActivityStream :activities="props.activities" />`，移除 DynamicScroller。如果性能问题，后续优化。

- [ ] **Step 2: 验证编译**

Run: `cd web && pnpm build`
Expected: 编译通过（可能有未使用导入警告，后续清理）

- [ ] **Step 3: 提交**

```bash
cd web && git add src/components/chat/ChatMessageList.vue
git commit -m "refactor(chat): ChatMessageList uses ActivityStream directly"
```

---

## Task 4: ChatPage.vue 简化 props

**Files:**
- Modify: `web/src/pages/ChatPage.vue`

**目标:** 移除 ConversationTurn 相关的 props 传递，改为单一 `:activities="session.activityTimeline.sortedActivities"`。

- [ ] **Step 1: 修改 ChatPage.vue**

把 L81-87 的多个 activity props 替换为：
```vue
:activities="session.activityTimeline.sortedActivities"
```

移除：
- `:activity-timeline-activities`
- `:activity-agent-key`
- `:activity-task-content`
- `:activity-tree`
- `:activity-raw-records`
- `:messages="session.displayMessages"`（如果不再用于渲染）—— **保留**，messageStore 还在，messages 可能用于 feedback/retry

- [ ] **Step 2: 验证编译**

Run: `cd web && pnpm build`
Expected: 编译通过

- [ ] **Step 3: 提交**

```bash
cd web && git add src/pages/ChatPage.vue
git commit -m "refactor(chat): simplify ChatPage props to single activities"
```

---

## Task 5: 删除 ConversationTurn.vue + useConversationTimeline.ts

**Files:**
- Delete: `web/src/components/chat/ConversationTurn.vue`
- Delete: `web/src/features/chat/composables/useConversationTimeline.ts`
- Delete: `web/src/features/chat/composables/__tests__/useConversationTimeline.spec.ts`（如存在）
- Modify: `web/src/features/chat/activityTimelineTypes.ts`（删除 ConversationTurn 类型，如不再被引用）

- [ ] **Step 1: 确认无引用**

Run: `cd web && grep -r "ConversationTurn\|useConversationTimeline" src/ --include="*.ts" --include="*.vue"`
Expected: 无引用（除自身定义）

- [ ] **Step 2: 删除文件**

- [ ] **Step 3: 验证编译**

Run: `cd web && pnpm build`
Expected: 编译通过

- [ ] **Step 4: 提交**

```bash
cd web && git add -A
git commit -m "refactor(chat): remove ConversationTurn and useConversationTimeline"
```

---

## Task 6: 删除 placeholder 机制

**Files:**
- Modify: `web/src/features/chat/streamHandlers.ts`（删除 createPlaceholderMessage）
- Delete: `web/src/features/chat/mergeSessionMessages.ts`
- Modify: `web/src/features/chat/composables/useChatSender.ts`（移除 placeholder 创建逻辑）
- Modify: `web/src/features/chat/composables/useChatWorkspace.ts`（移除 mergeSessionMessages 调用 + displayMessages）

- [ ] **Step 1: 确认无引用**

Run: `cd web && grep -r "createPlaceholderMessage\|mergeSessionMessages\|pending-user" src/ --include="*.ts" --include="*.vue"`

- [ ] **Step 2: 修改 useChatSender.ts**

移除 `createPlaceholderMessage` 调用，移除 `pendingUserId` 相关逻辑。发送时直接发 WS/HTTP，不创建本地 placeholder。后端 task activity created 事件会驱动 UI 渲染。

- [ ] **Step 3: 修改 useChatWorkspace.ts**

移除 `mergeSessionMessages` 导入和调用，移除 `displayMessages`（如果只用于 ConversationTurn）。保留 `messageStore` 用于其他功能（feedback/retry/regenerate）。

- [ ] **Step 4: 删除文件**

删除 `streamHandlers.ts` 中的 `createPlaceholderMessage` 和 `mergeSessionMessages.ts`。

- [ ] **Step 5: 验证**

Run: `cd web && pnpm lint && pnpm test && pnpm build`
Expected: 全部通过

- [ ] **Step 6: 提交**

```bash
cd web && git add -A
git commit -m "refactor(chat): remove placeholder mechanism, rely on task activity"
```

---

## Task 7: 清理 ChatMessagePanel.vue 透传 props

**Files:**
- Modify: `web/src/components/chat/ChatMessagePanel.vue`

- [ ] **Step 1: 清理 props**

移除 `activityTimelineActivities` / `activityAgentKey` / `activityTaskContent` / `activityTree` / `activityRawRecords`，改为单一 `activities`。

- [ ] **Step 2: 验证**

Run: `cd web && pnpm build`
Expected: 通过

- [ ] **Step 3: 提交**

```bash
cd web && git add src/components/chat/ChatMessagePanel.vue
git commit -m "refactor(chat): simplify ChatMessagePanel props"
```

---

## Task 8: 全量验证

- [ ] **Step 1: 前端全量验证**

Run: `cd web && pnpm lint && pnpm test && pnpm build`
Expected: 全部通过

- [ ] **Step 2: 后端验证（确认无破坏）**

Run: `cd f:\aranea-agents && go build ./...`
Expected: 通过（后端未改）

- [ ] **Step 3: 手动验证（如可能）**

启动应用，发送消息，确认：
- 用户消息显示（task → UserMessageBubble）
- 思考内容显示（thinking → ThinkingBlock）
- 回复内容显示（reply → ReplyBlock）
- 工具调用显示（action → ActionBlock）

- [ ] **Step 4: 最终提交**

```bash
cd web && git add -A
git commit -m "verify: chat render align Phase A complete"
```

---

## Self-Review

**1. Spec coverage:**
- §7.2 ActivityStream 按 kind 分发（含 task → UserMessageBubble）→ Task 1, 2 ✅
- §7.1.3 per-session Map → 已存在，保留 ✅
- 删除 ConversationTurn 绕道 → Task 5 ✅
- 删除 placeholder 机制 → Task 6 ✅
- 删除 streamEvents 死代码 → Task 1 ✅

**2. Placeholder scan:** 无 TBD/TODO，每个步骤有具体代码或命令。

**3. Type consistency:** `sortedActivities` 返回 `Activity[]`，ActivityStream 接收 `activities: Activity[]`，ChatMessageList 传递 `sortedActivities`。类型一致。

**未覆盖（后续 Phase）:**
- §8 工具类型细分（10 个 ToolDetail 组件）→ Phase C
- §10 Session 树 → Phase D
- §9.1.3 子 session 懒加载 → Phase E
- 删除 messageStore → Phase 1c（文档要求）
- 删除 Envelope → Phase 1c（文档要求）
