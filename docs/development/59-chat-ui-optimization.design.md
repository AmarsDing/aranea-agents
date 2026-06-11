# M59: Chat UI 优化 — 实现设计

> **版本**：2026-06-10 | **状态**：Implemented（P0/P0.5/OBS/M60 全阶段 + P1.5 + P1.6 规划中 + M69 修复全部完成）
> **合并来源**：原 M59（精灵模式 + 可观测性 + 并行编排）+ M69（时间线展示 + 团队列表修复 + useAgentBlocks 业务逻辑审查）
> **需求规格**：[59-chat-ui-optimization.md](./59-chat-ui-optimization.md)
> **开发计划**：[59-chat-ui-optimization.development.md](./59-chat-ui-optimization.development.md)
> **遵循**：[AGENT_RUNTIME_BOUNDARY.md](../AGENT_RUNTIME_BOUNDARY.md)

---

## 一、模块概述

### 1.1 设计定位

以精灵（Spirit）为 Chat 页面唯一对话入口，左侧列表从"Agent/Team 平铺"重构为"精灵 + 任务团队树"，中间面板支持三种模式（精灵对话 / 任务执行 / 成员只读），任务看板支持**树形嵌套展示**（任务-思考-工具-回复 统一结构 + sub_task_board 递归嵌入）。

核心架构为三层分离（对齐 APWA 论文 arXiv:2605.15132）：

- **Spirit = Manager**：高层规划，决定"做什么"和"谁来做"
- **Team = Worker**：任务委派，管理成员执行
- **Agent = Executor**：具体执行，独立上下文

设计融合多篇论文核心思想：
- **AdaptOrch** (arXiv:2602.16873)：拓扑路由算法
- **APWA** (arXiv:2605.15132)：Manager-Worker-Executor 三层分离
- **Maestro** (arXiv:2511.06134)：探索-合成分离
- **TaskBoard 树形嵌套**：对齐现实项目管理中"任务→子任务→子子任务"的心智模型

**前置依赖**：[system-builtin-agents-design](../superpowers/specs/2026-05-31-system-builtin-agents-design.md) 中精灵 Agent 定义、`plan_and_execute` 三阶段编排工具、Session 树状模型。

### 1.2 分层与依赖

```
api/kratos/session/v1/session.proto   ← Session 扩展字段（parent_session_id / dag_snapshot 等）
api/kratos/team/v1/team.proto         ← Team 扩展字段（spirit_session_id / dag_node_id / depends_on / parallel_config 等）
        ↓
internal/service/
  chat_orchestrator_turn.go           ← 精灵 Agent 走 runSingleAgentViaTRPC，通过 spiritCustomTools 注入工具
  chat_orchestrator_spirit.go         ← Spirit Team 构建 + 模式选择
  spirit_team.go                      ← TeamStarter（生命周期管理）+ SpiritTeamAssembler
  spirit_synthesis.go                 ← Synthesis Engine
  team_turn_hooks.go                  ← executeTeamTurnViaHooks → HandleTeamTurnResult
        ↓
internal/tools/
  spirit_tools.go                     ← plan_and_execute / check_progress / cancel_orchestration / synthesize_results
  orchestrator/build_graph.go         ← DAG 图构建
  orchestrator/verification.go        ← 验证节点类型定义 + injectVerificationNodes
        ↓
internal/biz/
  spirit_team_usecase.go              ← AssembleTeam / ListActiveTeams / CancelTeam / AutoArchiveCompletedTeams
  task_planner.go                     ← TaskPlannerPort（Plan 阶段）
  agent_allocator.go                  ← AgentAllocatorPort（Allocate 阶段）
  task_orchestrator.go                ← TaskOrchestratorPort（Orchestrate 阶段）
  spirit_task_dag.go                  ← TaskDAG 拓扑路由
  spirit_parallel_config.go           ← ParallelConfig
  spirit_synthesis.go                 ← Synthesis Engine
  spirit_orchestration_cache.go       ← DQ Score 驱动编排缓存
  evolution.go                        ← 编排优化建议生成 + 进化护栏
  session/usecase.go                  ← Session 树查询
  team_usecase.go                     ← Create 支持 AutoCreated / SpiritSessionID
        ↓
internal/data/ent/schema/
  team.go                             ← spirit_session_id 索引 idx_teams_spirit_session
        ↓
web/src/
  features/spirit/                    ← 精灵域（api.ts / types.ts / spiritUi.ts / observabilityConstants.ts）
  features/chat/                      ← Chat 域（types / timelineTypes / useAgentBlocks / useChatTimeline / useTodoBoard 等）
  composables/chat/                   ← useAutoCollapse / useContextualLoadingMessage / useStatusPulse
  stores/spirit/                      ← useSpiritTeamStore
  components/spirit/                  ← 精灵专用组件（14+ 个）
  components/chat/                    ← Chat 面板扩展（TaskBoard / TaskBoardNode / TimelineNode / ChatExecutionCard 等）
```

**红线**：
- `internal/biz` 不 import `pkg/trpc-agent-go`
- 精灵构建仅在 `internal/service`
- 展示组件不 import Store（props/emits 通信）
- 展示组件不直接调 API（Store action 中调用）
- Composable 不持有 UI 状态

### 1.3 影响域

| 包 | 变更类型 | 说明 |
|----|----------|------|
| `internal/biz/session` | 扩展 | Session 树查询、TaskSummary / TeamDisplayName 字段 |
| `internal/biz/team` | 扩展 | AutoCreated / SpiritSessionID / ListBySpiritSessionID |
| `internal/biz/spirit_team_usecase` | 新增 | 三阶段编排核心逻辑、DAG 拓扑、并行配额、自动归档 |
| `internal/biz/task_planner` | 新增 | Plan 阶段端口接口 |
| `internal/biz/agent_allocator` | 新增 | Allocate 阶段端口接口 |
| `internal/biz/task_orchestrator` | 新增 | Orchestrate 阶段端口接口 |
| `internal/biz/spirit_orchestration_cache` | 新增 | 编排缓存 + DQ Score 三元分解 |
| `internal/biz/spirit_synthesis` | 新增 | Synthesis Engine 逻辑 |
| `internal/service` | 新增 | spirit_team.go / spirit_synthesis.go / chat_orchestrator_spirit.go / team_turn_hooks.go |
| `internal/tools` | 新增 | spirit_tools.go / orchestrator/ |
| `internal/event` | 扩展 | spirit_team_assembled 等 15+ 个新 EnvelopeType |
| `internal/data/ent/schema/team` | 扩展 | spirit_session_id 索引 |
| `web/src/features/spirit` | 新增 | 类型、API、UI 工具函数、可观测性常量 |
| `web/src/features/chat` | 新增 | useAgentBlocks / useChatTimeline / useTodoBoard / timelineTypes / agentTreeTypes |
| `web/src/composables/chat` | 新增 | useAutoCollapse / useContextualLoadingMessage / useStatusPulse |
| `web/src/stores/spirit` | 新增 | useSpiritTeamStore |
| `web/src/components/spirit` | 新增 | 14+ 个新组件 |
| `web/src/components/chat` | 修改 | ChatEntitySidebar 重构、ChatMessagePanel 三模式 + TaskBoard 树形嵌套 + 可观测性集成 |
| `api/kratos/session/v1` | 扩展 | Session Proto 字段 |
| `api/kratos/team/v1` | 扩展 | Team Proto 字段 |

**不改动**：`internal/server` 直连 runtime；`internal/data` 除新增字段外无 schema 变更；Team 编译/运行流程不变。

---

## 二、Session 树状模型

### 2.1 数据结构

```
Spirit Session (root)
  ├── ParentSessionID: null
  ├── RootSessionID: self.ID
  ├── AgentDepth: 0
  └── OwnerType: "agent", AgentID: __spirit__
      │
      ├── Team Session A (child)
      │   ├── ParentSessionID: spirit_session.ID
      │   ├── RootSessionID: spirit_session.ID
      │   ├── AgentDepth: 1
      │   └── OwnerType: "team", TeamID: team_A.ID
      │       │
      │       └── SubAgent Session A.1 (grandchild)
      │           ├── ParentSessionID: team_A_session.ID
      │           ├── RootSessionID: spirit_session.ID
      │           ├── AgentDepth: 2
      │           └── OwnerType: "agent", AgentID: <sub-agent>
```

> **任务看板嵌套深度受 `MaxSessionDepth=2` 约束**：看板可下钻两层（精灵 → 团队 → 子 agent）。

### 2.2 Session 扩展字段

| 字段 | 类型 | 存储位置 | 说明 |
|------|------|---------|------|
| `TaskSummary` | `string` | `sessions.metadata_json.task_summary` | 精灵生成的任务摘要 |
| `TeamDisplayName` | `string` | `sessions.metadata_json.team_display_name` | 团队显示名称 |

### 2.3 Session 树查询接口

```go
type SessionTreeReader interface {
    ListByParentSessionID(ctx context.Context, parentSessionID string) ([]Session, error)
    GetRootSession(ctx context.Context, sessionID string) (Session, error)
}
```

Wire 绑定：`SessionUsecase` 实现 `SessionTreeReader`，通过 `SessionRepository` 查询。

---

## 三、Team 扩展

### 3.1 新增字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `SpiritSessionID` | `string` | 创建该团队的精灵 Session ID |
| `TaskDescription` | `string` | 任务描述 |
| `AutoCreated` | `bool` | 是否由精灵自动创建 |
| `DAGNodeID` | `string` | DAG 节点 ID |
| `DependsOn` | `[]string` | 依赖的 DAG 节点 ID 列表 |
| `ParallelConfigJSON` | `string` | 并行配置 JSON |
| `Readonly` | `bool` | 是否只读 |
| `Source` | `string` | 来源 |
| `Kind` | `string` | 类型 |

### 3.2 Team 创建流程（三阶段编排路径）

```
精灵调用 plan_and_execute 工具
  → Phase 1: Plan（TaskPlannerPort.Plan）
    ├── 生成 TaskPlan（复杂度级别、策略、子任务列表、拓扑提示）
    └── 策略为 StrategyDirect 时直接返回，跳过 Phase 2/3
  → Phase 2: Allocate（AgentAllocatorPort.Allocate）
    ├── 将每个子任务匹配到最佳 Agent 或 Team
    └── 分配失败时记录日志但不中断
  → Phase 3: Orchestrate（TaskOrchestratorPort.Orchestrate）
    ├── 构建 TaskDAG，路由拓扑（parallel / sequential / hybrid / coordinator）
    ├── 为每个子任务创建 Team + Team Session
    ├── 启动无依赖的团队（TeamStarter.StartTeamTurn）
    └── 发射 spirit_team_assembled / spirit_plan_created / spirit_allocation_created / spirit_orchestration_started 事件
```

### 3.3 DAG 拓扑路由

| 条件 | 拓扑 | 说明 |
|------|------|------|
| 无节点 | `coordinator` | 退化为协调者模式 |
| 所有节点都是根节点（无依赖） | `parallel` | 全并行 |
| 深度 > 3 | `coordinator` | 深度过深，需协调 |
| 最大宽度 > 1 | `hybrid` | 混合并行/顺序 |
| 其他 | `sequential` | 顺序执行 |

### 3.4 并行配置

```go
type ParallelConfig struct {
    MaxConcurrentTeams  int  `json:"max_concurrent_teams"`  // 最大并行团队数（默认 3）
    MaxTeamConcurrency  int  `json:"max_team_concurrency"`  // 单团队内最大并发数（默认 2）
    TeamTimeoutSeconds  int  `json:"team_timeout_seconds"`  // 团队超时秒数（默认 600）
    AutoArchiveSeconds  int  `json:"auto_archive_seconds"`  // 自动归档秒数（默认 3600）
    MaxSessionDepth     int  `json:"max_session_depth"`     // 最大会话树深度（默认 2 = 看板嵌套层级）
}
```

### 3.5 Team 状态机

```
pending → running → completed → archived
                 → failed → archived
                 → cancelled → archived
                 → interrupted → running
```

---

## 四、WS 事件协议

### 4.1 新增 EnvelopeType

```go
const (
    // 团队生命周期事件
    EnvelopeTypeSpiritTeamAssembled       EnvelopeType = "spirit_team_assembled"
    EnvelopeTypeSpiritTeamCompleted       EnvelopeType = "spirit_team_completed"
    EnvelopeTypeSpiritTeamFailed          EnvelopeType = "spirit_team_failed"
    EnvelopeTypeSpiritTeamProgress        EnvelopeType = "spirit_team_progress"
    EnvelopeTypeSpiritTeamsAllCompleted   EnvelopeType = "spirit_teams_all_completed"

    // 三阶段编排事件
    EnvelopeTypeSpiritPlanCreated              EnvelopeType = "spirit_plan_created"
    EnvelopeTypeSpiritAllocationCreated        EnvelopeType = "spirit_allocation_created"
    EnvelopeTypeSpiritOrchestrationStarted     EnvelopeType = "spirit_orchestration_started"
    EnvelopeTypeSpiritOrchestrationCheckpoint  EnvelopeType = "spirit_orchestration_checkpoint"
    EnvelopeTypeSpiritOrchestrationInterrupted EnvelopeType = "spirit_orchestration_interrupted"
    EnvelopeTypeSpiritSynthesisCompleted       EnvelopeType = "spirit_synthesis_completed"

    // Butler 编排事件
    EnvelopeTypeButlerOrchestrationStarted   EnvelopeType = "butler.orchestration.started"
    EnvelopeTypeButlerOrchestrationCompleted EnvelopeType = "butler.orchestration.completed"
    EnvelopeTypeButlerOrchestrationFailed    EnvelopeType = "butler.orchestration.failed"
)
```

### 4.2 事件载荷

**spirit_team_completed**：
```go
env.Metadata = map[string]any{
    "team_id":         teamID,
    "session_id":      teamSessionID,
    "result_summary":  resultSummary,
    "duration_ms":     durationMS,
    "total_token_in":  totalTokenIn,
    "total_token_out": totalTokenOut,
}
```

**spirit_team_progress**：
```go
env.Metadata = map[string]any{
    "team_id":         teamID,
    "status":          status,
    "progress_pct":    progressPct,
    "current_step":    currentStep,
    "completed_steps": completedSteps,
    "total_steps":     totalSteps,
}
```

### 4.3 复用现有事件

| EnvelopeType | 来源 | 精灵模式用途 |
|-------------|------|-------------|
| `team_step_started` | Team Runner | 成员开始执行 → 更新成员状态 |
| `team_step_finished` | Team Runner | 成员执行完成 |
| `member_message_start/delta/done` | Team Runner | 成员消息流 → 任务执行面板 |
| `session.status_changed` | SessionStatusPublisher | 团队 Session 状态 |
| `orchestration_agent_status` | StatusProjector | Agent 节点实时状态 |
| `team_run_started/finished/failed` | Team Runner | 团队 Run 生命周期 |
| `tool_call` / `tool_result` | Agent Runtime | Agent 级工具调用 → 语境加载消息 |

---

## 五、Session 与进化体系关联

### 5.1 Session 记录的信息

| 信息类别 | 字段/存储位置 | 进化用途 |
|---------|-------------|---------|
| 任务上下文 | `Session.MetadataJSON` | 编排进化：任务类型→模式推荐 |
| 团队组成 | `Team.DefinitionSnapshotJSON` | 编排进化：成员组合效率分析 |
| 执行轨迹 | `TeamRun` + `TeamRunStep` | 技能进化：工具调用模式检测 |
| 编排活动 | `OrchestrationStep` | 编排进化：DQ Score 计算 |
| 参与者画像 | `SessionParticipant` | Agent 进化：能力画像更新 |
| 工具调用明细 | `ChatMessage.OptionsJSON` | 技能进化：Skill 健康度分析 |
| 记忆提取锚点 | `Memory L1-L4` | 记忆：事实/实体/关系提取 |
| 进化指标 | `AgentRuntimeSettings.evolution_*` | Agent 进化：工具成功率/检索质量 |
| 父子关联 | `ParentSessionID` / `RootSessionID` | 全链路：从 Team 回溯到 Spirit |
| DAG 拓扑 | `Team.DAGNodeID` / `Team.DependsOn` | 编排进化：拓扑选择效率分析 |
| 并行配置 | `Team.ParallelConfigJSON` | 编排进化：并行策略优化 |

### 5.2 DQ Score 三元分解

```go
type DQScoreBreakdown struct {
    Validity     float64 `json:"validity"`      // 结果有效性（0-1）
    Specificity  float64 `json:"specificity"`    // 结果具体性（0-1）
    Correctness  float64 `json:"correctness"`    // 结果正确性（0-1）
    Overall      float64 `json:"overall"`        // 加权总分
    DurationMs   int64   `json:"duration_ms"`    // 执行时长
}
```

计算公式：`Overall = Validity * 0.4 + Specificity * 0.3 + Correctness * 0.3`

### 5.3 编排进化闭环

```
团队执行完成
  → 计算 DQ Score Breakdown
  → DQ Score > 0.7 → 缓存编排拓扑（OrchestrationCache）
  → DQ Score < 0.5 → EvolutionUsecase 生成编排优化建议
  → 下次 assemble_team → 先查缓存，命中则复用
```

---

## 六、前端 Chat UI 展示层（合并 M59 OBS + M69 展示模型）

### 6.1 左侧面板简化

**变更文件**：`web/src/components/chat/ChatEntitySidebar.vue`

通过 `spiritMode` prop 控制是否显示 Agent 列表：

```vue
<!-- ChatPage.vue -->
<ChatEntitySidebar :spirit-mode="spiritStore.activePanelMode === 'spirit'" />

<!-- ChatEntitySidebar.vue -->
<template v-if="!spiritMode">
  <ChatSectionHeader icon="smart_toy" ... />
  <ChatEntityGroup v-for="group in agentGroups" ... />
</template>
```

**团队列表数据加载**（M69 P0 修复）：

```typescript
// useChatWorkspace.ts
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
      if (spiritStore.teams.length > 0) {
        spiritStore.reset();
      }
    }
  },
  { immediate: true },
);

// WS 重连恢复
watch(streamManager.wsReplaying, (replaying, wasReplaying) => {
  if (wasReplaying && !replaying && spiritStore.currentSpiritSessionId) {
    spiritStore.reloadTeams();
  }
});
```

### 6.2 任务看板树形嵌套展示模型

#### 6.2.1 数据结构

**新增文件**：`web/src/features/chat/timelineTypes.ts`

```typescript
export type TaskBoardNodeKind =
  | 'task'            // 任务
  | 'thinking'        // 思考
  | 'action'          // 工具
  | 'reply'           // 回复
  | 'sub_task_board'  // 子任务看板（递归嵌套）
  | 'end'             // 结束
  | 'error'           // 错误

export type NodeStatus =
  | 'running'
  | 'tool_running'
  | 'tool_blocked'
  | 'completed'
  | 'failed'
  | 'partial_failure'
  | 'cancelled'

export interface TaskBoardNode {
  kind: TaskBoardNodeKind
  id: string
  timestamp: string
  collapsed: boolean
  content?: string
  reasoning?: string
  toolName?: string
  toolStatus?: string
  toolDuration?: number
  toolCallId?: string
  toolArguments?: string
  toolResult?: string
  childBoard?: AgentBlock
  status?: NodeStatus
  errorMessage?: string
  turnStatus?: string
}
```

**AgentBlock 结构**（`web/src/features/chat/agentTreeTypes.ts`）：

```typescript
export type AgentBlockStatus =
  | 'running'
  | 'tool_running'
  | 'tool_blocked'
  | 'completed'
  | 'failed'
  | 'partial_failure'
  | 'cancelled'

export interface AgentBlock {
  id: string
  agentKey: string
  agentName: string
  /** 当前 turn 的任务看板 */
  board: TaskBoardNode[]
  /** 子 agent 子任务看板（递归结构） */
  childBlocks: AgentBlock[]
  status: AgentBlockStatus
  /** 是否有工具失败但最终结果成功 */
  hasPartialFailure: boolean
  /** progress section（独立字段，渲染在 turn 头部） */
  progressSections: ProgressSection[]
  /** 兼容字段：result 不再驱动 UI 单一来源 */
  result?: string
}
```

#### 6.2.2 渲染规则

| 节点类型 | 图标 | 默认折叠 | 说明 |
|---------|------|---------|------|
| `task` | `task_alt` | 否 | 任务描述（用户/Agent 视角） |
| `thinking` | `psychology` | 是（完成后） | reasoning 内容 |
| `action` | `bolt` | 是（完成后） | 工具调用 |
| `reply` | `chat` | 否 | Agent 回复（含最终答案） |
| `sub_task_board` | `account_tree` | 否 | 子任务看板（递归渲染） |
| `end` | `check_circle` | 否 | 任务完成标记 |
| `error` | `error` | 否 | 错误信息 |

#### 6.2.3 折叠策略（重要：完成即展开）

- **`thinking` 和 `action` 完成后默认折叠**，展开显示完整内容
- **`task`、`reply`、`end` 始终展开**
- **已完成回合（`status === 'completed'`）的看板 `collapsed: false`（默认展开）**，让用户直达最终答案（修复 F-19）
- 全局"展开全部/折叠全部"按钮作用于所有层级
- 运行中的工具不受"折叠全部"影响

#### 6.2.4 节点构建逻辑

**核心 composable**：`web/src/features/chat/composables/useAgentBlocks.ts`

```typescript
export function useAgentBlocks(deps: {
  messages: ComputedRef<Message[]>
  plannerKind?: ComputedRef<string>
}) {
  const blocks = computed<AgentBlock[]>(() => {
    const turnBlocks = groupMessagesByTurn(deps.messages.value)
    return turnBlocks.map((turn) => buildAgentBlock(turn, deps.plannerKind?.value))
  })
  return { blocks }
}

function buildAgentBlock(turn: TurnBlock, plannerKind: string): AgentBlock {
  // 1. 构建 timeline（不含 progress）
  // 2. 提取 progressSections（独立字段，渲染在 turn 头部）
  // 3. 计算 status（含 tool_blocked / tool_running 显式状态）
  // 4. 计算 hasPartialFailure
  // 5. 递归构建 childBlocks（从 subagents_spawn tool 提取子 agent session）
  // 6. collapsed: false（已完成回合默认展开，修复 F-19）
  return { ... }
}
```

#### 6.2.5 组件结构

**新增文件**：
- `web/src/components/chat/TaskBoardNode.vue` — 单个节点（task/thinking/action/reply/sub_task_board/end/error）
- `web/src/components/chat/TaskBoard.vue` — 任务看板壳（递归渲染，支持树形嵌套）

```vue
<!-- TaskBoardNode.vue -->
<template>
  <div :class="['task-board-node', `task-board-node--${kind}`]">
    <!-- 节点轨道：左侧连接线 + 节点圆点 -->
    <div class="task-board-node__rail">
      <div class="task-board-node__dot" :class="dotClass"></div>
    </div>

    <!-- 节点体 -->
    <div class="task-board-node__body">
      <header class="task-board-node__header">
        <q-icon :name="iconForKind" />
        <span class="task-board-node__title">{{ title }}</span>
        <span v-if="status" class="task-board-node__status">{{ statusLabel }}</span>
      </header>

      <!-- task 节点 -->
      <div v-if="kind === 'task'" class="task-board-node__content">
        {{ content }}
      </div>

      <!-- thinking 节点 -->
      <q-expansion-item
        v-else-if="kind === 'thinking'"
        :default-opened="!collapsed"
      >
        <div class="task-board-node__reasoning">{{ reasoning }}</div>
      </q-expansion-item>

      <!-- action 节点 -->
      <q-expansion-item
        v-else-if="kind === 'action'"
        :default-opened="!collapsed"
      >
        <template #header>
          <span class="task-board-node__tool-name">{{ toolDisplayName }}</span>
          <span class="task-board-node__tool-status">{{ toolStatusLabel }}</span>
          <span class="task-board-node__tool-duration">{{ durationLabel }}</span>
        </template>
        <div class="task-board-node__args">{{ toolArguments }}</div>
        <div class="task-board-node__result">{{ toolResult }}</div>
      </q-expansion-item>

      <!-- reply 节点 -->
      <div v-else-if="kind === 'reply'" class="task-board-node__reply">
        <MarkdownView :source="content" />
      </div>

      <!-- sub_task_board 节点：递归渲染 -->
      <TaskBoard
        v-else-if="kind === 'sub_task_board' && childBoard"
        :block="childBoard"
        :depth="depth + 1"
      />

      <!-- end 节点 -->
      <div v-else-if="kind === 'end'" class="task-board-node__end">
        ✅ 完成
      </div>

      <!-- error 节点 -->
      <div v-else-if="kind === 'error'" class="task-board-node__error">
        ❌ {{ errorMessage }}
      </div>
    </div>
  </div>
</template>
```

#### 6.2.6 嵌套深度控制

```vue
<!-- TaskBoard.vue -->
<template>
  <div :class="['task-board', `task-board--depth-${depth}`]">
    <TaskBoardNode
      v-for="node in block.board"
      :key="node.id"
      :node="node"
      :depth="depth"
    />

    <!-- 嵌套子 agent 子任务看板（深度守卫） -->
    <template v-if="depth < MAX_DEPTH">
      <TaskBoard
        v-for="child in block.childBlocks"
        :key="child.id"
        :block="child"
        :depth="depth + 1"
      />
    </template>
  </div>
</template>

<script setup>
defineProps<{ block: AgentBlock; depth: number }>()

// 嵌套深度受 MaxSessionDepth=2 约束
const MAX_DEPTH = 2
</script>
```

#### 6.2.7 状态机（M69 P4 useAgentBlocks 修复）

**`AgentBlockStatus` 状态机**：

```typescript
function computeAgentStatus(block: AgentBlock): AgentBlockStatus {
  if (block.board.some(n => n.status === 'tool_blocked')) return 'tool_blocked'
  if (block.board.some(n => n.status === 'tool_running')) return 'tool_running'
  if (block.board.some(n => n.status === 'running')) return 'running'
  if (block.board.every(n => n.status === 'completed' || n.kind === 'end')) {
    return block.hasPartialFailure ? 'partial_failure' : 'completed'
  }
  if (block.board.some(n => n.status === 'failed' || n.status === 'error')) {
    return 'failed'
  }
  return block.status
}
```

**修复 F-13~F-21**（见 §D5 详细设计）

### 6.3 任务执行面板布局（v7 定稿）

**变更文件**：`web/src/components/spirit/TaskExecutionPanel.vue`

```
TaskExecutionPanel
  ├── 顶部导航栏（返回精灵 + 团队名称 + 状态 Badge + 编排模式标签）
  ├── 用户消息气泡
  ├── ThinkingArea              ← v7 新增：脑纹SVG + 流光 + 半透明span
  ├── UnifiedExecutionPanel     ← v7 新增：统一面板（替代原 ParallelTeamOverview + 独立 section）
  │     ├── PanelSection: 任务拆解（📋）
  │     │     └── TaskRow (×N)：编号圆圈 + 任务名 + 团队标签 + 状态
  │     ├── PanelSection: 依赖关系（🔀）
  │     │     └── DAGDiagramCard（流式节点图）
  │     └── PanelSection: 团队进度（📊）
  │           └── TeamProgressCard (×N)：可展开 + 恢复/取消按钮
  ├── 精灵回复（综合汇报）
  └── SpiritStatusBar           ← 已有组件
```

**关键变更（v7 vs 原设计）**：
1. **移除 `ParallelTeamOverview`**：其信息（DAG + 并行配额）合并到统一面板的依赖关系区
2. **新增 `ThinkingArea`**：替代原 `ChatReasoningPeek`，使用脑纹 SVG + 流光动画 + 半透明 span
3. **新增 `UnifiedExecutionPanel`**：单卡片容器，三个子区域纵向排列，细分隔线分开
4. **恢复/取消按钮移到团队卡片头部**：中断团队的 `TeamProgressCard` 头部右侧显示操作按钮
5. **精灵回复**：统一面板下方，综合汇报各团队状态

### 6.4 可观测性 UX 设计

#### 6.4.1 OBS-01 对话流自动折叠

`useAutoCollapse` composable 管理折叠/展开状态：

```typescript
export function useAutoCollapse() {
  const collapsedBlockKeys = ref<Set<number>>(new Set())
  function onBlockCompleted(blockKey: number) { collapsedBlockKeys.value.add(blockKey) }
  function expandAll() { collapsedBlockKeys.value.clear() }
  function toggleBlock(blockKey: number) { /* toggle */ }
  return { collapsedBlockKeys, onBlockCompleted, expandAll, toggleBlock }
}
```

**例外**：已完成回合的 `collapsed: false`（M69 P4 修复 F-19）。

#### 6.4.2 OBS-02 语境加载消息

事件到消息映射表定义在 `observabilityConstants.ts`：

| 事件 | 消息 |
|------|------|
| `butler.orchestration.started` | "正在处理任务…" |
| `spirit_plan_created` | "正在分析任务复杂度…" |
| `spirit_allocation_created` | "正在分配 Agent 角色…" |
| `spirit_orchestration_started` | "正在编排执行流程…" |
| `tool_call` | "{agentName} 正在{displayLabel}…" |
| `tool_result` | "{agentName} 完成，耗时 {durationSec}s" |

#### 6.4.3 OBS-03 Agent 状态标签

状态聚合映射将 17 种 `AgentNodeStatus` 聚合为 7 种展示标签（queued / active / suspended / done / failed / skipped / cancelled），加上 M69 P4 新增的 `tool_blocked` 显式状态。

组件 `AgentStatusLabel.vue` 使用 `q-badge` 渲染，active 状态带脉冲动画。

#### 6.4.4 OBS-04 底部状态栏

`SpiritStatusBar.vue` Props：`runningTeamCount` / `interruptedTeamCount` / `quotaUsed` / `quotaMax` / `tokenUsage?` / `lastEvent?`。

#### 6.4.5 OBS-05 侧边栏状态脉冲

`useStatusPulse` composable 触发 CSS 脉冲动画，WS 回放期间静默。

#### 6.4.6 OBS-06 可折叠工具输出

`ChatExecutionCard` 监听 `event.status` 变化，completed/failed/cancelled 时自动折叠。

#### 6.4.7 OBS-08 ChatExecutionCard 折叠增强

**架构：Provide/Inject + Signal**

```typescript
export const EXECUTION_COLLAPSE_CONTROL_KEY: InjectionKey<ExecutionCollapseControl> =
  Symbol('execution-collapse-control')

export interface ExecutionCollapseControl {
  expandAllSignal: Readonly<Ref<number>>
  collapseAllSignal: Readonly<Ref<number>>
}
```

**Provider**（`ChatMessagePanel.vue`）：`expandAll()`/`collapseAll()` 同时操作 TurnBlock 级 + ChatExecutionCard 级。

**Consumer**（`ChatExecutionCard.vue`）：inject signal，watch 变化后更新 `expanded`。运行中工具不响应 collapseAll。

**5s 耗时守卫**：
- `started_at` → `occurred_at` → `Date.now()` 三级降级
- running ≥5s 时显示实时计时器
- ≥60s 变为 `var(--color-warning)` 警告色

**折叠态摘要兜底**：

| tool_name | 摘要模板 |
|-----------|----------|
| `file_edit`/`file_write` | `修改 {filename}` |
| `file_read` | `读取 {filename}` |
| `grep`/`search_files` | `搜索 "{pattern}"` |
| `bash` | `> {command}` |
| 其他 | 空 |

#### 6.4.8 OBS-07 中断恢复提示

`InterruptedTeamCard.vue` 在 `status === 'interrupted'` 时显示。`canResume` 基于 `team.graphExecutionId` 是否存在判断。

### 6.5 TODO 看板与工具调用时间线（P1.6）

#### 6.5.1 TK-01 TODO 任务看板

**数据源优先级**：
1. session state `temp:todos[:<branch>]`（trpc-agent-go 权威源）
2. 最近一次 `todo_write` 工具 result.output.todos
3. 缺失 → 看板不渲染

**数据契约**：

```typescript
export type TodoStatus = 'pending' | 'in_progress' | 'completed'
export interface TodoItem {
  content: string
  activeForm: string
  status: TodoStatus
}
export interface TodoBoardState {
  todos: TodoItem[]
  lastUpdated: string
  source: 'session_state' | 'tool_result' | 'merged'
}
```

**组件拆分**：

| 组件 | 路径 | 职责 |
|------|------|------|
| `TodoKanbanBoard.vue` | `components/chat/` | 看板壳：折叠/展开 + 三列布局 + 空态 |
| `TodoColumn.vue` | `components/chat/` | 单列：列头 + 卡片列表 + 变更脉冲 |
| `TodoCard.vue` | `components/chat/` | 单卡：状态图标 + content + activeForm |
| `useTodoBoard.ts` | `features/chat/composables/` | 从 session 中抽取 `TodoBoardState`，订阅 WS |

#### 6.5.2 TK-02 工具调用时间线

**替换规则**：
```
条件：turn.tools.length >= 2
  → 渲染 <ToolCallTimeline :events="turn.tools" />
条件：turn.tools.length < 2
  → 沿用 <ChatExecutionCard />（避免视觉噪声）
```

**节点排序稳定性**：

```typescript
const sorted = computed(() =>
  [...props.events].sort((a, b) => {
    const ta = new Date(a.occurred_at).getTime()
    const tb = new Date(b.occurred_at).getTime()
    if (ta !== tb) return ta - tb
    return a.id.localeCompare(b.id)   // 同 ms 用 id 兜底
  })
)
```

**状态点规范**：

| 状态 | 颜色 | 图标 | 动画 |
|------|------|------|------|
| `running` | `var(--color-warning)` | `hourglass_top` | 脉冲 1s |
| `success` | `var(--color-success)` | `check_circle` | 无 |
| `failed` / `error` | `var(--color-danger)` | `error` | 无 |
| `cancelled` | `var(--color-text-tertiary)` | `cancel` | 无 |
| `blocked` | `var(--color-warning)` | `warning` | 无 |
| `stuck` | `var(--color-danger)` | `error_outline` | 行内展示"工具无返回结果" |

#### 6.5.3 TK-03 Stuck 工具可观测化

**检测条件**：

```typescript
export function isStuckTool(event: ToolUseEvent): boolean {
  return event.error_code === 'tool_timeout'
}
```

**i18n 键**：
- `chat.activity.stuckTool` — "工具无返回结果（turn completed without tool result）"
- `chat.activity.stuckToolBadge` — "⚠ {count} 工具未返回"

#### 6.5.4 决策记录：工具显示开关实现路径

**问题**：测试环境需看到工具执行全过程，生产环境应隐藏以保持 UI 简洁。

**候选方案**：

| 方案 | 优点 | 缺点 | 决策 |
|------|------|------|------|
| 前端 store + 路由配置 | 简单可靠，零网络开销，用户可手动切换 | WS 仍下发所有 envelope | ✅ **采纳** |
| 后端 Envelope 网关层过滤 | 性能最好，从源头切断 | 需新增字段下发，运维配置复杂 | ❌ |
| 按 user role 区分 | 多租户友好 | 实现复杂，与本需求过度耦合 | ❌ |

**采纳方案详细**：
- 控制位置：`useUiConfigStore`（Pinia），持久化 `localStorage.chat.ui.showToolCalls`
- 注入：通过 `provide(TOOL_DISPLAY_KEY, { showToolCalls })` 注入到 `ChatMessagePanel`
- 消费方：`ChatMessageList.vue` / `TurnBlock.vue` / `SpiritStatusBar.vue` 读取后条件渲染
- 影响范围：仅前端渲染层，后端 Envelope 协议零修改

### 6.6 工具显示开关（TK-04）

#### 6.6.1 数据契约

```typescript
// web/src/stores/uiConfig/index.ts
export interface UiConfigState {
  showToolCalls: boolean
}

export const useUiConfigStore = defineStore('uiConfig', () => {
  const showToolCalls = ref(
    localStorage.getItem('chat.ui.showToolCalls') !== 'false' // 默认 true
  )

  function setShowToolCalls(v: boolean) {
    showToolCalls.value = v
    localStorage.setItem('chat.ui.showToolCalls', String(v))
  }

  return { showToolCalls, setShowToolCalls }
})
```

#### 6.6.2 Provide/Inject 注入

```typescript
// web/src/components/chat/ChatMessagePanel.vue
import { TOOL_DISPLAY_KEY } from 'src/features/chat/types'

const uiConfig = useUiConfigStore()
provide(TOOL_DISPLAY_KEY, computed(() => ({
  showToolCalls: uiConfig.showToolCalls,
})))
```

#### 6.6.3 关闭时降级矩阵

| 组件 | 关闭时行为 |
|------|----------|
| `ChatExecutionCard` | 不挂载（v-if） |
| `ToolCallTimeline` / `ToolCallTimelineItem` | 不挂载 |
| `ToolStrip` | 不渲染 |
| `TodoKanbanBoard` | **保留显示**（todo 业务必需） |
| `ChatReasoningPeek` | **保留显示**（思考是辅助理解必需） |
| `TaskBoardNode` (kind=action) | 折叠隐藏 |
| `SpiritStatusBar` 的 `toolCount` 字段 | 不渲染 |
| `ToolStuckBadge` | 不渲染 |
| 工具调用前后的纯文本回复 | 不受影响 |

#### 6.6.4 UI 入口

```vue
<!-- UiConfigToggle.vue -->
<template>
  <q-btn
    flat
    dense
    :icon="showToolCalls ? 'build_circle' : 'build_circle_outlined'"
    @click="toggle"
  >
    <q-tooltip>{{ showToolCalls ? t('chat.uiConfig.hideToolCallsTooltip') : t('chat.uiConfig.showToolCallsTooltip') }}</q-tooltip>
  </q-btn>
</template>
```

放置位置：`ChatMessagePanel.vue` 顶部操作栏，与"展开全部/折叠全部"按钮同行右对齐。

### 6.7 代码块自动识别语言与高亮（TK-05）

#### 6.7.1 组件契约

```typescript
// web/src/components/chat/CodeBlock.vue
export interface CodeBlockProps {
  code: string           // 代码原文
  lang?: string          // fenced info string 中的语言标识（可选）
  defaultCollapsed?: boolean  // 是否默认折叠（>20 行时自动 true）
}
```

#### 6.7.2 自动检测算法

```typescript
// web/src/features/chat/lib/detectCodeLanguage.ts
import hljs from 'highlight.js/lib/core'
import typescript from 'highlight.js/lib/languages/typescript'
// ... 注册 12 种语言

const LANG_CANDIDATES = [
  'typescript', 'javascript', 'go', 'python', 'bash',
  'json', 'yaml', 'sql', 'rust', 'java', 'markdown', 'shell',
]

export function detectLanguage(code: string, hint?: string): string {
  // 1. 显式指定优先
  if (hint && LANG_CANDIDATES.includes(hint)) return hint

  // 2. 大文件保护
  if (code.length > 10 * 1024) return 'plaintext'

  // 3. auto 检测（限候选）
  const sample = code.slice(0, 500)
  const result = hljs.highlightAuto(sample, LANG_CANDIDATES)
  if (result.relevance < 0.5) return 'plaintext'

  return result.language ?? 'plaintext'
}

export function highlight(code: string, lang: string): string {
  try {
    return hljs.highlight(code, { language: lang, ignoreIllegals: true }).value
  } catch {
    return escapeHtml(code)
  }
}
```

#### 6.7.3 组件实现

```vue
<!-- CodeBlock.vue -->
<template>
  <div class="code-block" :class="`code-block--lang-${displayLang}`">
    <header class="code-block__header">
      <span class="code-block__lang-tag">{{ displayLang }}</span>
      <q-space />
      <q-btn
        flat dense size="sm"
        :icon="copied ? 'check' : 'content_copy'"
        @click="onCopy"
      >
        <q-tooltip>{{ copied ? t('chat.codeBlock.copied') : t('chat.codeBlock.copy') }}</q-tooltip>
      </q-btn>
    </header>

    <div v-if="!isCollapsed" class="code-block__body">
      <pre><code v-html="highlightedHtml" /></pre>
    </div>
    <div v-else class="code-block__collapsed" @click="isCollapsed = false">
      ▶ {{ t('chat.codeBlock.expandLine', { count: lineCount }) }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { detectLanguage, highlight } from 'src/features/chat/lib/detectCodeLanguage'

const props = withDefaults(defineProps<CodeBlockProps>(), {
  defaultCollapsed: false,
})

const { t } = useI18n()
const displayLang = computed(() => detectLanguage(props.code, props.lang))
const lineCount = computed(() => props.code.split('\n').length)
const isCollapsed = ref(props.defaultCollapsed || lineCount.value > 20)
const highlightedHtml = computed(() => highlight(props.code, displayLang.value))
const copied = ref(false)

async function onCopy() {
  await navigator.clipboard.writeText(props.code)
  copied.value = true
  setTimeout(() => (copied.value = false), 2000)
}
</script>

<style scoped lang="scss">
.code-block {
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-sm);
  background: var(--color-bg-elevated);
  font-family: var(--font-family-mono);
  font-size: var(--font-size-base);
  line-height: var(--line-height-base);
}
.code-block__header {
  display: flex;
  align-items: center;
  padding: 4px 8px;
  border-bottom: 1px solid var(--color-border-subtle);
}
.code-block__lang-tag {
  font-size: var(--font-size-xs);
  color: var(--color-text-tertiary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
.code-block__body {
  overflow-x: auto;
  padding: 8px 12px;
}
.code-block__collapsed {
  padding: 8px 12px;
  cursor: pointer;
  color: var(--color-text-secondary);
  font-family: var(--font-family-base);
}
</style>
```

#### 6.7.4 MarkdownView 集成

```typescript
// web/src/components/chat/MarkdownView.vue
import CodeBlock from './CodeBlock.vue'

// ~~方案 A（已否决）：markdown-it renderer override~~
// 问题：v-html 渲染后无法挂载 Vue 组件，只能做静态高亮

// 方案 B（采纳）：自定义组件渲染
// 在 MarkdownView 的 render 函数中，识别 <pre><code class="language-xxx"> 后
// 挂载 <CodeBlock> 组件替代默认的 <pre><code> 渲染
```

#### 6.7.5 视觉规范

| 维度 | 取值 |
|------|------|
| 字体 | `var(--font-family-mono)` |
| 字号 | `var(--font-size-base)`（不放大） |
| 行高 | `var(--line-height-base)` |
| 背景 | `var(--color-bg-elevated)` |
| 边框 | `1px solid var(--color-border-default)` |
| 圆角 | `var(--radius-sm)` |
| 主题色 | 跟随 `var(--color-text-base)`，高亮 token 用 `var(--color-primary)` / `var(--color-text-secondary)` 区分 |

不引入新 token，遵循 `UX 规范` 中的「代码块」既有定义（如有）。

### 6.8 思考节点 UI 不喧宾夺主（TK-06，v7 定稿）

#### 6.8.1 流式状态展示（v7 定稿）

```vue
<!-- ThinkingArea.vue - 流式态 -->
<template>
  <div class="thinking-area" style="margin-bottom: 12px; display: flex; align-items: flex-start; gap: 8px;">
    <!-- 脑纹 SVG 图标 + 流光 -->
    <span class="brain-icon">
      <svg viewBox="0 0 24 24" fill="none" stroke="var(--color-primary)" stroke-width="1.5">
        <path d="M12 2C8 2 5 5 5 9c0 2 1 3.5 2 4.5V20a2 2 0 002 2h6a2 2 0 002-2v-6.5c1-1 2-2.5 2-4.5 0-4-3-7-7-7z"/>
        <path d="M9 7c1-1 2-1 3 0s2 1 3 0" stroke-opacity="0.5"/>
        <path d="M8 11c1-1 2.5-1 4 0s2.5 1 4 0" stroke-opacity="0.3"/>
      </svg>
      <span class="flow-light"></span>
    </span>
    <!-- 思考内容 -->
    <div class="thinking-inline active">
      {{ reasoningContent }}<span class="cursor"></span>
    </div>
  </div>
</template>

<style>
.brain-icon {
  display: inline-flex; align-items: center; justify-content: center;
  width: 18px; height: 18px; position: relative; flex-shrink: 0;
}
.brain-icon svg { width: 18px; height: 18px; }
.brain-icon .flow-light {
  position: absolute; inset: 0;
  background: linear-gradient(90deg, transparent, rgba(91,138,245,0.6), transparent);
  animation: flowLight 2s ease-in-out infinite;
  border-radius: 50%;
}
@keyframes flowLight {
  0% { opacity: 0; transform: translateX(-8px); }
  50% { opacity: 1; transform: translateX(0); }
  100% { opacity: 0; transform: translateX(8px); }
}
.thinking-inline {
  display: inline-block;
  background: rgba(22, 33, 62, 0.6);
  border-radius: 4px;
  padding: 4px 10px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--color-text-secondary);
  max-width: 100%;
  max-height: 3em;
  overflow: hidden;
  transition: all 0.3s;
}
.thinking-inline.active {
  background: rgba(22, 33, 62, 0.8);
}
.thinking-inline .cursor {
  display: inline-block; width: 2px; height: 12px;
  background: var(--color-primary);
  animation: blink 0.8s step-end infinite;
  vertical-align: middle; margin-left: 2px;
}
@keyframes blink { 0%,100% { opacity: 1; } 50% { opacity: 0; } }
</style>
```

#### 6.8.2 无思考内容时折叠

```vue
<!-- ThinkingArea.vue - 折叠态（无思考内容） -->
<template>
  <div class="thinking-area thinking-area--collapsed" @click="toggle">
    <span class="brain-icon"><!-- 同上 SVG --></span>
    <span class="thinking-collapsed-btn">思考内容</span>
  </div>
</template>

<style>
.thinking-collapsed-btn {
  padding: 4px 10px;
  font-size: 12px;
  background: rgba(22, 33, 62, 0.6);
  border-radius: 4px;
  color: var(--color-text-secondary);
  cursor: pointer;
}
</style>
```

#### 6.8.3 完成态折叠

```vue
<!-- TaskBoardNode.vue - kind === 'thinking' 完成态 -->
<template>
  <div v-else class="thinking-node thinking-node--collapsed" @click="toggle">
    <span class="thinking-node__icon">🧠</span>
    <span v-if="!expanded" class="thinking-node__summary">{{ summary }}</span>
    <div v-else class="thinking-node__full">{{ node.reasoning }}</div>
  </div>
</template>

<script setup>
const expanded = ref(false)
const summary = computed(() => {
  const text = props.node.reasoning ?? ''
  if (text.length < 30) return text  // 例外：不折叠
  const firstSentence = text.split(/[。.!?！？\n]/)[0]
  return firstSentence.length > 60 ? firstSentence.slice(0, 60) + '…' : firstSentence
})
</script>

<style>
.thinking-node--collapsed {
  cursor: pointer;
  padding: 2px 0;
  font-family: var(--font-family-base);
  color: var(--color-text-secondary);
  font-size: var(--font-size-base);
  line-height: var(--line-height-base);
  background: transparent;
  transition: all 200ms;
}
.thinking-node--collapsed:hover {
  color: var(--color-text-base);
}
.thinking-node__full {
  margin-top: 4px;
  font-size: var(--font-size-base);  /* 与回复文本一致 */
  color: var(--color-text-secondary);
}
</style>
```

#### 6.8.4 关键约束

| 约束 | 取值 | 理由 |
|------|------|------|
| 脑纹图标尺寸 | 18×18px | 与行内文字对齐，不喧宾夺主 |
| 流光动画周期 | 2s ease-in-out | 柔和提示，不刺眼 |
| 内容容器背景 | `rgba(22, 33, 62, 0.6)` | 与背景色相近但略深，视觉融合 |
| 内容最大行数 | 2 行（`max-height: 3em`） | 不占据过多空间 |
| 光标闪烁周期 | 0.8s step-end | 标准终端光标节奏 |
| 折叠按钮背景 | 同内容容器 | 视觉一致性 |
| 字体 | `var(--font-family-base)` | 思考是辅助信息，混用 mono 会喧宾夺主 |
| 字号 | `var(--font-size-base)` | 与回复文本一致，不放大 |
| 颜色 | `var(--color-text-secondary)` | 比回复低一档亮度 |
| 背景 | `transparent`（完成态） | 区别于 ChatExecutionCard 的卡片样式 |
| 折叠阈值 | `< 30` 字符不折叠 | 信息密度过低，折叠反而干扰 |
| Summary 长度 | `≤ 60` 字 + `…` | 1 行内可读 |

---

## 七、前端架构

### 7.1 目录结构

```
web/src/features/spirit/
  types.ts                    ← SpiritTeam / SpiritMember / PanelMode / SpiritTeamStatus / SpiritTeamMode / TopologyType / ParallelConfig / AgentNodeStatusLabel
  api.ts                      ← listSpiritTeams / getSpiritTeamDetail / cancelSpiritTeam
  spiritUi.ts                 ← 状态映射、标签函数、AGENT_NODE_STATUS_MAP + STATUS_LABEL_CONFIG
  observabilityConstants.ts   ← 语境消息映射表、脉冲颜色配置

web/src/features/chat/
  types.ts                    ← ChatMessage / ToolUseEvent / EXECUTION_COLLAPSE_CONTROL_KEY / TOOL_DISPLAY_KEY
  timelineTypes.ts            ← TaskBoardNode / TaskBoardNodeKind / NodeStatus
  agentTreeTypes.ts           ← AgentBlock / AgentBlockStatus / PlanEntry / ProgressSection
  composables/
    useAgentBlocks.ts         ← 任务看板树形构建（M69 P4 修复）
    useChatTimeline.ts        ← 时间线计算属性
    useTodoBoard.ts           ← TODO 看板域内 composable
    useToolCallTimeline.ts    ← 工具时间线排序 + 折叠态共享
  lib/
    isStuckTool.ts            ← error_code === 'tool_timeout' 判断
    executionCardHelpers.ts   ← ChatExecutionCard 折叠控制 signal + 5s 计时器
    detectCodeLanguage.ts     ← highlight.js 语言自动检测（TK-05）

web/src/stores/
  spirit/index.ts             ← useSpiritTeamStore
  uiConfig/index.ts           ← useUiConfigStore（TK-04 工具显示开关）

web/src/composables/chat/
  useAutoCollapse.ts          ← OBS-01 自动折叠
  useContextualLoadingMessage.ts ← OBS-02 语境加载消息
  useStatusPulse.ts           ← OBS-05 侧边栏脉冲

web/src/components/spirit/
  SpiritEntry.vue             ← 精灵入口卡片
  ThinkingArea.vue            ← v7 思考区域（脑纹SVG+流光+半透明span+折叠按钮）
  UnifiedExecutionPanel.vue   ← v7 统一执行面板（任务拆解+依赖关系+团队进度 单卡片纵向分区）
  TeamTaskCard.vue            ← 团队任务卡片
  TeamProgressCard.vue        ← 团队进度卡片
  TeamAssemblyCard.vue        ← 团队组建卡片
  TaskExecutionPanel.vue      ← 任务执行面板（集成所有子组件 + TaskBoard）
  ParallelTeamOverview.vue    ← 并行团队概览
  DAGDiagramCard.vue          ← DAG 依赖图
  SynthesisResultCard.vue     ← 综合结果卡片
  OrchestrationModeBadge.vue  ← 编排模式徽章
  AgentStatusLabel.vue        ← Agent 状态标签（含 tool_blocked）
  SpiritStatusBar.vue         ← 底部状态栏
  InterruptedTeamCard.vue     ← 中断恢复提示卡片
  TeamMemberTreeNode.vue      ← 成员树形节点
  MemberReadOnlyPanel.vue     ← 成员只读面板
  ToolStuckBadge.vue          ← stuck 工具徽章

web/src/components/chat/
  ChatExecutionCard.vue       ← 工具执行卡片（增强折叠 + 5s 计时）
  ChatMessagePanel.vue        ← 三模式 + SpiritStatusBar + 语境加载消息 + TaskBoard
  ChatEntitySidebar.vue       ← 精灵模式重构 + useStatusPulse
  ChatMessageList.vue         ← 集成 TaskBoard 树形嵌套渲染
  TurnBlock.vue               ← 渲染 Agent Block Header（含 hasPartialFailure 徽章）
  ChatReasoningPeek.vue       ← 思考预览（脉冲+光标闪烁）
  TaskBoard.vue               ← 任务看板壳（递归渲染）— 新增
  TaskBoardNode.vue           ← 任务看板单个节点 — 新增
  TodoKanbanBoard.vue         ← TODO 三列任务看板
  TodoColumn.vue              ← TODO 单列
  TodoCard.vue                ← TODO 单卡
  ToolCallTimeline.vue        ← 工具调用纵向时间线
  ToolCallTimelineItem.vue    ← 时间线单个节点
  CodeBlock.vue               ← 代码块自动识别 + 高亮 + 复制 + 折叠（TK-05）
```

### 7.2 Store 设计

**useSpiritTeamStore**：

```typescript
interface SpiritTeamState {
  teams: SpiritTeam[]
  expandedTeamIds: Set<string>
  activePanelMode: SpiritPanelMode  // 'spirit' | 'team' | 'member'
  activeTeamId: string | null
  activeMemberId: string | null
  loading: boolean
  parallelConfig: ParallelConfig
  synthesisResult: SynthesisResult | null
}

interface SpiritTeam {
  id: string
  teamName: string
  taskSummary: string
  status: SpiritTeamStatus
  mode: SpiritTeamMode
  memberAvatars: string[]
  completedSteps: number
  totalSteps: number
  durationMs: number
  spiritSessionId: string
  teamSessionId: string
  members: SpiritMember[]
  sharedAgentIds: string[]
  dagNodeId?: string
  dependsOn?: string[]
  topologyReason?: string
}

interface SpiritMember {
  agentId: string
  agentKey: string
  displayName: string
  role: string
  status: 'idle' | 'working' | 'waiting' | 'completed' | 'failed'
  avatarUrl: string
}
```

核心 actions：`loadSpiritTeams` / `reloadTeams` / `selectTeam` / `selectMember` / `returnToSpirit` / `toggleTeamExpand` / `cancelTeam` / `updateTeamProgress` / `updateTeamStatus` / `addTeam` / `handleSpiritEnvelope` / `checkTeamProgress` / `synthesizeResults`。

### 7.3 ChatMessagePanel 三模式

| 模式 | 组件 | 输入框 | WS 连接 |
|------|------|--------|---------|
| `spirit` | 标准 ChatMessagePanel + ContextualLoadingMessage + SpiritStatusBar + TodoKanbanBoard | 显示 | Spirit Session WS |
| `team` | TaskExecutionPanel（集成 ParallelTeamOverview + AgentStatusLabel + InterruptedTeamCard + TaskBoard） | 隐藏 | Team Session WS |
| `member` | MemberReadOnlyPanel + TaskBoard | 隐藏 | 复用 Team Session WS（过滤） |

### 7.4 WS 回放兼容

所有 L1 环境层方案（OBS-02 语境消息、OBS-05 脉冲）需在 WS 回放期间静默，统一通过 `isReplaying` ref 控制。

---

## 八、API 扩展

### 8.1 Session Proto

```protobuf
message Session {
  string parent_session_id = 50;
  string root_session_id = 51;
  int32 agent_depth = 52;
}

message ListChildSessionsRequest {
  string parent_session_id = 1;
}
```

### 8.2 Team Proto

```protobuf
message Team {
  string spirit_session_id = 15;
  string task_description = 16;
  bool auto_created = 17;
  string dag_node_id = 18;
  repeated string depends_on = 19;
  string parallel_config_json = 20;
  bool readonly = 21;
  string source = 22;
  string kind = 23;
}
```

### 8.3 精灵团队查询 API

```protobuf
message ListSpiritTeamsRequest {
  string spirit_session_id = 1;
  repeated string status_filter = 2;
}

message SpiritTeamView {
  string team_id = 1;
  string team_name = 2;
  string task_summary = 3;
  string status = 4;
  string mode = 5;
  int32 completed_steps = 6;
  int32 total_steps = 7;
  string team_session_id = 8;
  repeated SpiritMemberView members = 9;
  repeated string shared_agent_ids = 10;
  string dag_node_id = 11;
  repeated string depends_on = 12;
  string topology_reason = 13;
  int64 duration_ms = 14;
}
```

---

## 九、D5: useAgentBlocks 业务逻辑修复设计（2026-06-10）

> **触发**：用户报告 CHAT UI 最终回复重复展示 → 静态代码审查 `useAgentBlocks.ts`
> **范围**：M59/M69 整合后时间线展示层（`web/src/features/chat/composables/useAgentBlocks.ts`）

### 9.1 D5.1 修复 SubAgentBuilder 状态机（AC-17、AC-18）

**问题**：
- `addTool` 未推入 `allToolMsgs`，导致 `allToolsDone` 恒为 `true`
- `tool_blocked` 状态被合并入 `running`，与"等用户确认"语义混淆

**修复**：

```typescript
// useAgentBlocks.ts - SubAgentBuilder
addTool(msg: Message, toolEv: ToolUseEvent): void {
  this.allToolMsgs.push(msg);  // 🟢 维护工具消息引用
  this.entries.push({
    kind: 'tool',
    section: buildToolSection(toolEv, `sub-${this.agentKey}-tool`),
    sortKey: this.sortCounter++,
  });
}

build(): AgentBlock {
  // 🟢 明确区分 streaming / tool_running / tool_blocked
  const isStreaming = this.allMemberMsgs.some((m) => m.status === 'streaming');
  const hasRunningTool = this.allToolMsgs.some((m) => {
    const ev = toolEventFromMessage(m);
    return ev?.status === 'running';
  });
  const hasBlockedTool = this.allToolMsgs.some((m) => {
    const ev = toolEventFromMessage(m);
    return ev?.status === 'blocked';
  });
  const allToolsDone = this.allToolMsgs.every((m) => {
    const ev = toolEventFromMessage(m);
    return !ev || ev.status === 'success' || ev.status === 'failed' || ev.status === 'cancelled';
  });

  let status: AgentBlockStatus;
  if (hasBlockedTool) status = 'tool_blocked';
  else if (isStreaming || hasRunningTool) status = 'running';
  else if (allToolsDone && hasMemberContent) status = 'completed';
  else status = 'running';
  // ...
}
```

**类型扩展**（`agentTreeTypes.ts`）：

```typescript
export type AgentBlockStatus =
  | 'running'
  | 'tool_running'
  | 'tool_blocked'
  | 'completed'
  | 'failed'
  | 'partial_failure'
  | 'cancelled'
```

**UI 联动**：`ChatExecutionCard.vue` 与 `TurnBlock.vue` 渲染 `tool_blocked` 时显示"🟡 等待您的输入"徽章。

### 9.2 D5.2 修复 PlanCard 状态机（AC-19）

```typescript
function resolvePlanStatus(
  planStatus: OrchestrationPlan['status'],
  agentStatus: AgentBlockStatus,
  planEntriesCount: number,  // 🟢 新增参数
): OrchestrationPlan['status'] {
  if (planStatus === 'executing') {
    if (agentStatus === 'completed') return 'completed';
    if (agentStatus === 'failed') return 'failed';
    return 'executing';
  }
  if (planStatus === 'planning') {
    if (agentStatus === 'running' && planEntriesCount > 0) return 'executing';
    if (agentStatus === 'completed' || agentStatus === 'failed') return agentStatus;
  }
  return planStatus;
}
```

### 9.3 D5.3 钳制 progress sortKey（AC-20）

```typescript
const offset = (section.startedAt - turnStartTs) / 1000;
const clamped = Math.max(0, offset);  // 🟢 钳制
const key = clamped - 0.5 + progressSortBase * 1e-6;
```

### 9.4 D5.4 Reply 去重与 resolveReplyContent 对齐（AC-21）

```typescript
const replyContent = resolveReplyContent(messagePresentation);
if (replyContent) {
  // 🟢 基于 presentation.mode 的条件去重
  const shouldDedupe =
    messagePresentation.mode !== 'react' &&
    replyContent === messagePresentation.reasoning?.trim();
  if (!shouldDedupe) {
    timeline.push({ kind: 'reply', ... });
  }
}
```

### 9.5 D5.5 Plan entry 匹配改用 agentKey（AC-22）

```typescript
// agentTreeTypes.ts - PlanEntry 新增字段
export interface PlanEntry {
  id: string;
  task: string;
  agentKey: string;        // 🟢 强匹配键
  agentName: string | null;
  // ...
}

// useAgentBlocks.ts - updatePlanEntryStatuses
function updatePlanEntryStatuses(entries: PlanEntry[], timeline: TimelineEntry[]): PlanEntry[] {
  return entries.map((entry) => {
    const matchingBlock = timeline.find(
      (t) => t.kind === 'subagent' && t.block.agentKey === entry.agentKey,
    );
    // ...
  });
}
```

### 9.6 D5.6 已完成回合默认展开（AC-23）

```typescript
// useAgentBlocks.ts#L575 / #L731
collapsed: false,  // 🟢 已完成回合默认展开
```

### 9.7 D5.7 暴露 hasPartialFailure 字段（AC-24）

```typescript
// agentTreeTypes.ts - AgentBlock 新增字段
export interface AgentBlock {
  // ...
  hasPartialFailure?: boolean;
}

// useAgentBlocks.ts - buildRootAgentBlock
const hasFailedTool = msgs.some((m) => {
  const ev = toolEventFromMessage(m);
  return ev?.status === 'failed';
});
const hasSuccessfulResult = assistant?.content_markdown?.trim() || assistant?.reasoning_markdown?.trim();
const hasPartialFailure = hasFailedTool && !!hasSuccessfulResult;
```

**UI 联动**：`TurnBlock.vue` 渲染 `hasPartialFailure === true` 时显示"⚠️ 部分工具失败"徽章。

### 9.8 D5.8 progress section 移到 turn 头部（AC-25）

```typescript
// agentTreeTypes.ts
export interface AgentBlock {
  // ...
  progressSections: ProgressSection[];  // 🟢 从 timeline 拆出
  timeline: TimelineEntry[];            // 不再含 kind: 'progress'
}
```

**UI 联动**：`ChatMessageList.vue` 在 user 消息行后渲染 `block.progressSections`。

### 9.9 数据流验证

```
useAgentBlocks(...)
  ├── classifyMessages → events
  ├── buildRootAgentBlock
  │     ├── timeline  ← 不含 progress
  │     ├── progressSections  ← 独立字段
  │     ├── result  ← 降级为兼容字段
  │     ├── hasPartialFailure  ← 新增
  │     └── status  ← 增加 tool_blocked / tool_running 显式状态
  └── 返回 AgentBlock[]

ChatMessageList
  ├── 头部：渲染 progressSections
  └── 主 timeline：含 kind: 'reply' 作为最终答案
    └── TaskBoard 树形嵌套渲染（任务-思考-工具-回复 统一结构）
```

### 9.10 文档边界

| 项 | 在 M59/M69 整合范围 | 不在范围 |
|----|-------------|---------|
| `AgentBlock.result` 降级为兼容字段 | ✅ | — |
| `SynthesisResultCard` 去重（与 timeline reply 互斥） | — | ❌ M59 拥有 |
| `TaskBoardNode.vue` 渲染 hasPartialFailure / tool_blocked 徽章 | ✅ | — |
| 合成卡片 UI 调整 | — | ❌ M59 拥有 |

---

## 十、第五轮审查修复记录（M69 useAgentBlocks）

> **日期**：2026-06-10 | **触发**：用户反馈 CHAT UI 最终回复重复 → 静态代码审查

| ID | 问题 | 根因 | 修复 | 影响文件 |
|----|------|------|------|----------|
| F-13 | 子代理工具未完成时状态被误判为 `completed` | `SubAgentBuilder.addTool` 未维护 `allToolMsgs` | `addTool(msg, toolEv)` 签名扩展 | `useAgentBlocks.ts` |
| F-14 | `tool_blocked` 状态被合并入 `running` | `computeAgentStatus` 仅识别 `running/streaming` | 状态机扩展 `tool_blocked` | `useAgentBlocks.ts` + `agentTreeTypes.ts` + `ChatExecutionCard.vue` |
| F-15 | `subagents_spawn` 路径下 PlanCard 永远停在"规划中" | `resolvePlanStatus` 漏 `running` 转换分支 | 补 `(planning && running && planEntries > 0)` → `'executing'` | `useAgentBlocks.ts` |
| F-16 | progress 卡片可能插到 user 消息之前 | `sortKey = offset - 0.5` 时钟漂移为负 | 钳制 `Math.max(0, offset) - 0.5` | `useAgentBlocks.ts` |
| F-17 | ReAct 模式下 reply 与最后 thinking 内容重复 | reply 去重仅在非 ReAct 模式生效 | 改为基于 `presentation.mode` 条件去重 | `useAgentBlocks.ts` |
| F-18 | 多子代理相似任务时 PlanCard 状态错配 | 匹配用 `agentName || task` | 改用 `agentKey` 强键 | `useAgentBlocks.ts` + `agentTreeTypes.ts` |
| F-19 | 已完成回合默认折叠，最终答案不可达 | `collapsed: status === 'completed'` | 改为 `collapsed: false` | `useAgentBlocks.ts` |
| F-20 | 部分工具失败被掩盖为 `completed` | 无信号输出 | AgentBlock 新增 `hasPartialFailure` 字段 | `useAgentBlocks.ts` + `agentTreeTypes.ts` + `TurnBlock.vue` |
| F-21 | progress section 与 timeline 主线混排 | progress 走 `timeline.sort()` 插入任意位置 | 拆出 `progressSections` 独立字段 | `useAgentBlocks.ts` + `agentTreeTypes.ts` + `ChatMessageList.vue` |

---

## 十一、UI 原型对齐优化（M69 P3）

| ID | 优化 | 文件 |
|----|------|------|
| T-15 | TurnBlock 添加 Agent Block Header（头像+名称+状态徽章+耗时+子任务数） | `TurnBlock.vue` |
| T-16 | ChatExecutionCard 添加 agent 首字头像 | `ChatExecutionCard.vue` |
| T-17 | 运行中耗时添加 `...` 后缀 | `ChatExecutionCard.vue` |
| T-18 | ChatReasoningPeek 添加脉冲圆点和光标闪烁指示器 | `ChatReasoningPeek.vue` |
| T-19 | ChatExecutionCard running 状态添加脉冲圆点 | `ChatExecutionCard.vue` |
| T-20 | 全局展开/折叠按钮始终可见 | `ChatMessagePanel.vue` |
| T-21 | Sub-Agent 嵌套缩进（左边框线+缩进） | `TurnBlock.vue` |
| T-22 | 执行结果区段标签 | `TurnBlock.vue` |

---

## 十二、测试策略

| 层 | 文件 | 覆盖 | 阶段 |
|----|------|------|------|
| Biz | `session_tree_test.go` | Session 树查询、深度限制 | P0 |
| Biz | `team_spirit_test.go` | AutoCreated Team 创建、SpiritSessionID 关联 | P0 |
| Biz | `spirit_team_usecase_test.go` | AssembleTeam / CancelTeam / AutoArchive / DAG 拓扑路由 | P0 |
| Biz | `task_planner_test.go` | Plan 阶段逻辑 | P0.5 |
| Biz | `agent_allocator_test.go` | Allocate 阶段逻辑 | P0.5 |
| Biz | `task_orchestrator_test.go` | Orchestrate 阶段逻辑 | P0.5 |
| Biz | `spirit_parallel_config_test.go` | ParallelConfig 默认值、校验 | P1 |
| Biz | `spirit_task_dag_test.go` | DAG 校验、环检测、拓扑路由 | P2 |
| Biz | `spirit_synthesis_test.go` | 合成策略（模板/LLM/混合） | P2 |
| Biz | `orchestration_cache_test.go` | 缓存命中、DQ Score 阈值 | P2 |
| Service | `spirit_team_test.go` | AssembleTeam 流程、Envelope 发射 | P0 |
| Service | `team_turn_hooks_test.go` | Team Turn 完成回调 | P0 |
| 前端 | `useSpiritTeamStore.spec.ts` | 团队列表加载、面板切换、事件处理 | P0 |
| 前端 | `TaskExecutionPanel.spec.ts` | 三区布局、WS 实时更新 | P0 |
| 前端 | `useAgentBlocks.test.ts` | 8 项业务逻辑修复（F-13~F-21） | M69 P4 |
| 前端 | `TaskBoardNode.spec.ts` | 树形嵌套渲染 | M69 P1 |

E2E：SP-E2E-01（精灵对话 → 组建团队 → 查看执行面板 → 下钻子 agent 子任务看板 → 返回精灵）。

---

## 十三、关联模块

| 模块 | 关系 |
|------|------|
| 1 Chat | 精灵对话面板、团队组建卡片、任务执行面板、任务看板树形嵌套 |
| 11 Team | 精灵自动创建 Team、TeamRun 状态追踪、TeamKey UUID、依赖调度 |
| 53 Orchestration | Agent 节点状态投影、执行时间线、Task DAG 拓扑路由 |
| 10 Session | Session 树状关联、深度限制（看板嵌套层级） |
| 7 Agent Evolution | DQ Score 驱动编排缓存、进化闭环 |
| 23 Tools | 工具调用结果、stuck 工具检测 |
| 39 Planner | 远期：A2UI Planner 生成结构化执行计划 |
| superpowers Builtin Agents | 精灵/编排管家定义、三阶段编排工具 |
