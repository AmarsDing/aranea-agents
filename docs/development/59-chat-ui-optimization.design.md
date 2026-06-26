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
  features/chat/                      ← Chat 域（types / agentTreeTypes / activityTypes / activityTimelineTypes / useActivityTimeline / useTodoBoard 等）
  features/chat/composables/          ← useContextualLoadingMessage / useStatusPulse 等（useConversationTimeline 已删除，由 useActivityTimeline 替代）
  features/session/                   ← Session 域（types.ts 含 SessionTreeNode 递归类型 / api.ts 含 GetSessionTree RPC）
  stores/spirit/                      ← useSpiritTeamStore
  components/spirit/                  ← 精灵专用组件（17 个）
  components/chat/                    ← Chat 面板扩展（ActivityStream / ThinkingBlock / ActionBlock / ReplyBlock / ChatExecutionCard / SessionTreeSidebar / SessionTreeNode / TeamStageBlock / GraphStageBlock 等）
```

> **架构变更（2026-06-27 更新）**：原 `ConversationTurn.vue` + `useConversationTimeline.ts` 已删除，由 `ActivityStream.vue` + `useActivityTimeline.ts` 替代（三模式统一渲染器）。旧 `TaskExecutionPanel.vue` / `MemberReadOnlyPanel.vue` / `TeamPanel.vue` / `OrchestrationTimeline.vue` 已删除。详见 §14.9（ActivityStream 统一渲染器设计）、§14.12（Session 树 UI 递归设计）、§14.13（TeamStageBlock / GraphStageBlock 设计）。

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
| `internal/event` | 扩展 | `team_stage`/`graph_stage`/`session`/`plan`/`notice` 等 ActivityKind + 7 种 ActivityEventType（替代旧 15+ EnvelopeType）；`ActivityEventBus`（biz.ActivityEvent）+ `MonitorEventBus`（contract.MonitorEvent），legacy Envelope Bus / SessionBus / MonitorBus 全部删除 |
| `internal/agent` | 新增 | `tool_category.go` — ToolCategorizer（10 种 ToolCategory：shell/browser/file_read/file_write/file_search/web_search/mcp/code/todo/other），注册表查询 + 前缀匹配兜底 |
| `internal/data/ent/schema/team` | 扩展 | spirit_session_id 索引 |
| `internal/data/ent/schema/activity` | 扩展 | 新增 `tool_category`/`stage`/`session_type`/`member_agent_key`/`execution_stage`/`completed_steps`/`total_steps`/`progress_pct` 字段；删除 `role`/`tool_icon`（`child_board_id` 保留） |
| `web/src/features/spirit` | 新增 | 类型、API、UI 工具函数、可观测性常量 |
| `web/src/features/chat` | 新增 | useActivityTimeline / useTodoBoard / agentTreeTypes / activityTypes / activityTimelineTypes |
| `web/src/features/chat/composables` | 新增 | useContextualLoadingMessage / useStatusPulse（useConversationTimeline 已删除） |
| `web/src/features/session` | 新增 | types.ts（SessionTreeNode 递归类型）/ api.ts（GetSessionTree RPC） |
| `web/src/stores/spirit` | 新增 | useSpiritTeamStore |
| `web/src/components/spirit` | 新增 | 17 个新组件 |
| `web/src/components/chat` | 修改 | ChatEntitySidebar 重构；ActivityStream 三模式统一渲染器（替代 ChatMessageList + TaskExecutionPanel + MemberReadOnlyPanel）；SessionTreeSidebar + SessionTreeNode 递归；TeamStageBlock / GraphStageBlock / ActionBlock（按 tool_category 分发 10 详情组件） |
| `api/kratos/session/v1` | 扩展 | Session Proto 字段（含 `session_type`/`member_agent_key`/`member_role`/`execution_stage`/`completed_steps`/`total_steps`/`progress_pct`，编号 53-59） |
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

> **架构变更（2026-06-27 更新）**：本节原列的 15+ 种 `EnvelopeType` 已**全部废弃并删除**。Envelope 结构体已删除（后端 `contract/envelope.go` + 前端 `realtime/envelope.ts` 均已删除，详见 ADR-03 Blocker G）。所有事件统一为 **10 种 ActivityKind** + **7 种 ActivityEventType**，通过 `ActivityEventBus`（biz.ActivityEvent）+ `MonitorEventBus`（contract.MonitorEvent）传输。WS 协议传输 `activity_event?` + `monitor_event?`（`envelope?` 已删除）。完整映射表见 §14.8（原 EnvelopeType → ActivityKind 映射）。

### 4.1 ActivityKind + ActivityEventType（替代旧 EnvelopeType）

> **完整定义见** §14.7（ActivityEvent 类型设计）。

**ActivityKind（10 种，无 error kind）**：
- 基础交互：`task` / `thinking` / `action` / `reply` / `plan` / `confirm` / `notice`
- Session 生命周期：`session`（合并 `session_created`/`session_status`/`session_completed`）
- Team/Graph 阶段：`team_stage`（合并 `spirit_team_*` / `team_run_*` / `team_step_*` / `orchestration_agent_status`）/ `graph_stage`（合并 `graph_node_*` / `graph_step` 等）

**ActivityEventType（7 种业务语义事件）**：
- `created` — Activity 创建（新增 Block 组件）
- `streaming` — 流式追加（替代技术术语 "delta"，追加文本）
- `updated` — 状态变更（非流式，更新 stage/progress/成员列表）
- `completed` — 正常完成
- `failed` — 失败（独立事件，非 completed + status=failed）
- `cancelled` — 取消（用户主动停止）
- `child_created` — 子 Activity 创建（在父 Block 下新增子 Block）

**原 EnvelopeType → ActivityKind 映射**（节选，完整表见 §14.14）：

| 原 EnvelopeType | ActivityKind | ActivityEventType |
|----------------|-------------|------------------|
| `spirit_team_assembled` | `team_stage` | `created`（stage=assembled） |
| `spirit_team_completed` | `team_stage` | `completed`（stage=completed） |
| `spirit_team_failed`/`interrupted` | `team_stage` | `failed`/`cancelled` |
| `spirit_team_progress` | `team_stage` | `updated`（meta.progress） |
| `spirit_plan_created` | `plan` | `created` |
| `spirit_allocation_created` | `notice` | `created`（meta.allocation） |
| `spirit_orchestration_started`/`checkpoint`/`interrupted` | `notice` | `updated`（meta.phase） |
| `spirit_synthesis_completed` | `reply` | `completed`（meta.synthesis） |
| `team_run_started`/`finished`/`failed` | `team_stage` | `created`/`completed`/`failed` |
| `team_step_started`/`finished`/`summary` | `team_stage` | `updated` |
| `member_message_start`/`delta`/`done` | `reply` | `created`/`streaming`/`completed` |
| `orchestration_agent_status` | `team_stage` | `updated`（meta.member_status） |
| `graph_node_start`/`end`/`error`/`custom` | `graph_stage` | `created`/`completed`/`failed`/`updated` |
| `error` | 对应 kind | `failed`（如 action+failed / team_stage+failed） |
| `text_delta`/`text_done` | `reply` | `streaming`/`completed` |
| `tool_call`/`tool_result` | `action` | `created`/`completed` |
| `butler_*`/`skill_*`/`borrow_*` | 删除 | 未使用 |

### 4.2 事件载荷（已废弃 — 保留作历史参考）

> **废弃说明（2026-06-27）**：以下载荷为旧 Envelope 格式，已由 `ActivityEvent` 替代。新代码请参考 §14.7（ActivityEvent 类型设计）。

**spirit_team_completed**（旧格式，现由 `ActivityEvent(event=completed, kind=team_stage, stage=completed)` 替代）：
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

**spirit_team_progress**（旧格式，现由 `ActivityEvent(event=updated, kind=team_stage, meta.changed_fields=progress)` 替代）：
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

### 4.3 复用现有事件（已废弃 — 已合并到 ActivityKind）

> **废弃说明（2026-06-27）**：以下事件已全部合并到对应 ActivityKind，详见 §4.1 映射表。

| 旧 EnvelopeType | 合并到 ActivityKind | 精灵模式用途 |
|-------------|------|-------------|
| `team_step_started`/`finished` | `team_stage`（`updated`） | 成员开始/完成执行 → 更新成员状态 |
| `member_message_start/delta/done` | `reply`（`created`/`streaming`/`completed`） | 成员消息流 → ActivityStream |
| `session.status_changed` | `session`（`updated`） | 团队 Session 状态 |
| `orchestration_agent_status` | `team_stage`（`updated`，meta.member_status） | Agent 节点实时状态 |
| `team_run_started/finished/failed` | `team_stage`（`created`/`completed`/`failed`） | 团队 Run 生命周期 |
| `tool_call` / `tool_result` | `action`（`created`/`completed`） | Agent 级工具调用 → ActionBlock |

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

**定义文件**：`web/src/features/chat/agentTreeTypes.ts`（TaskBoardNodeKind / NodeStatus / TaskBoardNode）、`web/src/features/chat/activityTypes.ts`（ActivityKind / ActivityStatus / Activity）

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

**核心 composable**：`web/src/features/chat/composables/useActivityTimeline.ts`（Activity-First 架构，替代原 `useAgentBlocks.ts`）

> **架构演进说明**：原 `useAgentBlocks.ts` 采用 Message-First 模型，需 13 层推理从消息恢复语义。Activity-First 架构（§十四）后，由 `useActivityTimeline.ts` 直接消费 Activity 事件，零推理构建 TaskBoardNode 树。以下代码示例保留原设计意图作为参考。

```typescript
// useActivityTimeline.ts（Activity-First 架构）
export function useActivityTimeline(deps: {
  activities: ComputedRef<Activity[]>
}) {
  const activityTree = computed<Activity[]>(() => buildActivityTree(deps.activities.value))
  const taskBoardNodes = computed<TaskBoardNode[]>(() => mapActivitiesToNodes(activityTree.value))
  return { activityTree, taskBoardNodes }
}

// 原 useAgentBlocks.ts 设计意图（已由 Activity-First 替代）：
// 1. 构建 timeline（不含 progress）
// 2. 提取 progressSections（独立字段，渲染在 turn 头部）
// 3. 计算 status（含 tool_blocked / tool_running 显式状态）
// 4. 计算 hasPartialFailure
// 5. 递归构建 childBlocks（从 subagents_spawn tool 提取子 agent session）
// 6. collapsed: false（已完成回合默认展开，修复 F-19）
```

#### 6.2.5 组件结构

**Activity-First 组件**（替代原 TaskBoardNode.vue / TaskBoard.vue）：
- `web/src/components/chat/ConversationTurn.vue` — 单个对话回合（替代原 TurnBlock.vue）
- `web/src/components/chat/ThinkingBlock.vue` — 思考节点（替代原 ChatReasoningPeek.vue）
- `web/src/components/chat/ActionBlock.vue` — 工具调用节点（替代原 ToolCallTimeline/ToolCallTimelineItem.vue）
- `web/src/components/chat/ReplyBlock.vue` — 回复节点
- `web/src/components/chat/PlanBlock.vue` — 计划节点
- `web/src/components/chat/NoticeBlock.vue` — 通知节点
- `web/src/components/chat/ErrorBlock.vue` — 错误节点
- `web/src/components/chat/ConfirmBlock.vue` — 确认节点
- `web/src/components/chat/TaskBoardSection.vue` — 任务看板区段
- `web/src/components/chat/DelegateActivity.vue` — 委派活动节点

> **架构演进说明**：原设计采用 `TaskBoardNode.vue`（单节点）+ `TaskBoard.vue`（递归壳）的双组件结构。Activity-First 架构后，按 ActivityKind 拆分为独立的 Block 组件，由 `ConversationTurn.vue` 统一组装。以下代码示例保留原设计意图作为参考。

```vue
<!-- 原 TaskBoardNode.vue 设计意图（已由 Activity-First Block 组件替代） -->
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
<!-- 原 TaskBoard.vue 设计意图（已由 ConversationTurn.vue + Activity-First 替代） -->
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

> **Activity-First 实现**：嵌套深度控制现由 `useActivityTimeline.ts` 中的 `buildActivityTree` 函数实现，通过 `Activity.parentActivityId` 构建树形结构，深度受 `MaxSessionDepth=2` 约束。`ConversationTurn.vue` 递归渲染子活动。

#### 6.2.7 状态机（M69 P4 修复，Activity-First 后由 useActivityTimeline 实现）

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
- 消费方：`ChatMessageList.vue` / `ConversationTurn.vue` / `SpiritStatusBar.vue` 读取后条件渲染
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

#### 6.7.4 Markdown 集成

```typescript
// web/src/features/chat/chatMessageMarkdown.ts（Markdown 渲染工具，替代原 MarkdownView.vue 设计）
import CodeBlock from '../../components/chat/CodeBlock.vue'

// ~~方案 A（已否决）：markdown-it renderer override~~
// 问题：v-html 渲染后无法挂载 Vue 组件，只能做静态高亮

// 方案 B（采纳）：自定义组件渲染
// 在 Markdown 渲染流程中，识别 <pre><code class="language-xxx"> 后
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
<!-- ThinkingBlock.vue - kind === 'thinking' 完成态（原 TaskBoardNode.vue 设计意图） -->
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

### 6.9 虚拟滚动（T8.3，2026-06-18 新增）

> **需求来源**：[2026-06-18-review-full-message-chain-and-solutions.md](../../reports/2026-06-18-review-full-message-chain-and-solutions.md) Sprint 8 / T8.3
> **目标**：长会话（>100 回合）启用虚拟滚动，避免 DOM 节点过多导致卡顿；同时保留短会话的简单 v-for 渲染路径。

#### 6.9.1 启用阈值与组件选型

| 维度 | 决策 | 理由 |
|------|------|------|
| 启用阈值 | `VIRTUAL_SCROLL_THRESHOLD = 100`（回合数） | 短会话保留原生 v-for 路径，避免虚拟滚动开销；长会话才需要回收 |
| 组件 | `vue-virtual-scroller` 的 `DynamicScroller` + `DynamicScrollerItem` | 支持动态高度（回合内容长度差异大），无需手动测量 |
| `min-item-size` | `80`px | 回合最小高度兜底，DynamicScroller 据此预分配空间 |
| `key-field` | `id`（ConversationTurn.id） | 稳定 key，避免回收后状态错乱 |
| 样式导入 | `vue-virtual-scroller/dist/vue-virtual-scroller.css` | 实际样式文件位于 dist 目录 |

#### 6.9.2 数据流与滚动管理

```
ChatMessagePanel
  ├─ messageListRef → ChatMessageList
  │   ├─ useVirtualScroll = computed(() => conversationTurns.length > 100)
  │   ├─ virtualScrollRef = ref<DynamicScroller>(null)  ← defineExpose
  │   ├─ scrollToTurnId(turnId)                          ← defineExpose
  │   └─ getScrollTarget()                               ← defineExpose
  ├─ useChatScrollTitle({ virtualScrollRef, useVirtualMessageList })
  │   └─ resolveScrollRoot() 优先取 virtualScrollRef.$el，否则取 messagesScrollEl
  └─ useChatMessageScroll({ messagesScrollEl })
      └─ scrollToTurnId() 通过 querySelector + scrollIntoView 高亮目标回合
```

**focusTurnId 处理流程**（虚拟滚动模式下）：

1. `ChatMessagePanel` watch `focusTurnId` 变化
2. 若 `useVirtualScroll` 为 true，先调用 `messageListRef.scrollToTurnId(turnId)`
   - `ChatMessageList.scrollToTurnId` 调用 `virtualScrollRef.scrollToItem(index)` 将离屏回合滚入视口
   - `await nextTick()` 等待 DOM 更新
3. 再调用 `useChatMessageScroll.scrollToTurnId(turnId)` 通过 `querySelector('[data-turn-id]')` 高亮目标
4. emit `focus-turn-cleared`

**关键设计决策**：

- **DynamicScrollerItem 的 `data-index`**：使用 slot prop `itemIndex`（数字），**不**使用 `item.id`（字符串）。`data-index` 是 DynamicScroller 用于内部定位的索引，必须是数字。
- **`getScrollTarget` 返回值**：虚拟滚动模式下返回 `virtualScrollRef.$el`（DynamicScroller 的根滚动容器），供 `useChatScrollTitle` 和 `useChatMessageScroll` 计算 viewport 位置。
- **ref 变量名避免遮蔽**：`getScrollTarget` 内部使用 `const vsRef = virtualScrollRef.value`，避免遮蔽 Vue 的 `ref` API。

#### 6.9.3 影响文件

| 文件 | 变更 |
|------|------|
| `web/src/components/chat/ChatMessageList.vue` | 新增 `DynamicScroller` / `DynamicScrollerItem` 导入；新增 `VIRTUAL_SCROLL_THRESHOLD`、`useVirtualScroll`、`virtualScrollRef`、`scrollToTurnId`；`defineExpose` 扩展；`getScrollTarget` 适配虚拟滚动 |
| `web/src/components/chat/ChatMessagePanel.vue` | `useChatScrollTitle` 参数适配（`virtualScrollRef` / `useVirtualMessageList` 从 `messageListRef` 计算）；`focusTurnId` watch 先调用 `messageListRef.scrollToTurnId` |
| `web/src/features/chat/useChatScrollTitle.ts` | 新增 `VirtualScrollInstance` 类型；新增 `resolveScrollRoot` 函数优先取虚拟滚动根元素 |

### 6.10 大消息折叠（T8.4，2026-06-18 新增）

> **需求来源**：[2026-06-18-review-full-message-chain-and-solutions.md](../../reports/2026-06-18-review-full-message-chain-and-solutions.md) Sprint 8 / T8.4
> **目标**：思考块和工具调用结果默认折叠，减少首屏高度；用户展开/折叠状态持久化到 sessionStorage，虚拟滚动回收后状态可恢复。

#### 6.10.1 useCollapseState composable

**位置**：`web/src/features/chat/composables/useCollapseState.ts`

**核心设计**：分离用户操作与系统操作的持久化策略，避免系统强制折叠覆盖用户偏好。

| 操作 | API | 持久化到 sessionStorage | 场景 |
|------|-----|------------------------|------|
| 用户点击展开/折叠 | `toggle()` | ✅ 是 | 用户主动操作，应被记住 |
| 系统强制折叠 | `setCollapsed(value)` | ❌ 否 | 流式开始/结束、内容超阈值自动折叠 |

**为什么分离**：

```
场景：用户手动展开了某个思考块（toggle → sessionStorage 记录 false）
      随后流式结束，ThinkingBlock 的 streaming watch 调用 setCollapsed(true)
      
若 setCollapsed 也持久化：
  → sessionStorage 被覆盖为 true
  → 用户刷新页面，思考块又变成折叠
  → 用户的展开偏好丢失

分离后：
  → setCollapsed(true) 只改当前 ref，不写 sessionStorage
  → 用户刷新页面，从 sessionStorage 读到 false（用户偏好）
  → 用户的展开偏好保留
```

**API 签名**：

```ts
export function useCollapseState(
  key: string,                    // sessionStorage key（前缀 'chat:collapse:'）
  defaultCollapsed: boolean = true,
): {
  collapsed: Ref<boolean>;
  toggle: () => void;             // 用户操作：切换 + 持久化
  setCollapsed: (v: boolean) => void;  // 系统操作：仅切换，不持久化
}
```

**存储 key 命名**：

| 组件 | key 格式 | 示例 |
|------|---------|------|
| ThinkingBlock | `chat:collapse:thinking:{messageId}` | `chat:collapse:thinking:msg-123` |
| ActionBlock | `chat:collapse:action:{activity.id}` | `chat:collapse:action:act-456` |

**降级策略**：sessionStorage 不可用（隐私模式、配额超限）时静默降级为内存 ref，不抛错。

#### 6.10.2 ThinkingBlock 默认折叠

**位置**：`web/src/components/chat/ThinkingBlock.vue`

| 场景 | 行为 | API |
|------|------|-----|
| 初始化 | 从 sessionStorage 读取，无记录则用 `defaultCollapsed`（默认 true） | `useCollapseState(key, props.defaultCollapsed)` |
| 流式开始 | 折叠（显示状态指示器） | `setCollapsed(true)` |
| 流式结束 | 保持折叠 | `setCollapsed(true)` |
| 外部 `defaultCollapsed` 变化 | 非流式时同步 | `setCollapsed(val)` |
| 用户点击 | 切换 + 持久化 | `toggle()` |
| ESC 键 | 折叠（不持久化） | `setCollapsed(true)` |

#### 6.10.3 ActionBlock 大消息自动折叠

**位置**：`web/src/components/chat/ActionBlock.vue`

| 阈值 | 行为 |
|------|------|
| `RESULT_COLLAPSE_THRESHOLD = 500` | `result.length + arguments.length > 500` 时默认折叠 |
| 初始化 | `defaultCollapsed = computed(() => contentLength.value > 500)` |
| 内容增长超阈值 | `watch(contentLength)` 检测从 ≤500 跨越到 >500，调用 `setCollapsed(true)` 自动折叠 |
| 用户点击 | `toggle()` 切换 + 持久化 |

**`contentLength` computed**：

```ts
const contentLength = computed(() => {
  const result = props.activity.tool.result ?? '';
  const args = props.activity.tool.arguments ?? '';
  return result.length + args.length;
});
```

**watch 自动折叠逻辑**：

```ts
watch(contentLength, (len, prevLen) => {
  if (len > RESULT_COLLAPSE_THRESHOLD && prevLen <= RESULT_COLLAPSE_THRESHOLD) {
    setCollapsed(true);  // 系统操作，不持久化
  }
});
```

#### 6.10.4 影响文件

| 文件 | 变更 |
|------|------|
| `web/src/features/chat/composables/useCollapseState.ts` | 新增 composable |
| `web/src/components/chat/ThinkingBlock.vue` | 替换本地 `collapsed` ref 为 `useCollapseState`；`onClick` 调用 `toggle()`；`onEscape` 调用 `setCollapsed(true)`；流式 watch 使用 `setCollapsed(true)` |
| `web/src/components/chat/ActionBlock.vue` | 新增 `RESULT_COLLAPSE_THRESHOLD`、`contentLength` computed、`defaultCollapsed` computed；集成 `useCollapseState`；新增 `watch(contentLength)` 自动折叠；`expanded` 改为 computed（`!collapsed`） |

#### 6.10.5 与虚拟滚动的协同

虚拟滚动回收组件后，重新挂载时会从 sessionStorage 读取折叠状态，确保用户偏好不丢失。这是 T8.3 与 T8.4 的核心协同点：

- **无 sessionStorage**：回收后状态丢失，用户需重新展开
- **有 sessionStorage + toggle 持久化**：回收后状态恢复，用户体验一致
- **setCollapsed 不持久化**：系统强制折叠不会污染用户偏好

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
  agentTreeTypes.ts           ← AgentBlock / AgentBlockStatus / PlanEntry / ProgressSection / TaskBoardNodeKind / NodeStatus / TaskBoardNode
  activityTypes.ts            ← ActivityKind / ActivityStatus / Activity（Activity-First 架构）
  activityTimelineTypes.ts    ← ConversationTurn / AgentWorkProcess / TeamPanel（Activity-First 消费层）
  chatMessageMarkdown.ts      ← Markdown 渲染（替代原 MarkdownView.vue）
  composables/
    useActivityTimeline.ts    ← Activity-First 树形构建（替代原 useAgentBlocks.ts）
    useConversationTimeline.ts ← 时间线计算属性（替代原 useChatTimeline.ts）
    useTodoBoard.ts           ← TODO 看板域内 composable
  lib/
    isStuckTool.ts            ← error_code === 'tool_timeout' 判断
    executionCardHelpers.ts   ← ChatExecutionCard 折叠控制 signal + 5s 计时器
    detectCodeLanguage.ts     ← highlight.js 语言自动检测（TK-05）

web/src/stores/
  spirit/index.ts             ← useSpiritTeamStore
  uiConfig/index.ts           ← useUiConfigStore（TK-04 工具显示开关）

web/src/features/chat/composables/
  useContextualLoadingMessage.ts ← OBS-02 语境加载消息
  useStatusPulse.ts           ← OBS-05 侧边栏脉冲
  useChatEntityCollapse.ts    ← OBS-01 自动折叠（替代原 useAutoCollapse.ts）

web/src/components/spirit/
  SpiritEntry.vue             ← 精灵入口卡片
  ThinkingArea.vue            ← v7 思考区域（脑纹SVG+流光+半透明span+折叠按钮）
  UnifiedExecutionPanel.vue   ← v7 统一执行面板（任务拆解+依赖关系+团队进度 单卡片纵向分区）
  TeamTaskCard.vue            ← 团队任务卡片
  TeamProgressCard.vue        ← 团队进度卡片
  TeamAssemblyCard.vue        ← 团队组建卡片
  TaskExecutionPanel.vue      ← 任务执行面板（集成所有子组件 + Activity-First 树）
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
  ChatExecutionCard.vue       ← 工具执行卡片（增强折叠 + 5s 计时 + autoCollapse prop）
  ChatMessagePanel.vue        ← 三模式 + SpiritStatusBar + 语境加载消息 + Activity-First 树
  ChatEntitySidebar.vue       ← 精灵模式重构 + useStatusPulse
  ChatMessageList.vue         ← 集成 Activity-First 树形嵌套渲染
  ConversationTurn.vue        ← 对话回合（替代原 TurnBlock.vue，含 hasPartialFailure 徽章）
  ThinkingBlock.vue           ← 思考节点（替代原 ChatReasoningPeek.vue，脉冲+光标闪烁）
  ActionBlock.vue             ← 工具调用节点（替代原 ToolCallTimeline/ToolCallTimelineItem.vue）
  ReplyBlock.vue              ← 回复节点
  PlanBlock.vue               ← 计划节点
  NoticeBlock.vue             ← 通知节点
  ErrorBlock.vue              ← 错误节点
  ConfirmBlock.vue            ← 确认节点
  TaskBoardSection.vue        ← 任务看板区段（替代原 TaskBoard.vue）
  DelegateActivity.vue        ← 委派活动节点
  TodoKanbanBoard.vue         ← TODO 三列任务看板
  TodoColumn.vue              ← TODO 单列
  TodoCard.vue                ← TODO 单卡
  CodeBlock.vue               ← 代码块自动识别 + 高亮 + 复制 + 折叠（TK-05）
  UiConfigToggle.vue          ← 工具显示开关（TK-04）
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

> **触发**：用户报告 CHAT UI 最终回复重复展示 → 静态代码审查原 `useAgentBlocks.ts`
> **范围**：M59/M69 整合后时间线展示层（原 `web/src/features/chat/composables/useAgentBlocks.ts`）
> **架构演进说明**：原 `useAgentBlocks.ts` 已由 Activity-First 架构的 `useActivityTimeline.ts` 替代（§十四）。以下设计保留原修复意图作为参考，对应的状态机逻辑已迁移到 Activity-First 消费层。

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

**UI 联动**：`ChatExecutionCard.vue` 与 `ConversationTurn.vue` 渲染 `tool_blocked` 时显示"🟡 等待您的输入"徽章。

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

**UI 联动**：`ConversationTurn.vue` 渲染 `hasPartialFailure === true` 时显示"⚠️ 部分工具失败"徽章。

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
| `ConversationTurn.vue` / Block 组件渲染 hasPartialFailure / tool_blocked 徽章 | ✅ | — |
| 合成卡片 UI 调整 | — | ❌ M59 拥有 |

---

## 十、第五轮审查修复记录（M69 useAgentBlocks）

> **文档边界说明**：本节原包含 F-13~F-21 问题清单（含根因、修复方案、影响文件），属于开发进度/修复记录，已迁移至开发计划文档。

> 详见 [59-chat-ui-optimization.development.md §8](./59-chat-ui-optimization.development.md#8-审查修复记录)（2026-06-11 v7 原型对齐审查修复记录 V7-R01~V7-R09，覆盖 F-13~F-21 完整修复详情）

---

## 十一、UI 原型对齐优化（M69 P3）

> **文档边界说明**：本节原包含 T-15~T-22 UI 优化任务清单（含文件路径），属于开发任务清单，已迁移至开发计划文档。

> 详见 [59-chat-ui-optimization.development.md §8](./59-chat-ui-optimization.development.md#8-审查修复记录)（UI 原型对齐优化 T-15~T-22 任务清单与状态）

---

## 十二、测试策略

> **文档边界说明**：本节描述测试分层策略（设计层面）。具体测试文件清单与阶段标记已迁移至开发计划文档。

**分层策略**：
- **Biz 层**：Session 树查询、Team 创建、三阶段编排（Plan/Allocate/Orchestrate）、ParallelConfig、TaskDAG、Synthesis、Orchestration Cache
- **Service 层**：AssembleTeam 流程、Envelope 发射、Team Turn 完成回调
- **前端**：Store 状态管理、面板布局、WS 实时更新、Activity-First 消费（useActivityTimeline）、树形嵌套渲染
- **E2E**：SP-E2E-01（精灵对话 → 组建团队 → 查看执行面板 → 下钻子 agent 子任务看板 → 返回精灵）

> 详见 [59-chat-ui-optimization.development.md §8](./59-chat-ui-optimization.development.md#8-审查修复记录)（测试文件清单与阶段标记）

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

---

## 十四、Activity-First 架构设计（2026-06-13 新增）

> **方案**：[2026-06-13-activity-first-restructure-optimized-proposal.md](../reports/2026-06-13-activity-first-restructure-optimized-proposal.md)
> **需求**：[59-chat-ui-optimization.md §15](./59-chat-ui-optimization.md)
> **原则**：后端语义直推前端，M59 展示层保留，渐进式迁移

### 14.1 设计定位

当前系统采用 Message-First 模型，后端按 LLM 调用轮次建模，前端需 13 层推理恢复语义。Activity-First 架构在数据源层引入 Activity 语义模型，后端通过 `ActivityProjector` 从运行时事件中投影出 Activity，通过 WS 事件直推前端，前端零推理消费。

**与 M59 的关系**：M59 是"展示层治理"（TaskBoard/ThinkingArea/UnifiedExecutionPanel），Activity-First 是"数据源层治理"。两者互补，Activity 替代 `useAgentBlocks` 的推理逻辑，M59 组件的数据源从 AgentBlock 切换到 Activity。

### 14.2 ActivityProjector 设计

#### 14.2.1 核心结构

```go
// internal/agent/activity_projector.go
type ActivityProjector struct {
    lg           loggateway.Logger
    eventBus     event.Bus
    sessionID    string
    turnID       string
    spiritCtx    *SpiritContext
    activities   map[string]*Activity
    rootActivity *Activity
    mu           sync.Mutex
}

type SpiritContext struct {
    SpiritSessionID string
    TeamID          string
    DAGNodeID       string
    DependsOn       []string
    AgentKey        string
    AgentName       string
}
// SpiritContext 承载 Spirit 模式专有上下文，与 EventProjector 的 ProjectMeta（传输层元数据）互补。
// ProjectMeta 提供 sessionID/turnID 等基础字段，SpiritContext 提供 DAG 依赖和团队归属等业务字段。
```

#### 14.2.2 投影规则

| 运行时事件 | 投影为 Activity | 说明 |
|-----------|----------------|------|
| Turn 开始 | `task` | 根 Activity，描述任务 |
| `reasoning_delta` | `thinking` | reasoning 内容流式推送 |
| `reasoning_done` + `reasoning_as_display=true` | `thinking` → `reply` | reasoning 即回复时升级 |
| `text_delta` | `reply` | 正常回复流式推送 |
| `member_message_delta`（Team 成员） | `reply`（`meta.member_id=author`） | AF-GAP-04：Team 成员消息通过 AF kind 渲染，前端 `applyMemberMetaToMessage` 映射 team_member 元数据 |
| `tool_call` | `action` | 工具调用开始 |
| `tool_result` | `action` (done) | 工具调用完成 |
| `tool_call(tool_name=subagents_spawn)` | `delegate` + `sub_task_board` | 委派子代理（工具调用语义重分类） |
| Team 组建 | `delegate` | 精灵委派团队 |
| Turn 完成 | `end` | 任务完成标记 |
| 错误 | `error` | 错误信息 |
| 编排事件 | `notice` | 语境加载消息 |

#### 14.2.3 关键改进：reasoning_as_display 流式解决

**当前问题**：`reasoning_as_display` 标志在 `DisplayMarkdownFromStream` 中设置，但仅在持久化后的 `options_json` 中可见，流式阶段前端无法知道。

**优化方案**：`ActivityProjector` 在 `OnReasoningDone` 时判断是否为 `reasoning_as_display`，如果是，直接发射 `activity_done(kind=reply)` 而非 `activity_done(kind=thinking)`。前端无需推理。

#### 14.2.4 与 EventProjector 的关系

`ActivityProjector` 与 `EventProjector` 并行运行，共享同一个 mutex 确保顺序发射。Phase 1 双发射阶段两者同时工作，Phase 3 停发旧事件后 `EventProjector` 标记 Deprecated。

### 14.3 ActivityEvent 协议（2026-06-27 更新：替代旧 Envelope 协议）

> **架构变更（2026-06-27 更新）**：原 4 种 `EnvelopeType`（`activity_start`/`activity_delta`/`activity_done`/`activity_child_start`）已升级为 **7 种 `ActivityEventType`**（`created`/`streaming`/`updated`/`completed`/`failed`/`cancelled`/`child_created`）。Envelope 结构体已删除，`ActivityEvent` 是 EventBus 和 WS 传输的唯一格式。详见 Chat 重构方案 §3.2（已归档）。

#### 14.3.1 ActivityEventType 定义（7 种业务语义事件）

```go
// internal/event/activity_event.go
type ActivityEventType string

const (
    ActivityEventCreated      ActivityEventType = "created"       // Activity 创建
    ActivityEventStreaming    ActivityEventType = "streaming"     // 流式追加（替代技术术语 "delta"）
    ActivityEventUpdated      ActivityEventType = "updated"       // 状态变更（非流式）
    ActivityEventCompleted    ActivityEventType = "completed"     // 正常完成
    ActivityEventFailed       ActivityEventType = "failed"        // 失败（独立事件）
    ActivityEventCancelled    ActivityEventType = "cancelled"     // 取消（用户主动停止）
    ActivityEventChildCreated ActivityEventType = "child_created" // 子 Activity 创建
)

// ActivityEvent 是 EventBus 和 WS 传输的唯一格式
type ActivityEvent struct {
    Event    ActivityEventType `json:"event"`
    Activity Activity           `json:"activity"`
}
```

**streaming vs updated 边界**（必须遵守）：

| 维度 | streaming | updated |
|------|-----------|---------|
| 变更类型 | 文本追加（content/reasoning/tool_arguments） | 非文本变更（status/stage/progress/成员列表） |
| 频率 | 高频（每 token） | 低频（阶段变更） |
| 前端行为 | 追加文本，光标闪烁 | 更新状态/进度，不追加文本 |
| 批量合并 | 是（16ms 窗口） | 否 |
| meta 字段 | `meta.delta_field` 标识追加字段 | `meta.changed_fields` 标识变更字段 |

**child_created 语义**：`child_created` 是**父 Activity 的事件**，通知前端在父 Block 下新增子 Block。子 Activity 有自己完整的生命周期（独立发送 `created`/`streaming`/`completed`/...），父子解耦。

#### 14.3.2 事件载荷（ActivityEvent 格式）

**ActivityEvent(event=created)**：

```go
activity := Activity{
    ID:               activityID,
    Kind:             kind,               // 10 种 ActivityKind 之一
    ParentActivityID: parentID,           // 树形嵌套
    SessionID:        sessionID,
    TurnID:           turnID,
    SpiritSessionID:  spiritSessionID,
    TeamID:           teamID,
    DagNodeID:        dagNodeID,
    AgentKey:         agentKey,
    AgentName:        agentName,
    Label:            label,
    ToolName:         toolName,
    ToolCategory:     toolCategory,       // 10 种 ToolCategory 之一
    ToolCallID:       toolCallID,
    ToolArguments:    toolArguments,
}
event := ActivityEvent{Event: ActivityEventCreated, Activity: activity}
```

**ActivityEvent(event=streaming)**（`meta.delta_field` 标识追加字段）：

```go
activity := Activity{
    ID: activityID,
    // 增量字段（按 delta_field 决定）：
    Reasoning:  reasoningDelta,   // delta_field=reasoning, kind=thinking
    Content:    contentDelta,     // delta_field=content, kind=reply
    ToolArguments: toolArgsDelta, // delta_field=tool_arguments, kind=action
}
event := ActivityEvent{Event: ActivityEventStreaming, Activity: activity}
```

**ActivityEvent(event=completed)**：

```go
activity := Activity{
    ID:            activityID,
    Status:        finalStatus,
    DurationMs:    durationMs,
    Collapsed:     collapsed,
    Reasoning:     fullReasoning,    // kind=thinking
    Content:       fullContent,      // kind=reply
    ToolResult:    fullToolResult,   // kind=action
    ToolDurationMs: totalDurationMs,
}
event := ActivityEvent{Event: ActivityEventCompleted, Activity: activity}
```

**ActivityEvent(event=failed)**（`meta.error_code` + `meta.error_message`）：

```go
activity := Activity{
    ID: activityID,
    // 错误信息：
    // meta.error_code = errorCode
    // meta.error_message = errorMessage
}
event := ActivityEvent{Event: ActivityEventFailed, Activity: activity}
```

**ActivityEvent(event=child_created)**（`meta.child_activity_id` 标识子 Activity）：

```go
activity := Activity{
    ID:               activityID,       // 父 Activity ID
    ParentActivityID: parentID,
    // meta.child_activity_id = childActivityID
    // meta.member_agent_key = childAgentKey（team_stage 场景）
}
event := ActivityEvent{Event: ActivityEventChildCreated, Activity: activity}
```

#### 14.3.3 与现有 M59 事件的关系（已废弃 — 已全部迁移到 ActivityEvent）

> **废弃说明（2026-06-27）**：以下迁移策略为 Phase AF-1 双发射阶段的历史记录。Phase AF-3 完成后，所有旧事件已删除，仅保留 ActivityEvent。完整映射表见 §14.8。

| 旧 M59 事件 | ActivityEvent | 迁移状态 |
|-------------|-------------|---------|
| `text_delta` / `text_done` | `ActivityEvent(event=streaming/completed, kind=reply)` | ✅ 已迁移 |
| `text_delta(含 reasoning)` | `ActivityEvent(event=streaming, kind=thinking, delta_field=reasoning)` | ✅ 已迁移 |
| `tool_call` / `tool_result` | `ActivityEvent(event=created/completed, kind=action)` | ✅ 已迁移 |
| `member_message_start/delta/done` | `ActivityEvent(event=child_created + created/streaming/completed, kind=reply)` | ✅ 已迁移 |
| `spirit_team_assembled` | `ActivityEvent(event=created, kind=team_stage, stage=assembled)` | ✅ 已迁移 |
| `spirit_plan_created` | `ActivityEvent(event=created, kind=plan)` | ✅ 已迁移 |
| `spirit_team_progress` | `ActivityEvent(event=updated, kind=team_stage, changed_fields=progress)` | ✅ 已迁移 |
| `butler_*` / `skill_*` | 删除（未使用） | ✅ 已删除 |

#### 14.3.4 AS-EVT-01 可靠性分级

| ActivityEvent | AS-EVT-01 级别 | 可靠性保证 | 说明 |
|--------------|---------------|-----------|------|
| `created` | Important | 异步持久化，失败重试 + 同步推送 | Activity 创建需可靠到达 |
| `streaming` | Informational | 异步持久化，失败丢弃 + 同步推送（可批量合并） | 流式增量，丢失可容忍 |
| `updated` | Informational | 异步持久化，失败丢弃 + 同步推送 | 状态变更，丢失可由后续 updated 补偿 |
| `completed` | Important | 异步持久化，失败重试 + 同步推送 | Activity 完成需可靠到达 |
| `failed` | Important | 异步持久化，失败重试 + 同步推送 | 失败事件需可靠到达 |
| `cancelled` | Important | 异步持久化，失败重试 + 同步推送 | 取消事件需可靠到达 |
| `child_created` | Important | 异步持久化，失败重试 + 同步推送 | 子 Activity 创建需可靠到达 |

### 14.4 activities 表 Schema

```sql
CREATE TABLE IF NOT EXISTS activities (
    id                TEXT PRIMARY KEY,
    kind              TEXT NOT NULL,
    session_id        TEXT NOT NULL,
    turn_id           TEXT NOT NULL,
    parent_activity_id TEXT,
    timestamp         TEXT NOT NULL,
    content           TEXT,
    reasoning         TEXT,
    tool_name         TEXT,
    tool_call_id      TEXT,
    tool_arguments    TEXT,
    tool_result       TEXT,
    tool_duration_ms  INTEGER,
    tool_error_code   TEXT,
    child_board_id    TEXT,
    spirit_session_id TEXT,
    team_id           TEXT,
    dag_node_id       TEXT,
    depends_on        TEXT,
    agent_key         TEXT,
    agent_name        TEXT,
    status            TEXT NOT NULL DEFAULT 'running',
    collapsed         INTEGER NOT NULL DEFAULT 0,
    duration_ms       INTEGER,
    label             TEXT,
    FOREIGN KEY (session_id) REFERENCES sessions(id),
    FOREIGN KEY (parent_activity_id) REFERENCES activities(id)
);

CREATE INDEX IF NOT EXISTS idx_activities_session_turn ON activities(session_id, turn_id);
CREATE INDEX IF NOT EXISTS idx_activities_parent ON activities(parent_activity_id);
CREATE INDEX IF NOT EXISTS idx_activities_spirit_session ON activities(spirit_session_id);
CREATE INDEX IF NOT EXISTS idx_activities_team ON activities(team_id);
```

**Ent Schema 规范**：
- `entsql.Annotation{Table: "activities"}` 显式映射表名
- 不使用 Ent Edge，使用手动 FK 字段
- `tool_arguments` 标记 `.Sensitive()`
- `tool_result` 标记 `.Sensitive()`（可能包含 API 密钥、凭证等敏感输出）
- `depends_on` 使用 `field.JSON()`
- 新增 Repo 接口遵循窄接口原则（Reader/Writer 拆分）

### 14.5 前端 Activity 消费层

#### 14.5.1 useActivityTimeline composable

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

  function handleActivityStart(env: Envelope) { /* ... */ }
  function handleActivityDelta(env: Envelope) { /* ... */ }
  function handleActivityDone(env: Envelope) { /* ... */ }

  return { activities, activityTree, taskBoardNodes, handleActivityStart, handleActivityDelta, handleActivityDone }
}
```

#### 14.5.2 Activity → TaskBoardNode 映射

```typescript
function activityToTaskBoardNode(activity: Activity): TaskBoardNode {
  return {
    kind: activity.kind as TaskBoardNodeKind,
    id: activity.id,
    timestamp: activity.timestamp,
    collapsed: activity.collapsed,
    content: activity.content,
    reasoning: activity.reasoning,
    toolName: activity.toolName,
    toolDuration: activity.toolDurationMs,
    toolCallId: activity.toolCallId,
    toolArguments: activity.toolArguments,
    toolResult: activity.toolResult,
    toolStatus: mapActivityStatusToNodeStatus(activity.status),
    errorMessage: activity.kind === 'error' ? activity.content : undefined,
  }
}
```

#### 14.5.3 与 M59 组件集成（2026-06-27 更新：ActivityStream 替代）

> **架构变更（2026-06-27 更新）**：原 `ConversationTurn.vue` / `UnifiedExecutionPanel.vue` 已删除，由 `ActivityStream.vue` 三模式统一渲染器替代。详见 §14.9（ActivityStream 统一渲染器设计）。

| M59 组件（Activity-First 后） | 数据源变更 | 说明 |
|---------|----------|------|
| `ActivityStream.vue`（新增，替代 ConversationTurn + TaskExecutionPanel + MemberReadOnlyPanel） | `useActivityTimeline.sortedActivities` | 三模式统一渲染，按 `activity.kind` 分发到 10 个 Block 组件 |
| `ThinkingBlock.vue`（原 ChatReasoningPeek.vue） | `Activity(kind=thinking)` | 流式态从 `ActivityEvent(event=streaming, delta_field=reasoning)` 获取 |
| `ActionBlock.vue`（原 ToolCallTimeline.vue） | `Activity(kind=action)` | 直接消费；按 `tool_category` 分发到 10 个详情组件（见 §14.11） |
| `ReplyBlock.vue` | `Activity(kind=reply)` | 流式态从 `ActivityEvent(event=streaming, delta_field=content)` 获取 |
| `TeamStageBlock.vue`（新增） | `Activity(kind=team_stage)` | 团队阶段+成员折叠（见 §14.13） |
| `GraphStageBlock.vue`（新增） | `Activity(kind=graph_stage)` | DAG 节点状态（见 §14.13） |
| `SessionTreeSidebar.vue` + `SessionTreeNode.vue`（新增） | Session 树 RPC | 递归渲染 Spirit→Team→Agent 树（见 §14.12） |
| `SpiritStatusBar.vue` | 多 computed 拼装 → Activity 聚合 | 简化 |
| `TodoKanbanBoard.vue` | `useTodoBoard` → `Activity(kind=action, toolCategory=todo)` | 特殊处理 |

#### 14.5.4 Store 集成

| Store | 变更 | 说明 |
|-------|------|------|
| `useChatStore` | 新增 `activitiesBySession` Map + Activity 事件处理 | Activity 数据入口，按 session_id 隔离 |
| `useAgentBlocksStore` | Phase AF-3 已废弃删除 | 双发射期已结束 |
| `useSpiritStore` | `spiritTeamAssembled` → `ActivityEvent(event=created, kind=team_stage, stage=assembled)` | 已切换 |
| `useTodoBoardStore` | `todo_write` 事件 → `Activity(kind=action, toolCategory=todo)` | 已切换 |

### 14.6 13 层推理消除映射（2026-06-27 更新：ActivityEvent 术语）

| # | 原推理步骤 | Activity-First 后 | 消除方式 |
|---|-----------|------------------|---------|
| 1 | `reasoning_as_display` 推断 | `ActivityEvent(event=completed, kind=reply)` | 后端投影器判断 |
| 2 | ReAct 标签解析 | `ActivityEvent(event=created, kind=thinking, label=xxx)` | 后端解析标签 |
| 3 | member ID 前缀约定 | `ActivityEvent(event=created, agentKey=xxx)` | 直接携带 |
| 4 | EnvelopeContent 无语义 | `ActivityEvent(event=created, kind=xxx)` | 语义在 kind 中 |
| 5 | snapshotStreamingMessage | `ActivityEvent(event=completed)` 替代 | 不再需要 snapshot |
| 6 | mergeSessionMessages 内容匹配 | `activity.id` 全局唯一 | ID 匹配替代内容匹配 |
| 7 | classifyActivityKind | `activity.kind` 直接给出 | 后端分类 |
| 8 | resolveAssistantPresentation | `activity.kind=reply` 直接给出 | 后端判断 |
| 9 | isReasoningAsDisplay | 不再需要 | 后端在投影器中处理 |
| 10 | reasoningMarkdown fallback | `activity.content` 或 `activity.reasoning` | 字段明确 |
| 11 | useConversationTimeline 推理 | `useActivityTimeline` 直接消费（已替代） | 无推理 |
| 12 | useAgentBlocks 构建 | `sortedActivities` 直接映射 | 无推理 |
| 13 | computeAgentStatus | `activity.status` 直接给出 | 后端计算 |

### 14.7 迁移策略（2026-06-27 更新：三阶段全部完成）

> **实际状态**：Phase AF-1 / AF-2 / AF-3 全部完成。后端 ADR-03 Phase 5 Blocker A-G 全部完成（`contract/envelope.go` 已删、8 个 consumer 全部迁移到 `ActivityEventBus`/`MonitorBus`）；前端 Envelope 路径已彻底删除。详见 ADR-03「Blocker G 前端完成总结」。

#### Phase AF-1：双发射（兼容期）— ✅ 已完成

- 后端同时发射旧事件和新 Activity 事件
- 前端仍消费旧事件，新事件仅记录日志
- `ActivityProjector` 与 `EventProjector` 并行运行
- 新增 `activities` 表和 Ent Schema

#### Phase AF-2：前端切换 — ✅ 已完成

- 前端新增 `useActivityTimeline` composable
- Feature flag 控制切换
- 逐步替换各组件数据源
- 保留旧事件消费路径作为 fallback

#### Phase AF-3：清理与优化 — ✅ 已完成

- 前端完全切换后停发旧事件
- 清理 `useAgentBlocks` 推理逻辑（已删除）
- 清理 `useConversationTimeline` 推理逻辑（已删除，由 `useActivityTimeline` 替代）
- `EventProjector` 标记 Deprecated（已删除）
- 后端 `contract/envelope.go` 删除（Blocker G ✅，活类型提取到 `envelope_types.go`）
- 前端 `realtime/envelope.ts`/`dispatcher.ts`/`data_channel.ts`/`event_replay.ts` + `features/chat/dispatcher.ts` 删除（任务 8 ✅）
- `ActivityStream.vue` 作为三模式统一渲染器（替代 ChatMessageList + TaskExecutionPanel + MemberReadOnlyPanel）

### 14.8 影响域（2026-06-27 更新）

| 包 | 变更类型 | 说明 |
|----|----------|------|
| `internal/agent/` | 新增 | `activity_projector.go`（含 `OnMemberMessageDelta`/`OnMemberMessageDone` AF-GAP-04）+ `tool_category.go`（ToolCategorizer 10 种类别） + `perf_bench_test.go`（T6.2 性能基准） |
| `internal/biz/` | 新增 | `ActivityRepo` 接口（Reader/Writer 拆分）+ `ActivityKind`/`ActivityEventType` 枚举（10 种 kind + 7 种 event） |
| `internal/data/` | 新增 | `activity_repo.go` + Ent Schema + DDL 迁移 + `activity_backfill_migrate.go`（T6.3 Pre-AF 数据回填） |
| `internal/event/` | 扩展 | `ActivityEventBus`（biz.ActivityEvent）+ `MonitorEventBus`（contract.MonitorEvent）；legacy Envelope Bus / SessionBus / MonitorBus / `contract/envelope.go` 全部删除（Blocker F/G ✅） |
| `internal/service/` | 修改 | 集成 ActivityProjector |
| `web/src/features/chat/` | 新增 | `activityTypes.ts` + `useActivityTimeline.ts`（含 AF-GAP-05 重试降级 + `ensureActivitiesLoaded` 缓存跳过） |
| `web/src/features/chat/` | 修改 | `streamHandlers.ts` 已删除（占位机制移除）；`inboundSyncRouting.ts`/`inboundSyncEnvelope.ts` 已迁移到 ActivityEvent |
| `web/src/features/chat/composables/` | 修改 | `useChatInboundSync.ts`（AF-GAP-02 当前 session 转发）+ `useChatWorkspace.ts`（envelope ID 去重） |
| `web/src/features/session/` | 新增 | `types.ts`（SessionTreeNode 递归类型）+ `api.ts`（GetSessionTree RPC） |
| `web/src/components/chat/` | 新增 | `ActivityStream.vue` + `SessionTreeSidebar.vue` + `SessionTreeNode.vue` + `TeamStageBlock.vue` + `GraphStageBlock.vue` + 10 个 ToolDetail 组件 |
| `web/src/components/chat/` | 删除 | `ConversationTurn.vue` / `TaskExecutionPanel.vue` / `MemberReadOnlyPanel.vue` / `TeamPanel.vue` / `OrchestrationTimeline.vue` |
| `web/src/realtime/` | 删除 | `envelope.ts` / `dispatcher.ts` / `data_channel.ts` / `event_replay.ts`（Blocker G 前端完成 ✅） |

**不改动**：M59 展示组件的模板和样式；`internal/server` 直连 runtime；Team 编译/运行流程。

---

### 14.9 ActivityStream 统一渲染器设计（2026-06-27 新增）

> **来源**：Chat 重构方案 §7.1-§7.2（已归档，设计内容已并入本文档）。
> **定位**：spirit/team/member 三模式唯一渲染入口，替代旧 ChatMessageList + TaskExecutionPanel + MemberReadOnlyPanel 三套渲染管线。

#### 14.9.1 渲染管线

```
WS ActivityEvent（created/streaming/updated/completed/failed/cancelled/child_created）
  ↓
useActivityTimeline（按 session_id 隔离 Map，见 §14.10）
  ↓
ActivityStream.vue（统一入口，替代 ChatMessageList + TaskExecutionPanel + MemberReadOnlyPanel）
  ↓
按 activity.kind 动态分发：
  ├── task → UserMessageBubble（用户消息）
  ├── thinking → ThinkingBlock
  ├── action → ActionBlock（按 tool_category 细分，见 §14.11；failed 事件高亮错误）
  ├── reply → ReplyBlock
  ├── plan → PlanBlock
  ├── confirm → ConfirmBlock
  ├── notice → NoticeBlock
  ├── session → SessionStageBlock（Session 生命周期）
  ├── team_stage → TeamStageBlock（团队阶段+成员折叠，见 §14.13；failed/cancelled 事件显示状态）
  └── graph_stage → GraphStageBlock（DAG 阶段，见 §14.13；failed 事件高亮错误节点）
```

**注意**：不保留 `error → ErrorBlock` 分支。错误通过对应 kind 的 `failed` 事件表达（如 `action+failed` 在 ActionBlock 内高亮错误，`team_stage+failed` 在 TeamStageBlock 内显示失败状态）。

#### 14.9.2 ActivityStream 组件实现

```vue
<!-- web/src/components/chat/ActivityStream.vue -->
<template>
  <DynamicScroller :items="sortedActivities" :min-item-size="60">
    <template #default="{ item: activity }">
      <component
        :is="resolveBlockComponent(activity.kind)"
        :activity="activity"
        @expand="onExpand"
        @collapse="onCollapse"
      />
    </template>
  </DynamicScroller>
</template>

<script setup lang="ts">
import type { Component } from 'vue';
import type { Activity, ActivityKind } from '../../features/chat/activityTypes';

const props = defineProps<{
  sessionId: string;
  activities: Activity[];
}>();

function resolveBlockComponent(kind: ActivityKind): Component {
  const map: Record<ActivityKind, Component> = {
    task: UserMessageBubble,
    thinking: ThinkingBlock,
    action: ActionBlock,
    reply: ReplyBlock,
    plan: PlanBlock,
    confirm: ConfirmBlock,
    notice: NoticeBlock,
    session: SessionStageBlock,
    team_stage: TeamStageBlock,
    graph_stage: GraphStageBlock,
  };
  return map[kind] ?? NoticeBlock; // 兜底
}
</script>
```

#### 14.9.3 动态渲染行为

**思考（thinking）**：
- `created` → 新增 ThinkingBlock（折叠状态，显示"思考中..."）
- `streaming`（delta_field=reasoning）→ 流式追加文本，光标闪烁
- `completed` → 停止光标，可展开查看完整推理
- `failed` → 显示"思考失败"，展示错误信息
- `cancelled` → 显示"已停止"

**回复（reply）**：
- `created` → 新增 ReplyBlock（流式渲染 markdown）
- `streaming`（delta_field=content）→ 流式追加文本
- `completed` → 完成 markdown 渲染
- `failed` → 显示"回复失败"
- `cancelled` → 显示"已停止"

**计划（plan）**：
- `created` → 新增 PlanBlock（显示计划标题）
- `streaming`（delta_field=steps）→ 渲染计划步骤列表（checkbox 形式）
- `updated`（changed_fields=step_status）→ 更新步骤状态（pending/completed/failed）
- `completed` → 完成计划展示，可折叠

**工具调用（action）**：详见 §14.11（ActionBlock tool_category 分发设计）。

**团队阶段（team_stage）** / **Graph 阶段（graph_stage）**：详见 §14.13（TeamStageBlock / GraphStageBlock 设计）。

---

### 14.10 useActivityTimeline session 隔离 + 缓存优化（2026-06-27 新增）

> **来源**：Chat 重构方案 §7.1.3 + §9.1.3（子 session Activity 加载）（已归档，设计内容已并入本文档）。

#### 14.10.1 按 session_id 隔离

```typescript
// web/src/features/chat/composables/useActivityTimeline.ts
const activitiesBySession = shallowRef<Map<string, Map<string, Activity>>>(new Map());

function getSessionActivities(sessionId: string): Map<string, Activity> {
    let map = activitiesBySession.value.get(sessionId);
    if (!map) {
        map = new Map();
        activitiesBySession.value.set(sessionId, map);
    }
    return map;
}

// 切换 session 时无需 reset，自然隔离
```

#### 14.10.2 ensureActivitiesLoaded 缓存跳过（Phase E 实现）

```typescript
async function ensureActivitiesLoaded(sessionId: string): Promise<void> {
  // 缓存命中（含空 Map）时跳过 API 调用
  if (activitiesBySession.value.has(sessionId)) {
    return;
  }
  try {
    const activities = await listActivities(sessionId);
    activitiesBySession.value.set(sessionId, new Map(activities.map(a => [a.id, a])));
  } catch (err) {
    // 失败时不写缓存，以便下次自动重试
    console.warn('Failed to load activities, will retry on next access', err);
  }
}
```

**语义保证**：
- 缓存命中（含空 Map）→ 跳过 API 调用
- 失败 → 不写缓存，下次访问自动重试
- WS replay → 重连后补齐缺失事件（替代旧 `RevisionTracker` + `requestSyncReplay` 机制，重连改用 `ListActivities` RPC）

#### 14.10.3 bindSessionView 集成

`bindSessionView` 改用 `ensureActivitiesLoaded` 替代 `loadActivitiesFromAPI`，成员切换瞬时响应（缓存命中时无 API 调用）。

---

### 14.11 ActionBlock tool_category 分发设计（2026-06-27 新增）

> **来源**：Chat 重构方案 §8（已归档，设计内容已并入本文档）。

#### 14.11.1 ToolCategory 枚举（10 种）

```go
// internal/biz/activity.go
type ToolCategory string

const (
    ToolCategoryShell       ToolCategory = "shell"        // Shell 命令执行
    ToolCategoryBrowser     ToolCategory = "browser"      // 浏览器操作
    ToolCategoryFileRead    ToolCategory = "file_read"    // 文件读取
    ToolCategoryFileWrite   ToolCategory = "file_write"   // 文件写入
    ToolCategoryFileSearch  ToolCategory = "file_search"  // 文件查找
    ToolCategoryWebSearch   ToolCategory = "web_search"   // 网络搜索
    ToolCategoryMCP         ToolCategory = "mcp"          // MCP 工具
    ToolCategoryCode        ToolCategory = "code"         // 代码执行
    ToolCategoryTodo        ToolCategory = "todo"         // Todo 管理
    ToolCategoryOther       ToolCategory = "other"        // 其他
)
```

#### 14.11.2 ToolCategorizer 后端识别器

```go
// internal/agent/tool_category.go

// ToolCategorizer 工具类型识别器（可注入，便于测试和扩展）
type ToolCategorizer interface {
    Categorize(toolName string) ToolCategory
}

// defaultToolCategorizer 默认实现：注册表查询 + 前缀匹配兜底
type defaultToolCategorizer struct {
    toolRegistry map[string]ToolCategory  // 由 ToolService 启动时填充
}

func (c *defaultToolCategorizer) Categorize(toolName string) ToolCategory {
    // 1. 优先查注册表（准确）
    if cat, ok := c.toolRegistry[toolName]; ok {
        return cat
    }
    // 2. 前缀/名称匹配兜底（覆盖未注册工具）
    switch {
    case strings.HasPrefix(toolName, "shell") || strings.HasPrefix(toolName, "bash"):
        return ToolCategoryShell
    case strings.HasPrefix(toolName, "browser") || strings.HasPrefix(toolName, "playwright"):
        return ToolCategoryBrowser
    // ... 其余 8 类前缀匹配
    default:
        return ToolCategoryOther
    }
}
```

**注入方式**：`ActivityProjector` 通过构造函数注入 `ToolCategorizer`，便于测试时 mock。

#### 14.11.3 前端 UI 表现

| 工具类型 | 图标 | 布局 | 折叠时显示 | 展开时显示 |
|---------|------|------|-----------|-----------|
| shell | `$` | 终端样式 | 命令摘要 | 完整命令 + stdout/stderr |
| browser | 🌐 | 网页卡片 | URL + 操作类型 | 截图 + DOM 操作详情 |
| file_read | 📖 | 文件卡片 | 文件路径 | 文件内容片段 |
| file_write | ✏️ | 文件卡片 | 文件路径 + 变更行数 | diff 视图 |
| file_search | 🔍 | 搜索结果 | 搜索条件 + 命中数 | 结果列表 |
| web_search | 🔎 | 搜索卡片 | 查询词 + 结果数 | 结果摘要列表 |
| mcp | 🔌 | 通用卡片 | MCP 服务名 + 方法 | 参数 + 结果 |
| code | 💻 | 代码块 | 语言 + 执行状态 | 代码 + 输出 |
| todo | ✅ | 看板卡片 | 进度条 | 任务列表 |
| other | 🔧 | 通用卡片 | 工具名 | 参数 + 结果 |

#### 14.11.4 ActionBlock 组件实现

```vue
<!-- web/src/components/chat/ActionBlock.vue -->
<template>
  <div class="action-block" :class="`tool-${category}`">
    <div class="action-header" @click="toggleExpand">
      <ToolIcon :category="category" :name="activity.tool_name" />
      <span class="action-title">{{ actionTitle }}</span>
      <StatusBadge :status="activity.status" />
      <DurationBadge v-if="activity.duration_ms" :ms="activity.duration_ms" />
    </div>

    <div v-if="expanded" class="action-detail">
      <component :is="detailComponent" :activity="activity" />
    </div>
  </div>
</template>

<script setup lang="ts">
const detailComponent = computed(() => {
  const map: Record<string, Component> = {
    shell: ShellToolDetail,
    browser: BrowserToolDetail,
    file_read: FileReadToolDetail,
    file_write: FileWriteToolDetail,
    file_search: FileSearchToolDetail,
    web_search: WebSearchToolDetail,
    mcp: McpToolDetail,
    code: CodeToolDetail,
    todo: TodoToolDetail,
    other: GenericToolDetail,
  };
  return map[category.value] ?? GenericToolDetail;
});
</script>
```

---

### 14.12 SessionTreeSidebar + SessionTreeNode 递归设计（2026-06-27 新增）

> **来源**：Chat 重构方案 §10（已归档，设计内容已并入本文档）。
> **实现状态**：Phase D 补全 Session 7 字段断层（proto `Session` 新增 `session_type`/`member_agent_key`/`member_role`/`execution_stage`/`completed_steps`/`total_steps`/`progress_pct`，编号 53-59），`SessionTreeNode.vue` UI 增强。

#### 14.12.1 左侧栏 Session 树（支持任意深度）

```
┌─────────────────────────────────┐
│ 💬 Spirit Sessions              │
├─────────────────────────────────┤
│ ▼ 帮我重构 chat 模块            │ ← Spirit Session（root，depth=0）
│   ├─ 🔄 团队 1：后端重构        │ ← Team Session（depth=1）
│   │   ├─ 👤 agent-go            │ ← Agent Session（depth=2，Team 成员）
│   │   │   └─ 👤 sub-agent       │ ← Agent Session（depth=3，子 Agent）
│   │   ├─ 👤 agent-ent           │ ← Agent Session（depth=2）
│   │   └─ 👤 agent-test          │ ← Agent Session（depth=2）
│   ├─ 👤 agent-direct-A          │ ← Agent Session（depth=1，Spirit 直接调度，无 Team）
│   └─ ✅ 团队 2：前端重构        │ ← Team Session（depth=1）
│       ├─ 👤 agent-vue           │ ← Agent Session（depth=2）
│       └─ 👤 agent-style         │ ← Agent Session（depth=2）
│                                 │
│ ▶ 另一个 Spirit Session         │
└─────────────────────────────────┘
```

**特点**：
- 支持任意深度递归（受 `max_session_depth` 限制）
- Team Session 和 Agent Session 都可作为 Spirit 的直接子节点
- Agent Session 可嵌套（子 Agent 递归调用）

#### 14.12.2 SessionTreeSidebar 组件

```vue
<!-- web/src/components/chat/SessionTreeSidebar.vue -->
<template>
  <div class="session-tree-sidebar">
    <div class="spirit-sessions">
      <SessionTreeNode
        v-for="spiritNode in spiritTreeNodes"
        :key="spiritNode.session.id"
        :node="spiritNode"
        :active-session-id="activeSessionId"
        @select="onSelectSession"
      />
    </div>
  </div>
</template>
```

#### 14.12.3 SessionTreeNode 递归组件

```vue
<!-- web/src/components/chat/SessionTreeNode.vue -->
<template>
  <div class="session-tree-node" :class="`depth-${node.session.agent_depth}`">
    <div class="node-header" :class="{ active: isActive }" @click="onSelect">
      <SessionTypeIcon :type="node.session.session_type" :stage="node.session.execution_stage" />
      <span class="node-title">{{ node.session.title }}</span>
      <DepthBadge v-if="node.session.agent_depth > 0" :depth="node.session.agent_depth" />
      <StageBadge v-if="node.session.execution_stage" :stage="node.session.execution_stage" />
      <ProgressMini v-if="node.session.total_steps > 0" :completed="node.session.completed_steps" :total="node.session.total_steps" />
    </div>

    <!-- 递归渲染子节点（支持任意深度） -->
    <div v-if="expanded && node.children.length > 0" class="node-children">
      <SessionTreeNode
        v-for="child in node.children"
        :key="child.session.id"
        :node="child"
        :active-session-id="activeSessionId"
        @select="onSelect"
      />
    </div>
  </div>
</template>
```

**实现状态（Phase D）**：`SessionTypeIcon`/`DepthBadge`/`StageBadge`/`ProgressMini` 内联为 computed 属性 + `q-badge` 元素（而非独立组件文件），避免过度拆分。

#### 14.12.4 视觉规范

| 元素 | 规则 |
|------|------|
| `session_type` 图标 | spirit→`auto_awesome` / team→`groups` / agent→`person` / standalone→`forum` |
| `execution_stage` 徽章颜色 | planning/allocating→blue / executing→orange / completed→green / failed→red |
| `agent_depth` 徽章 | `L{depth}`（depth > 0 时显示） |
| 进度显示 | `{completed}/{total}`（total_steps > 0 时显示） |
| i18n | `session.executionStage.*` 覆盖中英文 |

#### 14.12.5 前端类型定义

```typescript
// web/src/features/session/types.ts
interface SessionTreeNode {
  session: Session;
  children: SessionTreeNode[];  // 递归，支持任意深度
  activities?: Activity[];      // 按需加载
}
```

---

### 14.13 TeamStageBlock / GraphStageBlock 设计（2026-06-27 新增）

> **来源**：Chat 重构方案 §9（已归档，设计内容已并入本文档）。

#### 14.13.1 TeamStageBlock 组件（team_stage kind）

**事件响应行为**：

| ActivityEvent | 行为 |
|---------------|------|
| `created`（stage=assembled） | 新增 TeamStageBlock，显示"团队已组建" + 成员头像列表 + 任务摘要 |
| `updated`（changed_fields=stage, stage=executing） | 更新阶段为"执行中" + 进度条（completed_steps/total_steps）+ 停止/恢复按钮 |
| `updated`（changed_fields=progress） | 更新进度条 |
| `completed`（stage=completed） | 更新阶段为"已完成" + 最终结果摘要 + DQ 评分 |
| `failed`（stage=failed） | 更新阶段为"失败" + 失败原因 |
| `cancelled`（cancel_reason=xxx） | 更新阶段为"已取消" + 取消原因 |
| `child_created`（child_activity_id=xxx, member_agent_key=yyy） | 在成员列表新增成员 Block（折叠状态） |

**团队成员折叠展示**：

```
TeamStageBlock
  ├── 团队头部（阶段/进度/控制按钮）
  ├── 成员列表（可折叠）
  │   ├── Member 1（点击展开）
  │   │   └── 子 Activity 流（该成员的 thinking/action/reply）
  │   ├── Member 2
  │   └── Member 3
  └── 团队结果摘要
```

**子 session Activity 懒加载**：点击成员展开时，通过 `ensureActivitiesLoaded(memberSessionId)` 懒加载该成员 session 的 Activity（见 §14.10.2 缓存跳过语义）。

#### 14.13.2 GraphStageBlock 组件（graph_stage kind）

**事件响应行为**：

| ActivityEvent | 行为 |
|---------------|------|
| `created`（stage=planned） | 新增 GraphStageBlock，显示"Graph 已规划" + DAG 节点列表（依赖关系） |
| `updated`（changed_fields=current_node） | 高亮当前执行节点 + 展示已完成/进行中/待执行节点状态 |
| `completed`（stage=completed） | 所有节点标记完成 + 展示最终结果 |
| `failed`（error_node=xxx） | 高亮错误节点 + 展示错误详情 |
| `child_created`（child_activity_id=xxx） | 在 DAG 中新增子节点 |

**DAG 渲染**：

```vue
<!-- web/src/components/chat/GraphStageBlock.vue -->
<template>
  <div class="graph-stage-block">
    <div class="graph-header">
      <GraphIcon />
      <span>{{ stageTitle }}</span>
      <ProgressIndicator :completed="completedNodes" :total="totalNodes" />
    </div>

    <div v-if="expanded" class="graph-detail">
      <DagView
        :nodes="dagNodes"
        :current-node="currentNode"
        :completed-nodes="completedNodeIds"
      />
    </div>
  </div>
</template>
```

#### 14.13.3 编排阶段进度条

Spirit 视图顶部展示编排阶段进度条：

```
[规划] → [分配] → [执行] → [完成]
  ✅       ✅       🔄       ⏳
```

通过 `notice` kind + `meta.phase` 的 Activity 事件驱动：

```typescript
const phases = ['planning', 'allocating', 'orchestrating', 'completed'];
const currentPhaseIndex = computed(() => phases.indexOf(spiritStore.orchestrationPhase));
```

---

### 14.14 原 EnvelopeType → ActivityKind 完整映射表（2026-06-27 新增）

> **来源**：Chat 重构方案 §3.3（已归档，设计内容已并入本文档）。
> **说明**：所有原 EnvelopeType 已彻底合并到 ActivityKind + ActivityEventType。Envelope 结构体已删除。

| 原 EnvelopeType | 彻底合并后 | Activity kind | 事件类型 |
|----------------|-----------|--------------|---------|
| `activity_start`/`delta`/`done`/`child_start` | 保留语义，改名 | 按 kind | created/streaming/completed/child_created |
| `session_created` | 合并 | `session` | `created` |
| `session_status` | 合并 | `session` | `updated` |
| `session_completed` | 合并 | `session` | `completed` |
| `spirit_team_assembled` | 合并 | `team_stage` | `created`（stage=assembled） |
| `spirit_team_completed` | 合并 | `team_stage` | `completed`（stage=completed） |
| `spirit_team_failed`/`interrupted` | 合并 | `team_stage` | `failed`/`cancelled`（stage=failed/interrupted） |
| `spirit_team_cancelled` | 合并 | `team_stage` | `cancelled` |
| `spirit_team_progress` | 合并 | `team_stage` | `updated`（meta.progress） |
| `spirit_teams_all_completed` | 合并 | `team_stage` | `completed`（stage=all_completed） |
| `spirit_plan_created` | 合并 | `plan` | `created` |
| `spirit_allocation_created` | 合并 | `notice` | `created`（meta.allocation） |
| `spirit_orchestration_started`/`checkpoint`/`interrupted` | 合并 | `notice` | `updated`（meta.phase） |
| `spirit_synthesis_completed` | 合并 | `reply` | `completed`（meta.synthesis） |
| `team_run_started` | 合并 | `team_stage` | `created` |
| `team_run_finished` | 合并 | `team_stage` | `completed` |
| `team_run_failed` | 合并 | `team_stage` | `failed` |
| `team_step_started`/`finished`/`summary` | 合并 | `team_stage` | `updated` |
| `member_message_start`/`delta`/`done` | 合并 | `reply` | created/streaming/completed（agent_key=agent，session_type=agent） |
| `orchestration_agent_status` | 合并 | `team_stage` | `updated`（meta.member_status） |
| `graph_node_start` | 合并 | `graph_stage` | `created` |
| `graph_node_end` | 合并 | `graph_stage` | `completed` |
| `graph_node_error` | 合并 | `graph_stage` | `failed` |
| `graph_node_custom` | 合并 | `graph_stage` | `updated` |
| `graph_step`/`execution_done`/`replanned`/`topology_evolved` | 合并 | `graph_stage` | `updated` |
| `checkpoint` | 合并 | `task` | `completed`（meta.checkpoint） |
| `error` | 合并 | 对应 kind | `failed`（如 action+failed / team_stage+failed） |
| `token_usage` | 合并 | `task` | `completed`（meta.token_usage） |
| `run_completion` | 合并 | `task` | `completed` |
| `user_feedback` | 合并 | `notice` | `created`（meta.feedback） |
| `text_delta`/`text_done` | 删除 | `reply` | streaming/completed |
| `tool_call`/`tool_result` | 删除 | `action` | created/completed |
| `state_delta`/`transfer`/`context_usage`/`intent_pass` | 删除 | 合并到对应 Activity 的 `streaming`/`updated` |
| `monitor/*`（log/flow_log/mcp_*/alert_*/monitor_*） | 移出 chat | 不影响 | — |
| `butler_*`/`skill_*`/`borrow_*`/`organization_*`/`planning_phase_*` | 删除 | 未使用 | — |
