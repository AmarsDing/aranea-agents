# M59: Chat 管家模式 — 实现设计

> 对应需求：[59-chat-spirit-mode.md](./59-chat-spirit-mode.md)
> 遵循：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md) · [AGENT_RUNTIME_BOUNDARY.md](../AGENT_RUNTIME_BOUNDARY.md)
> **实现差距与迭代计划**以 [59-chat-spirit-mode-development.md](./59-chat-spirit-mode-development.md) 为准

---

## 一、模块概述

### 1.1 设计定位

以精灵（Spirit）为 Chat 页面唯一对话入口，左侧列表从"Agent/Team 平铺"重构为"精灵 + 任务团队树"：

- **精灵简视图**：用户只与精灵对话，不感知 Agent/Team 细节
- **任务团队视图**：精灵自动组建的团队在左侧动态展示，支持展开成员树
- **执行观测视图**：点击团队/成员进入任务执行面板或只读面板

**前置依赖**：[system-builtin-agents-design](../superpowers/specs/2026-05-31-system-builtin-agents-design.md) 中精灵 Agent 定义、`plan_and_execute` 三阶段编排工具、Session 树状模型。

### 1.2 分层与依赖

```
api/kratos/session/v1/session.proto   ← Session 扩展字段（parent_session_id 等）
api/kratos/team/v1/team.proto         ← Team 扩展字段（spirit_session_id / dag_node_id / depends_on 等）
        ↓
internal/service/
  chat_orchestrator_turn.go           ← 精灵 Agent 走 runSingleAgentViaTRPC，通过 spiritCustomTools 注入工具
  spirit_team.go                      ← TeamStarter（生命周期管理）+ SpiritTeamAssembler（组装/查询/取消）
  team_turn_hooks.go                  ← executeTeamTurnViaHooks → HandleTeamTurnResult → 生命周期事件
        ↓
internal/tools/
  spirit_tools.go                     ← plan_and_execute / check_progress / cancel_orchestration / synthesize_results
  orchestrator/build_graph.go         ← build_orchestration_graph（DAG 编排图构建）
        ↓
internal/biz/
  spirit_team_usecase.go              ← AssembleTeam / ListActiveTeams / CancelTeam / AutoArchiveCompletedTeams
  task_planner.go                     ← TaskPlannerPort（Plan 阶段）
  agent_allocator.go                  ← AgentAllocatorPort（Allocate 阶段）
  task_orchestrator.go                ← TaskOrchestratorPort（Orchestrate 阶段）
  spirit_task_dag.go                  ← TaskDAG 拓扑路由（parallel / sequential / hybrid / coordinator）
  spirit_parallel_config.go           ← ParallelConfig（并行配额 + 自动归档）
  session/usecase.go                  ← Session 树查询（ListByParentSessionID）
  team_usecase.go                     ← Create 支持 AutoCreated / SpiritSessionID / ListBySpiritSessionID
        ↓
internal/data/ent/schema/
  team.go                             ← spirit_session_id 索引 idx_teams_spirit_session
        ↓
web/src/
  features/spirit/                    ← 精灵域（api.ts / types.ts / spiritUi.ts）
  stores/spirit/                      ← useSpiritTeamStore
  components/spirit/                  ← 精灵专用组件（9 个）
  components/chat/                    ← Chat 面板扩展
```

**红线**：`internal/biz` 不 import `pkg/trpc-agent-go`；精灵构建仅在 `internal/service`；Team 编译仅在 `internal/team`。

### 1.3 影响域

| 包 | 变更类型 | 说明 |
|----|----------|------|
| `internal/biz/session` | 扩展 | Session 树查询、TaskSummary / TeamDisplayName 字段 |
| `internal/biz/team` | 扩展 | AutoCreated / SpiritSessionID / ListBySpiritSessionID |
| `internal/biz/spirit_team_usecase` | 新增 | 三阶段编排核心逻辑、DAG 拓扑、并行配额、自动归档 |
| `internal/biz/task_planner` | 新增 | Plan 阶段端口接口 |
| `internal/biz/agent_allocator` | 新增 | Allocate 阶段端口接口 |
| `internal/biz/task_orchestrator` | 新增 | Orchestrate 阶段端口接口 |
| `internal/service` | 新增 | spirit_team.go（TeamStarter + SpiritTeamAssembler）、team_turn_hooks.go |
| `internal/tools` | 新增 | spirit_tools.go（三阶段编排工具）、orchestrator/build_graph.go |
| `internal/event` | 扩展 | spirit_team_assembled 等 11 个新 EnvelopeType |
| `internal/data/ent/schema/team` | 扩展 | spirit_session_id 索引 |
| `web/src/features/spirit` | 新增 | 类型、API、UI 工具函数 |
| `web/src/stores/spirit` | 新增 | useSpiritTeamStore |
| `web/src/components/spirit` | 新增 | 9 个新组件 |
| `web/src/components/chat` | 修改 | ChatEntitySidebar 重构、ChatMessagePanel 三模式 |
| `api/kratos/session/v1` | 扩展 | Session Proto 字段（parent_session_id = 50, root_session_id = 51, agent_depth = 52） |
| `api/kratos/team/v1` | 扩展 | Team Proto 字段（spirit_session_id = 15, task_description = 16, auto_created = 17, dag_node_id = 18, depends_on = 19, parallel_config_json = 20） |

**不改动**：`internal/server` 直连 runtime；`internal/data` 除新增字段外无 schema 变更；Team 编译/运行流程不变。

---

## 二、Session 树状模型

### 2.1 数据结构

基于 [system-builtin-agents-design](../superpowers/specs/2026-05-31-system-builtin-agents-design.md) 已定义的 `ParentSessionID` / `RootSessionID` / `AgentDepth`：

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
      │
      └── Team Session B (child)
          └── ...同上
```

### 2.2 Session 扩展字段

| 字段 | 类型 | 存储位置 | 说明 |
|------|------|---------|------|
| `TaskSummary` | `string` | `sessions.metadata_json.task_summary` | 精灵生成的任务摘要 |
| `TeamDisplayName` | `string` | `sessions.metadata_json.team_display_name` | 团队显示名称 |

> **注意**：`ChildSessionIDs`（`metadata_json.child_session_ids`）已在 P0 Review 修复中移除（M2），子 Session 通过 `ListByParentSessionID` 反查即可。

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
| `TaskDescription` | `string` | 任务描述（来自 `plan_and_execute` 调用） |
| `AutoCreated` | `bool` | 是否由精灵自动创建 |
| `DAGNodeID` | `string` | DAG 节点 ID（用于依赖编排） |
| `DependsOn` | `[]string` | 依赖的 DAG 节点 ID 列表 |
| `ParallelConfigJSON` | `string` | 并行配置 JSON |

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
    │   ├── teamUC.Create(ctx, Team{
    │   │     SpiritSessionID: spiritSessionID,
    │   │     TaskDescription: taskDesc,
    │   │     AutoCreated: true,
    │   │     DAGNodeID: nodeID,
    │   │     DependsOn: dependsOn,
    │   │     DefinitionJSON: generatedDefJSON,
    │   │ })
    │   └── sessionUC.Create(ctx, Session{
    │         OwnerType: "team",
    │         TeamID: team.ID,
    │         ParentSessionID: spiritSessionID,
    │         RootSessionID: spiritSessionID,
    │         AgentDepth: parentSession.AgentDepth + 1,
    │       })
    ├── 启动无依赖的团队（TeamStarter.StartTeamTurn）
    └── 发射 spirit_team_assembled / spirit_plan_created / spirit_allocation_created / spirit_orchestration_started 事件
```

### 3.3 DAG 拓扑路由

`TaskDAG.RouteTopology()` 根据任务图结构自动选择拓扑：

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
    MaxConcurrentTeams  int  // 最大并行团队数（默认 3）
    MaxTeamConcurrency  int  // 单团队内最大并发数（默认 2）
    TeamTimeoutSeconds  int  // 团队超时秒数（默认 600）
    AutoArchiveSeconds  int  // 自动归档秒数（默认 3600）
    MaxSessionDepth     int  // 最大会话树深度（默认 2）
}
```

解析：从 Agent 的 `metadata_json.parallel_config` 字段提取。

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

**spirit_team_assembled**：

```go
env.Metadata = map[string]any{
    "team_id":     teamID,
    "team_name":   teamName,
    "session_id":  teamSessionID,
    "members":     memberNames,
    "mode":        mode,
    "task_summary": taskSummary,
}
```

**spirit_team_completed**：

```go
env.Metadata = map[string]any{
    "team_id":        teamID,
    "session_id":     teamSessionID,
    "result_summary": resultSummary,
    "duration_ms":    durationMS,
}
```

**spirit_team_failed**：

```go
env.Metadata = map[string]any{
    "team_id":       teamID,
    "session_id":    teamSessionID,
    "error_message": errMsg,
    "failed_step":   failedStep,
}
```

**spirit_team_progress**：

```go
env.Metadata = map[string]any{
    "team_id":         teamID,
    "status":          status, // running / completed / failed / cancelled / interrupted
    "completed_steps": completedSteps,
    "total_steps":     totalSteps,
}
```

**spirit_teams_all_completed**：

```go
env.Metadata = map[string]any{
    "spirit_session_id": spiritSessionID,
    "total_teams":       totalTeams,
    "completed_teams":   completedTeams,
    "failed_teams":      failedTeams,
}
```

**spirit_plan_created** / **spirit_allocation_created** / **spirit_orchestration_started** / **spirit_orchestration_checkpoint** / **spirit_orchestration_interrupted** / **spirit_synthesis_completed**：载荷由各阶段实现定义，通过 `internal/agent/` 下对应实现发布。

### 4.3 复用现有事件

| EnvelopeType | 来源 | 精灵模式用途 |
|-------------|------|-------------|
| `team_step_started` | Team Runner | 成员开始执行 → 更新成员状态 |
| `team_step_finished` | Team Runner | 成员执行完成 → 更新成员状态 |
| `member_message_start/delta/done` | Team Runner | 成员消息流 → 任务执行面板 |
| `session.status_changed` | SessionStatusPublisher | 团队 Session 状态 → 团队卡片 Badge |
| `orchestration_agent_status` | StatusProjector | Agent 节点实时状态 → 成员树/时间线 |
| `team_run_started/finished/failed` | Team Runner | 团队 Run 生命周期 → 团队卡片状态 |

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

### 5.2 进化数据流

```
Session 执行轨迹
    ├──→ 技能进化：工具调用模式 → Skill 提案 → 技能管家分析
    ├──→ Agent 进化：多 Team 表现 → 能力画像 → tool_weight_json 调整
    ├──→ 记忆：L0-L4 全链路提取 → 记忆管家 dream_cycle 输入
    ├──→ 知识图谱：协作关系 + 产出物 → L4 实体-关系 → GraphRAG
    └──→ 编排进化：模式成功率 + DQ Score → 编排策略优化
```

---

## 六、前端架构

### 6.1 目录结构

```
web/src/features/spirit/
  types.ts                    ← SpiritTeam / SpiritMember / PanelMode / SpiritTeamStatus / SpiritTeamMode / TopologyType / ParallelConfig 等类型
  api.ts                      ← listSpiritTeams / getSpiritTeamDetail / cancelSpiritTeam
  spiritUi.ts                 ← 状态映射和标签函数

web/src/stores/spirit/
  index.ts                    ← useSpiritTeamStore

web/src/components/spirit/
  SpiritEntry.vue             ← 精灵入口卡片
  TeamTaskCard.vue            ← 团队任务卡片（侧边栏用，含展开/折叠）
  TeamProgressCard.vue        ← 团队进度卡片（执行面板用，含进度条/取消按钮/依赖提示）
  TeamAssemblyCard.vue        ← 精灵对话中的团队组建卡片
  TaskExecutionPanel.vue      ← 任务执行面板
  ParallelTeamOverview.vue    ← 并行团队概览（配额进度条 + DAG 图 + 团队列表）
  DAGDiagramCard.vue          ← DAG 依赖图简化文本视图
  SynthesisResultCard.vue     ← 综合结果卡片（合成策略 + 各团队结果）
  OrchestrationModeBadge.vue  ← 编排模式徽章（parallel / sequential / hybrid / coordinator）
```

### 6.2 Store 设计

**useSpiritTeamStore**：

```typescript
interface SpiritTeamState {
  teams: SpiritTeam[]
  expandedTeamIds: Set<string>
  activePanelMode: SpiritPanelMode  // 'spirit' | 'team' | 'member'
  activeTeamId: string | null
  activeMemberId: string | null
  loading: boolean
}

type SpiritTeamStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled' | 'interrupted' | 'archived'
type SpiritTeamMode = 'coordinator' | 'sequential' | 'parallel' | 'critic_loop' | 'swarm' | 'adaptive'
type TopologyType = 'parallel' | 'sequential' | 'hybrid' | 'coordinator'

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

核心 actions：

- `loadSpiritTeams(spiritSessionId)` — 加载精灵下的团队列表
- `reloadTeams()` — 重新加载当前会话的团队
- `selectTeam(teamId)` — 切换到团队执行面板
- `selectMember(memberId)` — 切换到成员只读面板
- `returnToSpirit()` — 返回精灵对话
- `toggleTeamExpand(teamId)` — 展开/折叠团队成员树
- `cancelTeam(teamId)` — 取消团队运行
- `updateTeamProgress(progress)` — 更新团队进度
- `updateTeamStatus(teamId, status)` — 更新团队状态
- `addTeam(team)` — 添加/更新团队（去重）
- `handleSpiritEnvelope(env)` — 处理 WebSocket 事件信封

### 6.3 ChatEntitySidebar 重构

现有 `ChatEntitySidebar.vue` 接收 `agents` / `teams` props，按行业/部门分组展示。

重构为精灵模式：

```vue
<template>
  <div class="spirit-sidebar">
    <SpiritEntry
      :active="panelMode === 'spirit'"
      @click="returnToSpirit"
    />
    <ChatSectionHeader title="进行中的团队" :count="activeTeams.length" />
    <TeamTaskCard
      v-for="team in activeTeams"
      :key="team.id"
      :team="team"
      :expanded="expandedTeamIds.has(team.id)"
      :active="activeTeamId === team.id"
      @click="selectTeam(team.id)"
      @toggle-expand="toggleTeamExpand(team.id)"
    />
    <ChatSectionHeader
      v-if="completedTeams.length"
      title="已完成的团队"
      :count="completedTeams.length"
      collapsible
    />
    <!-- 已完成团队折叠区 -->
  </div>
</template>
```

### 6.4 ChatMessagePanel 三模式

```typescript
type SpiritPanelMode = 'spirit' | 'team' | 'member'
```

| 模式 | 组件 | 输入框 | WS 连接 |
|------|------|--------|---------|
| `spirit` | 标准 ChatMessagePanel | 显示（`!panelMode \|\| panelMode === 'spirit'`） | Spirit Session WS |
| `team` | TaskExecutionPanel | 隐藏 | Team Session WS（订阅） |
| `member` | 占位符（P1 实现 MemberReadOnlyPanel） | 隐藏 | 复用 Team Session WS（过滤） |

### 6.5 面包屑导航

```
精灵 > 后端 API 开发团队 > Golang 工程师
```

> **当前状态**：面包屑导航未实现。当前仅有"返回精灵"按钮。P1 阶段需实现 `useSpiritWorkspace` composable 维护 `breadcrumbItems`。

---

## 七、API 扩展

### 7.1 Session Proto

```protobuf
message Session {
  // 已有字段...
  string parent_session_id = 50;
  string root_session_id = 51;
  int32 agent_depth = 52;
}

message ListChildSessionsRequest {
  string parent_session_id = 1;
}

message ListChildSessionsResponse {
  repeated Session sessions = 1;
}
```

### 7.2 Team Proto

```protobuf
message Team {
  // 已有字段...
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

### 7.3 精灵团队查询 API

> **当前状态**：`ListSpiritTeams` RPC 未在 Proto 中定义。前端通过 `createSpiritService()` 调用 `/v1/spirit/{spiritSessionId}/teams`，但后端未注册对应 HTTP handler。后端通过 `TeamReader.ListBySpiritSessionID()` 在 biz 层实现了查询，但未暴露为 HTTP 端点。P1 阶段需补齐此 API。

```protobuf
message ListSpiritTeamsRequest {
  string spirit_session_id = 1;
  repeated string status_filter = 2; // pending, running, completed, failed, cancelled, interrupted, archived
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

message SpiritMemberView {
  string agent_id = 1;
  string agent_key = 2;
  string display_name = 3;
  string role = 4;
  string status = 5;
}
```

---

## 八、测试策略

| 层 | 文件 | 覆盖 |
|----|------|------|
| Biz | `session_tree_test.go` | Session 树查询、深度限制 |
| Biz | `team_spirit_test.go` | AutoCreated Team 创建、SpiritSessionID 关联 |
| Biz | `spirit_team_usecase_test.go` | AssembleTeam / CancelTeam / AutoArchive / DAG 拓扑路由 |
| Biz | `task_planner_test.go` | Plan 阶段逻辑 |
| Biz | `agent_allocator_test.go` | Allocate 阶段逻辑 |
| Biz | `task_orchestrator_test.go` | Orchestrate 阶段逻辑 |
| Service | `spirit_team_test.go` | AssembleTeam 流程、Envelope 发射、TeamStarter 生命周期 |
| Service | `team_turn_hooks_test.go` | Team Turn 完成回调、生命周期事件发布 |
| 前端 | `useSpiritTeamStore.spec.ts` | 团队列表加载、面板切换、展开状态、事件处理 |
| 前端 | `TaskExecutionPanel.spec.ts` | 三区布局、WS 实时更新 |
| 前端 | `MemberReadOnlyPanel.spec.ts` | 只读模式、输入框隐藏（P1） |

E2E：SP-E2E-01（精灵对话 → 组建团队 → 查看执行面板 → 查看成员 → 返回精灵）。

---

## 九、与关联模块

| 模块 | 关系 |
|------|------|
| 1 Chat | 精灵对话面板、团队组建卡片、任务执行面板 |
| 11 Team | 精灵自动创建 Team、TeamRun 状态追踪 |
| 53 Orchestration | Agent 节点状态投影、执行时间线 |
| 10 Session | Session 树状关联 |
| superpowers Builtin Agents | 精灵/编排管家定义、三阶段编排工具（plan_and_execute / check_progress / cancel_orchestration / synthesize_results / build_orchestration_graph） |
| superpowers Memory/Butler | Session 数据 → 记忆管家/技能管家分析输入 |
| 1 Chat Execution Trace | ChatExecutionCard 复用 |

---

## 十、P0 实施优化记录

> 2026-06-01：P0 全量实施完成后的代码质量优化记录。

### 10.1 后端优化

| 优化项 | 原问题 | 修复方案 |
|--------|--------|----------|
| `biz.SpiritAgentKey` 常量统一 | `spiritAgentKey` 在 `spirit_team.go` 和 `seed_system_admin.go` 各定义一次 | 抽取到 `internal/biz/agent_types.go`，两处统一引用 `biz.SpiritAgentKey` |
| `CompressorDeps` 聚合接口 | `session.Compressor` 接收 7 个独立接口参数，Wire 绑定困难 | 定义 `CompressorDeps` 聚合接口嵌入 7 个子接口，`NewCompressor` 简化为接收 `deps CompressorDeps` |
| `GetRootSession` 循环保护 | 无限循环风险（数据循环引用时） | 增加最大遍历深度限制 `maxDepth = 10` |
| `truncateTaskDesc` rune 截断 | 按字节截断可能截断中文等多字节字符 | 改用 `utf8.RuneCountInString` + `[]rune` 截断 |
| `seed_system_admin.go` kerrors | 3 处使用 `fmt.Errorf` 违反红线 #10 | 改为 `kerrors.InternalServer("SEED", ...)` |
| Proto 缺失字段 | `micro_compact_enabled`/`memory_compact_enabled`/`tool_result_gate_enabled` 未在 proto 定义 | 添加到 `agent.proto` 字段号 121-123 |
| `plugin.NewUsecase` 缺 lg | Wire 注入缺少 `loggateway.Logger` 参数 | 添加 `lg loggateway.Logger` 参数 |
| `ReadinessProbe` Wire 绑定 | `server.ReadinessProbe` 接口未绑定实现 | 添加 `wire.Bind(new(server.ReadinessProbe), new(*data.Data))` |

### 10.2 前端优化

| 优化项 | 原问题 | 修复方案 |
|--------|--------|----------|
| TaskExecutionPanel XSS | `v-html` 渲染未经 sanitize 的内容，`renderMarkdown` 是空壳 | 接入项目已有的 `renderChatMarkdown`（markdown-it + DOMPurify） |
| `archiveTeam` 错误 API | Store 中 `archiveTeam` 调用 `getSpiritTeamDetail` 而非归档 API | P0 阶段改为本地移除（后端归档 API 在 P1 实现） |
| `api.ts` 内联 import | `mapSpiritMember` 使用内联 `import("./types")` | 改为顶部 `import type { SpiritMember } from "./types"` |

---

## 十一、P0.5 三阶段编排实施记录

> 2026-06-06：P0.5 三阶段编排（Plan → Allocate → Orchestrate）实施完成后的记录。

### 11.1 架构演进

| 维度 | P0 设计 | P0.5 实际实现 |
|------|---------|--------------|
| 精灵路由 | `AgentKey == "__spirit__"` 硬编码拦截 | 精灵走 `runSingleAgentViaTRPC`，通过 `spiritCustomTools` 注入工具 |
| 核心工具 | `assemble_team` | `plan_and_execute`（三阶段统一入口） |
| 辅助工具 | `list_butlers` / `query_butler_status` | `check_progress` / `cancel_orchestration` / `synthesize_results` / `build_orchestration_graph` |
| 团队组建 | 路由层无条件组建 | LLM 自主决策是否调用 `plan_and_execute` |
| 拓扑选择 | 硬编码 `coordinator` | `TaskDAG.RouteTopology()` 自动路由 |
| 并行支持 | 无 | `ParallelConfig` + DAG 依赖调度 |
| 结果合成 | 无 | `synthesize_results` 工具 + `SynthesisResultCard` |
| 自动归档 | 无 | `AutoArchiveCompletedTeams`（可配置秒数） |

### 11.2 工具演进

| 工具名 | 状态 | 替代方案 |
|--------|------|---------|
| `assemble_team` | DEPRECATED | `plan_and_execute` |
| `assess_complexity` | DEPRECATED | `plan_and_execute`（Plan 阶段内含复杂度评估） |
| `check_team_progress` | DEPRECATED | `check_progress` |
| `cancel_team` | DEPRECATED | `cancel_orchestration` |
| `plan_and_execute` | 活跃 | — |
| `check_progress` | 活跃 | — |
| `cancel_orchestration` | 活跃 | — |
| `synthesize_results` | 活跃 | — |
| `build_orchestration_graph` | 活跃 | — |

### 11.3 已知技术债

| 编号 | 问题 | 位置 | 优先级 |
|------|------|------|--------|
| TD-1 | 前端 `api.ts` 双键名兼容（`raw.teamName ?? raw.team_name`） | `web/src/features/spirit/api.ts` | P1 |
| TD-2 | `ListSpiritTeams` RPC 未暴露为 HTTP 端点 | `api/kratos/team/v1/team.proto` | P1 |
| TD-3 | `ArchiveTeam` RPC 未定义，归档仅后端自动触发 | `api/kratos/team/v1/team.proto` | P1 |
| TD-4 | `MemberReadOnlyPanel` 仅有占位符 | `web/src/components/chat/ChatMessagePanel.vue` | P1 |
| TD-5 | `TeamMemberTreeNode` 未实现，仅有扁平头像列表 | `web/src/components/spirit/` | P1 |
| TD-6 | 面包屑导航未实现 | — | P1 |
| TD-7 | 重试失败团队功能未实现 | — | P1 |
