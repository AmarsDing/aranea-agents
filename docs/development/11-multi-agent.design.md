# Multi-Agent Team 编排模块 — 实现设计文档

> 对应需求：[11 multi-agent.md](./11%20multi-agent.md)
> 遵循规范：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)
> **实现差距与迭代计划**以 [11-multi-agent.development.md](./11-multi-agent.development.md) 为准
> **M53 编排融合**：[53-team-graph-orchestration.design.md](./53-team-graph-orchestration.design.md) — Graph 编译、观测台、HITL、容错

---

## 一、模块概述

Team 多智能体编排：将多个 Agent 按 Definition JSON 组装为 Graph 运行时，经 Chat Service 桥点执行，事件经 EventBus 投影为 WS Envelope。

### 分层与依赖

```
api/kratos/team/v1/team.proto     ← 对外契约（24 RPC）
        ↓
internal/service/team.go          ← Proto ↔ biz；CRUD + RunTeamTest / CancelTeamRun
internal/service/team_observatory.go  ← 观测台 RPC
internal/service/team_compile.go      ← 编译预览 RPC
internal/service/team_compile_view.go ← 编译视图辅助
internal/service/team_resume.go       ← HITL / Checkpoint 恢复
internal/service/team_dead_letter.go  ← 死信队列 + Spirit 集成 RPC
internal/service/team_orchestration_spec.go ← OrchestrationSpec Proto 映射
internal/service/team_run_registry_adapter.go ← RunRegistry 适配
internal/service/team_runner_wire_adapter.go   ← Runner 适配 + Mediator + Coord
internal/service/team_turn_hooks.go            ← ChatOrchestrator Team hooks
internal/service/chat_native.go   ← Team Session Turn → team.Runner
        ↓
internal/biz/team_usecase.go      ← 领域校验与 CRUD（禁止 import trpc-agent-go）
internal/biz/team_types.go        ← 领域模型 + 状态常量
internal/biz/team_ports.go        ← biz 层 port 接口（TeamTurnRuntime / TeamMediatorPort 等）
internal/biz/team_interfaces.go   ← Team 层窄接口（TeamUsageQuerier / TeamAgentLookup 等）
internal/biz/team_state_machine.go    ← Team 状态机（AS-FSM-01）
internal/biz/team_run_state_machine.go ← TeamRun 状态机（AS-FSM-01）
internal/biz/team_summary.go      ← 运行汇总聚合
internal/biz/team_fallback.go     ← Fallback 策略
internal/biz/team_compiler.go     ← 编译器 biz 适配
internal/biz/team_graph.go        ← Graph 集成
internal/biz/team_graph_constants.go ← Graph 常量
internal/biz/team_graph_plugin.go    ← Graph 插件
internal/biz/team_graph_knowledge.go ← Graph 知识
internal/biz/team_agent_ports.go  ← Agent 依赖端口
        ↓
internal/data/team_repo.go        ← Ent ORM 持久化
internal/data/team_graph_session_repo.go ← Graph Session 持久化
internal/data/team_graph_session_schema.go ← Graph Session DDL
        ↓
internal/team/                    ← 框架运行时组装（definition / graph_compile / runner / status_projector）
        ↓
pkg/trpc-agent-go/team            ← 框架 Team / Swarm 真相源
```

**红线**：`internal/biz` 不 import `pkg/trpc-agent-go`；Team 构建与 `Run` 仅在 `internal/team` + `internal/service`。

### 编排模式映射

| Definition `mode` | Graph 编译产物 | 说明 |
|-------------------|---------------|------|
| `sequential` | `CompileToGraphBuildConfig` → 线性图 | 顺序链，事件传递给下一成员 |
| `parallel` | `CompileToGraphBuildConfig` → 并行图 | 并行 worker，需 synthesizer 成员或 `synthesizer_agent_id` |
| `coordinator` | `CompileToGraphBuildConfig` → 星形图 | 首成员为 coordinator，其余为 AgentTool |
| `critic_loop` | `CompileToGraphBuildConfig` → 循环图 | 生成-评审循环 + `escalationFunc` |
| `swarm` | `CompileToGraphBuildConfig` → 全连接图 | 成员间 `transfer_to_agent` |
| `adaptive` | 与 swarm 相同编译路径 | 与 swarm 相同构建路径，UI 面向用户的 Swarm 别名 |

前端 `modeOptions` 展示 `adaptive` 而非 `swarm`；API / 校验层两者均合法。

### 核心执行流

```
用户消息 → ChatService (owner_type=team)
             ↓
         team.Runner.runTeamTRPCFromInput()
             ↓
         compileTeamRuntime() → CompileToGraphRuntimeConfigFromJSON(def) → GraphAgent
             ↓
         graph_node_start → TeamGraphTaskBridge（task/review 节点建 Task）
             ↓
         StatusProjector → member_* / team_* / graph_* Envelope → EventBus → /v1/ws
             ↓
         persistStep + UpdateTeamRun + team_summary
```

编译/构建失败不 silent fallback，直接返回错误。

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
| `ListSpiritTeamsRequest/Response` | Spirit Team 列表 |
| `SynthesizeResultsRequest/Response` | Spirit 结果合成 |
| `ArchiveTeamRequest/Response` | Team 归档 |
| `RetryTeamRequest/Response` | Team 重试 |

### 2.2 RPC 一览（24 个）

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
| `ListSpiritTeams` | `GET /v1/spirit/{spirit_session_id}/teams` | Spirit Team 列表 |
| `SynthesizeResults` | `POST /v1/spirit/{spirit_session_id}/synthesize` | Spirit 结果合成 |
| `ArchiveTeam` | `POST /v1/teams/{team_id}/archive` | 归档 Team |
| `RetryTeam` | `POST /v1/teams/{team_id}/retry` | 重试 Team |

---

## 三、Biz 层

### 3.1 领域模型

文件：`internal/biz/team_types.go`

```go
// TECH-DEBT(COG): struct字段=23, 上限=15 — 下一迭代拆分
type Team struct {
    ID                  string
    TeamKey             string
    DisplayName         string
    Status              string   // pending | running | completed | failed | cancelled | interrupted | archived | blocked(virtual)
    IsDefault           bool
    DefinitionJSON      string
    ADKAppName          string
    DepartmentID        string
    DeptLeadAgentID     string
    Deliverables        string
    InputContract       string
    CrossDeptMemberIDs  string
    LinkedGraphID       string
    SpiritSessionID     string
    TaskDescription     string
    AutoCreated         bool
    DagNodeID           string
    DependsOn           []string
    ParallelConfigJSON  string
    Topology            string
    Readonly            bool
    Kind                string   // user | system_builtin | ecosystem_preset | marketplace | certified
    Source              string   // user | system | imported
    InterruptReason     string
    CreatedAt           string
    UpdatedAt           string
    DeletedAt           string
}

type TeamRun struct {
    ID                     string
    TeamID                 string
    SessionID              string
    MessageID              string
    Mode                   string
    Status                 string   // pending | running | success | failed | cancelled | waiting_human
    InputPreview           string
    OutputPreview          string
    TokenIn                int
    TokenOut               int
    CostMicroUSD           int64
    DurationMS             int
    ErrorMessage           string
    TopologyJSON           string
    GraphExecutionID       string
    DefinitionSnapshotJSON string
    TraceID                string
    StartedAt              string
    FinishedAt             string
    CreatedAt              string
    UpdatedAt              string
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
    Status        string   // ok | error | skipped
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
```

### 3.2 状态机（AS-FSM-01）

**Team 状态机** — 文件：`internal/biz/team_state_machine.go`

```
Pending → Running (start)
Pending → Cancelled (cancel)
Running → Completed (complete)
Running → Failed (fail)
Running → Cancelled (cancel)
Running → Interrupted (interrupt)
Interrupted → Running (recover)
Completed → Archived (archive)
Failed → Archived (archive)
Failed → Pending (recover)
Cancelled → Archived (archive)
Cancelled → Pending (recover)
```

7 种状态（含虚拟 `blocked`），8 种事件。终端状态：`archived`。

**TeamRun 状态机** — 文件：`internal/biz/team_run_state_machine.go`

```
Pending → Running (start)
Pending → Cancelled (cancel)
Running → WaitingHuman (await_human)
Running → Success (succeed)
Running → Failed (fail)
Running → Cancelled (cancel)
WaitingHuman → Running (resume)
WaitingHuman → Success (succeed)
WaitingHuman → Failed (fail)
WaitingHuman → Cancelled (cancel)
```

6 种状态，6 种事件。终端状态：`success` / `failed` / `cancelled`。

### 3.3 窄接口

文件：`internal/biz/team_usecase.go`

```go
// Stability:stable
type TeamReader interface {
    ListTeams(ctx) ([]Team, error)
    ListTeamsByStatus(ctx, status) ([]Team, error)
    GetTeamByID(ctx, id) (Team, error)
    GetTeamByKey(ctx, teamKey) (Team, error)
    ListBySpiritSessionID(ctx, spiritSessionID) ([]Team, error)
    ListTeamsByDepartmentID(ctx, deptID) ([]Team, error)
}

// Stability:stable
type TeamWriter interface {
    CreateTeam(ctx, Team) (Team, error)
    UpdateTeam(ctx, Team) (Team, error)
    DeleteTeam(ctx, id) error
    BatchArchiveTeams(ctx, ids) (int, error)
}

// Stability:stable
type TeamRunReader interface {
    ListTeamRuns(ctx, teamID, limit) ([]TeamRun, error)
    ListTeamRunsByTeamIDs(ctx, teamIDs, limit) (map[string][]TeamRun, error)
    HasActiveTeamRun(ctx, teamID) (bool, error)
    GetTeamRunByID(ctx, id) (TeamRun, error)
    ListTeamRunSteps(ctx, runID) ([]TeamRunStep, error)
}

// Stability:stable
type TeamRunWriter interface {
    CreateTeamRun(ctx, TeamRun) (TeamRun, error)
    UpdateTeamRun(ctx, TeamRun) error
    UpdateTeamRunGraphExecutionID(ctx, runID, graphExecutionID) error
    UpdateTeamRunTraceID(ctx, runID, traceID) error
    UpdateTeamRunSummaryJSON(ctx, runID, summaryJSON) error
    CreateTeamRunStep(ctx, TeamRunStep) (TeamRunStep, error)
}

// Stability:evolving
type OrchestrationStepRepo interface {
    BatchCreateOrchestrationSteps(ctx, []OrchestrationStep) error
    ListOrchestrationSteps(ctx, teamRunID, nodeID, limit) ([]OrchestrationStep, error)
}

// Stability:evolving
type TaskDeadLetterRepo interface {
    CreateTaskDeadLetter(ctx, TaskDeadLetter) error
    ListTaskDeadLetters(ctx, TaskDeadLetterListFilter) ([]TaskDeadLetter, error)
    ResolveTaskDeadLetter(ctx, id) (TaskDeadLetter, error)
}
```

### 3.4 Service 层 port 接口

文件：`internal/biz/team_ports.go`

| 接口 | 用途 | Stability |
|------|------|-----------|
| `TeamTurnRuntime` | Team Turn 执行端口 | — |
| `TeamMediatorPort` | Runner ↔ Coordinator 中介 | evolving |
| `TeamGraphRunFinisherPort` | Graph 运行终结器 | evolving |
| `TeamGraphCoordPort` | Graph 运行协调器 | evolving |
| `TeamTurnRunnerPort` | Team Turn Runner | evolving |
| `TeamRunnerWirePort` | Runner Wire 组合 | evolving |
| `RunRegistryPort` | 运行注册表 | evolving |
| `TeamRunObserver` | 运行生命周期观察 | — |
| `TeamBuildRunner` | TurnExecutor build hook | — |
| `TeamPersistTurnRecord` | TurnExecutor persist hook | — |
| `TeamProjectRuntimeEvent` | TurnExecutor project hook | — |
| `TeamRunStatusTransitioner` | 运行状态转换 | evolving |

### 3.5 Team 层窄接口

文件：`internal/biz/team_interfaces.go`

| 接口 | 用途 |
|------|------|
| `TeamUsageQuerier` | Usage 记录子集 |
| `TeamSessionManager` | Session 操作子集 |
| `TeamAgentLookup` | Agent 查询子集 |
| `TeamToolLookup` | Tool 查询子集 |
| `TeamModelCatalog` | 模型目录子集 |
| `TeamSkillLookup` | Skill 查询子集 |

### 3.6 Definition 结构

文件：`internal/team/definition.go`

```go
type Definition struct {
    Version            int               `json:"version"`
    Mode               string            `json:"mode"`
    SynthesizerAgentID string            `json:"synthesizer_agent_id"`
    Members            []MemberDef       `json:"members"`
    MaxConcurrency     int               `json:"max_concurrency"`
    TimeoutSeconds     int               `json:"timeout_seconds"`
    LoopMaxIterations  int               `json:"loop_max_iterations,omitempty"`
    RuntimeEngine      string            `json:"runtime_engine,omitempty"`
    TeamGraphRuntime   bool              `json:"team_graph_runtime,omitempty"`
    CriticLoop         *CriticLoopConfig `json:"critic_loop,omitempty"`
    IntentAnchorAgentID string           `json:"intent_anchor_agent_id,omitempty"`
    Swarm              *SwarmConfigDef   `json:"swarm,omitempty"`
    MemberTool         *MemberToolDef    `json:"member_tool_config,omitempty"`
    FailurePolicy      *FailurePolicy    `json:"failure_policy,omitempty"`
}
```

### 3.7 校验规则

- `mode` ∈ sequential / parallel / coordinator / critic_loop / swarm / adaptive
- 至少一个 enabled member；member `agent_id` 非空
- parallel：需 synthesizer 成员或 `synthesizer_agent_id`
- critic_loop：需 generator + critic 成员
- 默认 Team（`IsDefault=true`）不可删除
- UpdateSwarmMembers：仅 swarm / adaptive

---

## 四、Data 层

文件：`internal/data/team_repo.go` · Ent Schema：`internal/data/ent/schema/`

| 表 | Schema 文件 | 关键字段 |
|----|------------|----------|
| `teams` | `team.go` | id, team_key (unique), display_name, status, is_default, kind, source, readonly, definition_json, adk_app_name, department_id, spirit_session_id, task_description, auto_created, dag_node_id, depends_on_json, parallel_config_json, topology, deliverables, input_contract, dept_lead_agent_id, cross_dept_member_ids, linked_graph_id, interrupt_reason, deleted_at |
| `team_runs` | `team_run.go` | id, team_id, session_id, message_id, mode, status, token_*, cost_micro_usd, duration_ms, topology_json, graph_execution_id, definition_snapshot_json, trace_id |
| `team_run_steps` | `team_run_step.go` | id, run_id, team_id, agent_id, agent_key, agent_name, role, sort_order, status, token_*, tool_call_count, duration_ms |
| `orchestration_steps` | `orchestration_step.go` | id, team_run_id, graph_execution_id, node_id, activity_snapshot_json, status, started_at, finished_at |
| `task_dead_letters` | `task_dead_letter.go` | id, source_type, source_id, team_id, team_run_id, session_id, graph_execution_id, error_message, payload_json, status |
| `orchestrations` | `orchestration.go` | id, task_plan_id, allocation_id, spirit_session_id, trace_id, strategy, graph_execution_id, team_ids_json, status, checkpoint_id, synthesis_result_json |
| `team_graph_sessions` | `team_graph_session_schema.go`（DDL） | exec_id, team_run_id, team_id, session_id, input_preview, definition_json, status, registered_at, last_activity_at |

---

## 五、Service 层

文件：`internal/service/team.go`（主文件）+ 分文件

### 5.1 依赖注入

```go
type TeamService struct {
    uc          *biz.TeamUsecase
    graphUC     *biz.GraphUsecase
    agents      *biz.AgentUsecase
    sessions    *biz.SessionUsecase
    teamRunner  biz.TeamTurnRunnerPort
    runs        biz.RunRegistryPort
    eventBus    event.Bus
    lg          loggateway.Logger
    synthesis   *SpiritSynthesisService
}
```

### 5.2 RPC 实现分布

| 文件 | RPC |
|------|-----|
| `team.go` | ListTeams, CreateTeam, GetTeam, UpdateTeam, DeleteTeam, DuplicateTeam, ListTeamRuns, GetTeamRun, CancelTeamRun, RunTeamTest, ListTeamRunSteps, UpdateSwarmMembers, ExportTeamStructure, GetTeamRunSummary |
| `team_observatory.go` | GetTeamRunObservatory, GetTeamRunObservatoryTimeline |
| `team_compile.go` | CompileTeamGraph |
| `team_compile_view.go` | 编译视图辅助（buildCompiledGraphNodeViews） |
| `team_resume.go` | ResumeTeamRunExecution |
| `team_dead_letter.go` | ListTaskDeadLetters, ResolveTaskDeadLetter, ListSpiritTeams, SynthesizeResults, ArchiveTeam, RetryTeam |
| `team_orchestration_spec.go` | OrchestrationSpec Proto ↔ biz 映射 |
| `team_run_registry_adapter.go` | RunRegistryPort 适配 |
| `team_runner_wire_adapter.go` | TeamRunnerWirePort / TeamMediatorPort / TeamGraphCoordPort 适配 |
| `team_turn_hooks.go` | ChatOrchestrator Team turn hooks |

### 5.3 关键行为

| RPC | 实现要点 |
|-----|----------|
| `RunTeamTest` | 创建 `owner_type=team` 临时 Session → `teamRunner.RunTurnFromInput` → 查最近 TeamRun → defer 删除 Session |
| `CancelTeamRun` | 校验 running/pending → `RunRegistryPort.Cancel(sessionID)` → `CancelSessionRunSideEffects` → 更新 status=cancelled |
| `ExportTeamStructure` | 经编译器 `exportStructureViaCompiler` 生成拓扑 |
| `CompileTeamGraph` | 调用 `CompileToCompiledTeam` 返回编译后节点/边/校验问题 |
| `ResumeTeamRunExecution` | HITL 审核后恢复 Graph 运行 |
| `GetTeamRunObservatory` | 返回 ActivitySnapshot + AgentNodeState 快照 |
| `ArchiveTeam` | 经 TeamUsecase.TransitionStatus → archived |
| `RetryTeam` | 经 TeamUsecase.RetryTeam → 重置状态并重新启动 |
| `ListSpiritTeams` | 经 TeamReader.ListBySpiritSessionID |
| `SynthesizeResults` | SpiritSynthesisService 合成多 Team 结果 |

错误映射：`sql.ErrNoRows` → `kerrors.NotFound("TEAM", ...)`。

---

## 六、Team 运行时

### 6.1 构建

| 路径 | 文件 | 说明 |
|------|------|------|
| **Graph（唯一路径）** | `graph_compile.go` · `graph_runtime_config.go` · `embedded_graph.go` | `CompileToGraphRuntimeConfig` → `GraphAgent`；embedded `agent`/`task`/`review`/`subgraph`/`function` 节点 |
| **成员构建** | `trpc_build.go` | `BuildTeamMemberAgents` — 构建成员 trpc Agent + lookup map |

Graph 路径要点：

- `runner_team_compiler.go`：`compileTeamRuntime` — Graph 编译 + 构建；编译/构建失败不 silent fallback，直接返回错误
- `team_graph_task_bridge.go` + `task_creator.go`：`graph_node_start` 时为 task/review 节点创建 Task（经 `ChatService` 注入 `TaskUsecase`）
- `ResumeTeamRunExecution`：Graph checkpoint / HITL resume（见 M53 Phase 6）
- `graph_runtime.go`：Graph 运行时开关（`TeamGraphRuntimeEnabled` / `SupportsTeamGraphRuntimeMode`）
- `graph_runtime_canary.go`：灰度控制
- `status_projector.go`：WS 状态投影（`orchestration_agent_status` 等）
- `activity_step_flusher.go`：Activity 步骤刷盘
- `runner_mediator.go`：Mediator 模式（HITL defer）
- `team_graph_run_coordinator.go`：Graph 运行协调器
- `team_graph_run_finisher.go`：Graph 运行终结器
- `team_graph_run_context.go`：Graph 运行上下文
- `team_graph_execution_tracker.go`：执行追踪器
- `fallback_policy.go`：编译/构建失败诊断错误（不执行 fallback）
- `template_registry.go`：Mode 模板注册
- `compile_snapshot.go`：编译快照
- `graph_definition_json.go`：Definition JSON 图结构解析
- `graph_structure.go`：图结构工具
- `graph_loader.go`：图加载器
- `graph_runtime_options.go`：运行时选项
- `builder.go`：构建器
- `agent_keys.go`：Agent Key 解析
- `llm_catalog.go`：LLM 目录
- `usage_tokens.go` / `usage_record.go`：Usage 记录
- `safety_adapter.go` / `export_adapter.go` / `compiler_adapter.go`：适配器

成员 Agent 经 `chatagent.BuildTRPCAgent(Cached)` 构建，deps 含 `PluginsForAgent`、有效工具集。

### 6.2 执行

文件：`internal/team/runner_team_trpc.go` · `internal/team/runner_helpers.go` · `internal/team/runner_finish_steps.go`

- 创建 TeamRun（status=running）→ 发射 `team_run_started`
- `TraceEmitter`：`team.run.start` / `team.run.execute` / `team.run.finish`
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
| `graph_node_end` | Graph 节点执行完成 |
| `graph_node_error` | Graph 节点执行错误 |
| `graph_node_custom` | Graph 节点自定义事件 |

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
| `web/src/components/teams/TeamCard.vue` | 卡片：模式、成员、操作 |
| `web/src/components/teams/TeamToolbar.vue` | 搜索 / 模式 / 状态 / 行业筛选 |
| `web/src/components/teams/TeamEditorDialog.vue` | 新建 / 编辑 / 模板（含编译预览侧栏） |
| `web/src/components/teams/TeamRunsDialog.vue` | 运行记录 + WS 实时 |
| `web/src/components/teams/TeamTestDialog.vue` | 运行测试 |
| `web/src/components/teams/TeamCompilePreview.vue` | 编译预览侧栏 |
| `web/src/components/teams/TeamMemberKanban.vue` | 成员看板（按角色分列） |
| `web/src/components/teams/TeamOrchestrateNodePanel.vue` | 编排页节点详情面板 |
| `web/src/components/teams/TeamOrchestrateRuntimePanel.vue` | 编排页运行时/容错面板 |
| `web/src/components/teams/TeamsListSection.vue` | Team 列表区域 |
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
| `web/src/features/teams/useTeamCompilePreview.ts` | TeamCompilePreview composable |

### Store

| 路径 | 职责 |
|------|------|
| `web/src/stores/teams/index.ts` | Pinia 状态（teams / runs / steps / summaries / WS 事件更新） |

### Chat 集成

| 路径 | 职责 |
|------|------|
| `web/src/components/chat/ChatTeamMemberStrip.vue` | Chat 子 Agent 流式状态 chip |
| `web/src/components/chat/TeamProgressSection.vue` | Chat Team 进度区 |
| `web/src/components/chat/TeamPanel.vue` | Chat Team 面板 |

### Orchestration 集成

| 路径 | 职责 |
|------|------|
| `web/src/features/orchestration/teamGraphAdapter.ts` | Graph 适配器 |
| `web/src/features/orchestration/teamNodeDisplay.ts` | 节点展示 |

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
| `internal/biz/team_state_machine_test.go` | Team 状态机 |
| `internal/biz/team_run_state_machine_test.go` | TeamRun 状态机 |

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
| `internal/team/team_graph_run_context_test.go` | 运行上下文 |
| `internal/team/graph_structure_test.go` | 图结构 |
| `internal/team/graph_definition_json_test.go` | Definition JSON 图解析 |
| `internal/team/graph_runtime_options_test.go` | 运行时选项 |
| `internal/team/usage_tokens_test.go` | Usage tokens |
| `internal/team/parity_test.go` / `parity_run_test.go` / `parity_runtime_test.go` / `parity_run_e2e_test.go` | 编译/运行对比 |
| `internal/team/runner_helpers_test.go` | Runner 辅助 |
| `internal/team/activity_step_flusher_test.go` | Activity 步骤刷盘 |
