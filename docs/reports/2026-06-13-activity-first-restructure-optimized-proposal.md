# Activity-First 全重构方案 — 评估优化报告

> **版本**：2026-06-13 | **状态**：Proposal
> **前置**：M59 Chat UI 优化（全部阶段 ✅ 已完成）+ v7 原型设计
> **目标**：根治"前端 13 层推理"问题，将语义从后端源头直推到前端

---

## 一、问题根源回顾

### 1.1 当前架构的核心矛盾

当前系统采用 **Message-First** 模型：后端按 LLM 调用轮次建模（Message），前端需要"用户可理解的活动"（Activity）。两者之间存在 **语义鸿沟**，前端必须执行 13 层推理才能从非结构化载体中恢复语义：

| # | 推理步骤 | 脆弱度 | 来源 |
|---|---------|--------|------|
| 1 | `reasoning_as_display` 标志在 `options_json` 中，流式阶段不可见 | 🔴 | `turn_stream_helpers.go` |
| 2 | ReAct Planner 标签 `/*PLANNING*/` 等嵌入纯文本 Content | 🔴 | `reactPlannerParse.ts` |
| 3 | Team 成员消息用 `member-${agentKey}` ID 前缀约定 | 🟠 | `messageOrigin.ts` |
| 4 | `EnvelopeContent` 仅有 Text + Reasoning，无语义标记 | 🔴 | `envelope.go` |
| 5 | 工具调用触发 `snapshotStreamingMessage()` 转换 ID 前缀 | 🟠 | `streamContentPatch.ts` |
| 6 | `mergeSessionMessages` 用内容匹配关联本地/服务端消息 | 🟠 | `mergeSessionMessages.ts` |
| 7 | `classifyActivityKind` 按工具名推断活动类型 | 🟡 | `activityPresentation.ts` |
| 8 | `resolveAssistantPresentation` 分发器推断展示模式 | 🟡 | `messagePlannerPresentation.ts` |
| 9 | `isReasoningAsDisplay()` 从持久化后的 `options_json` 读取 | 🔴 | `streamContentPatch.ts` |
| 10 | `reasoningMarkdown()` 拼接 reasoning + body 作为 fallback | 🟡 | `streamContentPatch.ts` |
| 11 | `useConversationTimeline` 从 Message 反推 Activity 列表 | 🟠 | `useConversationTimeline.ts` |
| 12 | `useAgentBlocks` 从 Message 构建 AgentBlock 树 | 🟠 | `useAgentBlocks.ts` |
| 13 | `computeAgentStatus` 从工具状态推断 Agent 状态 | 🟡 | `useAgentBlocks.ts` |

**核心问题**：后端知道自己在做什么（思考/回复/调用工具/委派团队），但不告诉前端；前端必须从文本内容、ID 前缀、持久化标志中"猜"语义。

### 1.2 M59 已解决的问题与遗留问题

M59 在 **展示层** 做了大量工作（全部 ✅ 已完成），但 **数据源层** 的语义鸿沟未根治：

| M59 已解决 | 仍依赖前端推理 |
|-----------|--------------|
| TaskBoard 树形嵌套渲染 | `useAgentBlocks` 仍从 Message 推理节点类型 |
| ThinkingArea v7 设计 | 思考/回复的区分仍靠 `reasoning_as_display` 标志 |
| UnifiedExecutionPanel | 团队/成员/任务信息仍从多个 WS 事件拼装 |
| 可观测性三层架构 | 语境消息仍靠事件类型映射表推断 |
| TODO 看板 + 工具时间线 | 工具调用语义仍靠 `classifyActivityKind` 推断 |
| 代码块高亮 + 思考折叠 | 折叠策略仍靠前端判断"完成"状态 |

**结论**：M59 是"展示层治理"，Activity-First 是"数据源层治理"。两者互补，不冲突。

---

## 二、方案 C 原始设计评估

### 2.1 原始方案 C 核心内容

| 维度 | 原始设计 |
|------|---------|
| 核心思想 | 后端按 Activity 建模，前端直接消费 |
| ActivityKind | `think` / `say` / `act` / `delegate` / `notice`（5 种） |
| 新 Envelope | `activity_start` / `activity_delta` / `activity_done` |
| 持久化 | 新增 `activities` 表 |
| 迁移 | 双发射（旧 + 新事件并行）→ 逐步停发旧事件 |

### 2.2 与 M59 需求的差距分析

| # | 差距 | 严重度 | 说明 |
|---|------|--------|------|
| G-1 | ActivityKind 与 TaskBoardNodeKind 不对齐 | 🔴 | 方案 C 的 5 种 vs M59 的 7 种（task/thinking/action/reply/sub_task_board/end/error），缺少 task、sub_task_board、end、error |
| G-2 | 未考虑 Spirit 三层架构 | 🔴 | 方案 C 的 `delegate` 无法表达 Spirit→Team→Agent 的层级关系 |
| G-3 | 未考虑 TaskBoard 树形嵌套 | 🔴 | Activity 无 parent_id，无法递归嵌套 |
| G-4 | 未考虑 DAG 依赖关系 | 🟠 | Activity 无 dag_node_id / depends_on 字段 |
| G-5 | 未考虑 UnifiedExecutionPanel v7 | 🟡 | 面板需要任务拆解/依赖关系/团队进度三区数据，Activity 模型未覆盖 |
| G-6 | 未考虑 ThinkingArea v7 | 🟡 | 思考节点的流式/折叠/脑纹 SVG 等展示需求未在 Activity 模型中体现 |
| G-7 | 未考虑可观测性三层架构 | 🟡 | Ambient/Structural/Evidential 分层与 Activity 的关系未定义 |
| G-8 | 未考虑 TODO 看板 | 🟡 | `todo_write` 工具产生的 Activity 需特殊处理 |
| G-9 | 未考虑工具时间线 | 🟡 | 多工具调用的时序关系在 Activity 模型中未体现 |
| G-10 | 未考虑 Stuck 工具 | 🟡 | `tool_timeout` 场景的 Activity 状态未定义 |
| G-11 | 未考虑工具显示开关 | 🟢 | 纯前端控制，与 Activity 模型无关 |
| G-12 | 未考虑代码块高亮 | 🟢 | 纯前端渲染，与 Activity 模型无关 |
| G-13 | 迁移策略未考虑 M59 已完成 | 🟠 | M59 已全部实现，迁移需兼容现有 WS 事件和 Store |

### 2.3 评估结论

方案 C 的 **方向正确**（后端语义直推前端），但 **模型粒度不足**（5 种 Activity 无法覆盖 M59 的 7 种 TaskBoardNode + Spirit 三层 + DAG 依赖）。需要 **扩展 Activity 模型** 以融合 M59 需求，而非推翻 M59 重建。

---

## 三、优化方案：Activity-First + M59 融合

### 3.1 核心原则

1. **后端语义直推**：后端知道自己在做什么，直接通过 Activity 事件告诉前端
2. **M59 展示层保留**：TaskBoard 树形嵌套、ThinkingArea、UnifiedExecutionPanel 等 M59 成果不变
3. **渐进式迁移**：双发射阶段保留现有 WS 事件，前端逐步切换到 Activity 消费
4. **数据源单一化**：Activity 是语义的唯一来源，Message 降级为原始数据存档

### 3.2 Activity 模型重设计

#### 3.2.1 ActivityKind 扩展（对齐 TaskBoardNodeKind）

```typescript
type ActivityKind =
  // === 对齐 M59 TaskBoardNodeKind ===
  | 'task'            // 任务描述（用户/Agent 视角）
  | 'thinking'        // reasoning 内容
  | 'action'          // 工具调用
  | 'reply'           // Agent 回复（含最终答案）
  | 'sub_task_board'  // 子任务看板（递归嵌套）
  | 'end'             // 任务完成标记
  | 'error'           // 错误信息
  // === Spirit 扩展 ===
  | 'delegate'        // 精灵委派团队（Spirit→Team）
  | 'notice'          // 系统通知（语境加载消息、状态变更提示）
```

**与原始方案 C 的映射**：

| 方案 C | 优化后 | 说明 |
|--------|--------|------|
| `think` | `thinking` | 对齐 M59 命名 |
| `say` | `reply` | 对齐 M59 命名 |
| `act` | `action` | 对齐 M59 命名 |
| `delegate` | `delegate` + `sub_task_board` | delegate 表达委派动作，sub_task_board 表达嵌套结构 |
| `notice` | `notice` + `end` + `error` | 拆分为更精确的语义 |
| — | `task` | 新增：任务描述节点 |

#### 3.2.2 Activity 数据结构

```typescript
interface Activity {
  // === 基础字段 ===
  id: string                    // 全局唯一 ID
  kind: ActivityKind            // 活动类型
  sessionId: string             // 所属 Session
  turnId: string                // 所属 Turn
  parentActivityId: string | null  // 父 Activity（树形嵌套）
  timestamp: string             // ISO8601

  // === 语义字段（按 kind 可选） ===
  content?: string              // task/reply/notice/end/error 的文本内容
  reasoning?: string            // thinking 的 reasoning 内容
  toolName?: string             // action 的工具名
  toolCallId?: string           // action 的工具调用 ID
  toolArguments?: string        // action 的工具参数（JSON）
  toolResult?: string           // action 的工具结果
  toolDurationMs?: number       // action 的工具耗时
  toolErrorCode?: string        // action 的错误码（如 tool_timeout）
  childBoardId?: string         // sub_task_board 的子看板根 Activity ID

  // === Spirit 扩展字段 ===
  spiritSessionId?: string      // 精灵 Session ID
  teamId?: string               // 关联的 Team ID
  dagNodeId?: string            // DAG 节点 ID
  dependsOn?: string[]          // 依赖的 DAG 节点 ID 列表
  agentKey?: string             // 执行 Agent 的 key
  agentName?: string            // 执行 Agent 的显示名

  // === 状态字段 ===
  status: ActivityStatus        // 活动状态
  collapsed: boolean            // 是否折叠（后端建议，前端可覆盖）
  durationMs: number | null     // 持续时间（完成后填充）
  label?: string                // 自定义标签（如"规划"/"推理"/"重规划"）
}

type ActivityStatus =
  | 'pending'        // 等待中
  | 'running'        // 运行中
  | 'tool_running'   // 工具执行中
  | 'tool_blocked'   // 等待用户输入
  | 'completed'      // 已完成
  | 'failed'         // 失败
  | 'partial_failure' // 部分失败
  | 'cancelled'      // 已取消
  | 'interrupted'    // 已中断
```

#### 3.2.3 与 M59 TaskBoardNode 的映射

| ActivityKind | TaskBoardNodeKind | 映射方式 |
|-------------|-------------------|---------|
| `task` | `task` | 直接映射 |
| `thinking` | `thinking` | 直接映射 |
| `action` | `action` | 直接映射 |
| `reply` | `reply` | 直接映射 |
| `sub_task_board` | `sub_task_board` | Activity.parentActivityId → childBoard |
| `end` | `end` | 直接映射 |
| `error` | `error` | 直接映射 |
| `delegate` | — | 前端渲染为 TeamAssemblyCard |
| `notice` | — | 前端渲染为语境加载消息 |

**关键改进**：前端不再需要 `useAgentBlocks` 的 13 层推理，直接从 Activity 树构建 TaskBoardNode 树：

```typescript
// 优化后：Activity → TaskBoardNode 直接映射
function activityToTaskBoardNode(activity: Activity): TaskBoardNode {
  return {
    kind: activity.kind as TaskBoardNodeKind,
    id: activity.id,
    timestamp: activity.timestamp,
    collapsed: activity.collapsed,
    content: activity.content,
    reasoning: activity.reasoning,
    toolName: activity.toolName,
    toolStatus: mapActivityStatusToNodeStatus(activity.status),
    toolDuration: activity.toolDurationMs,
    toolCallId: activity.toolCallId,
    toolArguments: activity.toolArguments,
    toolResult: activity.toolResult,
    status: mapActivityStatusToNodeStatus(activity.status),
    errorMessage: activity.kind === 'error' ? activity.content : undefined,
  }
}
```

### 3.3 Envelope 协议重设计

#### 3.3.1 新增 EnvelopeType

```go
// Activity 生命周期事件
EnvelopeTypeActivityStart  EnvelopeType = "activity_start"
EnvelopeTypeActivityDelta  EnvelopeType = "activity_delta"
EnvelopeTypeActivityDone   EnvelopeType = "activity_done"

// Activity 树结构事件
EnvelopeTypeActivityChildStart EnvelopeType = "activity_child_start"  // 子 Activity 开始
```

#### 3.3.2 事件载荷

**activity_start**：

```go
env.Metadata = map[string]any{
    "activity_id":        activityID,
    "kind":               kind,           // ActivityKind
    "parent_activity_id": parentID,       // 树形嵌套
    "session_id":         sessionID,
    "turn_id":            turnID,
    "spirit_session_id":  spiritSessionID,  // Spirit 扩展
    "team_id":            teamID,           // Spirit 扩展
    "dag_node_id":        dagNodeID,        // Spirit 扩展
    "agent_key":          agentKey,
    "agent_name":         agentName,
    "label":              label,            // 自定义标签
    "tool_name":          toolName,         // kind=action
    "tool_call_id":       toolCallID,       // kind=action
    "tool_arguments":     toolArguments,    // kind=action
}
```

**activity_delta**：

```go
env.Metadata = map[string]any{
    "activity_id": activityID,
    "kind":        kind,
    // kind=thinking: reasoning 增量
    "reasoning_delta": reasoningDelta,
    // kind=reply: content 增量
    "content_delta": contentDelta,
    // kind=action: 状态变更
    "status":          newStatus,
    "tool_result":     toolResult,
    "tool_duration_ms": durationMs,
    "tool_error_code": errorCode,
}
```

**activity_done**：

```go
env.Metadata = map[string]any{
    "activity_id":   activityID,
    "kind":          kind,
    "status":        finalStatus,
    "duration_ms":   durationMs,
    "collapsed":     collapsed,      // 后端建议折叠状态
    // kind=thinking: 完整 reasoning
    "reasoning":     fullReasoning,
    // kind=reply: 完整 content
    "content":       fullContent,
    // kind=action: 完整结果
    "tool_result":   fullToolResult,
    "tool_duration_ms": totalDurationMs,
}
```

#### 3.3.3 与现有 M59 事件的关系

| 现有 M59 事件 | Activity 事件 | 迁移策略 |
|-------------|-------------|---------|
| `text_delta` / `text_done` | `activity_delta(kind=reply)` | 双发射阶段并行 |
| `reasoning_delta` / `reasoning_done` | `activity_delta(kind=thinking)` | 双发射阶段并行 |
| `tool_call` / `tool_result` | `activity_start(kind=action)` + `activity_done` | 双发射阶段并行 |
| `member_message_start/delta/done` | `activity_child_start` + `activity_delta` | 双发射阶段并行 |
| `spirit_team_assembled` | `activity_start(kind=delegate)` | 双发射阶段并行 |
| `spirit_plan_created` 等 | `activity_start(kind=notice)` | 双发射阶段并行 |
| `spirit_team_progress` | `activity_delta(kind=notice)` | 双发射阶段并行 |

### 3.4 后端 Activity 投影器设计

#### 3.4.1 ActivityProjector

在 `internal/agent/` 中新增 `ActivityProjector`，负责从 trpc-agent-go 运行时事件中投影出 Activity：

```go
// internal/agent/activity_projector.go
type ActivityProjector struct {
    lg           loggateway.Logger
    eventBus     *event.Bus
    sessionID    string
    turnID       string
    spiritCtx    *SpiritContext  // Spirit 扩展上下文
    activities   map[string]*Activity  // 当前 turn 的 Activity 树
    rootActivity *Activity            // 当前 turn 的根 Activity
}

type SpiritContext struct {
    SpiritSessionID string
    TeamID          string
    DAGNodeID       string
    DependsOn       []string
    AgentKey        string
    AgentName       string
}

// 核心方法
func (p *ActivityProjector) OnTurnStart(ctx context.Context)        // 创建根 task Activity
func (p *ActivityProjector) OnReasoningDelta(ctx context.Context, delta string)  // thinking Activity
func (p *ActivityProjector) OnTextDelta(ctx context.Context, delta string)       // reply Activity
func (p *ActivityProjector) OnToolCall(ctx context.Context, ev ToolCallEvent)    // action Activity
func (p *ActivityProjector) OnToolResult(ctx context.Context, ev ToolResultEvent) // action done
func (p *ActivityProjector) OnTurnDone(ctx context.Context)         // end Activity
func (p *ActivityProjector) OnError(ctx context.Context, err error) // error Activity
func (p *ActivityProjector) OnDelegate(ctx context.Context, teamID string) // delegate Activity
```

#### 3.4.2 投影规则

| 运行时事件 | 投影为 Activity | 说明 |
|-----------|----------------|------|
| Turn 开始 | `task` | 根 Activity，描述任务 |
| `reasoning_delta` | `thinking` | reasoning 内容流式推送 |
| `reasoning_done` + `reasoning_as_display=true` | `thinking` → `reply` | reasoning 即回复时，升级为 reply |
| `text_delta` | `reply` | 正式回复流式推送 |
| `tool_call` | `action` | 工具调用开始 |
| `tool_result` | `action` (done) | 工具调用完成 |
| `subagents_spawn` | `delegate` + `sub_task_board` | 委派子代理 |
| Team 组建 | `delegate` | 精灵委派团队 |
| Turn 完成 | `end` | 任务完成标记 |
| 错误 | `error` | 错误信息 |
| 编排事件 | `notice` | 语境加载消息 |

#### 3.4.3 关键改进：reasoning_as_display 在流式阶段解决

**当前问题**：`reasoning_as_display` 标志在 `DisplayMarkdownFromStream` 中设置，但仅在持久化后的 `options_json` 中可见，流式阶段前端无法知道。

**优化方案**：`ActivityProjector` 在 `OnReasoningDone` 时判断是否为 `reasoning_as_display`，如果是，直接发射 `activity_done(kind=reply)` 而非 `activity_done(kind=thinking)`。前端无需推理。

### 3.5 持久化设计

#### 3.5.1 activities 表

```sql
CREATE TABLE IF NOT EXISTS activities (
    id              TEXT PRIMARY KEY,
    kind            TEXT NOT NULL,           -- ActivityKind
    session_id      TEXT NOT NULL,
    turn_id         TEXT NOT NULL,
    parent_activity_id TEXT,                 -- 树形嵌套
    timestamp       TEXT NOT NULL,           -- ISO8601

    -- 语义字段
    content         TEXT,                    -- task/reply/notice/end/error
    reasoning       TEXT,                    -- thinking
    tool_name       TEXT,                    -- action
    tool_call_id    TEXT,                    -- action
    tool_arguments  TEXT,                    -- action (JSON)
    tool_result     TEXT,                    -- action
    tool_duration_ms INTEGER,                -- action
    tool_error_code TEXT,                    -- action
    child_board_id  TEXT,                    -- sub_task_board

    -- Spirit 扩展
    spirit_session_id TEXT,
    team_id          TEXT,
    dag_node_id      TEXT,
    depends_on       TEXT,                   -- JSON array
    agent_key        TEXT,
    agent_name       TEXT,

    -- 状态
    status          TEXT NOT NULL DEFAULT 'running',
    collapsed       INTEGER NOT NULL DEFAULT 0,
    duration_ms     INTEGER,
    label           TEXT,

    -- 索引
    FOREIGN KEY (session_id) REFERENCES sessions(id),
    FOREIGN KEY (parent_activity_id) REFERENCES activities(id)
);

CREATE INDEX IF NOT EXISTS idx_activities_session_turn ON activities(session_id, turn_id);
CREATE INDEX IF NOT EXISTS idx_activities_parent ON activities(parent_activity_id);
CREATE INDEX IF NOT EXISTS idx_activities_spirit_session ON activities(spirit_session_id);
CREATE INDEX IF NOT EXISTS idx_activities_team ON activities(team_id);
```

#### 3.5.2 Ent Schema

新增 `ent/schema/activity.go`，遵循项目规范：
- `entsql.Annotation{Table: "activities"}` 显式映射表名
- 不使用 Ent Edge，使用手动 FK 字段（`parent_activity_id`、`session_id`）
- 敏感字段（`tool_arguments`）标记 `.Sensitive()`
- JSON 字段（`depends_on`）使用 `field.JSON()`

### 3.6 前端消费层设计

#### 3.6.1 新增 Composable：useActivityTimeline

```typescript
// web/src/features/chat/composables/useActivityTimeline.ts
export function useActivityTimeline(deps: {
  sessionId: ComputedRef<string | undefined>
}) {
  const activities = ref<Activity[]>([])
  const activityTree = computed<ActivityTreeNode[]>(() =>
    buildActivityTree(activities.value)
  )
  const taskBoardNodes = computed<TaskBoardNode[]>(() =>
    activityTree.value.map(activityToTaskBoardNode)
  )

  // WS 事件处理
  function handleActivityStart(env: Envelope) { /* ... */ }
  function handleActivityDelta(env: Envelope) { /* ... */ }
  function handleActivityDone(env: Envelope) { /* ... */ }

  return { activities, activityTree, taskBoardNodes, handleActivityStart, handleActivityDelta, handleActivityDone }
}
```

#### 3.6.2 与 M59 组件的集成

| M59 组件 | 数据源变更 | 说明 |
|---------|----------|------|
| `TaskBoard.vue` | `useAgentBlocks.blocks` → `useActivityTimeline.taskBoardNodes` | 直接消费 Activity 树 |
| `ThinkingArea.vue` | `ChatReasoningPeek` → `Activity(kind=thinking)` | 流式态从 `activity_delta` 获取 |
| `UnifiedExecutionPanel.vue` | 多 Store 拼装 → `Activity` 树过滤 | delegate/sub_task_board 过滤 |
| `ChatExecutionCard.vue` | `ToolUseEvent` → `Activity(kind=action)` | 直接消费 |
| `TurnBlock.vue` | `AgentBlock` → `Activity` 树根节点 | 简化 |
| `SpiritStatusBar.vue` | 多 computed 拼装 → `Activity` 聚合 | 简化 |
| `TodoKanbanBoard.vue` | `useTodoBoard` → `Activity(kind=action, toolName=todo_write)` | 特殊处理 |
| `ToolCallTimeline.vue` | `ToolUseEvent[]` → `Activity(kind=action)[]` | 直接消费 |

#### 3.6.3 13 层推理的消除

| # | 原推理步骤 | Activity-First 后 | 消除方式 |
|---|-----------|------------------|---------|
| 1 | `reasoning_as_display` 推断 | `activity_done(kind=reply)` 直接告知 | 后端投影器判断 |
| 2 | ReAct 标签解析 | `activity_start(kind=thinking, label="规划")` | 后端解析标签 |
| 3 | member ID 前缀约定 | `activity_start(agentKey=xxx)` | 直接携带 |
| 4 | EnvelopeContent 无语义 | `activity_start(kind=xxx)` | 语义在 kind 中 |
| 5 | snapshotStreamingMessage | `activity_done` 替代 | 不再需要 snapshot |
| 6 | mergeSessionMessages 内容匹配 | `activity.id` 全局唯一 | ID 匹配替代内容匹配 |
| 7 | classifyActivityKind | `activity.kind` 直接给出 | 后端分类 |
| 8 | resolveAssistantPresentation | `activity.kind=reply` 直接给出 | 后端判断 |
| 9 | isReasoningAsDisplay | 不再需要 | 后端在投影器中处理 |
| 10 | reasoningMarkdown fallback | `activity.content` 或 `activity.reasoning` | 字段明确 |
| 11 | useConversationTimeline 推理 | `useActivityTimeline` 直接消费 | 无推理 |
| 12 | useAgentBlocks 构建 | `activityTree` 直接映射 | 无推理 |
| 13 | computeAgentStatus | `activity.status` 直接给出 | 后端计算 |

### 3.7 迁移策略

#### Phase 1：双发射（兼容期）

- 后端同时发射旧事件（`text_delta`、`tool_call` 等）和新事件（`activity_start`、`activity_delta`、`activity_done`）
- 前端仍消费旧事件，新事件仅记录日志
- `ActivityProjector` 在 `internal/agent/` 中与 `EventProjector` 并行运行
- 新增 `activities` 表和 Ent Schema

**验证**：`make api && make wire && make build && make test && make lint`

#### Phase 2：前端切换

- 前端新增 `useActivityTimeline` composable
- 通过 feature flag 控制切换：`useActivityTimeline` vs `useAgentBlocks`
- 逐步替换各组件的数据源
- 保留旧事件消费路径作为 fallback

**验证**：`cd web && pnpm lint && pnpm test && pnpm build`

#### Phase 3：停发旧事件

- 前端完全切换到 Activity 消费后，停发旧事件
- 清理 `useAgentBlocks` 中的推理逻辑
- 清理 `useConversationTimeline` 中的推理逻辑
- `EventProjector` 保留但标记 Deprecated

**验证**：全量回归测试

### 3.8 与 M59 各需求的兼容性

| M59 需求 | Activity-First 兼容方式 |
|---------|----------------------|
| US-01 精灵为唯一入口 | `Activity(spiritSessionId=xxx)` 过滤 |
| US-02 简单/任务型区分 | `Activity(kind=delegate)` 存在即为任务型 |
| US-03 团队卡片展示 | `Activity(kind=delegate, teamId=xxx)` 聚合 |
| US-04 任务执行面板 | `Activity` 树过滤 delegate/sub_task_board |
| US-05 TaskBoard 树形嵌套 | `Activity.parentActivityId` 递归构建 |
| US-11 对话流自动折叠 | `Activity.collapsed` 后端建议 + 前端覆盖 |
| US-12 语境加载消息 | `Activity(kind=notice)` |
| US-13 Agent 状态标签 | `Activity.status` 聚合 |
| US-19 TODO 看板 | `Activity(kind=action, toolName=todo_write)` 特殊处理 |
| US-20 工具时间线 | `Activity(kind=action)[]` 按 timestamp 排序 |
| US-21 Stuck 工具 | `Activity(kind=action, toolErrorCode=tool_timeout)` |
| US-22 工具显示开关 | 纯前端控制，与 Activity 无关 |
| US-23 代码块高亮 | 纯前端渲染，与 Activity 无关 |
| US-24 思考不喧宾夺主 | `Activity(kind=thinking)` 的 collapsed/label 由后端建议 |
| US-25 统一执行面板 | `Activity` 树的三区过滤（task/delegate/action） |

---

## 四、v7 原型对齐

### 4.1 ThinkingArea v7

v7 原型中的 ThinkingArea 设计与 Activity-First 完美兼容：

| v7 设计元素 | Activity-First 映射 |
|-----------|-------------------|
| 脑纹 SVG + 流光动画 | `Activity(kind=thinking, status=running)` → 前端渲染动画 |
| 半透明深色 span | `activity_delta(kind=thinking, reasoning_delta=xxx)` → 流式追加 |
| 闪烁光标 | `Activity.status === 'running'` → CSS 动画 |
| 最多 2 行 | 前端渲染控制，与 Activity 无关 |
| 无思考时折叠为按钮 | `Activity(kind=thinking, reasoning='')` → 前端折叠 |
| 完成后折叠为 span | `Activity(kind=thinking, status=completed, collapsed=true)` → 前端折叠 |

### 4.2 UnifiedExecutionPanel v7

| v7 三区 | Activity-First 数据源 |
|--------|---------------------|
| 任务拆解 | `Activity(kind=task)[]` + `Activity(kind=delegate)[]` |
| 依赖关系 | `Activity.dagNodeId` + `Activity.dependsOn` 构建 DAG |
| 团队进度 | `Activity(kind=delegate, teamId=xxx)` 聚合 + 子 Activity 树 |

### 4.3 侧边栏

| v7 设计元素 | Activity-First 映射 |
|-----------|-------------------|
| 精灵入口 | 固定渲染，与 Activity 无关 |
| 团队卡片 | `Activity(kind=delegate)` 聚合 |
| 运行中/已完成/中断分组 | `Activity.status` 过滤 |
| 进度条 | `Activity(kind=action, status=completed).length / total` |
| 脉冲动画 | `Activity.status` 变更触发 |

---

## 五、风险与缓解

| 风险 | 严重度 | 缓解 |
|------|--------|------|
| 双发射增加 WS 消息量 | 🟠 | Activity 事件可复用现有 Envelope 的 Content 字段，增量约 20% |
| ActivityProjector 与 EventProjector 竞态 | 🔴 | 共享同一个 mutex，顺序发射 |
| activities 表数据量增长 | 🟡 | 按 session_id 分区 + 定期归档 |
| 前端切换期两套数据源并存 | 🟠 | Feature flag 控制，逐组件切换 |
| M59 已完成代码的回归风险 | 🔴 | Phase 2 保留旧路径 fallback，Phase 3 全量回归 |
| `reasoning_as_display` 判断时机 | 🟡 | ActivityProjector 在 `OnReasoningDone` 时判断，与现有逻辑一致 |
| Team 模式下 member Activity 的归属 | 🟠 | `Activity.parentActivityId` 指向 delegate Activity |
| Graph DAG 执行的 Activity 嵌套 | 🟡 | `Activity.dagNodeId` + `dependsOn` 表达依赖 |

---

## 六、实施路线图

### Phase 1：后端 Activity 投影（2 周）

| 任务 | 影响域 |
|------|--------|
| 新增 `ent/schema/activity.go` | `internal/data/ent/schema/` |
| 新增 `ActivityRepo` 接口和实现 | `internal/biz/` + `internal/data/` |
| 新增 `ActivityProjector` | `internal/agent/` |
| 集成到 `chat_orchestrator_turn.go` | `internal/service/` |
| 新增 EnvelopeType 注册 | `internal/event/contract/envelope.go` |
| 双发射：旧事件 + Activity 事件并行 | `internal/agent/` |

### Phase 2：前端 Activity 消费（2 周）

| 任务 | 影响域 |
|------|--------|
| 新增 `useActivityTimeline` composable | `web/src/features/chat/composables/` |
| 新增 `Activity` 类型定义 | `web/src/features/chat/types.ts` |
| Feature flag 控制切换 | `web/src/stores/` |
| 逐步替换组件数据源 | `web/src/components/chat/` + `spirit/` |
| 保留旧路径 fallback | 全局 |

### Phase 3：清理与优化（1 周）

| 任务 | 影响域 |
|------|--------|
| 停发旧事件 | `internal/agent/` |
| 清理 `useAgentBlocks` 推理逻辑 | `web/src/features/chat/composables/` |
| 清理 `useConversationTimeline` 推理逻辑 | `web/src/features/chat/composables/` |
| 标记 `EventProjector` 为 Deprecated | `internal/agent/` |
| 全量回归测试 | 全局 |

---

## 七、总结

### 7.1 方案对比

| 维度 | 当前（M59 已实现） | 方案 C 原始 | 优化方案（本报告） |
|------|-----------------|-----------|----------------|
| 数据源 | Message-First（前端推理） | Activity-First（5 种） | Activity-First（9 种，对齐 M59） |
| 语义传递 | 13 层前端推理 | 0 层推理 | 0 层推理 |
| M59 兼容 | — | 不兼容 | 完全兼容 |
| Spirit 支持 | 前端拼装 | 未考虑 | 原生支持 |
| 树形嵌套 | `useAgentBlocks` 构建 | 未考虑 | `parentActivityId` 递归 |
| 迁移风险 | — | 高（推翻重建） | 低（渐进式） |
| v7 原型对齐 | 已实现 | 未考虑 | 完全对齐 |

### 7.2 核心收益

1. **根治 13 层推理**：后端语义直推前端，前端零推理
2. **M59 成果保留**：TaskBoard、ThinkingArea、UnifiedExecutionPanel 等展示层不变
3. **Spirit 原生支持**：Activity 携带 spirit_session_id / dag_node_id / depends_on
4. **树形嵌套原生支持**：parentActivityId 递归构建，无需前端推理
5. **reasoning_as_display 流式解决**：投影器在流式阶段即判断，前端无需等持久化
6. **渐进式迁移**：双发射 + feature flag，风险可控

### 7.3 下一步

1. 评审本报告，确认优化方案
2. 进入 sddflow brainstorming 阶段，生成 proposal.md
3. 进入 sddflow spec 阶段，生成规格和计划
4. 进入 sddflow build 阶段，TDD 实施
