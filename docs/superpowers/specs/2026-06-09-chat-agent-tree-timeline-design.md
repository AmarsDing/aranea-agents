# Chat Agent Tree Timeline — Design Spec

> 日期: 2026-06-09
> 状态: Implemented (Phase 1-2)

## 1. 问题

当前 Chat UI 在 Team 模式下，所有思考内容合并到一个 `ChatReasoningPeek` 组件中，无法区分时间线；团队成员的发言和工具调用混在 `TurnBlock` 的 `tools/members` 列表中，缺乏层次感。用户无法直观地看到"哪个 Agent 做了什么"。

## 2. 设计目标

1. **每个 Agent 一个大块**：主 Agent（精灵）和子 Agent 各自拥有独立的可折叠会话块
2. **统一块内结构**：任务 → 思考 → 执行/工具 → 结果，所有 Agent 块内部结构一致
3. **树形时间线**：主 Agent 包含子 Agent，子 Agent 缩进排列，左侧有连线轨道
4. **自动折叠**：Agent 完成后自动折叠为头部摘要行
5. **独立折叠层级**：主 Agent 折叠 = 整个任务会话折叠；子 Agent 折叠 = 仅折叠自己的内容
6. **流式实时展示**：思考流式传输时显示预览 + 脉冲动画，完成后自动折叠

## 3. 视觉设计

### 3.1 整体布局

```
┌─────────────────────────────────────┐
│  用户消息气泡（右对齐）                │
├─────────────────────────────────────┤
│ ┌─ 主 Agent 块 ──────────────────┐  │
│ │ [头像] 精灵助手  ✓已完成  12.5s │  │
│ │                                 │  │
│ │ 📋 接收任务                     │  │
│ │   组建一个团队来完成市场分析报告  │  │
│ │                                 │  │
│ │ 🧠 思考  3s  ▶                  │  │
│ │   需要组建包含市场研究...        │  │
│ │                                 │  │
│ │ ⚡ 组建团队  ✓  1.2s  ▶         │  │
│ │                                 │  │
│ │ ┃ ┌─ 子 Agent 块 ────────────┐ │  │
│ │ ┃ │ [研] 研究员  ✓  4.2s  ▶ │ │  │
│ │ ┃ │ 负责市场数据收集...       │ │  │
│ │ ┃ └─────────────────────────┘ │  │
│ │ ┃                              │  │
│ │ ┃ ┌─ 子 Agent 块（展开）────┐ │  │
│ │ ┃ │ [析] 分析师  ✓  6.8s  ▼ │ │  │
│ │ ┃ │                          │ │  │
│ │ ┃ │ 📋 接收任务              │ │  │
│ │ ┃ │   对市场数据建模...       │ │  │
│ │ ┃ │ 🧠 思考  2s  ▶           │ │  │
│ │ ┃ │ ⚡ data_analysis  ✓  ▶   │ │  │
│ │ ┃ │ 📊 结果                  │ │  │
│ │ ┃ │   市场规模约 120 亿...    │ │  │
│ │ ┃ └──────────────────────────┘ │  │
│ │                                 │  │
│ │ 🧠 汇总思考  5s  ▶              │  │
│ │                                 │  │
│ │ 📊 最终结果                     │  │
│ │   团队已组建完成！              │  │
│ └─────────────────────────────────┘  │
└─────────────────────────────────────┘
```

### 3.2 Agent 块结构

每个 Agent 块由 **头部** + **内容区** 组成：

**头部（始终可见）**：
- Agent 头像（圆形，带首字/图标）
- Agent 名称（颜色与头像一致）
- 状态标签：运行中（脉冲）/ 已完成（绿色）/ 失败（红色）
- 耗时
- 子任务数（仅主 Agent，折叠时显示）
- 展开/折叠箭头

**内容区（可折叠）**：
| 区段 | 图标 | 颜色 | 折叠行为 |
|------|------|------|----------|
| 接收任务 | 📋 | 黄色 | 不可折叠 |
| 思考 | 🧠 | 灰色 | 默认折叠，显示最后 2 行预览 |
| 执行/工具 | ⚡ | Agent 色 | 默认折叠，显示工具名+状态 |
| 结果 | 📊 | 绿色 | 不可折叠 |

### 3.3 思考块生命周期

1. **流式传输中**：显示最后 2 行预览 + 脉冲动画 + 光标闪烁
2. **思考完成**：自动折叠为摘要行（最后 2 行截断 + 时长）
3. **用户点击**：展开显示完整 markdown 内容，可滚动
4. **再次点击**：折叠回摘要行

### 3.4 树形缩进

- 子 Agent 块在主 Agent 内容区内缩进，左侧有 2px 连线轨道
- 连线颜色与子 Agent 主题色一致（半透明）
- 子 Agent 之间通过连线串联，体现时间顺序
- 子 Agent 块的折叠/展开独立于主 Agent

### 3.5 折叠规则

| 层级 | 触发条件 | 折叠后显示 |
|------|----------|-----------|
| 主 Agent | 所有子 Agent 完成 + 最终结果输出 | 头部：名称 + 状态 + 时长 + 子任务数 |
| 子 Agent | Agent 执行完成 | 头部：名称 + 状态 + 时长 + 结果摘要 |
| 思考区段 | 思考流式完成 | 摘要行：最后 2 行截断 |
| 工具区段 | 工具执行完成 | 摘要行：工具名 + 状态 + 时长 |

## 4. 数据模型

### 4.1 AgentBlock 类型（已实现在 `agentTreeTypes.ts`）

```typescript
// features/chat/agentTreeTypes.ts
type AgentBlockStatus = 'running' | 'completed' | 'failed';

interface AgentBlock {
  id: string;
  agentKey: string;
  agentName: string;
  agentIcon: string;
  agentColor: string;      // CSS variable: var(--color-accent) for root, var(--agent-palette-N) for sub
  status: AgentBlockStatus;
  durationMs: number | null;
  collapsed: boolean;
  task: string | null;
  timeline: TimelineEntry[];  // Chronologically ordered entries
  result: string | null;
  startedAt: string;
  finishedAt: string | null;
}

// Discriminated union for timeline entries (strict chronological order)
type TimelineEntry =
  | { kind: 'thinking'; section: ThinkingSection; sortKey: number }
  | { kind: 'tool'; section: ToolSection; sortKey: number }
  | { kind: 'subagent'; block: AgentBlock; sortKey: number };

interface ThinkingSection {
  id: string;
  content: string;
  durationMs: number;
  collapsed: boolean;
  streaming: boolean;
}

interface ToolSection {
  id: string;
  toolName: string;
  toolLabel: string;
  status: 'running' | 'success' | 'failed' | 'blocked' | 'cancelled';
  durationMs: number | null;
  arguments: string | null;
  result: string | null;
  error: string | null;
  collapsed: boolean;
}
```

### 4.2 数据来源映射

| AgentBlock 字段 | 数据来源 |
|-----------------|---------|
| agentKey | `Message.agent_ref.agent_key` / `Message.origin.agentKey` / `ToolUseEvent.agent_key` |
| agentName | `Message.agent_ref.name` / `Message.team_member.name` / `ToolUseEvent.agent_name` |
| agentColor | `agentColorFromKey(agentKey)` — root: `var(--color-accent)`, sub: `var(--agent-palette-N)` |
| task | `block.user.content_markdown` (root) / `memberMsg.content_markdown` (sub) |
| timeline | Chronologically interleaved: thinking + tools + subagents, ordered by message position |
| result | `presentation.bodyMarkdown` (root) / last member message content (sub) |

## 5. 组件架构（已实施）

### 5.1 新增文件

| 文件 | 职责 |
|------|------|
| `features/chat/agentTreeTypes.ts` | AgentBlock / TimelineEntry / ThinkingSection / ToolSection 类型 + agentColorFromKey |
| `features/chat/agentTreeUtils.ts` | 共享工具函数（formatDuration） |
| `features/chat/composables/useAgentBlocks.ts` | 从消息构建 AgentBlock 树（timeline 按时间顺序交替排列） |
| `components/chat/AgentBlock.vue` | Agent 大块容器（头部 + task/timeline/result section + 子 Agent 缩进） |
| `components/chat/AgentThinkingSection.vue` | 思考区段（流式预览 → 折叠 → 展开） |
| `components/chat/AgentToolSection.vue` | 工具区段（参数/结果折叠展开） |
| `components/chat/AgentTreeTimeline.vue` | 顶层编排组件（用户气泡 + Agent 块列表） |

### 5.2 修改文件

| 文件 | 修改内容 |
|------|---------|
| `components/chat/ChatMessageList.vue` | 新增 `agentBlocks` prop，Team 模式下渲染 `AgentTreeTimeline` |
| `components/chat/ChatMessagePanel.vue` | 引入 `useAgentBlocks`，传递 `agentBlocks` 给 `ChatMessageList` |

### 5.3 渲染切换逻辑

```
ChatMessageList.vue (normal scroll path):
  isTeamSession && agentBlocks.length > 0
    → AgentTreeTimeline (新)
  else if useTurnBlockMode && timelineElements.length > 0
    → TimelineNode (现有)
  else if useTurnBlockMode
    → TurnBlock (现有)
  else
    → ChatMessageRow (现有)
```

## 6. 交互规格

### 6.1 折叠/展开

- **点击 Agent 头部**：toggle 整个 Agent 块的折叠状态
- **点击思考区段标签**：toggle 该思考段的折叠状态
- **点击工具区段标签**：toggle 该工具的折叠状态
- **折叠状态管理**：`AgentTreeTimeline.vue` 使用 `reactive(Map)` 管理所有块的折叠状态

### 6.2 自动折叠

- Agent 块：`status === 'completed'` 时默认折叠
- 思考段：默认折叠，显示最后 2 行预览
- 工具段：`status === 'success' | 'failed' | 'cancelled'` 时默认折叠

### 6.3 流式状态

- Agent 块头部：运行中显示脉冲动画（CSS animation）
- 思考段：流式时显示脉冲点 + 光标闪烁
- 工具段：运行中显示沙漏图标

## 7. 与现有系统的兼容

### 7.1 非 Team 模式

非 Team 会话（单 Agent）继续使用现有 TurnBlock + TimelineNode 渲染，不受影响。

### 7.2 渐进式切换

通过 `isTeamSession && agentBlocks.length > 0` 判断：
- Team 模式 + 有 Agent 数据 → 使用 AgentTreeTimeline
- 其他情况 → 使用现有 TurnBlock/TimelineNode

### 7.3 数据兼容

AgentBlock 从现有 Message 数据构建，不需要后端 API 变更。所有数据已通过 WebSocket envelope 推送到前端。

## 8. 已完成实施

### Phase 1-2（基础架构）
- [x] `agentTreeTypes.ts` — 类型定义 + 颜色调色板
- [x] `useAgentBlocks.ts` — 从消息构建 AgentBlock 树
- [x] `AgentBlock.vue` — Agent 大块容器（头部 + 内容 + 子 Agent 缩进）
- [x] `AgentThinkingSection.vue` — 思考区段（流式 → 折叠 → 展开）
- [x] `AgentToolSection.vue` — 工具区段（参数/结果折叠展开）
- [x] `AgentTreeTimeline.vue` — 顶层编排 + 折叠状态管理
- [x] `ChatMessageList.vue` — Team 模式下使用 AgentTreeTimeline
- [x] `ChatMessagePanel.vue` — 引入 useAgentBlocks + 传递 agentBlocks

### Phase 3（编排 + 树形 + 自动折叠）
- [x] `agentTreeTypes.ts` — 新增 OrchestrationPlan / PlanEntry / TeamStatusSummary 类型
- [x] `useAgentBlocks.ts` — 重写时间线构建算法：TimestampedEvent 分类 + 严格时间顺序交替排列
- [x] `useAgentBlocks.ts` — 新增编排计划提取（plan_and_execute / subagents_spawn 参数解析）
- [x] `useAgentBlocks.ts` — 新增 Team 状态聚合（TeamStatusSummary: total/running/completed/failed）
- [x] `PlanCard.vue` — 新增编排任务卡片组件（Trae 风格，显示任务分配方案）
- [x] `AgentBlock.vue` — 新增子任务数显示（"N 个子任务"）
- [x] `AgentBlock.vue` — 新增 Team 运行状态摘要（运行中/已完成/失败 计数）
- [x] `AgentBlock.vue` — 新增树形连线节点（tree-connector: 竖线 + 状态圆点 + Agent 主题色）
- [x] `AgentBlock.vue` — 新增自动折叠（Agent 完成后 800ms 延迟折叠）
- [x] `AgentBlock.vue` — 新增全局展开/折叠事件监听
- [x] `AgentThinkingSection.vue` — 新增流式完成自动折叠（500ms 延迟）
- [x] `AgentTreeTimeline.vue` — 新增全局展开/折叠工具栏

## 9. 待优化（Phase 4）

- [ ] 折叠状态持久化（sessionStorage）
- [ ] 虚拟滚动支持（当前仅普通滚动模式）
- [ ] 子 Agent 连线颜色与 agent 主题色一致（节点已实现，连线待优化）
- [ ] PlanCard 展开/折叠动画
- [ ] Agent 块完成后的折叠动画（当前使用 transition，待优化缓动）
