# LLM 回复数据顺序与 Plan 执行架构重设计

> **设计稿** · 2026-07-02 · 状态：待用户复核
>
> **范围**：chat 模块「思考-act-回复」显示顺序 + 任务计划面板执行机制
> **方式**：推倒重来（按层切换）
> **前置 spec**：[2026-06-27-chat-ui-streaming-fix-design.md](./2026-06-27-chat-ui-streaming-fix-design.md)（部分实施后仍有顺序/状态同步问题，本 spec 为根本性重设计）

---

## 一、背景与问题陈述

### 1.1 用户报告

LLM 回复数据的「思考-action-回复」显示顺序问题，**经过一周修改优化仍未完全解决**。同时任务计划面板的执行机制存在状态不同步、孤儿 step、ChatSessionID 传播脆弱等问题。

### 1.2 当前实现的根因（一周改动无效的原因）

| 问题类别 | 根因 |
|---------|------|
| 排序错乱 | Timestamp 不稳定（updated/completed 事件覆盖 created 时间）+ `compareActivities` 4 条 patch 规则不可扩展 + 同毫秒冲突 |
| 父子关系错乱 | 三方（service/team/agent）共享确定性 ID 函数但靠约定，re-parenting 是后端 Projector 配置缺陷的前端补救 |
| Plan 状态不同步 | service 层 `updatePlanStepForTeam` 反向同步绕过 sequencer，与 projector 内部状态机重复且竞态 |
| 刷新丢数据 | direct-publish 路径无重试无死信，失败仅 warn 日志 |
| Streaming 与 created 乱序 | WS 无序到达 + 前端 `Object.assign` 合并靠注释维护 |
| Meta 类型不安全 | `map[string]any` + 链式类型断言 |
| 双路径创建 dedup | 同一 plan 在 planner 路径和 projector 路径都会创建，靠 fallback key |
| 双轨 Plan 模型 | `Plan/PlanStep`（废弃但保留）+ `TaskPlan/SubTask`（实际使用），状态机形同虚设 |
| 无中央 Plan Executor | 当前靠 team 完成事件反向更新 plan step，无正向调度器 |

### 1.3 当前两条发布路径不对等（核心复杂度来源）

```
路径 A：trpc-agent-go runtime 回调
  → ActivityProjector.ProcessEvent
  → ActivityEventSequencer.publish（256 buffer + 16ms 批合并 + 单 worker FIFO）
  → processTask：分配版本 → 推 persistChan → bus.Publish
  → 持久化：UpsertActivity（VersionLT 守卫）+ 5 次重试 + 512 死信
  → SequencerHandled=true，Bus 跳过持久化

路径 B：service/team 包 direct-publish（team_stage / graph_stage / plan / session）
  → 直接 bus.Publish，SequencerHandled=false
  → Bus：规范化 SessionID + atomic 分配版本 + safego.Go 异步 UpsertActivity
  → 无批合并、无 FIFO、无重试、无死信
```

**痛点**：thinking/action/reply 走 A 路径有完整保障，team_stage/plan/session 走 B 路径基本"裸奔"。最致命的是 `updatePlanStepForTeam` 绕过 sequencer，靠同步 `UpsertActivity` 兜底 stale read，与 projector 内部的 plan 状态机逻辑重复且可能竞态。

---

## 二、设计目标

### 2.1 功能性目标

1. **顺序保证**：thinking → action → reply 严格按业务顺序显示，无乱序
2. **跨层嵌套**：spirit → team → member 三层结构清晰，子活动正确挂载到父节点
3. **Plan 状态实时同步**：team 完成 → plan step 立即更新，刷新不丢失
4. **Plan 执行可视化**：用户能看到依赖图、并行/串行、执行进度
5. **失败可追溯**：每个 step 失败原因、责任方、级联影响清晰

### 2.2 非功能性目标

1. **前端零推理**：分组/顺序/父子关系全部由后端编码进数据，前端只读不推理
2. **类型安全**：用 typed struct 替代 `map[string]any`
3. **单管道**：所有事件统一走 sequencer，享有同等保障
4. **可扩展**：新增 kind 不需要修改排序规则
5. **三层硬约束**：spirit → team → member，不支持递归 team

### 2.3 非目标（明确排除）

- 不支持 team 内 team 递归
- 不保留历史会话数据（全新数据库）
- 不重构 WS 传输层、鉴权、路由、DI 框架
- 不重构 trpc-agent-go 集成层（stream_consumer 改造即可）
- 不引入新依赖（不引 d3/rxjs 等）

---

## 三、架构设计

### 3.1 架构原则

1. **数据即真相**：分组/顺序/父子关系全部由后端编码进数据
2. **Seq 替代 Timestamp**：后端分配单调递增 Seq，前端按 Seq 排序
3. **单管道发布**：所有事件源走同一个 sequencer
4. **显式 PlanExecutor**：正向调度器替代反向同步
5. **类型安全**：typed struct 替代 `Meta map[string]any`
6. **三层硬约束**：spirit → team → member

### 3.2 Entity 层级与数据模型

#### 3.2.1 层级结构

```
Session (spirit_session_id)
└── Task (root_activity_id, user message)
    ├── PlanBoard? (可选)
    │   └── PlanStep (id, depends_on, mapped_team_stage_id, status)
    └── Turn (turn_id, seq)         ← 最小对话单元
        ├── ThinkingStep (seq=1)
        ├── ActionStep   (seq=2)
        ├── ReplyStep     (seq=3, is_final)
        ├── TeamStage?     ← turn 内触发 team 执行
        │   └── TeamRun (run_id, dag_node_id=plan_step.id)
        │       └── MemberSession (agent_key)
        │           └── Turn (member 自己的 turn, seq)
        │               └── ThinkingStep / ActionStep / ReplyStep
        └── (并行其他 TeamStage)
```

**关键设计**：每个 entity 都有：
- `task_id`（聚合根，按 task 索引）
- `seq`（排序，后端分配）
- `parent_id`（父子关系）
- `session_id`（按 session 过滤 WS 推送）

前端按 `task_id` 索引、按 `seq` 排序，**不需要 computed 重算树**。

#### 3.2.2 后端数据模型

##### Session（新增）

```go
// internal/biz/session.go
type Session struct {
    ID             string  // spirit_session_id
    UserID         string
    SpiritAgentID  string  // spirit agent 配置 ID
    Status         SessionStatus  // active/completed/failed
    CreatedAt      time.Time
    UpdatedAt      time.Time
}

type SessionStatus string
const (
    SessionStatusActive    SessionStatus = "active"
    SessionStatusCompleted SessionStatus = "completed"
    SessionStatusFailed    SessionStatus = "failed"
)
```

##### Task（替代 root activity）

```go
// internal/biz/task.go
type Task struct {
    ID            string  // = root activity id（前端 task 节点）
    SessionID     string  // spirit_session_id
    UserMessage   string
    Status        TaskStatus  // pending/running/completed/failed/cancelled
    Seq           int64   // 在 session 内的序号
    CreatedAt     time.Time
    UpdatedAt     time.Time
    CompletedAt   *time.Time
}

type TaskStatus string
const (
    TaskStatusPending   TaskStatus = "pending"
    TaskStatusRunning   TaskStatus = "running"
    TaskStatusCompleted TaskStatus = "completed"
    TaskStatusFailed    TaskStatus = "failed"
    TaskStatusCancelled TaskStatus = "cancelled"
)
```

##### Turn（最小对话单元，新增）

```go
// internal/biz/turn.go
type Turn struct {
    ID            string  // turn_id
    TaskID        string  // parent task
    SessionID     string  // 当前 session（spirit 或 team 或 member session）
    SpiritSessionID string  // 始终指向 spirit root session（WS 过滤用）
    ParentTurnID  string  // 嵌套时填：team member 的 turn 的 parent 是 team_stage 的某个 run turn
    AgentKey      string  // 谁的 turn
    TeamID        string  // 所属 team（spirit turn 为空）
    TeamStageID   string  // 所属 team_stage（member turn 时填）
    Seq           int64   // 在 task 内的全局序号（后端分配，单调递增）
    Status        TurnStatus
    StartedAt     time.Time
    CompletedAt    *time.Time
}

type TurnStatus string
const (
    TurnStatusRunning   TurnStatus = "running"
    TurnStatusCompleted TurnStatus = "completed"
    TurnStatusFailed    TurnStatus = "failed"
)
```

##### Step（turn 内的工作步骤）

```go
// internal/biz/step.go
type Step struct {
    ID            string  // step_id
    TurnID        string  // parent turn
    TaskID        string  // 冗余，便于按 task 索引
    SessionID     string  // 当前 session
    SpiritSessionID string  // spirit root session
    Kind          StepKind
    Seq           int64   // turn 内的序号（1, 2, 3...）
    Content       string  // 文本内容
    Reasoning     string  // thinking 的推理
    ToolName      string  // action 的工具
    ToolCallID    string
    ToolArgs      json.RawMessage  // 类型安全的 JSON
    ToolResult    json.RawMessage
    ToolDurationMs int64
    ToolErrorCode  string
    Status        StepStatus
    IsFinal       bool  // reply 是否为最终回复
    StartedAt     time.Time
    CompletedAt    *time.Time
}

type StepKind string
const (
    StepKindThinking StepKind = "thinking"
    StepKindAction   StepKind = "action"
    StepKindReply    StepKind = "reply"
    StepKindNotice   StepKind = "notice"
    StepKindConfirm  StepKind = "confirm"
    StepKindError    StepKind = "error"
)

type StepStatus string
const (
    StepStatusPending     StepStatus = "pending"
    StepStatusRunning     StepStatus = "running"
    StepStatusToolRunning StepStatus = "tool_running"
    StepStatusToolBlocked StepStatus = "tool_blocked"
    StepStatusCompleted   StepStatus = "completed"
    StepStatusFailed      StepStatus = "failed"
    StepStatusCancelled   StepStatus = "cancelled"
)
```

##### TeamStage（task 内的团队执行阶段）

```go
// internal/biz/team_stage.go
type TeamStage struct {
    ID            string  // team_stage_id
    TaskID        string  // parent task
    TurnID        string  // 触发 team 的 turn
    SessionID     string  // spirit_session_id
    TeamID        string
    DagNodeID     string  // 对应 plan_step.id（如有）
    DependsOn     []string  // 其他 team_stage.id（DAG 依赖）
    Status        TeamStageStatus
    Stage         TeamStageStage  // assembled/planning/executing/completed/failed
    Members       []MemberInfo  // 类型安全
    Strategy      string  // parallel/dag/coordinator
    StartedAt     time.Time
    CompletedAt    *time.Time
    Seq           int64   // 在 task 内的序号
}

type TeamStageStatus string
const (
    TeamStageStatusPending   TeamStageStatus = "pending"
    TeamStageStatusRunning   TeamStageStatus = "running"
    TeamStageStatusCompleted TeamStageStatus = "completed"
    TeamStageStatusFailed    TeamStageStatus = "failed"
    TeamStageStatusCancelled TeamStageStatus = "cancelled"
    TeamStageStatusWaitingHuman TeamStageStatus = "waiting_human"  // HITL
)

type TeamStageStage string
const (
    TeamStageStageAssembled  TeamStageStage = "assembled"
    TeamStageStagePlanning   TeamStageStage = "planning"
    TeamStageStageExecuting  TeamStageStage = "executing"
    TeamStageStageCompleted  TeamStageStage = "completed"
    TeamStageStageFailed     TeamStageStage = "failed"
)

type MemberInfo struct {
    AgentKey     string
    AgentName    string
    AvatarURL    string
    ChildSessionID string  // member 自己的 session ID
    Status       string  // pending/running/completed/failed/skipped
}
```

##### TeamRun（team 内的一次执行）

```go
// internal/biz/team_run.go
type TeamRun struct {
    ID            string  // run_id
    TeamStageID   string  // parent team_stage
    TaskID        string
    SessionID     string
    SpiritSessionID string
    DagNodeID     string  // 对应 plan_step.id
    DependsOn     []string  // 其他 run.id
    Status        TeamRunStatus  // running/completed/failed/cancelled
    StartedAt     time.Time
    CompletedAt    *time.Time
    Seq           int64
}

type TeamRunStatus string
const (
    TeamRunStatusRunning   TeamRunStatus = "running"
    TeamRunStatusCompleted TeamRunStatus = "completed"
    TeamRunStatusFailed    TeamRunStatus = "failed"
    TeamRunStatusCancelled TeamRunStatus = "cancelled"
)
```

##### MemberSession（team 内的成员会话）

```go
// internal/biz/member_session.go
type MemberSession struct {
    ID            string  // = SessionActivityID(teamID, agentKey)
    TeamRunID     string  // parent team_run
    TeamStageID   string
    TaskID        string
    SessionID     string  // member 自己的 session ID（用于 lazy load）
    SpiritSessionID string
    AgentKey      string
    AgentName     string
    AvatarURL     string
    Status        MemberSessionStatus
    Seq           int64
}

type MemberSessionStatus string
const (
    MemberSessionStatusPending   MemberSessionStatus = "pending"
    MemberSessionStatusRunning   MemberSessionStatus = "running"
    MemberSessionStatusCompleted MemberSessionStatus = "completed"
    MemberSessionStatusFailed    MemberSessionStatus = "failed"
    MemberSessionStatusSkipped   MemberSessionStatus = "skipped"
)
```

##### PlanBoard（任务计划面板）

```go
// internal/biz/plan_board.go
type PlanBoard struct {
    ID            string  // plan_id
    TaskID        string  // parent task
    TurnID        string  // 触发 plan 的 turn
    SessionID     string  // spirit_session_id
    Strategy      PlanStrategy  // sequential/parallel/dag/coordinator
    Status        PlanStatus
    Steps         []PlanStep  // 数组，每个 step 有 id 和 depends_on
    StartedAt     time.Time
    CompletedAt    *time.Time
    Seq           int64
}

type PlanStrategy string
const (
    PlanStrategySequential  PlanStrategy = "sequential"
    PlanStrategyParallel    PlanStrategy = "parallel"
    PlanStrategyDAG         PlanStrategy = "dag"
    PlanStrategyCoordinator PlanStrategy = "coordinator"
)

type PlanStatus string
const (
    PlanStatusPlanning    PlanStatus = "planning"
    PlanStatusExecuting   PlanStatus = "executing"
    PlanStatusCompleted   PlanStatus = "completed"
    PlanStatusFailed      PlanStatus = "failed"
    PlanStatusPartialFailure PlanStatus = "partial_failure"
)
```

##### PlanStep（计划步骤）

```go
// internal/biz/plan_step.go
type PlanStep struct {
    ID              string  // step_id
    PlanID          string  // parent plan_board
    TaskID          string  // 冗余，便于按 task 索引
    Label           string
    Description     string
    DependsOn       []string  // 其他 plan_step.id
    MappedTeamStageID string  // 执行该 step 的 team_stage（如有；coordinator 模式下所有 step 共享一个 team_stage）
    Status          PlanStepStatus
    AutoSynthesis   bool  // 是否为汇总报告 step（无 team 映射，依赖完成自动触发）
    StartedAt       time.Time
    CompletedAt     *time.Time
    Seq             int64  // 在 plan 内的序号
    Result          *StepResult  // 完成时携带
    Error           *StepError    // 失败时携带
}

type PlanStepStatus string
const (
    PlanStepStatusPending        PlanStepStatus = "pending"
    PlanStepStatusRunning        PlanStepStatus = "running"
    PlanStepStatusCompleted      PlanStepStatus = "completed"
    PlanStepStatusFailed        PlanStepStatus = "failed"
    PlanStepStatusSkipped       PlanStepStatus = "skipped"  // 依赖失败导致跳过
    PlanStepStatusPartialFailure PlanStepStatus = "partial_failure"
)

type StepResult struct {
    Output         string  // team 的输出摘要
    MemberReports  []MemberReport
    TokensUsed     TokenUsage
    DurationMs     int64
}

type StepError struct {
    Code          string  // tool_failed / llm_timeout / team_failed / dependency_failed
    Message        string
    Retryable      bool
    FailedMember   *MemberReport  // 哪个 member 失败
}

type MemberReport struct {
    AgentKey    string
    AgentName   string
    Output      string
    TokensUsed  TokenUsage
    DurationMs  int64
    Error        string  // 失败时填
}

type TokenUsage struct {
    PromptTokens     int64
    CompletionTokens int64
    TotalTokens      int64
}
```

#### 3.2.3 数据库 Schema（Ent）

新增 7 张表（替代 `activities` 表）：

| 表名 | 主键 | 关键字段 | 索引 |
|------|------|---------|------|
| `sessions` | id | user_id, spirit_agent_id, status | (user_id, status) |
| `tasks` | id | session_id, status, seq | (session_id, seq) |
| `turns` | id | task_id, session_id, spirit_session_id, parent_turn_id, agent_key, team_id, team_stage_id, seq, status | (task_id, seq), (spirit_session_id, seq) |
| `steps` | id | turn_id, task_id, session_id, spirit_session_id, kind, seq, status, is_final | (turn_id, seq), (task_id, seq), (spirit_session_id, seq) |
| `team_stages` | id | task_id, turn_id, team_id, dag_node_id, status, stage, strategy, seq | (task_id, seq), (spirit_session_id, seq) |
| `team_runs` | id | team_stage_id, task_id, dag_node_id, status, seq | (team_stage_id, seq) |
| `member_sessions` | id | team_run_id, team_stage_id, task_id, agent_key, status, seq | (team_run_id, seq), (agent_key) |

新增 plan 相关表（替代 `task_plans` + activity Meta.steps）：

| 表名 | 主键 | 关键字段 |
|------|------|---------|
| `plan_boards` | id | task_id, turn_id, strategy, status, seq |
| `plan_steps` | id | plan_id, task_id, label, depends_on, mapped_team_stage_id, status, auto_synthesis, seq |

**废弃表**：`activities`、`task_plans`（旧 Plan 模型）、`plan_steps`（旧 PlanStep 模型，与新表名冲突需 rename）。

#### 3.2.4 ID 生成策略

- **Session ID**：用户发起会话时由 service 层生成（UUID v4）
- **Task ID**：用户每次输入由 service 层生成（UUID v4）
- **Turn ID**：每次 LLM 回合开始时由 projector 生成（UUID v4）
- **Step ID**：每个 step 创建时由 projector 生成（UUID v4）
- **TeamStage ID**：`uuid.NewSHA1(aranea.team_stage.v2, teamID)`（保留确定性，便于跨包引用）
- **TeamRun ID**：team 启动时由 runner 生成（UUID v4）
- **MemberSession ID**：`uuid.NewSHA1(aranea.member_session.v2, teamID:agentKey)`
- **PlanBoard ID**：planner 创建时生成（UUID v4）
- **PlanStep ID**：planner 解析 LLM 输出时生成（UUID v4 或 LLM 提供的 id）

**版本号 v2**：与旧 v1 namespace 隔离，避免历史数据混淆。

#### 3.2.5 Seq 分配策略

```go
// internal/agent/seq_assigner.go
type SeqAssigner struct {
    counters sync.Map  // sessionID → *atomic.Int64
}

func (s *SeqAssigner) NextSeq(spiritSessionID string) int64 {
    v, _ := s.counters.LoadOrStore(spiritSessionID, &atomic.Int64{})
    return v.(*atomic.Int64).Add(1)
}
```

- 每个 spirit session 维护一个 atomic counter
- Task/Turn/Step/TeamStage/TeamRun/MemberSession/PlanBoard/PlanStep 都从同一个 counter 取 Seq
- **同一个 spirit session 内的所有 entity 按 Seq 全局排序**
- 不同 entity 类型共享同一 counter，避免跨类型 Seq 冲突

**Seq 持久化**：重启后从 DB 恢复（查询 `MAX(seq) WHERE spirit_session_id = ?`）。

### 3.3 单管道发布架构

#### 3.3.1 统一入口

```go
// internal/agent/activity_event_sequencer.go
type Sequencer struct {
    publishQueue  chan publishTask  // 256 buffer
    persistChan   chan persistItem   // 256 buffer
    activityRepo  ActivityRepo
    eventBus      EventBus
    seqAssigner   *SeqAssigner
    versionSeq    atomic.Int64
    deadLetter    []biz.Activity  // ring buffer 512
    done          chan struct{}
}

// Publish 统一入口（替代 direct-publish）
func (s *Sequencer) Publish(ctx context.Context, event Event) {
    task := publishTask{
        event:   event,
        persist: s.shouldPersist(event),  // streaming 不持久化单 chunk
    }
    select {
    case s.publishQueue <- task:
    case <-ctx.Done():
        return
    }
}
```

#### 3.3.2 事件类型体系

```go
// internal/biz/event.go
type Event interface {
    EventKind() EventKind
    SpiritSessionID() string
    TaskID() string
}

type EventKind string
const (
    EventKindTaskCreated       EventKind = "task.created"
    EventKindTaskUpdated       EventKind = "task.updated"
    EventKindTaskCompleted     EventKind = "task.completed"
    EventKindTaskFailed        EventKind = "task.failed"
    
    EventKindTurnStarted       EventKind = "turn.started"
    EventKindTurnCompleted     EventKind = "turn.completed"
    
    EventKindStepCreated       EventKind = "step.created"
    EventKindStepStreaming     EventKind = "step.streaming"
    EventKindStepUpdated       EventKind = "step.updated"
    EventKindStepCompleted     EventKind = "step.completed"
    EventKindStepFailed        EventKind = "step.failed"
    
    EventKindTeamStageCreated  EventKind = "team_stage.created"
    EventKindTeamStageUpdated  EventKind = "team_stage.updated"
    EventKindTeamStageCompleted EventKind = "team_stage.completed"
    EventKindTeamStageFailed   EventKind = "team_stage.failed"
    
    EventKindTeamRunStarted    EventKind = "team_run.started"
    EventKindTeamRunCompleted  EventKind = "team_run.completed"
    EventKindTeamRunFailed     EventKind = "team_run.failed"
    
    EventKindMemberSessionCreated  EventKind = "member_session.created"
    EventKindMemberSessionUpdated  EventKind = "member_session.updated"
    
    EventKindPlanBoardCreated  EventKind = "plan_board.created"
    EventKindPlanBoardUpdated  EventKind = "plan_board.updated"
    EventKindPlanStepUpdated   EventKind = "plan_step.updated"
    EventKindPlanStepStarted   EventKind = "plan_step.started"
    EventKindPlanStepCompleted EventKind = "plan_step.completed"
    EventKindPlanStepFailed    EventKind = "plan_step.failed"
    EventKindPlanStepSkipped   EventKind = "plan_step.skipped"
)
```

#### 3.3.3 发布流程

```
所有事件源（4 类）
  ├─ ActivityProjector（trpc-agent-go runtime 回调）
  ├─ Service 层（spirit_team 等）
  ├─ Team 层（runner）
  └─ PlanExecutor
        │
        ▼ Sequencer.Publish(ctx, event)
   ActivityEventSequencer
   ├─ 16ms 批合并（仅 step.streaming）
   ├─ FIFO 单 worker
   ├─ 分配 Seq（SeqAssigner）+ Version（atomic）
   ├─ persistChan（统一 5 次重试 + 512 死信）
   └─ bus.Publish → WS subscriber → 前端
```

**取消 direct-publish 路径**：service/team 包不再直接 `bus.Publish`，统一走 `Sequencer.Publish`。

#### 3.3.4 streaming 批合并规则

```go
// 16ms 窗口内可合并的条件：
// 1. 都是 EventKindStepStreaming
// 2. StepID 相同
// 3. DeltaField 相同（content / reasoning / tool_args）
func (s *Sequencer) canMergeDeltas(pending, current *publishTask) bool {
    if pending.event.EventKind() != EventKindStepStreaming ||
       current.event.EventKind() != EventKindStepStreaming {
        return false
    }
    pendingStep := pending.event.(*StepStreamingEvent)
    currentStep := current.event.(*StepStreamingEvent)
    return pendingStep.StepID == currentStep.StepID &&
           pendingStep.DeltaField == currentStep.DeltaField
}
```

#### 3.3.5 持久化策略

每个 entity 类型有独立的 Repo 和 Upsert 方法：

| Entity | Repo | 持久化策略 |
|--------|------|----------|
| Task | TaskRepo | UpsertTask（VersionLT 守卫） |
| Turn | TurnRepo | UpsertTurn |
| Step | StepRepo | UpsertStep（streaming chunk 不入库；step.created/updated/completed 事件触发持久化，最终 content/reasoning 在 completed 时落库） |
| TeamStage | TeamStageRepo | UpsertTeamStage |
| TeamRun | TeamRunRepo | UpsertTeamRun |
| MemberSession | MemberSessionRepo | UpsertMemberSession |
| PlanBoard | PlanBoardRepo | UpsertPlanBoard（含 steps JSON） |
| PlanStep | PlanStepRepo | UpsertPlanStep |

**streaming 持久化细节**：
- `step.streaming` 事件**不入库**（仅推送 WS，前端累加 content/reasoning）
- sequencer 在内存维护 step 的累积 content（同 step ID 的 streaming chunk 累加）
- `step.created` 事件：插入 step row（content 为空）
- `step.updated`/`step.completed` 事件：更新 content/reasoning/status（VersionLT 守卫）
- 进程崩溃恢复：DB 中 step.content 是最后一次 updated/completed 时的快照，可能丢失最后几个未 flush 的 streaming chunk（可接受，前端 WS 实时收到完整内容）

**统一重试**：5 次指数退避（100/200/400/800/1600ms）+ 512 死信环形缓冲（按 entity ID 去重）。

### 3.4 ActivityProjector 改造

ActivityProjector 适配新模型，按 trpc-agent-go 回调生成对应 entity 事件：

| trpc-agent-go 回调 | 生成事件 |
|-------------------|---------|
| `OnTurnStart` | `task.created` + `turn.started` |
| `OnReasoningDelta` | `step.streaming`（kind=thinking, delta_field=reasoning） |
| `OnReasoningDone` | `step.completed`（thinking） |
| `OnTextDelta` | `step.streaming`（kind=reply, delta_field=content） |
| `OnTextDone` | `step.completed`（reply, is_final=true 如果是最后一个） |
| `OnMemberMessageDelta` | member turn 的 `step.streaming`（kind=reply） |
| `OnMemberMessageDone` | member turn 的 `step.completed` |
| `OnToolCall` | `step.created` + `step.updated`（kind=action, status=tool_running） |
| `OnToolResult` | `step.completed`/`step.failed`（action） |
| `OnNotice` | `step.created` + `step.completed`（kind=notice） |
| `OnConfirmRequest` | `step.created`（kind=confirm, status=tool_blocked） |
| `OnConfirmResult` | `step.completed`/`step.cancelled`（confirm） |
| `OnTurnEnd` | `turn.completed` + `task.completed`（root） |
| `OnError` | `task.failed` |

**ProjectMeta** 适配：

```go
type ProjectMeta struct {
    SessionID        string  // 当前 session
    SpiritSessionID  string  // spirit root session
    TaskID           string  // parent task
    TurnID           string  // current turn
    ParentTurnID     string  // 嵌套时填
    TeamID           string
    TeamStageID      string
    TeamRunID        string
    MemberSessionID  string
    AgentKey         string
    AgentName        string
    MemberAgentKeys  map[string]struct{}
    TaskContent      string
}
```

**取消**：`ParentActivityID`、`ActivityKind`、`ActivityStatus`、`Meta map[string]any`。

### 3.5 PlanExecutor 设计

#### 3.5.1 组件定位

```go
// internal/service/plan_executor.go
type PlanExecutor struct {
    taskRepo        TaskRepo
    planBoardRepo   PlanBoardRepo
    planStepRepo    PlanStepRepo
    teamStageRepo   TeamStageRepo
    teamRunRepo     TeamRunRepo
    sequencer       *Sequencer
    teamOrchestrator TeamOrchestrator  // 现有
    logger          loggateway.Logger
    
    // 订阅中的 plan：planID → *executionContext
    contexts sync.Map
}

type executionContext struct {
    PlanBoard      *PlanBoard
    PendingSteps   map[string]bool  // 未完成的 step
    RunningSteps   map[string]bool  // 运行中的 step
    CompletedSteps map[string]bool
    FailedSteps    map[string]bool
    SkippedSteps   map[string]bool
    
    // 事件通道：team 完成事件
    TeamCompleteCh chan TeamCompleteEvent
}
```

#### 3.5.2 执行流程

```
1. PlanExecutor.Subscribe(planBoard)
   ├─ 解析 DAG → 拓扑排序
   ├─ 识别并行层（同层 step 无依赖关系）
   ├─ 创建 executionContext
   └─ 启动第一批 step（无依赖的 root steps）

2. 派发 step 给 TeamOrchestrator
   ├─ PlanExecutor 创建 TeamStage（声明 entity，status=pending, dag_node_id=step.id）
   ├─ PlanExecutor 发布 team_stage.created 事件 → sequencer → WS
   ├─ PlanExecutor 调用 TeamOrchestrator.Orchestrate（异步执行）
   │   └─ TeamOrchestrator 内部创建 TeamRun + MemberSession，启动 runner
   ├─ PlanExecutor 更新 step.status=running（通过状态机 Transition）
   └─ PlanExecutor 发布 plan_step.started 事件 → sequencer → WS

3. 监听 team 完成事件
   ├─ PlanExecutor 接收 TeamCompleteEvent（来自 TeamOrchestrator 的回调）
   ├─ 查找 dag_node_id == team_stage.dag_node_id 的 step
   ├─ 更新 step.status = completed/failed（通过状态机 Transition）
   ├─ 持久化 PlanStep（PlanStepRepo.UpsertPlanStep，同步）
   ├─ 发布 plan_step.completed/failed 事件 → sequencer → WS
   └─ 触发 checkDownstream(step)

4. checkDownstream(completedStep)
   ├─ 找出所有 depends_on 包含 completedStep.id 的 step
   ├─ 对每个下游 step：
   │   ├─ 检查所有 depends_on 是否都 completed
   │   ├─ 是 → 派发该 step 给 TeamOrchestrator
   │   └─ 否 → 等待
   └─ 如果 completedStep.status=failed 且 strategy=DAG：
       └─ 级联失败所有下游 step（status=skipped, reason=dependency_failed）

5. handleSynthesisStep
   ├─ 每个 step 完成后检查所有 auto_synthesis=true 的 step
   ├─ 如果其 depends_on 全部 completed/failed → 标记 completed（状态机 Transition）
   └─ 发布 plan_step.completed 事件 → sequencer → WS

6. checkAllDone
   ├─ 所有 step terminal（completed/failed/skipped）
   ├─ 更新 plan_board.status：
   │   ├─ 全 completed → completed
   │   ├─ 有 failed → failed
   │   └─ 有 skipped → partial_failure
   └─ 推送 system message 给 Spirit LLM → 触发综合回合
```

**责任划分**：
- **PlanExecutor**：调度，创建 TeamStage（声明 entity）、跟踪 step 状态、触发下游、处理 synthesis、通知 Spirit LLM
- **TeamOrchestrator**：执行，创建 TeamRun + MemberSession、启动 runner、监听 runner 完成事件、回调 PlanExecutor

#### 3.5.3 执行策略实现

```go
func (p *PlanExecutor) dispatchStep(ctx context.Context, plan *PlanBoard, step *PlanStep) error {
    switch plan.Strategy {
    case PlanStrategySequential:
        // 一次只派发一个 step，等完成再派下一个
        return p.teamOrchestrator.Orchestrate(ctx, step)
        
    case PlanStrategyParallel:
        // 所有无依赖的 step 同时派发
        return p.teamOrchestrator.Orchestrate(ctx, step)
        
    case PlanStrategyDAG:
        // 按拓扑序，同层并行，跨层串行
        return p.teamOrchestrator.Orchestrate(ctx, step)
        
    case PlanStrategyCoordinator:
        // 单 team 容纳所有 agents，coordinator agent 内部分派
        if step.Seq == 1 {  // 仅第一个 step 派发
            return p.teamOrchestrator.OrchestrateCoordinator(ctx, plan)
        }
        return nil  // 后续 step 由 coordinator 内部触发
    }
    return nil
}
```

#### 3.5.4 Step 级状态机

```go
// internal/biz/plan_step_state_machine.go
var planStepTransitions = map[PlanStepStatus][]PlanStepStatus{
    PlanStepStatusPending: {
        PlanStepStatusRunning,
        PlanStepStatusSkipped,  // 依赖失败时跳过
    },
    PlanStepStatusRunning: {
        PlanStepStatusCompleted,
        PlanStepStatusFailed,
        PlanStepStatusSkipped,
        PlanStepStatusPartialFailure,
    },
    PlanStepStatusCompleted:    {},  // terminal
    PlanStepStatusFailed:       {PlanStepStatusRunning},  // 允许重试
    PlanStepStatusSkipped:      {},  // terminal
    PlanStepStatusPartialFailure: {PlanStepStatusRunning},  // 允许重试
}

func (s *PlanStep) Transition(to PlanStepStatus) error {
    allowed, ok := planStepTransitions[s.Status]
    if !ok {
        return fmt.Errorf("unknown source status: %s", s.Status)
    }
    for _, a := range allowed {
        if a == to {
            s.Status = to
            return nil
        }
    }
    return fmt.Errorf("invalid transition: %s → %s", s.Status, to)
}
```

**禁止跳过状态机直接赋值**（解决当前 `updatePlanStepForTeam` 直接赋值的痛点）。

#### 3.5.5 反馈机制

##### 反馈对象与形式

| 反馈对象 | 反馈内容 | 形式 | 用途 |
|---------|---------|------|------|
| **PlanExecutor** | step.status 变更 | 内部 channel 事件 | 触发下游 step / 触发综合回合 |
| **DB** | step.status 变更 | PlanStepRepo.UpsertPlanStep（同步） | 持久化状态 |
| **前端 WS** | step.status 变更 | `plan_step.started/completed/failed/skipped` 事件 | UI 更新 |
| **Spirit LLM** | 全部完成 | system push 消息（注入到 spirit session） | 触发综合回合 |
| **plan_and_execute 工具调用方** | 整体完成/失败 | tool_result 返回值 | LLM 工具调用反馈 |

##### UI 通知策略

- **step 完成不弹通知**（避免噪音），仅在 PlanBoard 内更新状态色
- **整个 PlanBoard 完成**：在 task 下方插入新 turn，显示「正在综合所有团队结果...」
- **step 失败**：PlanBoard 顶部红色 banner + 失败 step 节点红色高亮 + 展开错误详情
- **synthesis 触发**：在 task 下方插入新 turn，spirit LLM 开始推理

### 3.6 前端组件架构

#### 3.6.1 Store 数据结构

```typescript
// web/src/features/chat/stores/useChatStore.ts
interface ChatStore {
    sessions: Map<string, Session>
    tasks: Map<string, Task>
    turns: Map<string, Turn>
    steps: Map<string, Step>  // streaming 修改 step.content/reasoning
    teamStages: Map<string, TeamStage>
    teamRuns: Map<string, TeamRun>
    memberSessions: Map<string, MemberSession>
    planBoards: Map<string, PlanBoard>
    planSteps: Map<string, PlanStep>
    
    // 按 task 聚合
    getTaskTree(taskId: string): TaskTree
    getSessionTasks(sessionId: string): Task[]
}
```

**扁平存储**：所有 entity 按 ID 存 Map，通过 `task_id` 索引关联。**不需要 computed 重算树**。

#### 3.6.2 渲染树构造

```typescript
// web/src/features/chat/composables/useTaskTree.ts
function buildTaskTree(taskId: string): TaskTree {
    const task = store.tasks.get(taskId)
    
    // 按 task_id 索引所有子 entity
    const turns = filterByTask(store.turns, taskId).sort(bySeq)
    const teamStages = filterByTask(store.teamStages, taskId).sort(bySeq)
    const planBoards = filterByTask(store.planBoards, taskId)
    
    // 每个 turn 的 steps
    const turnTrees = turns.map(turn => ({
        turn,
        steps: filterByTurn(store.steps, turn.id).sort(bySeq),
        teamStages: teamStages.filter(ts => ts.turnId === turn.id),
    }))
    
    return { task, planBoards, turnTrees, teamStages }
}
```

**排序规则**：`bySeq = (a, b) => a.seq - b.seq`，**无 patch**。

#### 3.6.3 组件层级

```
ChatPage
└── SessionPanel.vue
    └── TaskList.vue (按 task.seq)
        └── TaskCard.vue (用户消息 + 状态)
            ├── PlanBoardCard.vue? (可选)
            │   ├── PlanDAG.vue (SVG 依赖图)
            │   │   └── PlanStepNode.vue (DAG 节点)
            │   └── PlanStepDetailPanel.vue (点击节点展开)
            │
            └── TurnList.vue (按 turn.seq)
                └── TurnContainer.vue
                    ├── ThinkingBlock.vue (step.kind=thinking)
                    ├── ActionBlock.vue (step.kind=action)
                    ├── ReplyBlock.vue (step.kind=reply, is_final 标记)
                    ├── NoticeBlock.vue (step.kind=notice)
                    ├── ConfirmBlock.vue (step.kind=confirm)
                    ├── ErrorBlock.vue (step.kind=error)
                    │
                    └── TeamStagePanel.vue (turn 内有 team 执行时)
                        └── TeamRunCard.vue (按 team_run.seq)
                            └── MemberSessionPanel.vue (agent_key)
                                └── TurnList.vue (递归，三层封顶)
```

#### 3.6.4 WS 事件处理

```typescript
// web/src/features/chat/composables/useChatEventRouter.ts
function handleEvent(event: ChatEvent) {
    switch (event.kind) {
        case 'task.created':
            store.tasks.set(event.task.id, event.task)
            break
        case 'step.streaming':
            // 直接修改 step 的 content/reasoning（不触发 tree 重算）
            const step = store.steps.get(event.stepId)
            if (step) {
                step[event.deltaField] += event.deltaChunk
            }
            break
        case 'plan_step.completed':
            const ps = store.planSteps.get(event.stepId)
            if (ps) {
                ps.status = 'completed'
                ps.result = event.result
            }
            break
        // ... 其他事件
    }
}
```

**不再有**：
- `useActivityTimeline` computed 树
- `compareActivities` 4 条 patch
- re-parenting hack
- orphan 节点处理
- 跨 session 收集 + dedup
- `Object.assign(existing, {...snapshot, ...existing, timestamp: snapshot.timestamp})` 合并

#### 3.6.5 滚动与定位

保留 `useScrollToActivity` 模块级 ref 单例机制，适配新模型：
- `locate(agentKey, teamStageId, memberSessionId)` → 设置 `locateCommand`
- `TurnList` watch `locateCommand` → `autoExpandFor.value = agentKey || teamStageId`
- 通过 `data-agent-key` / `data-team-stage-id` 属性定位 DOM

### 3.7 Plan 依赖图可视化

#### 3.7.1 DAG 渲染

```vue
<!-- PlanBoardCard.vue -->
<template>
  <div class="plan-board-card">
    <header>
      <span class="plan-status" :class="statusClass">{{ statusLabel }}</span>
      <span class="plan-progress">{{ completedCount }}/{{ totalSteps }}</span>
    </header>
    
    <PlanDAG :steps="plan.steps" @select="onSelectStep" />
    
    <PlanStepDetailPanel v-if="selectedStep" :step="selectedStep" />
  </div>
</template>
```

#### 3.7.2 DAG 布局算法

```typescript
// web/src/features/chat/composables/usePlanDAGLayout.ts
interface DAGNode {
    step: PlanStep
    x: number  // 列坐标
    y: number  // 行坐标
}

interface DAGEdge {
    from: string  // step.id
    to: string
    type: 'dependency'  // 仅依赖关系
}

// 拓扑分层算法
function layoutDAG(steps: PlanStep[]): { nodes: DAGNode[], edges: DAGEdge[] } {
    // 1. 计算每个 step 的层级（最长路径 from root）
    const levels = computeLevels(steps)
    
    // 2. 同层 step 垂直排列
    // 3. 跨层 step 横向连线
    // 4. 节点状态色：pending=灰、running=青色脉冲、completed=绿、failed=红、skipped=暗灰
    
    return { nodes, edges }
}
```

**视觉规则**：
- 同层 step 横向排列（并行执行）
- 不同层 step 纵向排列（串行依赖）
- 边用直线/贝塞尔曲线表达依赖
- 节点尺寸 80×40px，间距 40px

#### 3.7.3 节点状态色

| Status | 颜色 | 动效 |
|--------|------|------|
| pending | 灰色 `#9e9e9e` | 无 |
| running | 青色 `#00bcd4` | 脉冲动画（box-shadow 2s infinite） |
| completed | 绿色 `#4caf50` | ✓ 图标 |
| failed | 红色 `#f44336` | ✗ 图标 + 抖动 |
| skipped | 暗灰 `#616161` | — 图标 |
| partial_failure | 橙色 `#ff9800` | ⚠ 图标 |

#### 3.7.4 交互

- 点击节点 → 展开详情面板（输出/错误/member 列表）
- hover 节点 → 高亮所有上下游依赖路径
- 不做缩放/平移（YAGNI，节点数通常 < 20，固定布局足够；后续如需再迭代）

### 3.8 阻塞判定

#### 3.8.1 简化规则

```typescript
// web/src/features/chat/composables/useBlockedStatus.ts
function detectBlocked(turn: Turn, steps: Step[]): BlockedInfo | null {
    // 1. 工具阻塞：action.status == 'tool_running' && 无对应 ToolResult
    const toolBlocked = steps.find(s => 
        s.kind === 'action' && s.status === 'tool_running'
    )
    if (toolBlocked) return { type: 'tool', step: toolBlocked }
    
    // 2. 确认阻塞：confirm.status == 'tool_blocked'
    const confirmBlocked = steps.find(s => 
        s.kind === 'confirm' && s.status === 'tool_blocked'
    )
    if (confirmBlocked) return { type: 'confirm', step: confirmBlocked }
    
    // 3. LLM 阻塞：thinking/reply.status == 'running' && 5s 内 content/reasoning 未增长
    const llmBlocked = steps.find(s => 
        (s.kind === 'thinking' || s.kind === 'reply') &&
        s.status === 'running' &&
        !isStreamingActive(s)  // 检查最近 5s 是否有 streaming chunk
    )
    if (llmBlocked) return { type: 'llm', step: llmBlocked }
    
    return null
}

function isStreamingActive(step: Step): boolean {
    // 检查 step 的 lastStreamingAt 字段（前端维护）
    return Date.now() - step.lastStreamingAt < 5000
}
```

**改进**：
- 不再用 `meta.streaming === false`（依赖后端正确设置）
- 改用前端时间窗口判断（5s 内 content 是否增长）
- 后端兜底：LLM 超时（30-120min）+ `OnTurnEnd` 强制完成

#### 3.8.2 HITL 阻塞

```typescript
function detectHITLBlocked(teamStage: TeamStage): BlockedInfo | null {
    if (teamStage.status === 'waiting_human') {
        return { type: 'hitl', teamStage }
    }
    return null
}
```

---

## 四、数据库迁移策略

### 4.1 全新数据库（用户确认）

- **新建 7 + 2 = 9 张表**：sessions/tasks/turns/steps/team_stages/team_runs/member_sessions/plan_boards/plan_steps
- **废弃表**：activities、task_plans、旧 plan_steps（rename 为 plan_steps_legacy 后删除）
- **历史数据**：不迁移，开发环境直接重建，生产环境接受历史会话丢失
- **迁移方式**：Ent Schema 新增 → `go generate ./internal/data/ent` → L1 Auto-Migration（启动时自动建表）

### 4.2 Ent Schema 清单

新增 Schema 文件：

| 文件 | 表名 |
|------|------|
| `internal/data/ent/schema/session.go` | sessions |
| `internal/data/ent/schema/task.go` | tasks |
| `internal/data/ent/schema/turn.go` | turns |
| `internal/data/ent/schema/step.go` | steps |
| `internal/data/ent/schema/team_stage.go` | team_stages |
| `internal/data/ent/schema/team_run.go` | team_runs |
| `internal/data/ent/schema/member_session.go` | member_sessions |
| `internal/data/ent/schema/plan_board.go` | plan_boards |
| `internal/data/ent/schema/plan_step.go` | plan_steps |

**删除 Schema**：
- `internal/data/ent/schema/activity.go`
- `internal/data/ent/schema/task_plan.go`
- `internal/data/ent/schema/plan.go`（旧 PlanStep 模型）
- 所有 FTS5 索引引用 activities 表的迁移（如有）

### 4.3 DDL Migration Registry

新增 DDL 迁移：
- `sql/migrations/20260702_create_session_task_turn_step.sql` — 创建 9 张表 + 索引
- `sql/migrations/20260702_drop_activities_table.sql` — 删除 activities 表
- `sql/migrations/20260702_drop_task_plans_table.sql` — 删除 task_plans 表

---

## 五、实施分阶段（按层切换）

### Phase 1：后端新模型 + 新表（独立可验证）

**目标**：后端 entity + 数据库 + sequencer 上线，旧前端兼容新 WS 事件格式。

**改动**：
- 新建 `internal/biz/session.go`/`task.go`/`turn.go`/`step.go`/`team_stage.go`/`team_run.go`/`member_session.go`/`plan_board.go`/`plan_step.go`
- 新建 Ent Schema（9 张表）
- 重写 `internal/agent/activity_event_sequencer.go`：统一入口 + SeqAssigner
- 改造 `internal/agent/activity_projector.go`：适配新模型，按 trpc 回调生成新事件
- 改造 `internal/service/spirit_team.go`：移除 `updatePlanStepForTeam`，改为发事件到 sequencer
- 新建 `internal/service/plan_executor.go`
- 新建 `internal/biz/plan_step_state_machine.go`
- 改造 WS 推送层（`internal/server/ws_io_pump.go`）：发送新事件格式

**兼容层策略**（关键）：
- **新旧并行**：旧 `Activity` 类型 + 旧 sequencer 路径保留，仅用于旧前端兼容；新事件走新 sequencer + 新 entity 表
- **WS 双格式**：WS 推送时同时发新事件格式 + 旧 `ActivityEvent` 格式（adapter 转换），让旧前端继续工作
- **trpc-agent-go 回调**：新 ActivityProjector 处理回调，生成新事件；同时通过 adapter 转换为旧 `Activity` 写入旧表
- **数据库**：新旧表共存（activities 表保留只读，新表写入）
- **Phase 2 完成后删除兼容层**：旧前端重写完成后，删除 adapter + 旧表 + 旧 sequencer

**验证**：
- 后端单测：每个 entity 的 Repo + Sequencer + PlanExecutor
- 集成测试：发起 chat，验证新表数据正确
- 旧前端可继续工作（兼容层 adapter）

### Phase 2：前端重写

**目标**：前端按新模型重写，移除兼容层。

**改动**：
- 删除 `web/src/features/chat/composables/useActivityTimeline.ts`
- 删除 `web/src/features/chat/activityTypes.ts`、`streamEventTypes.ts`、`activityEvent.ts`
- 新建 `web/src/features/chat/stores/useChatStore.ts`：扁平 Map 存储
- 新建 `web/src/features/chat/composables/useChatEventRouter.ts`：WS 事件路由
- 新建 `web/src/features/chat/composables/useTaskTree.ts`：按 seq 排序
- 重写 `web/src/components/chat/ActivityStream.vue` → 拆分为 `TaskList.vue` + `TurnList.vue` + `TurnContainer.vue`
- 新建 `web/src/components/chat/PlanBoardCard.vue` + `PlanDAG.vue` + `PlanStepNode.vue`
- 重写 `web/src/components/chat/TeamCard.vue` → `TeamStagePanel.vue` + `TeamRunCard.vue`
- 重写 `web/src/components/chat/AgentCard.vue` → `MemberSessionPanel.vue`
- 保留：`ThinkingBlock.vue`/`ActionBlock.vue`/`ReplyBlock.vue`（仅适配新 props）
- 改造 `useBlockedStatus.ts`、`useChatMessageScroll.ts`、`useScrollToActivity.ts`

**验证**：
- 前端单测：store + tree 构造 + 事件路由
- E2E：发起 chat，验证 UI 正确显示
- 删除兼容层代码

### Phase 3：清理与优化

**目标**：删除旧代码，性能优化。

**改动**：
- 删除 `internal/biz/activity.go`、`internal/biz/activity_event.go`、`internal/biz/plan.go`（旧 Plan 模型）
- 删除 `internal/data/ent/schema/activity.go`、`task_plan.go`、`plan.go`
- 删除旧 DDL 迁移引用 activities 表的部分
- 性能优化：批量查询、缓存、lazy load

**验证**：
- 全量测试：`make api && make wire && make build && make test && make lint`
- 前端：`cd web && pnpm lint && pnpm test && pnpm build`

---

## 六、范围与边界

### 6.1 推倒重来的范围

**重写**：
- `internal/biz/activity*.go` → 拆分为 9 个新 entity 文件
- `internal/agent/activity_projector.go` → 适配新模型
- `internal/agent/activity_event_sequencer.go` → 统一入口 + SeqAssigner
- `internal/service/spirit_team.go` → 拆分，移除 `updatePlanStepForTeam`
- `web/src/features/chat/composables/useActivityTimeline.ts` → 删除，改为扁平 store
- `web/src/components/chat/ActivityStream.vue` → 拆分为按类型组件
- `web/src/components/chat/PlanBlock.vue` → `PlanBoardCard.vue` + SVG 依赖图
- `web/src/components/chat/TeamCard.vue`/`AgentCard.vue` → `TeamStagePanel.vue`/`MemberSessionPanel.vue`

**新增**：
- `internal/service/plan_executor.go`
- `internal/biz/plan_step_state_machine.go`
- 9 个 Ent Schema 文件
- 前端 Plan DAG 可视化组件

**保留**：
- WS 传输层（`ws-transport.ts`/`globalWsHub.ts`/`ws_io_pump.go`）
- trpc-agent-go 集成（`stream_consumer.go` 改造）
- 鉴权/路由/DI 框架
- `ThinkingBlock.vue`/`ActionBlock.vue`/`ReplyBlock.vue`（仅适配新 props）

### 6.2 三层硬约束

仅支持 `spirit → team → member` 三层，**不支持 team 内 team 递归**：
- spirit 是 root session
- team 是 spirit 触发的子执行（每个 team 属于一个 spirit task）
- member 是 team 内的 agent 执行（每个 member 属于一个 team_run）
- member 内的 turn 是终止层，不能再生 team

**校验**：PlanExecutor 在派发 step 时检查 `parent_team_stage_id`，禁止 member turn 内创建新 team_stage。

### 6.3 非目标（明确排除）

- 不支持 team 内 team 递归
- 不保留历史会话数据
- 不重构 WS 传输层、鉴权、路由、DI 框架
- 不重构 trpc-agent-go 集成层（stream_consumer 改造即可）
- 不引入新依赖（不引 d3/rxjs 等，DAG 用原生 SVG + Canvas）

---

## 七、验收标准

### 7.1 功能验收

| 验收项 | 标准 |
|--------|------|
| 顺序保证 | thinking → action → reply 严格按业务顺序显示，无乱序（10 次测试无失败） |
| 跨层嵌套 | spirit → team → member 三层结构清晰，子活动正确挂载到父节点 |
| Plan 状态实时同步 | team 完成 → plan step 立即更新（< 100ms），刷新不丢失 |
| Plan 依赖图可视化 | DAG 正确绘制，并行/串行清晰，状态色正确 |
| 失败可追溯 | 每个 step 失败原因、责任方、级联影响清晰可见 |
| 刷新数据完整 | 重新加载页面，所有 entity 从 DB 恢复，无丢失 |
| Streaming 流畅 | 60fps 流式输出，无卡顿 |
| 阻塞判定准确 | 工具阻塞/LLM 阻塞/确认阻塞/HITL 阻塞均正确判定 |

### 7.2 非功能验收

| 验收项 | 标准 |
|--------|------|
| 前端零推理 | 前端代码无 `compareActivities` patch 规则、无 re-parenting hack、无 orphan 处理 |
| 类型安全 | 后端无 `map[string]any` 类型断言 |
| 单管道 | 所有事件源走 sequencer，无 direct-publish |
| 显式 PlanExecutor | plan 执行通过 PlanExecutor 调度，无反向同步 |
| 状态机 | PlanStep 状态变更通过状态机，无直接赋值 |

### 7.3 测试验收

| 测试类型 | 范围 |
|---------|------|
| 后端单测 | 每个 entity Repo + Sequencer + PlanExecutor + 状态机 |
| 后端集成测试 | 发起 chat 全流程，验证数据正确入库 |
| 前端单测 | store + tree 构造 + 事件路由 + DAG 布局 |
| 前端 E2E | 发起 chat，验证 UI 显示 + Plan DAG 交互 |
| 全量回归 | `make api && make wire && make build && make test && make lint` + 前端 `pnpm lint && pnpm test && pnpm build` |

---

## 八、风险与缓解

| 风险 | 缓解 |
|------|------|
| 推倒重来工作量巨大 | 按层切换（Phase 1/2/3），每层独立验证；Phase 1 完成后旧前端仍可工作 |
| 全新数据库丢失历史数据 | 用户已确认接受；开发环境直接重建 |
| PlanExecutor 引入新 bug | 完整单测覆盖调度逻辑、状态机、失败处理；先在测试环境验证 |
| DAG 可视化复杂度 | 仅做基础拓扑分层 + 状态色，不引 d3；后续可迭代 |
| trpc-agent-go 集成改造风险 | `stream_consumer.go` 仅适配新 ProjectMeta 字段，不改 trpc 回调接口 |
| Seq 持久化重启丢失 | 启动时从 DB 查询 `MAX(seq) WHERE spirit_session_id = ?` 恢复 |

---

## 九、关键文件清单（推倒重来后）

### 9.1 后端新增/重写

| 文件 | 状态 | 用途 |
|------|------|------|
| `internal/biz/session.go` | 新增 | Session entity |
| `internal/biz/task.go` | 新增 | Task entity |
| `internal/biz/turn.go` | 新增 | Turn entity |
| `internal/biz/step.go` | 新增 | Step entity |
| `internal/biz/team_stage.go` | 新增 | TeamStage entity |
| `internal/biz/team_run.go` | 新增 | TeamRun entity |
| `internal/biz/member_session.go` | 新增 | MemberSession entity |
| `internal/biz/plan_board.go` | 新增 | PlanBoard entity |
| `internal/biz/plan_step.go` | 新增 | PlanStep entity |
| `internal/biz/plan_step_state_machine.go` | 新增 | Step 状态机 |
| `internal/biz/event.go` | 新增 | Event 接口 + 类型 |
| `internal/agent/seq_assigner.go` | 新增 | Seq 分配器 |
| `internal/agent/activity_event_sequencer.go` | 重写 | 统一 sequencer |
| `internal/agent/activity_projector.go` | 重写 | 适配新模型 |
| `internal/agent/project_meta.go` | 重写 | 新 ProjectMeta |
| `internal/agent/stream_consumer.go` | 改造 | 适配新 ProjectMeta |
| `internal/service/plan_executor.go` | 新增 | Plan 调度器 |
| `internal/service/spirit_team.go` | 重写 | 移除 updatePlanStepForTeam |
| `internal/data/ent/schema/session.go` ~ `plan_step.go` | 新增 | 9 张表 Schema |
| `internal/data/*_repo.go` | 新增 | 9 个 Repo |

### 9.2 后端删除

| 文件 | 原因 |
|------|------|
| `internal/biz/activity.go` | 拆分为 9 个 entity |
| `internal/biz/activity_event.go` | 替换为 `event.go` |
| `internal/biz/plan.go` | 旧 Plan 模型废弃 |
| `internal/biz/plan_state_machine.go` | 旧状态机废弃 |
| `internal/biz/task_plan.go` | 替换为 `plan_board.go` + `plan_step.go` |
| `internal/data/ent/schema/activity.go` | 表废弃 |
| `internal/data/ent/schema/task_plan.go` | 表废弃 |
| `internal/data/ent/schema/plan.go` | 表废弃 |
| `internal/data/activity_repo.go` | 替换为 9 个 Repo |
| `internal/data/task_plan_repo.go` | 替换 |

### 9.3 前端新增/重写

| 文件 | 状态 | 用途 |
|------|------|------|
| `web/src/features/chat/stores/useChatStore.ts` | 新增 | 扁平 Map store |
| `web/src/features/chat/composables/useChatEventRouter.ts` | 新增 | WS 事件路由 |
| `web/src/features/chat/composables/useTaskTree.ts` | 新增 | 按 seq 排序 |
| `web/src/features/chat/composables/usePlanDAGLayout.ts` | 新增 | DAG 布局算法 |
| `web/src/components/chat/SessionPanel.vue` | 新增 | 顶层 |
| `web/src/components/chat/TaskList.vue` | 新增 | task 列表 |
| `web/src/components/chat/TaskCard.vue` | 新增 | task 卡片 |
| `web/src/components/chat/TurnList.vue` | 新增 | turn 列表 |
| `web/src/components/chat/TurnContainer.vue` | 新增 | turn 容器 |
| `web/src/components/chat/PlanBoardCard.vue` | 新增 | 计划面板 |
| `web/src/components/chat/PlanDAG.vue` | 新增 | SVG 依赖图 |
| `web/src/components/chat/PlanStepNode.vue` | 新增 | DAG 节点 |
| `web/src/components/chat/PlanStepDetailPanel.vue` | 新增 | 节点详情 |
| `web/src/components/chat/TeamStagePanel.vue` | 新增 | team 执行面板 |
| `web/src/components/chat/TeamRunCard.vue` | 新增 | team run 卡片 |
| `web/src/components/chat/MemberSessionPanel.vue` | 新增 | member 面板 |
| `web/src/features/chat/composables/useBlockedStatus.ts` | 重写 | 简化阻塞判定 |
| `web/src/features/chat/composables/useChatMessageScroll.ts` | 改造 | 适配新模型 |
| `web/src/features/chat/composables/useScrollToActivity.ts` | 改造 | 适配新模型 |
| `web/src/components/chat/ThinkingBlock.vue` | 改造 | 适配新 props |
| `web/src/components/chat/ActionBlock.vue` | 改造 | 适配新 props |
| `web/src/components/chat/ReplyBlock.vue` | 改造 | 适配新 props |

### 9.4 前端删除

| 文件 | 原因 |
|------|------|
| `web/src/features/chat/composables/useActivityTimeline.ts` | 替换为扁平 store |
| `web/src/features/chat/activityTypes.ts` | 替换 |
| `web/src/features/chat/streamEventTypes.ts` | 替换 |
| `web/src/realtime/activityEvent.ts` | 替换 |
| `web/src/components/chat/ActivityStream.vue` | 拆分 |
| `web/src/components/chat/PlanBlock.vue` | 替换为 PlanBoardCard + DAG |
| `web/src/components/chat/TeamCard.vue` | 替换 |
| `web/src/components/chat/AgentCard.vue` | 替换 |

---

## 十、决策记录

| 决策 | 选择 | 理由 |
|------|------|------|
| 排序机制 | Seq 替代 Timestamp | Timestamp 不稳定（updated/completed 事件覆盖 created 时间），同毫秒冲突；Seq 由后端分配单调递增，前端零推理 |
| Plan 调度 | 显式 PlanExecutor | 当前靠 team 完成事件反向更新，无正向调度；PlanExecutor 统一调度逻辑，支持状态机 |
| 实施方式 | 推倒重来 | 一周改动无效，patch 已无法修复根因 |
| 数据库迁移 | 全新数据库 | 历史数据无价值（开发环境），避免迁移负担 |
| 实施路径 | 按层切换 | 后端先上线（兼容层）→ 前端重写 → 删除旧代码，每层独立验证 |
| Plan 可视化 | SVG 依赖图 | 当前仅文字列表，用户无法直观看到并行/串行；SVG 轻量，不引 d3 |
| 嵌套深度 | spirit → team → member 三层 | 业务场景无递归 team 需求，硬约束简化模型 |

---

## 十一、后续步骤

1. **用户复核本 spec** → 确认设计方向无误
2. **调用 writing-plans skill** → 生成细粒度实施计划（按 Phase 1/2/3 拆分任务）
3. **执行实施** → 按 plan 推进
4. **完成前验证** → 运行全量测试 + build + lint
5. **代码审查** → 按 aranea-review SKILL 维度审查

---

## 附录 A：与当前实现的对照

### A.1 当前 → 新的映射

| 当前 | 新 |
|------|-----|
| `Activity` struct（30 字段 + Meta） | 9 个独立 entity（typed struct） |
| `ActivityKind` (10 种) | `StepKind` (6 种) + TeamStage/TeamRun/MemberSession/PlanBoard/PlanStep entity |
| `ActivityStatus` (10 种) | 每个 entity 独立 status enum |
| `Activity.Meta` map | typed struct 字段 |
| `ActivityEvent` (6 种 event) | `Event` interface + 30+ 具体事件类型 |
| `ActivityEventSequencer` 双路径 | 单 `Sequencer` 统一入口 |
| `updatePlanStepForTeam` 反向同步 | `PlanExecutor` 正向调度 |
| `compareActivities` 4 条 patch | Seq ASC 排序 |
| `useActivityTimeline` computed 树 | `useChatStore` 扁平 Map + `useTaskTree` 按 seq 排序 |
| `re-parenting hack` | parent_id 显式字段 |
| `orphan 节点处理` | 不存在（每个 entity 都有 parent_id） |
| `Meta.is_final` 标志 | `Step.IsFinal` 字段 |
| `Meta.streaming` 阻塞判定 | 时间窗口判断 + step.status |

### A.2 移除的痛点

- 排序：4 条 patch 规则、Timestamp 不稳定、同毫秒冲突
- 嵌套：re-parenting hack、orphan 处理、跨 session 收集 dedup
- Plan：双路径创建 dedup、反向同步绕过 sequencer、ChatSessionID 传播脆弱、synthesis step 兜底
- 持久化：B 路径无重试无死信
- 类型安全：Meta map 类型断言
- 状态机：step 状态直接赋值
