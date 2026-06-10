# M69: Chat 时间线展示与团队列表修复 — 开发计划

> **版本**：2026-06-09 | **状态**：Implemented
> **需求规格**：[69-chat-timeline-observability.md](./69-chat-timeline-observability.md)
> **技术设计**：[69-chat-timeline-observability.design.md](./69-chat-timeline-observability.design.md)

---

## 1. 现状评估

| 维度 | 状态 | 说明 |
|------|------|------|
| 团队列表数据 | 致命 | `loadSpiritTeams` 从未被调用 |
| 时间线展示 | 部分 | ReAct 步骤有展示但非时间线流式，Team 会话禁用 TurnBlock |
| 三区布局 | 未集成 | 组件已编写但未引用 |
| 左侧面板 | 不符合需求 | 仍显示完整 Agent 列表 |

---

## 2. 开发阶段

### Phase 0: 致命 Bug 修复（P0） ✅

| 任务 ID | 任务 | 涉及文件 | 验收标准 | 状态 |
|---------|------|----------|----------|------|
| T-01 | 在 useChatWorkspace 中添加 Spirit session 选择 watch，调用 loadSpiritTeams | `useChatWorkspace.ts` | 选择 Spirit session 后团队列表有数据 | ✅ 完成 |
| T-02 | 在 useChatWorkspace 中监听 wsReplaying 信号，重连后调用 reloadTeams | `useChatWorkspace.ts` | WS 重连后团队列表恢复 | ✅ 完成 |
| T-03 | 在 session 切换时调用 spiritStore.reset() 清理旧数据 | `useChatWorkspace.ts` | 切换到非 Spirit session 时团队列表清空 | ✅ 完成 |

### Phase 1: 时间线展示核心（P1） ✅

| 任务 ID | 任务 | 涉及文件 | 验收标准 | 状态 |
|---------|------|----------|----------|------|
| T-04 | 定义 TimelineElement 类型和 timelineTypes.ts | `features/chat/timelineTypes.ts`(新) | 类型编译通过 | ✅ 完成 |
| T-05 | 扩展 useChatTimeline，新增 timelineElements 计算属性 | `useChatTimeline.ts` | timelineElements 正确拆解消息为时间线元素 | ✅ 完成 |
| T-06 | 实现 TimelineNode 展示组件 | `components/chat/TimelineNode.vue`(新) | 各类型节点正确渲染 | ✅ 完成 |
| T-07 | ChatMessageList 集成时间线渲染模式 | `ChatMessageList.vue` | TurnBlock 内使用时间线渲染 | ✅ 完成 |
| T-08 | Team 会话启用时间线模式 | `useChatTimeline.ts` | Team 会话显示时间线 | ✅ 完成 |
| T-09 | 折叠交互：thinking/action 完成后自动折叠 | `TimelineNode.vue` | 完成的步骤自动折叠 | ✅ 完成 |

### Phase 2: 布局集成（P2） ✅

| 任务 ID | 任务 | 涉及文件 | 验收标准 | 状态 |
|---------|------|----------|----------|------|
| T-10 | TaskExecutionPanel 集成 TeamProgressCard | `TaskExecutionPanel.vue` | 团队进度卡片显示 | ✅ 完成 |
| T-11 | TaskExecutionPanel 集成 SynthesisResultCard | `TaskExecutionPanel.vue` | 综合结果区显示 | ✅ 完成 |
| T-12 | 左侧面板简化为精灵+团队树 | `ChatEntitySidebar.vue`, `ChatPage.vue` | Spirit 模式下不显示 Agent 列表 | ✅ 完成 |

> **注意**：原计划 T-10~T-13 中 ParallelTeamOverview 和 InterruptedTeamCard 已在 TaskExecutionPanel 中集成，无需重复操作。实际仅需集成 TeamProgressCard 和 SynthesisResultCard，以及左侧面板简化。

---

## 3. 验收标准

| ID | 验收项 | 对应任务 |
|----|--------|----------|
| AC-01 | 选择 Spirit session 后，左侧面板显示团队列表 | T-01 |
| AC-02 | WS 重连后团队列表自动恢复 | T-02 |
| AC-03 | 单 Agent 会话按时间线展示思考-动作-总结-结束 | T-05~T-07 |
| AC-04 | Team 会话按时间线展示思考-动作-总结-结束 | T-08 |
| AC-05 | 思考和动作元素完成后自动折叠 | T-09 |
| AC-06 | TaskExecutionPanel 展示三区布局 | T-10~T-13 |
| AC-07 | 左侧面板仅显示精灵+团队树 | T-14 |
| AC-08 | `pnpm lint && pnpm test && pnpm build` 通过 | 全部 |

---

## 4. 依赖关系

```
T-01 ──→ T-02 ──→ T-03  (P0 顺序执行)
T-04 ──→ T-05 ──→ T-06 ──→ T-07 ──→ T-08 ──→ T-09  (P1 顺序执行)
T-10 ──→ T-11 ──→ T-12 ──→ T-13  (P2 可并行)
T-14  (P2 独立)
```

Phase 0 和 Phase 1 必须顺序执行。Phase 2 内部任务可并行。
