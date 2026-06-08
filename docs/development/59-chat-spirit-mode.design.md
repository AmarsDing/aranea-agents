# M59: Chat 精灵模式 — 实现设计（M59+OBS+M60 合并版）

> 版本：2026-06-08
> 对应需求：[59-chat-spirit-mode.md](./59-chat-spirit-mode.md)
> 开发计划：[59-chat-spirit-mode.development.md](./59-chat-spirit-mode.development.md)
> 遵循：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md) · [AGENT_RUNTIME_BOUNDARY.md](../AGENT_RUNTIME_BOUNDARY.md)

---

## 一、模块概述

### 1.1 设计定位

以精灵（Spirit）为 Chat 页面唯一对话入口，左侧列表从"Agent/Team 平铺"重构为"精灵 + 任务团队树"：

- **精灵简视图**：用户只与精灵对话，不感知 Agent/Team 细节
- **任务团队视图**：精灵自动组建的团队在左侧动态展示，支持展开成员树
- **执行观测视图**：点击团队/成员进入任务执行面板或只读面板

核心架构为三层分离（对齐 APWA 论文 arXiv:2605.15132）：

- **Spirit = Manager**：高层规划，决定"做什么"和"谁来做"
- **Team = Worker**：任务委派，管理成员执行
- **Agent = Executor**：具体执行，独立上下文

设计融合三篇论文核心思想：
- **AdaptOrch** (arXiv:2602.16873)：拓扑路由算法，根据任务 DAG 自动选择最优编排拓扑
- **APWA** (arXiv:2605.15132)：Manager-Worker-Executor 三层分离
- **Maestro** (arXiv:2511.06134)：探索-合成分离，并行 Team 做发散探索，Spirit 做收敛合成

可观测性 UX 遵循"可观测性强但不影响主要内容显示"原则，基于三层可观测性架构（L1 环境层 → L2 结构层 → L3 证据层），核心模式为"完成即折叠"。

**前置依赖**：[system-builtin-agents-design](../superpowers/specs/2026-05-31-system-builtin-agents-design.md) 中精灵 Agent 定义、`plan_and_execute` 三阶段编排工具、Session 树状模型。

### 1.2 分层与依赖

```
api/kratos/session/v1/session.proto   ← Session 扩展字段（parent_session_id / dag_snapshot 等）
api/kratos/team/v1/team.proto         ← Team 扩展字段（spirit_session_id / dag_node_id / depends_on / parallel_config 等）
        ↓
internal/service/
  chat_orchestrator_turn.go           ← 精灵 Agent 走 runSingleAgentViaTRPC，通过 spiritCustomTools 注入工具
  chat_orchestrator_spirit.go         ← Spirit Team 构建 + 模式选择（GAP-3 AdaptiveTeamMode）
  spirit_team.go                      ← TeamStarter（生命周期管理）+ SpiritTeamAssembler（组装/查询/取消）
  spirit_synthesis.go                 ← Synthesis Engine（P2）
  team_turn_hooks.go                  ← executeTeamTurnViaHooks → HandleTeamTurnResult → 生命周期事件
        ↓
internal/tools/
  spirit_tools.go                     ← plan_and_execute / check_progress / cancel_orchestration / synthesize_results
  spirit_complexity.go                ← GAP-1: 复杂度评估工具 + 规则引擎（DEPRECATED，委托 plan_and_execute）
  orchestrator/build_graph.go         ← GAP-2: build_orchestration_graph（DAG 编排图构建）
  orchestrator/verification.go        ← GAP-4: 验证节点类型定义 + injectVerificationNodes
  orchestrator/verify_funcs.go        ← GAP-4: 验证函数实现
        ↓
internal/biz/
  spirit_team_usecase.go              ← AssembleTeam / ListActiveTeams / CancelTeam / AutoArchiveCompletedTeams
  task_planner.go                     ← TaskPlannerPort（Plan 阶段）
  agent_allocator.go                  ← AgentAllocatorPort（Allocate 阶段）
  task_orchestrator.go                ← TaskOrchestratorPort（Orchestrate 阶段）
  spirit_task_dag.go                  ← TaskDAG 拓扑路由（parallel / sequential / hybrid / coordinator）
  spirit_parallel_config.go           ← ParallelConfig（并行配额 + 自动归档）
  spirit_synthesis.go                 ← Synthesis Engine 逻辑（P2）
  spirit_orchestration_cache.go       ← DQ Score 驱动编排缓存（P2）
  evolution.go                        ← 编排优化建议生成 + 进化护栏
  session/usecase.go                  ← Session 树查询（ListByParentSessionID）
  team_usecase.go                     ← Create 支持 AutoCreated / SpiritSessionID / ListBySpiritSessionID
        ↓
internal/data/ent/schema/
  team.go                             ← spirit_session_id 索引 idx_teams_spirit_session
        ↓
web/src/
  features/spirit/                    ← 精灵域（api.ts / types.ts / spiritUi.ts / observabilityConstants.ts）
  composables/chat/                   ← useAutoCollapse / useContextualLoadingMessage / useStatusPulse
  stores/spirit/                      ← useSpiritTeamStore
  components/spirit/                  ← 精灵专用组件（12+ 个）
  components/chat/                    ← Chat 面板扩展
```

**红线**：`internal/biz` 不 import `pkg/trpc-agent-go`；精灵构建仅在 `internal/service`；Team 编译仅在 `internal/team`。展示组件不 import Store（props/emits 通信）；展示组件不直接调 API（Store action 中调用）；Composable 不持有 UI 状态。

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
| `internal/tools` | 新增 | spirit_tools.go / spirit_complexity.go / orchestrator/ |
| `internal/event` | 扩展 | spirit_team_assembled 等 15+ 个新 EnvelopeType |
| `internal/data/ent/schema/team` | 扩展 | spirit_session_id 索引 |
| `web/src/features/spirit` | 新增 | 类型、API、UI 工具函数、可观测性常量 |
| `web/src/composables/chat` | 新增 | useAutoCollapse / useContextualLoadingMessage / useStatusPulse |
| `web/src/stores/spirit` | 新增 | useSpiritTeamStore |
| `web/src/components/spirit` | 新增 | 12+ 个新组件 |
| `web/src/components/chat` | 修改 | ChatEntitySidebar 重构、ChatMessagePanel 三模式 + 可观测性集成 |
| `api/kratos/session/v1` | 扩展 | Session Proto 字段（parent_session_id = 50, root_session_id = 51, agent_depth = 52） |
| `api/kratos/team/v1` | 扩展 | Team Proto 字段（spirit_session_id = 15, task_description = 16, auto_created = 17, dag_node_id = 18, depends_on = 19, parallel_config_json = 20, readonly = 21, source = 22, kind = 23） |

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

### 3.3 TeamKey UUID 后缀

同一精灵 Session 第二次创建时避免冲突：

```go
teamKey := fmt.Sprintf("spirit_%s_%s", params.SpiritSessionID, uuid.New().String()[:8])
```

### 3.4 DAG 拓扑路由

`TaskDAG.RouteTopology()` 根据任务图结构自动选择拓扑：

| 条件 | 拓扑 | 说明 |
|------|------|------|
| 无节点 | `coordinator` | 退化为协调者模式 |
| 所有节点都是根节点（无依赖） | `parallel` | 全并行 |
| 深度 > 3 | `coordinator` | 深度过深，需协调 |
| 最大宽度 > 1 | `hybrid` | 混合并行/顺序 |
| 其他 | `sequential` | 顺序执行 |

### 3.5 并行配置

```go
type ParallelConfig struct {
    MaxConcurrentTeams  int  `json:"max_concurrent_teams"`  // 最大并行团队数（默认 3）
    MaxTeamConcurrency  int  `json:"max_team_concurrency"`  // 单团队内最大并发数（默认 2）
    TeamTimeoutSeconds  int  `json:"team_timeout_seconds"`  // 团队超时秒数（默认 600）
    AutoArchiveSeconds  int  `json:"auto_archive_seconds"`  // 自动归档秒数（默认 3600）
    MaxSessionDepth     int  `json:"max_session_depth"`     // 最大会话树深度（默认 2）
}

func (c ParallelConfig) TeamTimeout() time.Duration {
    return time.Duration(c.TeamTimeoutSeconds) * time.Second
}

func (c ParallelConfig) AutoArchiveAfter() time.Duration {
    return time.Duration(c.AutoArchiveSeconds) * time.Second
}
```

存储位置：`AgentRuntimeSettings.ExtraJSON` 中 `parallel_config` 键，精灵 Agent 种子数据中注入默认值。

### 3.6 SpiritTeamAssemblerPort 接口扩展

代码中拆分为 3 个小接口（遵循接口隔离原则）：

```go
// 团队组装端口
type SpiritTeamAssemblerPort interface {
    AssembleTeam(ctx context.Context, params biz.SpiritTeamParams) (biz.Team, biz.Session, error)
    SuggestTopology(ctx context.Context, taskDescription string) (string, bool)
}

// 团队查询端口
type SpiritTeamQueryPort interface {
    ListActiveTeams(ctx context.Context, spiritSessionID string) ([]biz.Team, error)
    ListAllTeams(ctx context.Context, spiritSessionID string) ([]biz.Team, error)
    GetMaxParallelTeams(ctx context.Context, spiritSessionID string) int
}

// 团队控制端口
type SpiritTeamControllerPort interface {
    CancelTeam(ctx context.Context, teamID string) error
    CheckTeamProgress(ctx context.Context, spiritSessionID string) ([]biz.TeamProgress, error)
    StartTeam(ctx context.Context, teamID string, initialMessage string) (biz.Session, error)
}
```

### 3.7 Team 状态机

```
pending → running → completed → archived
                 → failed → archived
                 → cancelled → archived
                 → interrupted → running
```

完整状态常量（`internal/biz/team_types.go`）：

| 常量 | 值 | 说明 |
|------|-----|------|
| `TeamStatusPending` | `"pending"` | 已创建，等待执行 |
| `TeamStatusRunning` | `"running"` | 正在执行 |
| `TeamStatusCompleted` | `"completed"` | 成功完成 |
| `TeamStatusFailed` | `"failed"` | 执行失败 |
| `TeamStatusCancelled` | `"cancelled"` | 已取消 |
| `TeamStatusInterrupted` | `"interrupted"` | 异常中断，可恢复 |
| `TeamStatusArchived` | `"archived"` | 自动归档 |
| `TeamStatusBlocked` | `"blocked"` | 虚拟状态，仅用于级联阻塞结果展示，不持久化 |

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
    "total_token_in": totalTokenIn,   // OBS 扩展
    "total_token_out": totalTokenOut, // OBS 扩展
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
    "status":          status,
    "progress_pct":    progressPct,
    "current_step":    currentStep,
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

### 5.2 进化数据流

```
Session 执行轨迹
    ├──→ 技能进化：工具调用模式 → Skill 提案 → 技能管家分析
    ├──→ Agent 进化：多 Team 表现 → 能力画像 → tool_weight_json 调整
    ├──→ 记忆：L0-L4 全链路提取 → 记忆管家 dream_cycle 输入
    ├──→ 知识图谱：协作关系 + 产出物 → L4 实体-关系 → GraphRAG
    └──→ 编排进化：模式成功率 + DQ Score → 编排策略优化
```

### 5.3 DQ Score 三元分解

```go
type DQScoreBreakdown struct {
    Validity     float64 `json:"validity"`      // 结果有效性（0-1）
    Specificity  float64 `json:"specificity"`    // 结果具体性（0-1）
    Correctness  float64 `json:"correctness"`    // 结果正确性（0-1）
    Overall      float64 `json:"overall"`        // 加权总分
    DurationMs   int64   `json:"duration_ms"`    // 执行时长
}
```

计算公式：`Overall = Validity * DQWeightValidity + Specificity * DQWeightSpecificity + Correctness * DQWeightCorrectness`（0.4 / 0.3 / 0.3）

数据来源：
- **Validity**：基于团队最终状态（completed=1.0, 否则=0.0），二值判断
- **Specificity**：从团队 Summary 长度推断（>50 字符=0.85, >100 字符=1.0, 基准=0.7），KeyFindings 额外 +0.15
- **Correctness**：基于执行时长的代理指标（每分钟 -0.1，上限 5 分钟惩罚）

### 5.4 编排进化闭环

```
团队执行完成
  → 计算 DQ Score Breakdown
  → DQ Score > 0.7 → 缓存编排拓扑（OrchestrationCache）
  → DQ Score < 0.5 → EvolutionUsecase 生成编排优化建议
  → 下次 assemble_team → 先查缓存，命中则复用
```

`OrchestrationCache` 持久化到 `AgentRuntimeSettings.ExtraJSON` 中 `orchestration_cache` 键。

---

## 六、可观测性 UX 设计

### 6.1 OBS-01 对话流自动折叠

#### 数据结构扩展

`TurnBlockGroup` 增加 `isCompleted` 计算属性：

```typescript
export type TurnBlockGroup = {
  key: number;
  turnId: string;
  user: Message | null;
  assistant: Message | null;
  tools: Message[];
  members: Message[];
  isCompleted: boolean;  // true = block 内所有工具调用均 completed/failed/interrupted，且 assistant 消息已到达
};
```

`isCompleted` 判断逻辑：assistant 消息必须已到达（非 streaming），且所有工具调用状态为 success/failed/cancelled。

#### 折叠态摘要行

| block 类型 | 折叠态摘要 |
|-----------|-----------|
| 纯工具调用 block | "🔧 {tool_name} → ✓ 1.5s" 或多工具 "🔧 3 tools → ✓ 4.2s" |
| 团队组建 block | "🏗️ 组建团队 → {team_name} ✓" |
| 团队完成 block | "✅ 任务完成 → {team_name} 3m 20s" |
| interrupted block | "⏸ 已中断 → {team_name} 3/5 步骤" |

#### Composable: useAutoCollapse

```typescript
export function useAutoCollapse() {
  const collapsedBlockKeys = ref<Set<number>>(new Set());
  function onBlockCompleted(blockKey: number) { collapsedBlockKeys.value.add(blockKey); }
  function expandAll() { collapsedBlockKeys.value.clear(); }
  function toggleBlock(blockKey: number) { /* toggle */ }
  return { collapsedBlockKeys, onBlockCompleted, expandAll, toggleBlock };
}
```

### 6.2 OBS-02 语境加载消息

事件到消息映射表定义在 `observabilityConstants.ts`，包含编排阶段事件（`butler.orchestration.started` → "正在处理任务…"、`spirit_plan_created` → "正在分析任务复杂度…"、`spirit_allocation_created` → "正在分配 Agent 角色…"、`spirit_orchestration_started` → "正在编排执行流程…"）和 Agent 级事件（`tool_call` → "{agentName} 正在{displayLabel}…"、`tool_result` → "{agentName} 完成，耗时 {durationSec}s"）。

Composable `useContextualLoadingMessage` 在 WS 回放期间静默（`isReplaying` 标志），团队完成/失败时清除加载消息。

### 6.3 OBS-03 Agent 状态标签

状态聚合映射将 17 种 `AgentNodeStatus` 聚合为 7 种 `AgentNodeStatusLabel`（queued / active / suspended / done / failed / skipped / cancelled），每种标签有 text / color / icon / animated 配置。

组件 `AgentStatusLabel.vue` 使用 `q-badge` 渲染，active 状态带脉冲动画。

双状态源策略：

| 使用场景 | 状态源 | 映射 |
|---------|--------|------|
| 侧边栏 `TeamTaskCard` 折叠态 | `SpiritMember.status`（idle/running/error） | idle→queued, running→active, error→failed |
| 任务执行面板 `TaskExecutionPanel` | `AgentNodeStatus`（17 值） | 通过 `AGENT_NODE_STATUS_MAP` 聚合为 7 种标签 |

### 6.4 OBS-04 底部状态栏

```
┌──────────────────────────────────────────────────────────────┐
│ ⚡ 2 running │ ⏸ 1 interrupted │ 📊 2/3 quota │ ✅ Team A  │
└──────────────────────────────────────────────────────────────┘
```

Props：`runningTeamCount` / `interruptedTeamCount` / `quotaUsed` / `quotaMax` / `tokenUsage?` / `lastEvent?`。

后端扩展：`spirit_team_completed` / `spirit_teams_all_completed` 事件需增加 `total_token_in` / `total_token_out` 字段。

### 6.5 OBS-05 侧边栏状态脉冲

Composable `useStatusPulse` 在团队状态变化时触发 CSS 脉冲动画，WS 回放期间静默。脉冲颜色和持续时间通过 `PULSE_COLOR_MAP` / `PULSE_DURATION_MAP` 配置。

CSS 动画：`.team-card--pulse` 使用 `@keyframes status-pulse` 实现 1.5s 渐隐效果。

### 6.6 OBS-06 可折叠工具输出

`ChatExecutionCard` 监听 `event.status` 变化，completed/failed/cancelled 时自动折叠（`expanded = false`）。历史消息恢复时从 `OptionsJSON.tool_event.status` 判断是否应折叠。

### 6.8 OBS-08 ChatExecutionCard 独立折叠增强

> 对应任务：SP-FE-27~31（P1.5 阶段）。详细设计参考：[proposal](../../reports/2026-06-09-proposal-chat-execution-card-folding.md)，本文档为权威设计摘要。

#### 6.8.1 架构：Provide/Inject + Signal

不提升 ChatExecutionCard 的折叠状态，保持其内部 `expanded` ref 不变。通过 `provide/inject` 传递全局控制信号，ChatExecutionCard 自行决定是否响应。

```
ChatMessagePanel
├── provide(EXECUTION_COLLAPSE_CONTROL_KEY, { expandAllSignal, collapseAllSignal })
│
├── TurnBlock (useAutoCollapse 管理 block 级折叠，不变)
│   └── ChatMessageRow → ChatExecutionCard (inject signal)
│
└── TaskExecutionPanel → ChatExecutionCard (同样 inject，自动生效)
```

**类型定义**（`features/chat/types.ts`）：

```typescript
export const EXECUTION_COLLAPSE_CONTROL_KEY: InjectionKey<ExecutionCollapseControl> =
  Symbol('execution-collapse-control');

export interface ExecutionCollapseControl {
  expandAllSignal: Readonly<Ref<number>>;    // 递增计数器
  collapseAllSignal: Readonly<Ref<number>>;  // 递增计数器
}
```

**Provider**（`ChatMessagePanel.vue`）：`expandAll()`/`collapseAll()` 同时操作 TurnBlock 级 + ChatExecutionCard 级。

**Consumer**（`ChatExecutionCard.vue`）：inject signal，watch 变化后更新 `expanded`。运行中工具不响应 collapseAll。

#### 6.8.2 五秒耗时守卫（Fazm 模式）

ChatExecutionCard 内部新增 elapsed timer：

- `started_at` → `occurred_at` → `Date.now()` 三级降级获取起始时间
- running ≥5s 时显示实时计时器（`5s` → `6s` → `1m 12s`）
- ≥60s 变为 `var(--color-warning)` 警告色
- `onBeforeUnmount` 清理 `setInterval`

#### 6.8.3 折叠态摘要兜底

`event.summary` 为空时，前端根据 `tool_name` + `arguments` 生成摘要：

| tool_name | 摘要模板 |
|-----------|----------|
| `file_edit`/`file_write` | `修改 {filename}` |
| `file_read` | `读取 {filename}` |
| `grep`/`search_files` | `搜索 "{pattern}"` |
| `bash` | `> {command}` |
| 其他 | 空 |

#### 6.8.4 ToolStrip 摘要增强

ToolStrip 折叠态从 `"N tools · Xs"` 增强为 `"3 file_read · 2.5s"` 或 `"2 grep + 1 bash · 2.5s · 1 failed"`，需 import `toolEventFromMessage`。

#### 6.8.5 死代码清理

`ToolUseEvent.expanded` 字段存在于类型定义中但从未被消费，在 P1.5 阶段与折叠增强一起清理（SP-FE-31）。

### 6.7 OBS-07 中断恢复提示

组件 `InterruptedTeamCard.vue` 在团队 `status === 'interrupted'` 时显示，包含中断原因、已完成步骤数、恢复/取消按钮。`canResume` 基于 `team.graphExecutionId` 是否存在判断。

---

## 七、并行编排设计

### 7.1 Phase 1：基础并行

#### 移除单活跃团队限制

`assemble_team` 工具改为查询 `ListActiveTeams` 并与 `MaxConcurrentTeams` 比较，超限返回 `BadRequest` 错误。

#### 自动启动策略

`assemble_team` 内部自动调用 `StartTeamTurn`，`TaskPrompt` 字段自动作为初始消息发送给团队：

```go
if params.TaskDescription != "" {
    starter.StartTeamTurn(ctx, session.ID, params.TaskDescription)
}
```

`TeamStarterPort` 接口定义在 biz 层，`ChatOrchestrator` 实现：

```go
type TeamStarterPort interface {
    StartTeamTurn(ctx context.Context, sessionID string, content string) error
    HandleTeamTurnResult(ctx context.Context, spiritSessionID, teamID, status, errMsg string)
}
```

#### 依赖团队激活后自动启动

`scheduleDependentTeams` 在激活团队时同步触发 Runner（使用 `safego.Go` 异步执行）。

#### 精灵 Observer

精灵 Session 注册为 Event Observer，订阅子团队完成/失败事件，所有团队完成后发布 `spirit_teams_all_completed`。

### 7.2 Phase 2：智能编排

#### Task DAG 数据模型

```go
type TaskNode struct {
    ID           string   `json:"id"`
    TaskPrompt   string   `json:"task_prompt"`
    AgentIDs     []string `json:"agent_ids"`
    Mode         string   `json:"mode"`
    Dependencies []string `json:"dependencies"`
    Priority     int      `json:"priority"`
}

type TaskDAG struct {
    Nodes           []*TaskNode `json:"nodes"`
    SpiritSessionID string      `json:"spirit_session_id"`
}
```

`TaskDAG.Validate()` 包含重复节点检测、依赖存在性校验、DFS 环检测。

#### 依赖感知调度

`DependencyScheduler` 已删除（深度架构审查后），实际调度由 `team_turn_hooks.go` 中的 `scheduleDependentTeams` 实现。

#### Synthesis Engine

对齐 Maestro 的探索-合成分离，4 种合成策略：

| 场景 | 策略 |
|------|------|
| 全部成功 + < 3 团队 | 模板合成（拼接各团队摘要） |
| 全部成功 + >= 3 团队 | 混合合成（模板 + Prompt 综合摘要） |
| 部分失败 | 混合合成（成功团队模板 + 失败团队标注 + Prompt 总结） |
| 全部失败 | 混合合成（分析失败原因 + 建议） |

`ExtractTeamOutput` 从团队 Session 的最后一条 Assistant 消息中提取 Summary 和 KeyFindings。部分失败合成包含 failed 团队但标注原因，级联标注被阻塞的下游团队。

#### DAG 文本展示

`TaskDAG.ToTextDiagram()` 返回文本形式的依赖图，在 `AssembleTeamOutput.DAGDiagram` 中返回。

### 7.3 Phase 4：智能增强

#### GAP-1：TaskComplexityClassifier

`assess_complexity` 工具（DEPRECATED，委托 `plan_and_execute` 三阶段流程）+ `ComplexityRuleEngine` 规则引擎。规则引擎基于关键词匹配（简单问答 / 复杂任务 / 中等任务），安全默认值为 moderate。

Spirit Prompt 强制决策规则：先调用 `assess_complexity` 评估复杂度 → simple 直接回答 → moderate 单一管家 → complex 编排管家。

#### GAP-2：GraphOrchestration

`build_orchestration_graph` 工具动态生成 `GraphBuildConfig`，包含入口节点 → Agent 节点（按依赖关系连边）→ merge_results → finish。DFS 三色标记法循环检测，检测到环时降级为顺序链。

`GraphBuilderPort` 接口定义在 biz 层，支持 BuildAndExecute。

P0 阶段 `assemble_team` 和 `build_orchestration_graph` 共存，P1 阶段统一走 Graph 引擎。

#### GAP-3：AdaptiveTeamMode

```go
type SpiritTeamMode string
const (
    SpiritModeCoordinator SpiritTeamMode = "coordinator"
    SpiritModeSwarm       SpiritTeamMode = "swarm"
    SpiritModeDirect      SpiritTeamMode = "direct"  // 精灵路由层内部使用，不暴露给前端
)
```

| assess_complexity 输出 | Team 模式 | 说明 |
|----------------------|----------|------|
| `direct_answer` | Direct | 不构建 Team |
| `single_butler` | Direct | 直接路由到目标管家 |
| `orchestrator` | Coordinator | 完整 Team 编排 |

`buildSpiritTeam` 在 `chat_orchestrator_spirit.go` 中实现，`runSingleAgentViaTRPC` 集成模式选择。

#### GAP-4：VerificationGate

3 种验证节点类型：

| 验证类型 | 注入位置 | FailureAction | 说明 |
|---------|---------|---------------|------|
| output_format | merge 后 | Skip | 格式验证失败不阻塞 |
| task_completion | merge 后 | RetryThenBlock | 完成度不足则重试 |
| human_approval | 关键节点前 | interrupt_before | 暂停等待人工确认 |

`injectVerificationNodes` 在 Graph 配置中插入验证节点，重写边：source → verify_node → AfterNode。

---

## 八、前端架构

### 8.1 目录结构

```
web/src/features/spirit/
  types.ts                    ← SpiritTeam / SpiritMember / PanelMode / SpiritTeamStatus / SpiritTeamMode / TopologyType / ParallelConfig / AgentNodeStatusLabel 等类型
  api.ts                      ← listSpiritTeams / getSpiritTeamDetail / cancelSpiritTeam
  spiritUi.ts                 ← 状态映射、标签函数、AGENT_NODE_STATUS_MAP + STATUS_LABEL_CONFIG
  observabilityConstants.ts   ← 语境消息映射表、脉冲颜色配置

web/src/composables/chat/
  useAutoCollapse.ts          ← OBS-01 自动折叠 composable
  useContextualLoadingMessage.ts ← OBS-02 语境加载消息 composable
  useStatusPulse.ts           ← OBS-05 侧边栏脉冲 composable

web/src/stores/spirit/
  index.ts                    ← useSpiritTeamStore

web/src/components/spirit/
  SpiritEntry.vue             ← 精灵入口卡片
  TeamTaskCard.vue            ← 团队任务卡片（侧边栏用，含展开/折叠 + AgentStatusLabel + 脉冲动画）
  TeamProgressCard.vue        ← 团队进度卡片（执行面板用，含进度条/取消按钮/依赖提示/耗时显示）
  TeamAssemblyCard.vue        ← 精灵对话中的团队组建卡片
  TaskExecutionPanel.vue      ← 任务执行面板（集成 ParallelTeamOverview + AgentStatusLabel + InterruptedTeamCard）
  ParallelTeamOverview.vue    ← 并行团队概览（配额进度条 + DAG 图 + 团队列表）
  DAGDiagramCard.vue          ← DAG 依赖图简化文本视图
  SynthesisResultCard.vue     ← 综合结果卡片（合成策略 + 各团队结果）
  OrchestrationModeBadge.vue  ← 编排模式徽章（parallel / sequential / hybrid / coordinator）
  AgentStatusLabel.vue        ← OBS-03 Agent 状态标签组件
  SpiritStatusBar.vue         ← OBS-04 底部状态栏组件
  InterruptedTeamCard.vue     ← OBS-07 中断恢复提示卡片

web/src/components/chat/
  ChatExecutionCard.vue       ← 修改：增加自动折叠逻辑（OBS-06）
  ChatMessagePanel.vue        ← 修改：三模式 + SpiritStatusBar + 语境加载消息
  ChatEntitySidebar.vue       ← 修改：精灵模式重构 + useStatusPulse
```

### 8.2 Store 设计

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

interface ParallelConfig {
  maxConcurrentTeams: number
  maxTeamConcurrency: number
  teamTimeoutMs: number
  autoArchiveAfterMs: number
  maxSessionDepth: number
}

interface SynthesisResult {
  taskResults: TaskSynthesis[]
  summary: string
  allSuccess: boolean
}
```

核心 actions：`loadSpiritTeams` / `reloadTeams` / `selectTeam` / `selectMember` / `returnToSpirit` / `toggleTeamExpand` / `cancelTeam` / `updateTeamProgress` / `updateTeamStatus` / `addTeam` / `handleSpiritEnvelope` / `checkTeamProgress` / `synthesizeResults`。

团队列表排序 computed（running → pending → completed → failed → cancelled）。

`updateTeamStatus` 参数为 `SpiritTeamStatus` 类型，新增 `isValidTeamStatus` 类型守卫验证 WS 推送状态，禁止 running→pending 回退。

### 8.3 ChatEntitySidebar 重构

现有 `ChatEntitySidebar.vue` 重构为精灵模式：顶部 `SpiritEntry` → 进行中的团队 `TeamTaskCard` 列表 → 已完成的团队折叠区。

### 8.4 ChatMessagePanel 三模式

| 模式 | 组件 | 输入框 | WS 连接 |
|------|------|--------|---------|
| `spirit` | 标准 ChatMessagePanel + ContextualLoadingMessage + SpiritStatusBar | 显示 | Spirit Session WS |
| `team` | TaskExecutionPanel（集成 ParallelTeamOverview + AgentStatusLabel + InterruptedTeamCard） | 隐藏 | Team Session WS（订阅） |
| `member` | 占位符（P1 实现 MemberReadOnlyPanel） | 隐藏 | 复用 Team Session WS（过滤） |

### 8.5 面包屑导航

```
精灵 > 后端 API 开发团队 > Golang 工程师
```

> **当前状态**：面包屑导航未实现。当前仅有"返回精灵"按钮。P1 阶段需实现 `useSpiritWorkspace` composable 维护 `breadcrumbItems`。

### 8.6 WS 回放兼容

所有 L1 环境层方案（OBS-02 语境消息、OBS-05 脉冲）需在 WS 回放期间静默，统一通过 `isReplaying` ref 控制。

---

## 九、API 扩展

### 9.1 Session Proto

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

### 9.2 Team Proto

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

### 9.3 精灵团队查询 API

> **当前状态**：`ListSpiritTeams` RPC 未在 Proto 中定义。前端通过 `createSpiritService()` 调用 `/v1/spirit/{spiritSessionId}/teams`，但后端未注册对应 HTTP handler。P1 阶段需补齐此 API。

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

message SpiritMemberView {
  string agent_id = 1;
  string agent_key = 2;
  string display_name = 3;
  string role = 4;
  string status = 5;
}
```

### 9.4 精灵并行查询 API

```protobuf
message ListActiveTeamsRequest {
  string spirit_session_id = 1;
}

message TeamProgressView {
  string team_id = 1;
  string team_name = 2;
  string status = 3;
  double progress_pct = 4;
  string current_step = 5;
  int64 duration_ms = 6;
}

message SynthesizeResultsRequest {
  string spirit_session_id = 1;
}

message SynthesizeResultsResponse {
  string summary = 1;
  bool all_success = 2;
  repeated TaskSynthesisView task_results = 3;
}
```

---

## 十、测试策略

| 层 | 文件 | 覆盖 | 阶段 |
|----|------|------|------|
| Biz | `session_tree_test.go` | Session 树查询、深度限制 | P0 |
| Biz | `team_spirit_test.go` | AutoCreated Team 创建、SpiritSessionID 关联 | P0 |
| Biz | `spirit_team_usecase_test.go` | AssembleTeam / CancelTeam / AutoArchive / DAG 拓扑路由 | P0 |
| Biz | `task_planner_test.go` | Plan 阶段逻辑 | P0.5 |
| Biz | `agent_allocator_test.go` | Allocate 阶段逻辑 | P0.5 |
| Biz | `task_orchestrator_test.go` | Orchestrate 阶段逻辑 | P0.5 |
| Biz | `spirit_parallel_config_test.go` | ParallelConfig 默认值、校验 | P1 |
| Biz | `spirit_team_parallel_test.go` | ListActiveTeams、并行度检查 | P1 |
| Biz | `spirit_task_dag_test.go` | DAG 校验、环检测、拓扑路由 | P2 |
| Biz | `spirit_synthesis_test.go` | 合成策略（模板/LLM/混合） | P2 |
| Biz | `orchestration_cache_test.go` | 缓存命中、DQ Score 阈值 | P2 |
| Biz | `spirit_complexity_test.go` | 规则引擎 simple/moderate/complex 三级 | P4 |
| Service | `spirit_team_test.go` | AssembleTeam 流程、Envelope 发射、TeamStarter 生命周期 | P0 |
| Service | `team_turn_hooks_test.go` | Team Turn 完成回调、生命周期事件发布 | P0 |
| Service | `spirit_synthesis_test.go` | Synthesize 流程 | P2 |
| 前端 | `useSpiritTeamStore.spec.ts` | 团队列表加载、面板切换、展开状态、事件处理、并行团队状态 | P0 |
| 前端 | `TaskExecutionPanel.spec.ts` | 三区布局、WS 实时更新 | P0 |
| 前端 | `ParallelTeamOverview.spec.ts` | 并行团队总览卡片 | P1 |
| 前端 | `MemberReadOnlyPanel.spec.ts` | 只读模式、输入框隐藏（P1） | P1 |

E2E：SP-E2E-01（精灵对话 → 组建团队 → 查看执行面板 → 查看成员 → 返回精灵）。

---

## 十一、关联模块

| 模块 | 关系 |
|------|------|
| 1 Chat | 精灵对话面板、团队组建卡片、任务执行面板 |
| 11 Team | 精灵自动创建 Team、TeamRun 状态追踪、TeamKey UUID、依赖调度 |
| 53 Orchestration | Agent 节点状态投影、执行时间线、Task DAG 拓扑路由、Graph 引擎复用 |
| 10 Session | Session 树状关联、深度限制 |
| 7 Agent Evolution | DQ Score 驱动编排缓存、进化闭环 |
| 39 Planner | 远期：A2UI Planner 生成结构化执行计划 |
| superpowers Builtin Agents | 精灵/编排管家定义、三阶段编排工具 + 3 个新工具 |
| superpowers Memory/Butler | Session 数据 → 记忆管家/技能管家分析输入 |
| superpowers Learning Loop | 编排策略 Pattern 检测 → Proposal |
| 1 Chat Execution Trace | ChatExecutionCard 复用 |

---

## 十二、实施记录

### 12.1 P0 实施优化记录（2026-06-01）

**后端优化**：

| 优化项 | 修复方案 |
|--------|----------|
| `biz.SpiritAgentKey` 常量统一 | 抽取到 `internal/biz/agent_types.go` |
| `CompressorDeps` 聚合接口 | 定义聚合接口嵌入 7 个子接口，简化 Wire 绑定 |
| `GetRootSession` 循环保护 | 增加最大遍历深度限制 `maxDepth = 10` |
| `truncateTaskDesc` rune 截断 | 改用 `utf8.RuneCountInString` + `[]rune` |
| `seed_system_admin.go` kerrors | 改为 `kerrors.InternalServer` |
| Proto 缺失字段 | 添加 `micro_compact_enabled`/`memory_compact_enabled`/`tool_result_gate_enabled` |
| `plugin.NewUsecase` 缺 lg | 添加 `loggateway.Logger` 参数 |
| `ReadinessProbe` Wire 绑定 | 添加 `wire.Bind` |

**前端优化**：

| 优化项 | 修复方案 |
|--------|----------|
| TaskExecutionPanel XSS | 接入 `renderChatMarkdown`（markdown-it + DOMPurify） |
| `archiveTeam` 错误 API | 改为本地移除（后端归档 API 在 P1 实现） |
| `api.ts` 内联 import | 改为顶部 import |

### 12.2 P0.5 三阶段编排实施记录（2026-06-06）

| 维度 | P0 设计 | P0.5 实际实现 |
|------|---------|--------------|
| 精灵路由 | `AgentKey == "__spirit__"` 硬编码拦截 | 精灵走 `runSingleAgentViaTRPC`，通过 `spiritCustomTools` 注入工具 |
| 核心工具 | `assemble_team` | `plan_and_execute`（三阶段统一入口） |
| 辅助工具 | `list_butlers` / `query_butler_status` | `check_progress` / `cancel_orchestration` / `synthesize_results` / `build_orchestration_graph` |
| 团队组建 | 路由层无条件组建 | LLM 自主决策是否调用 `plan_and_execute` |
| 拓扑选择 | 硬编码 `coordinator` | `TaskDAG.RouteTopology()` 自动路由 |
| 并行支持 | 无 | `ParallelConfig` + DAG 依赖调度 |
| 结果合成 | 无 | `synthesize_results` 工具 + `SynthesisResultCard` |
| 自动归档 | 无 | `AutoArchiveCompletedTeams` |

**工具演进**：`assemble_team` / `assess_complexity` / `check_team_progress` / `cancel_team` → DEPRECATED，由 `plan_and_execute` / `check_progress` / `cancel_orchestration` / `synthesize_results` / `build_orchestration_graph` 替代。

### 12.3 深度架构审查修复记录（2026-06-06）

对 Spirit Team 全链路进行深度架构审查，发现并修复 7 个严重问题 + 5 个中等问题 + 3 个轻微问题。

**严重问题**：OrchestrationCache 死锁（提取 `listLocked()`）、超时回调不触发依赖调度（新增 `TimeoutHandler` 接口）、`interrupted` 状态语义不一致（switch 增加分支）、前后端枚举不一致（对齐 `SpiritTeamMode` / `SpiritTeamStatus`）、SynthesisResultCard XSS（替换为 `renderChatMarkdown`）、cancelTeam 移除团队改为状态更新。

**中等问题**：HandleTeamTurnResult 未取消超时定时器、BuildGraphConfig 无循环检测、前端状态回退校验、mode 默认值运算符、时间戳格式化。

**迭代建议修复**（10 项全部实施）：Options 模式重构 `NewSpiritTeamUsecase`、`TeamGraphSessionRepo` 拆分、`RecordCompletionWithAgents` 原子操作、app.go 日志统一、`SetTimeoutHandler` sync.Once、魔法数字命名常量、`BuildGraphConfig` 拆分子方法、`buildSpiritTeamDefinitionJSON` 命名常量、`AutoArchiveCompletedTeams` 批量归档。

**二轮审查阻塞项**（8 项全部修复）：`TeamOrchestrationDeps` 移除上帝接口、`TeamRepository` 标记 Deprecated + 迁移、超时回调 context 超时控制、前端 `as any` 类型逃逸、废弃状态值清理、类型参数强化、`direct` 死代码移除。

**三轮审查修复**（6 项）：`isValidTeamStatus` 运行时必崩修复、类型参数二次修复、`direct` 死代码残留清理、`TaskExecutionPanel.vue` 遗漏 `as any`、DQ 权重硬编码替换为常量。后端建议项 8 项：`Runner` 依赖拆分、`TeamRepository` 迁移计划注释、超时回调 `Stop()` 方法、`kerrors` 错误链保留、截断长度命名常量、`AssembleTeam` 子方法提取、N+1 查询批量优化、`SpiritTeamController` 窄接口。

### 12.4 已知技术债

| 编号 | 问题 | 优先级 |
|------|------|--------|
| TD-1 | 前端 `api.ts` 双键名兼容（`raw.teamName ?? raw.team_name`） | P1 |
| TD-2 | `ListSpiritTeams` RPC 未暴露为 HTTP 端点 | P1 |
| TD-3 | `ArchiveTeam` RPC 未定义，归档仅后端自动触发 | P1 |
| TD-4 | `MemberReadOnlyPanel` 仅有占位符 | P1 |
| TD-5 | `TeamMemberTreeNode` 未实现，仅有扁平头像列表 | P1 |
| TD-6 | 面包屑导航未实现 | P1 |
| TD-7 | 重试失败团队功能未实现 | P1 |
| TD-8 | `TeamRepository` 23 方法上帝接口待迁移 | P2 |
