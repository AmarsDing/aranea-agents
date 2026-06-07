# Multi-Agent Team 编排模块 — 实现设计文档

> 对应需求：[11 multi-agent.md](./11%20multi-agent.md)
> 遵循规范：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md) · 运行时边界：[AGENT_RUNTIME_BOUNDARY.md](../AGENT_RUNTIME_BOUNDARY.md)
> **实现差距与迭代计划**以 [11-multi-agent-development.md](./11-multi-agent-development.md) 为准
> **M53 编排融合**：[53-team-graph-orchestration.design.md](./53-team-graph-orchestration.design.md) — Graph 编译、观测台、HITL、容错

---

## 一、模块概述

Team 多智能体编排：将多个 Agent 按 Definition JSON 组装为 Graph 运行时（默认）或 trpc-agent-go Team / Chain / Parallel / Cycle / Swarm 运行时（应急），经 Chat Service 桥点执行，事件经 EventBus 投影为 WS Envelope。

### 分层与依赖

```
api/kratos/team/v1/team.proto     ← 对外契约（20 RPC）
        ↓
internal/service/team.go          ← Proto ↔ biz；CRUD + RunTeamTest / CancelTeamRun
internal/service/team_observatory.go  ← 观测台 RPC
internal/service/team_compile.go      ← 编译预览 RPC
internal/service/team_resume.go       ← HITL / Checkpoint 恢复
internal/service/team_dead_letter.go  ← 死信队列 RPC
internal/service/chat_native.go   ← Team Session Turn → team.Runner
        ↓
internal/biz/team_usecase.go      ← 领域校验与 CRUD（禁止 import trpc-agent-go）
internal/biz/team_types.go        ← 领域模型 + 窄接口定义
internal/biz/team_summary.go      ← 运行汇总聚合
        ↓
internal/data/team_repo.go        ← Ent ORM 持久化
        ↓
internal/team/                    ← 框架运行时组装（definition / graph_compile / runner / status_projector）
        ↓
pkg/trpc-agent-go/team            ← 框架 Team / Swarm 真相源（Native 应急路径）
```

**红线**：`internal/biz` 不 import `pkg/trpc-agent-go`；Team 构建与 `Run` 仅在 `internal/team` + `internal/service`。

### 编排模式映射

| Definition `mode` | Graph 编译产物 | Native 组件（Deprecated） | 说明 |
|-------------------|---------------|--------------------------|------|
| `sequential` | `CompileToGraphBuildConfig` → 线性图 | `chainagent.New` | 顺序链，事件传递给下一成员 |
| `parallel` | `CompileToGraphBuildConfig` → 并行图 | `parallelagent.New` | 并行 worker，需 synthesizer 成员或 `synthesizer_agent_id` |
| `coordinator` | `CompileToGraphBuildConfig` → 星形图 | `trpcteam.New` | 首成员为 coordinator，其余为 AgentTool |
| `critic_loop` | `CompileToGraphBuildConfig` → 循环图 | `cycleagent.New` | 生成-评审循环 + `escalationFunc` |
| `swarm` | `CompileToGraphBuildConfig` → 全连接图 | `trpcteam.NewSwarm` | 成员间 `transfer_to_agent` |
| `adaptive` | 与 swarm 相同编译路径 | `trpcteam.NewSwarm` | 与 swarm 相同构建路径，UI 面向用户的 Swarm 别名 |

前端 `modeOptions` 展示 `adaptive` 而非 `swarm`；API / 校验层两者均合法。

### 核心执行流

> **M53 Phase 7**：默认 Graph 路径；Native 见 §6.1 应急说明。

```
用户消息 → ChatService (owner_type=team)
             ↓
         team.Runner.runTeamTRPC()
             ↓
         CompileToGraphRuntimeConfig(def) → GraphAgent
             ↓
         graph_node_start → TeamGraphTaskBridge（task/review 节点建 Task）
             ↓
         StatusProjector → member_* / team_* / graph_* Envelope → EventBus → /v1/ws
             ↓
         persistStep + UpdateTeamRun + team_summary
```

应急 Native（`ARANEA_TEAM_NATIVE=1` 或 Graph 构建失败且显式启用）：

```
BuildTRPCTeam(def) → trpc-agent-go Agent → ConsumeEventStream → EventProjector
```

---

## 二、Proto 层

文件：`api/kratos/team/v1/team.proto`

### 2.1 核心消息

| 消息 | 用途 |
|------|------|
| `Team` | Team 实体 |
| `TeamRun` | 运行记录 |
| `TeamRunStep` | 成员步骤 |
| `TeamRunSummary` / `TeamRunMemberSummary` | 运行汇总 |
| `StructureNode` / `StructureEdge` / `StructureSurface` | 结构导出 |
| `OrchestrationSpec` / `OrchestrationMember` | 编排规格 v2 |
| `EmbeddedGraph` / `EmbeddedGraphNode` / `EmbeddedGraphEdge` | 内嵌图定义 |
| `FailurePolicySpec` / `TeamRetryPolicySpec` / `CircuitBreakerSpec` | 容错策略 |
| `ActivitySnapshotView` / `AgentNodeStateView` | 观测台快照 |
| `ActivityTimelineRow` | 观测台时间线 |
| `CompileTeamGraphRequest/Response` | 编译预览 |
| `TaskDeadLetter` | 死信 |

### 2.2 RPC 一览

| RPC | HTTP | 用途 |
|-----|------|------|
| `ListTeams` | `GET /v1/teams` | 列表 |
| `CreateTeam` | `POST /v1/teams` | 创建 |
| `GetTeam` | `GET /v1/teams/{id}` | 详情 |
| `UpdateTeam` | `PATCH /v1/teams/{id}` | 更新 |
| `DeleteTeam` | `DELETE /v1/teams/{id}` | 软删除 |
| `DuplicateTeam` | `POST /v1/teams/{id}/duplicate` | 复制 |
| `ListTeamRuns` | `GET /v1/team-runs` | 运行列表 |
| `GetTeamRun` | `GET /v1/team-runs/{id}` | 运行详情 |
| `CancelTeamRun` | `POST /v1/team-runs/{id}/cancel` | 取消（RunRegistry + run_status） |
| `ListTeamRunSteps` | `GET /v1/team-runs/{run_id}/steps` | 步骤列表 |
| `UpdateSwarmMembers` | `POST /v1/teams/{team_id}/swarm-members` | 动态成员 |
| `ExportTeamStructure` | `GET /v1/teams/{team_id}/structure` | 拓扑导出 |
| `RunTeamTest` | `POST /v1/teams/{id}/run-test` | 测试运行（临时 Session） |
| `GetTeamRunSummary` | `GET /v1/team-runs/{id}/summary` | 结构化汇总（含 tool_call_count） |
| `GetTeamRunObservatory` | `GET /v1/team-runs/{run_id}/observatory` | 运行观测台快照 |
| `GetTeamRunObservatoryTimeline` | `GET /v1/team-runs/{run_id}/observatory/timeline` | 观测台时间线 |
| `ResumeTeamRunExecution` | `POST /v1/team-runs/{run_id}/resume` | HITL / Checkpoint 恢复 |
| `CompileTeamGraph` | `POST /v1/teams/{team_id}/compile-graph` | 编译预览 |
| `ListTaskDeadLetters` | `GET /v1/task-dead-letters` | 死信列表 |
| `ResolveTaskDeadLetter` | `POST /v1/task-dead-letters/{id}/resolve` | 解决死信 |

---

## 三、Biz 层

### 3.1 领域模型

文件：`internal/biz/team_types.go`

```go
type Team struct {
    ID                string
    TeamKey           string
    DisplayName       string
    Status            string   // pending | running | completed | failed | cancelled | interrupted | archived | blocked
    IsDefault         bool
    Kind              string   // user | system_builtin | ecosystem_preset | marketplace | certified
    Source            string   // user | system | imported
    Readonly          bool
    DefinitionJSON    string
    ADKAppName        string
    DagNodeID         string
    DependsOn         []string
    ParallelConfigJSON string
    CreatedAt         string
    UpdatedAt         string
    DeletedAt         string
}

type TeamRun struct {
    ID                      string
    TeamID                  string
    SessionID               string
    MessageID               string
    Mode                    string
    Status                  string   // pending | running | success | failed | cancelled | waiting_human
    InputPreview            string
    OutputPreview           string
    TokenIn                 int
    TokenOut                int
    CostMicroUSD            int64
    DurationMS              int
    ErrorMessage            string
    TopologyJSON            string
    GraphExecutionID        string
    DefinitionSnapshotJSON  string
    TraceID                 string
    StartedAt               string
    FinishedAt              string
    CreatedAt               string
    UpdatedAt               string
}

type TeamRunStep struct {
    ID            string
    RunID         string
    TeamID        string
    AgentID       string
    AgentKey      string
    AgentName     string
    Role          string
    SortOrder     int
    Status        string
    InputPreview  string
    OutputPreview string
    TokenIn       int
    TokenOut      int
    CostMicroUSD  int64
    DurationMS    int
    ToolCallCount int
    ErrorMessage  string
    StartedAt     string
    FinishedAt    string
    CreatedAt     string
}

type TeamStructureSnapshot struct {
    EntryNodeID string
    Nodes       []StructureNode
    Edges       []StructureEdge
    Surfaces    []StructureSurface
}

type TeamRunSummaryData struct { /* ... */ }
type TeamRunMemberSummaryData struct { /* ... */ }
type OrchestrationDecision string  // approved | rejected | needs_revision | escalate
```

### 3.2 窄接口（已拆分）

文件：`internal/biz/team_usecase.go`

```go
type TeamReader interface {
    ListTeams(ctx context.Context) ([]Team, error)
    ListTeamsByStatus(ctx context.Context, status string) ([]Team, error)
    GetTeamByID(ctx context.Context, id string) (Team, error)
    GetTeamByKey(ctx context.Context, key string) (Team, error)
    ListBySpiritSessionID(ctx context.Context, sessionID string) ([]Team, error)
}

type TeamWriter interface {
    CreateTeam(ctx context.Context, t Team) (Team, error)
    UpdateTeam(ctx context.Context, t Team) (Team, error)
    DeleteTeam(ctx context.Context, id string) error
    BatchArchiveTeams(ctx context.Context, ids []string) error
}

type TeamRunReader interface {
    ListTeamRuns(ctx context.Context, teamID string, limit int) ([]TeamRun, error)
    ListTeamRunsByTeamIDs(ctx context.Context, teamIDs []string, limit int) ([]TeamRun, error)
    HasActiveTeamRun(ctx context.Context, teamID string) (bool, error)
    GetTeamRunByID(ctx context.Context, id string) (TeamRun, error)
    ListTeamRunSteps(ctx context.Context, runID string) ([]TeamRunStep, error)
}

type TeamRunWriter interface {
    CreateTeamRun(ctx context.Context, r TeamRun) (TeamRun, error)
    UpdateTeamRun(ctx context.Context, r TeamRun) error
    UpdateTeamRunGraphExecutionID(ctx context.Context, id, graphExecID string) error
    UpdateTeamRunTraceID(ctx context.Context, id, traceID string) error
    UpdateTeamRunSummaryJSON(ctx context.Context, id, summaryJSON string) error
    CreateTeamRunStep(ctx context.Context, s TeamRunStep) (TeamRunStep, error)
}

type OrchestrationStepRepo interface {
    BatchCreateOrchestrationSteps(ctx context.Context, steps []OrchestrationStep) error
    ListOrchestrationSteps(ctx context.Context, runID, nodeID string) ([]OrchestrationStep, error)
}

type TaskDeadLetterRepo interface {
    CreateTaskDeadLetter(ctx context.Context, dl TaskDeadLetter) error
    ListTaskDeadLetters(ctx context.Context, req ListTaskDeadLettersReq) ([]TaskDeadLetter, error)
    ResolveTaskDeadLetter(ctx context.Context, id, resolution string) error
}

// TeamRepository（Deprecated 聚合接口）组合以上所有窄接口
type TeamRepository interface {
    TeamReader
    TeamWriter
    TeamRunReader
    TeamRunWriter
    OrchestrationStepRepo
    TaskDeadLetterRepo
}
```

### 3.3 Definition 结构

文件：`internal/team/definition.go`

```go
type Definition struct {
    Version             int               `json:"version"`
    Mode                string            `json:"mode"`
    SynthesizerAgentID  string            `json:"synthesizer_agent_id"`
    Members             []MemberDef       `json:"members"`
    MaxConcurrency      int               `json:"max_concurrency"`
    TimeoutSeconds      int               `json:"timeout_seconds"`
    LoopMaxIterations   int               `json:"loop_max_iterations,omitempty"`
    CriticLoop          *CriticLoopConfig `json:"critic_loop,omitempty"`
    IntentAnchorAgentID string            `json:"intent_anchor_agent_id,omitempty"`
    Swarm               *SwarmConfigDef   `json:"swarm,omitempty"`
    MemberTool          *MemberToolDef    `json:"member_tool_config,omitempty"`
    Graph               *EmbeddedGraphDef `json:"graph,omitempty"`
    FailurePolicy       *FailurePolicy    `json:"failure_policy,omitempty"`
    CircuitBreaker      *CircuitBreakerPolicyDef `json:"circuit_breaker,omitempty"`
}
```

### 3.4 校验规则（validateTeamDefinition）

- `mode` ∈ sequential / parallel / coordinator / critic_loop / swarm / adaptive
- 至少一个 enabled member；member `agent_id` 非空
- parallel：需 synthesizer 成员或 `synthesizer_agent_id`
- critic_loop：需 generator + critic 成员
- 默认 Team（`IsDefault=true`）不可删除
- UpdateSwarmMembers：仅 swarm / adaptive

---

## 四、Data 层

文件：`internal/data/team_repo.go` · Ent Schema：`internal/data/ent/schema/team.go` 等

| 表 | 关键字段 |
|----|----------|
| `teams` | id, team_key (unique), display_name, status, is_default, kind, source, readonly, definition_json, adk_app_name, deleted_at |
| `team_runs` | id, team_id, session_id, message_id, mode, status, token_*, cost_micro_usd, duration_ms, topology_json, graph_execution_id, trace_id |
| `team_run_steps` | id, run_id, team_id, agent_id, agent_key, role, sort_order, status, token_*, tool_call_count, duration_ms |
| `orchestration_steps` | id, run_id, node_id, agent_key, status, started_at, finished_at |
| `task_dead_letters` | id, task_id, run_id, node_id, error_message, resolution |

---

## 五、Service 层

文件：`internal/service/team.go`（主文件）+ 分文件

### 5.1 依赖注入

```go
type TeamService struct {
    uc         *biz.TeamUsecase
    sessions   *biz.SessionUsecase      // RunTeamTest 临时 Session
    teamRunner *team.Runner             // RunTeamTest → RunTurn
    runs       *rt.RunRegistry          // CancelTeamRun
    eventBus   event.Bus                // Cancel → run_status Envelope
}
```

### 5.2 RPC 实现分布

| 文件 | RPC |
|------|-----|
| `team.go` | ListTeams, CreateTeam, GetTeam, UpdateTeam, DeleteTeam, DuplicateTeam, ListTeamRuns, GetTeamRun, CancelTeamRun, RunTeamTest, ListTeamRunSteps, UpdateSwarmMembers, ExportTeamStructure, GetTeamRunSummary |
| `team_observatory.go` | GetTeamRunObservatory, GetTeamRunObservatoryTimeline |
| `team_compile.go` | CompileTeamGraph |
| `team_resume.go` | ResumeTeamRunExecution |
| `team_dead_letter.go` | ListTaskDeadLetters, ResolveTaskDeadLetter |

### 5.3 关键行为

| RPC | 实现要点 |
|-----|----------|
| `RunTeamTest` | 创建 `owner_type=team` 临时 Session → `teamRunner.RunTurn` → 查最近 TeamRun → defer 删除 Session |
| `CancelTeamRun` | 校验 running/pending → `RunRegistry.Cancel(sessionID)` → `CancelSessionRunSideEffects` → 更新 status=cancelled |
| `ExportTeamStructure` | 经编译器 `exportStructureViaCompiler` 生成拓扑 |
| `CompileTeamGraph` | 调用 `CompileToCompiledTeam` 返回编译后节点/边/校验问题 |
| `ResumeTeamRunExecution` | HITL 审核后恢复 Graph 运行 |
| `GetTeamRunObservatory` | 返回 ActivitySnapshot + AgentNodeState 快照 |
| 其余 CRUD | 直接委托 TeamUsecase |

错误映射：`sql.ErrNoRows` → `kerrors.NotFound("TEAM", ...)`。

---

## 六、Team 运行时

### 6.1 构建

| 路径 | 文件 | 说明 |
|------|------|------|
| **Graph（默认）** | `graph_compile.go` · `graph_runtime_config.go` · `embedded_graph.go` | `CompileToGraphRuntimeConfig` → `GraphAgent`；embedded `agent`/`task`/`review`/`subgraph`/`function` 节点 |
| **Native（应急）** | `trpc_build.go` | `BuildTRPCTeam` **Deprecated**；仅 `ARANEA_TEAM_NATIVE=1` |

Graph 路径要点：

- `runner_team_trpc.go`：Graph 优先；编译/构建失败 **不 silent fallback**
- `team_graph_task_bridge.go` + `task_creator.go`：`graph_node_start` 时为 task/review 节点创建 Task（经 `ChatService` 注入 `TaskUsecase`）
- `ResumeTeamRunExecution`：Graph checkpoint / HITL resume（见 M53 Phase 6）
- `graph_runtime.go`：Graph 运行时执行路径
- `graph_runtime_canary.go`：灰度控制
- `status_projector.go`：WS 状态投影（`orchestration_agent_status` 等）
- `activity_step_flusher.go`：Activity 步骤刷盘
- `runner_mediator.go`：Mediator 模式（HITL defer）
- `team_graph_run_coordinator.go`：Graph 运行协调器
- `team_graph_run_finisher.go`：Graph 运行终结器
- `team_graph_run_context.go`：Graph 运行上下文
- `team_graph_execution_tracker.go`：执行追踪器
- `fallback_policy.go`：Native fallback 策略
- `template_registry.go`：Mode 模板注册
- `compile_snapshot.go`：编译快照

Native 应急（`trpc_build.go`）：

- `BuildTRPCTeam(ctx, def, deps, lookupAgent)`：按 mode 分发
- `buildSwarmOptions`：SwarmConfig + CrossRequestTransfer + SwarmHandoffInputBuilder
- `buildCoordinatorOptions`：MemberToolConfig 映射
- `buildEscalationFunc`：CriticLoop ScoreThreshold + approved 关键字

成员 Agent 经 `chatagent.BuildTRPCAgent( Cached )` 构建，deps 含 `PluginsForAgent`、有效工具集。

### 6.2 执行

文件：`internal/team/runner_team_trpc.go` · `internal/team/runner_helpers.go`

- 创建 TeamRun（status=running）→ 发射 `team_run_started`
- `TraceEmitter`：`team.run.start` / `team.run.execute` / `team.run.finish`（FlowLogger）
- `ConsumeEventStream` + `ProjectMeta.MemberAgentKeys` → `member_message_start` / `member_delta` / `member_message_done`
- `persistStep` → `team_step_finished` + Usage `team_member`
- 成功/失败 → `team_run_finished` / `team_run_failed` + `publishTeamRunSummary`

### 6.3 运行汇总

| 层级 | 文件 | 职责 |
|------|------|------|
| Biz | `internal/biz/team_summary.go` | `BuildTeamRunSummaryData` — 单一聚合源 |
| Biz | `internal/biz/team_usecase.go` | `GetRunSummary` — 读路径收拢（GetRun + ListRunSteps） |
| Runtime | `internal/team/summary.go` | `BuildTeamRunSummary` / `SummaryMapFromData` — WS `team_summary` |
| Service | `internal/service/team.go` | `toProtoTeamRunSummary` — RPC 映射 |

```go
// biz
func BuildTeamRunSummaryData(run biz.TeamRun, steps []biz.TeamRunStep) biz.TeamRunSummaryData
func (u *TeamUsecase) GetRunSummary(ctx context.Context, runID string) (biz.TeamRunSummaryData, error)

// team（WS）
func BuildTeamRunSummary(run biz.TeamRun, steps []biz.TeamRunStep) map[string]any
func TeamSummaryEnvelope(run biz.TeamRun, steps []biz.TeamRunStep) event.Envelope

// service（RPC）
func toProtoTeamRunSummary(data biz.TeamRunSummaryData) *v1.TeamRunSummary
```

**汇总字段**（run 级 + 每成员）：

| 字段 | run | member | 说明 |
|------|-----|--------|------|
| `run_id` / `team_id` / `session_id` / `mode` / `status` | ✅ | — | 来自 `TeamRun` |
| `token_in` / `token_out` / `cost_micro_usd` / `duration_ms` | ✅ | ✅ | Usage 聚合 |
| `tool_call_count` | ✅（求和） | ✅ | TEAM-04，`MemberToolCalls` 落 step 后汇总 |
| `member_count` | ✅ | — | `len(steps)` |
| `output_preview` | ✅（512） | ✅（256） | 截断预览 |
| `error_message` | ✅ | — | run 级错误 |
| `agent_id` / `agent_key` / `agent_name` / `role` / `sort_order` / `status` | — | ✅ | 成员身份 |

**已有库迁移**：`docs/sql/03_session_team_run_steps_tool_call_count.sql`

**一致性保障**：`internal/service/team_summary_parity_test.go` 断言 WS map 与 RPC proto 字段对齐。

### 6.4 call_agent 工具

- 定义：`internal/a2a/tool.go` → `NewCallAgentTool()`
- 注入：`internal/tools/trpc/toolsets.go` 在 `cfg.CallAgent == true` 时挂载
- 开关：`internal/agent/trpc_build.go` 读取 Agent 有效工具集 `biz.ToolKeyCallAgent`
- 执行：`internal/service/chat_native.go` 实现 `a2a.AgentTurnRunner`

---

## 七、WS / EventBus 事件模型

主链路：`internal/event` + `internal/server/ws.go`。Team 相关 Envelope 类型（`internal/event/envelope.go`）：

| EnvelopeType | 发射时机 |
|--------------|----------|
| `team_run_started` | Run 创建后 |
| `team_run_finished` | Run 成功结束 |
| `team_run_failed` | Run 失败 |
| `team_step_started` | 成员步骤开始 |
| `team_step_finished` | 每成员 step 持久化后 |
| `team_summary` | Run 结束（成功/失败）后聚合 steps |
| `member_message_start` | 子 Agent 首次输出（EventProjector） |
| `member_delta` | 子 Agent 流式增量 |
| `member_message_done` | 子 Agent 输出完成 |
| `intent_pass` | 意图传递 |
| `transfer` | Swarm handoff |
| `run_status` | CancelTeamRun 取消 |
| `orchestration_agent_status` | Agent 节点状态变更（StatusProjector） |
| `graph_node_start` | Graph 节点开始执行 |
| `graph_node_finish` | Graph 节点执行完成 |

前端映射：

- `web/src/features/teams/teamRunEventFromEnvelope.ts` — TeamRunsDialog / Monitor
- `web/src/realtime/useEnvelopeStream.ts` — WS Envelope 流
- `web/src/features/chat/composables/useChatStreamManager.ts` — Team 分栏

---

## 八、前端层

### 页面

| 路径 | 职责 |
|------|------|
| `web/src/pages/TeamsPage.vue` | Team 管理主页（列表/筛选/编辑/运行轨迹/测试/行业分组/拖拽排序） |
| `web/src/pages/TeamOrchestratePage.vue` | 编排画布页（Graph 编辑/节点面板/运行时面板/成员看板/实时运行） |
| `web/src/pages/TeamRunObservatoryPage.vue` | 运行观测台（Agent 看板/Timeline/Summary/HITL/任务看板） |

### 组件

| 路径 | 职责 |
|------|------|
| `web/src/components/teams/TeamCard.vue` | 卡片：模式、成员、操作（编排/观测/测试/Chat/轨迹/复制/编辑/删除） |
| `web/src/components/teams/TeamToolbar.vue` | 搜索 / 模式 / 状态 / 行业筛选 |
| `web/src/components/teams/TeamEditorDialog.vue` | 新建 / 编辑 / 模板（含编译预览侧栏） |
| `web/src/components/teams/TeamRunsDialog.vue` | 运行记录 + WS 实时 |
| `web/src/components/teams/TeamTestDialog.vue` | 运行测试 |
| `web/src/components/teams/TeamCompilePreview.vue` | 编译预览侧栏（CompileTeamGraph API） |
| `web/src/components/teams/TeamMemberKanban.vue` | 成员看板（按角色分列） |
| `web/src/components/teams/TeamOrchestrateNodePanel.vue` | 编排页节点详情面板 |
| `web/src/components/teams/TeamOrchestrateRuntimePanel.vue` | 编排页运行时/容错面板 |
| `web/src/components/teams/teamUtils.ts` | 模板、modeOptions、Definition 默认值、校验 |
| `web/src/components/teams/teamConstants.ts` | 状态/模式/角色/模板/运行时引擎/失败策略常量 |
| `web/src/components/teams/teamTemplates.ts` | 四种模板工厂 |

### Features

| 路径 | 职责 |
|------|------|
| `web/src/features/teams/api.ts` | Kratos Client + Envelope 订阅 + 全部 API 调用 |
| `web/src/features/teams/types.ts` | Team / TeamDefinition / TeamRun / TeamRunStep / TeamRunSummary / TaskDeadLetter 类型 |
| `web/src/features/teams/teamRunEventFromEnvelope.ts` | WS Envelope → TeamRunEvent 映射 |
| `web/src/features/teams/graphUtils.ts` | 根据 mode 生成 graph 节点/边 |
| `web/src/features/teams/orchestrationSpec.ts` | OrchestrationSpec v2 类型与转换 |
| `web/src/features/teams/useTeamsPage.ts` | TeamsPage composable |
| `web/src/features/teams/useTeamOrchestratePage.ts` | TeamOrchestratePage composable |
| `web/src/features/teams/useTeamRunObservatoryPage.ts` | TeamRunObservatoryPage composable |

### Store

| 路径 | 职责 |
|------|------|
| `web/src/stores/teams/index.ts` | Pinia 状态（teams / runs / steps / summaries / WS 事件更新） |

### Chat 集成

| 路径 | 职责 |
|------|------|
| `web/src/components/chat/ChatTeamMemberStrip.vue` | Chat 子 Agent 流式状态 chip |

数据流：`TeamsPage` → `features/teams/api` → `services/kratos/team/v1`；实时经 `createEnvelopeStream` + `GLOBAL_WS_SESSION_ID`。

---

## 九、与关联模块

| 模块 | 关系 |
|------|------|
| Chat (M1) | Team Session Turn 入口；共享 RunGateway / RunRegistry |
| Agent (M2–8) | 成员 Agent 构建；call_agent 依赖 A2A 工具启用 |
| Session (M10) | owner_type=team；RunTeamTest 临时 Session |
| Monitor (M18) | EventTimeline 订阅 Team Envelope |
| Usage/Token (M29) | `team_turn` / `team_member` usage_kind |
| A2A (M26) | call_agent 远程 Invoke |
| Graph (M36) | Team 编译为 Graph 运行时；embedded graph 定义；CompileTeamGraph / ResumeTeamRunExecution |
| Task (M53) | task/review 节点创建 Task；TaskDeadLetter 死信队列 |
| Spirit/Pack | Team Kind/Source 分类；ResolveMemberAgentKeys / SaveTeamWithGraph 导入 |

---

## 十、测试

### Service 层

| 文件 | 覆盖 |
|------|------|
| `internal/service/team_test.go` | CRUD |
| `internal/service/team_run_test.go` | Run 管理 |
| `internal/service/team_cancel_test.go` | CancelTeamRun + run_status Envelope |
| `internal/service/team_compile_test.go` | CompileTeamGraph |
| `internal/service/team_compile_view_test.go` | 编译视图 |
| `internal/service/team_dead_letter_test.go` | 死信 |
| `internal/service/team_observatory_test.go` | 观测台 |
| `internal/service/team_summary_parity_test.go` | WS map ↔ RPC proto 字段 parity |
| `internal/service/team_orchestration_spec_test.go` | OrchestrationSpec |
| `internal/service/service_team_mapping_test.go` | 映射 |

### Biz 层

| 文件 | 覆盖 |
|------|------|
| `internal/biz/team_usecase_test.go` | Usecase 主逻辑 |
| `internal/biz/team_usecase_delete_test.go` | 删除逻辑 |
| `internal/biz/team_types_test.go` | 类型/状态转换 |
| `internal/biz/team_summary_test.go` | Summary 聚合 |
| `internal/biz/team_modes_test.go` | 六种 mode |
| `internal/biz/team_fallback_test.go` | Fallback 策略 |
| `internal/biz/team_graph_function_resolver_test.go` | Function 节点解析 |
| `internal/biz/team_graph_linked_test.go` | Linked graph |
| `internal/biz/graph_team_execution_test.go` | Graph 执行 |
| `internal/biz/spirit_team_usecase_test.go` | Spirit 集成 |

### Runtime 层

| 文件 | 覆盖 |
|------|------|
| `internal/team/graph_compile_test.go` | 编译器 |
| `internal/team/graph_runtime_test.go` | Graph 运行时 |
| `internal/team/graph_runtime_e2e_test.go` | E2E |
| `internal/team/graph_runtime_canary_test.go` | 灰度 |
| `internal/team/embedded_graph_test.go` | Embedded graph |
| `internal/team/compile_snapshot_test.go` | 编译快照 |
| `internal/team/definition_test.go` | Definition 解析 |
| `internal/team/status_projector_test.go` | 状态投影 |
| `internal/team/summary_test.go` | Summary |
| `internal/team/team_graph_task_bridge_test.go` | Task bridge |
| `internal/team/team_graph_run_coordinator_test.go` | 运行协调 |
| `internal/team/team_graph_run_finisher_test.go` | 运行终结 |
| `internal/team/trpc_build_test.go` | TRPC 构建 |
| `internal/team/parity_test.go` / `parity_run_test.go` | Native vs Graph 对比 |

Runner 端到端集成测试见开发计划 EP-TEST-01。
