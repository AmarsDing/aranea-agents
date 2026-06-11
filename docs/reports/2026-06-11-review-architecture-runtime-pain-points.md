# Review: Architecture Runtime Pain Points & Optimization

> **日期**：2026-06-11
> **版本**：v2.0（v1.0 基础上增加评审修正与方案细化）
> **范围**：业务运行时核心（ChatOrchestrator / EventBus / Session State / Team Orchestration / HITL）

---

## 摘要

对 Aranea-Agents 业务运行时进行系统性架构评审，识别出 5 大痛点：ChatOrchestrator 上帝对象、EventBus 无持久化、缺少显式状态机、HITL 非一等公民、多 Agent 编排模式单一。通过竞品对标（LangGraph / AutoGen / Bisheng / OpenAI Agents SDK）和项目整体性分析，给出分阶段最优解决方案，并补充 6 项建设性架构评判标准。

**v2.0 修订要点**：
- 修正痛点 1 量化数据（实际 35 字段而非 26），补充 SessionUsecase 上帝对象
- 修正痛点 2 EventWAL 方案（WBPF 仅覆盖 Critical 低频事件、Recover 幂等、Event Sourcing 降级为远期可选）
- 修正痛点 3 状态机放置层级（biz 层定义规则、统一泛型接口、补充 SessionRunPhase 和跨实体关联校验）
- 修正痛点 4 HumanLoopGate 接口（Suspend/Resume 两阶段替代阻塞式 Request、移除 feedback 模式）
- 修正痛点 5 编排模式（Handoff 作为 swarm 配置项而非新模式、Native 清理提前至 P1）
- 修正依赖关系图（事件持久化非独立可并行，是横切关注点）
- 细化 6 项 AS 标准的落地路径和分层上限

---

## 一、评审方法与维度

| 维度 | 评判标准 | 权重 |
|------|---------|------|
| 职责分离 | 每个模块有单一明确职责，依赖方向清晰 | 高 |
| 可测试性 | 业务逻辑可通过接口 mock 独立测试 | 高 |
| 可演进性 | 架构能随需求增长而扩展 | 高 |
| 认知复杂度 | 新人能合理时间内理解系统全貌 | 中高 |
| 运行时弹性 | 支持水平扩展、故障隔离、优雅降级 | 中高 |
| 领域忠实度 | 代码结构反映业务领域概念 | 中 |
| 可观测性 | 全链路追踪、事件审计、状态快照可查 | 中 |
| 开发效率 | 常规开发任务路径短、改动面小 | 中 |

---

## 二、痛点系统性评审

### 痛点 1：ChatOrchestrator 上帝对象

**严重度**：高 | **影响面**：所有 Turn 执行路径 | **竞品对标**：LangGraph StateGraph/Node/Edge 分离

#### 2.1.1 现状量化

| 指标 | 当前值 | 建议上限 | 超标倍数 |
|------|--------|---------|---------|
| 注入字段数 | 35（27 业务字段 + 4 sync.Map + 3 子管理器 + 1 chan） | 15 | 2.3x |
| sync.Map 数 | 4 | 0（应提取） | - |
| 核心方法行数（runSingleAgentViaTRPC） | 571（精确计量） | 80 | 7.1x |
| biz 层依赖数 | 20+ | 8 | 2.5x |
| 总代码行数（Orchestrator 相关） | ~4173 | 2000 | 2.1x |

> **v2.0 修正**：原 v1.0 报告"注入字段数 26"，实际 ChatOrchestrator struct 含 27 个业务字段 + 4 个 sync.Map + 3 个已提取子管理器接口 + 1 个 sweepStop channel = 35 个字段。`runSingleAgentViaTRPC` 精确行数为 571 行（chat_orchestrator_turn.go:446-1016）。

#### 2.1.2 已完成的提取

- `sessionStateTransitor` — 会话状态转换（接口薄层，仅委托 `sessions.TransitionStatus()`）
- `turnRecorder` — 指标记录（接口薄层，委托 `usage.RecordTurnUsage()`）
- `turnEventPublisher` — 事件发布（接口薄层，委托 `event.BumpAndPublishSessionRevision()`）

> **v2.0 评审**：三个子管理器均为接口薄层，仅做委托调用，未减少 Orchestrator 的方法数或字段数，核心方法 `runSingleAgentViaTRPC` 的认知复杂度未显著降低。

#### 2.1.3 尚未提取的职责域

| 职责域 | 涉及方法 | 涉及 sync.Map | 建议提取名 |
|--------|---------|--------------|-----------|
| Run 状态管理 | setRunStatus / persistRunStatus / hydrateRunStatusFromSession / publishRunStatus | - | `RunStatusTracker` |
| Await/Resume 管理 | makeAwaitReplyFunc / tryBeginResume / endResume / awaitMetaCache / RegisterAwaitChannel / DeleteAwaitChannel / LoadAwaitChannel | awaitMetaCache, resumeInFlight | `AwaitCoordinator` |
| Pending Queue 管理 | processPendingQueue / EnqueueUserMessage / DequeuePendingMessage / SetSessionPendingMergeFollowup | pendingMergeFollowup | `PendingQueueManager` |
| Session Run 生命周期 | beginSessionRunLifecycle / finishSessionRunLifecycle / escalateSessionRunToDurable | sessionRunBindings | `SessionRunLifecycle` |
| Agent 构建+工具注入 | runSingleAgentViaTRPC 中的 TRPCBuilderDeps 构建 | - | `AgentBuildDirector` |

> **v2.0 补充**：`RegisterAwaitChannel / DeleteAwaitChannel / LoadAwaitChannel` 三个公开方法属于 `AwaitChannelRegistry` 职责，应随 AwaitCoordinator 一并提取。

#### 2.1.4 sync.Map 类型安全问题

4 个 sync.Map 均使用 `timestampedEntry{value: any}` 包装，每次 Load 需双重类型断言（至少 8 处重复代码）。sweep 协程清理 `resumeInFlight` 时存在竞态风险（30 分钟超时窗口 vs LLM 长阻塞）。

#### 2.1.5 Turn 生命周期阶段映射

`runSingleAgentViaTRPC` 的 571 行按逻辑阶段分解：

| 阶段 | 行范围 | 职责 | 行数 |
|------|--------|------|------|
| ADMISSION | 446-473 | AgentKey 校验、RunID 生成、Durable 上下文解析 | ~28 |
| BUILD | 474-636 | Trace 初始化、TRPCBuilderDeps 组装（30+ 字段）、Agent 构建、Runner 创建、defer 注册 | ~163 |
| EXECUTE | 637-846 | UserOptions 构建、Intent Pass、用户消息持久化、LLM 调用、流消费 | ~210 |
| PERSIST | 893-996 | 超时降级、空回复检测、助手消息持久化、上下文使用量更新 | ~104 |
| POST-PROCESS | 997-1016 | Metrics 记录、状态完成、Revision bump、Hooks 通知 | ~20 |

> **v2.0 补充**：5 个阶段混杂在单一方法中，无显式阶段边界。defer 嵌套 3 层（外层 trace + 中层 rollback + 内层 userMsgStatus），执行顺序隐式。`processPendingQueue` 在 defer 中调用，可能递归调用 `runSingleAgentViaTRPC`，存在死锁风险。

#### 2.1.6 评审结论

**根因**：ChatOrchestrator 承担了 Turn 生命周期的所有阶段（准入→构建→执行→持久化→后处理），缺少按阶段拆分的子管理器。

**风险**：
- 修改任一阶段逻辑需理解全部 4173 行代码
- 无法对单个阶段独立测试
- 新增阶段（如 Durable Checkpoint）只能继续堆叠在 Orchestrator 上
- `processPendingQueue` 递归调用 `runSingleAgentViaTRPC` 存在死锁风险
- escalateSessionRunToDurable 中 checkpoint/phase 不一致风险

#### 2.1.7 遗漏的上帝对象：SessionUsecase

> **v2.0 新增**

`SessionUsecase`（`internal/biz/session/usecase.go:471-506`）有 **25+ 注入字段**，含 `metricsDeltaMu sync.Mutex + metricsDeltas map`，同样违反 AS-COG-01。它是 ChatOrchestrator 的核心下游依赖，拆分 Orchestrator 时必然触及。

**建议**：将 SessionUsecase 拆分纳入阶段二，至少将 `SessionMetricsUsecase` 和 `SessionCompressionUsecase` 从内嵌 Facade 提升为独立注入的 port 接口，消除 `metricsDeltas map + metricsDeltaMu` 的内嵌状态管理。

---

### 痛点 2：EventBus 无持久化

**严重度**：高 | **影响面**：所有事件驱动路径 | **竞品对标**：LangGraph Durable Execution + Checkpoint

#### 2.2.1 事件丢失场景分析

| 事件阶段 | 进程崩溃影响 | 持久化状态 |
|----------|-------------|-----------|
| Bus channel 中 | 全部丢失 | 无 |
| Buffer 中（环形缓冲区） | 全部丢失 | 无（200条/会话，30min TTL） |
| persistHandler job channel 中 | 全部丢失 | 无（512 容量，满则丢弃） |
| 已写入 SQLite EventStore | 安全 | SQLite 持久化 |
| StateDelta 已写入 DB | 安全 | SQLite 持久化 |

#### 2.2.2 关键事件保护评估

`criticalTypeSet` 定义了 13 种不可静默丢弃的事件类型，但保护机制有限：
- `BlockUpTo` 仅阻塞 100ms，超时后仍降级为 `DropOldest`
- 无 ACK/确认机制，消费者崩溃后事件丢失
- 异步持久化无重试机制

#### 2.2.3 事件回放能力评估

| 机制 | 容量 | 持久化 | 用途 |
|------|------|--------|------|
| 内存 Buffer.Replay | 200条/会话 | 无 | WS 重连回放 |
| EventStore List | 无限（7天TTL） | SQLite | API 查询展示 |
| 状态重建 | 不支持 | - | 不通过回放事件重建状态 |

**不存在 CQRS/Event Sourcing 模式**：系统通过直接 DB 读写状态，不通过回放事件重建。

#### 2.2.4 评审结论

**根因**：EventBus 设计为纯内存传输层，持久化是后置的、尽力而为的异步操作。

**风险**：
- 进程崩溃导致关键业务事件（ToolResult、TokenUsage、Checkpoint）丢失
- 无法实现真正的 Durable Execution
- 无法支持事件回放/状态重建

> **v2.0 补充**：痛点 2 不是"相对独立可并行推进"的，它是痛点 1/3/4 的**横切关注点**。状态机转换事件、HITL 挂起/恢复事件、Orchestrator 生命周期事件都需要可靠投递。依赖关系图需修正。

---

### 痛点 3：缺少显式状态机

**严重度**：中高 | **影响面**：Session/Run/TeamRun 状态转换 | **竞品对标**：LangGraph StateGraph

#### 2.3.1 Run 状态机（隐式）

状态值：`running` / `completed` / `failed` / `cancelled` / `awaiting_user`

转换散布在 `setRunStatus`、`cancelActiveRun`、`runSingleAgentViaTRPC`、`executeTeamTurnViaHooks`、`resumeAwaitAfterRestart` 等方法中。**无集中定义，无转换合法性校验。**

#### 2.3.2 Session 状态机（完全合规）

> **v2.0 修正**：原 v1.0 报告称 Session 状态机"部分显式"，实际 `SessionStatusMachine`（`internal/biz/session/status_machine.go`）有完整 struct + `TransitionTo` + `CanTransitionTo` + 单元测试，**已完全合规**。

#### 2.3.3 TeamRun 状态机

TeamRun 状态（pending/running/success/failed/waiting_human/cancelled）有函数式校验（`ValidateTeamRunTransition`），但非 struct 模式，缺 Guard 条件。

#### 2.3.4 现有状态机实现不一致

> **v2.0 新增**

| 实体 | 实现方式 | 合规性 |
|------|---------|--------|
| Session | struct + `TransitionTo(target, reason)` + `CanTransitionTo` + 单测 | 完复合规 |
| Team | 函数式 `ValidTeamStatusTransition(from, to) bool` | 不合规（非 struct、无 Guard） |
| TeamRun | 函数式 `ValidateTeamRunTransition(from, to) bool` | 不合规（非 struct、无 Guard） |
| Run | 无任何转换校验 | 严重缺失 |
| SessionRunPhase | 5 种阶段，无任何转换校验 | 严重缺失（v1.0 遗漏） |
| GraphExecution | Status 为裸 string，无任何转换校验 | 严重缺失 |

#### 2.3.5 跨实体状态关联缺失

> **v2.0 新增**

Run/TeamRun/Session 之间存在状态联动，但当前无关联校验：

| 关联场景 | 当前问题 | 风险 |
|---------|---------|------|
| Run → Session | Run 进入 `awaiting_user` 时 Session 应进入 `awaiting_confirmation` | 代码中手动配对，易遗漏 |
| TeamRun → Run | TeamRun 进入 `waiting_human` 时，下属 Run 应暂停 | 无级联校验 |
| GraphExecution → Run | Graph 节点中断时，对应 Run 应进入 `awaiting_user` | 无关联机制 |

#### 2.3.6 评审结论

**根因**：Run 状态机从未被显式定义，所有转换都是命令式的 `SetStatus` 调用。各实体状态机实现不一致，跨实体状态关联无校验。

**风险**：
- 非法转换（如 `completed` → `running`）无法在编译期/运行期检测
- 状态转换逻辑散落各处，修改需搜索全部代码
- 无法生成状态转换图供文档/调试使用
- 跨实体状态不一致（如 TeamRun 已 cancelled 但 Run 仍 running）

---

### 痛点 4：HITL 非一等公民

**严重度**：中高 | **影响面**：人工审批/确认/反馈场景 | **竞品对标**：Bisheng HIL 一等公民、LangGraph Approval Node、n8n Wait Node

#### 2.4.1 当前 HITL 能力

| 层级 | 机制 | 评估 |
|------|------|------|
| 单 Agent | await_user_reply（工具等待用户回复） | 可用，但仅限工具回调场景 |
| Graph | InterruptBefore/InterruptAfter + waiting_human 状态 | 较完整，支持检查点恢复 |
| TeamRun | DeferTeamRunSuccessIfHITL + SLA 保护 | 较完整，支持超时和延期 |
| Session | awaiting_confirmation 状态 | 仅状态标记，无通用交互协议 |

#### 2.4.2 缺失的 HITL 模式

| 模式 | 描述 | 竞品支持 | Aranea 现状 |
|------|------|---------|------------|
| **审批门** | 执行前等待人类确认 | LangGraph Approval Node | 无（仅工具级 tool_confirm） |
| **检查点干预** | 执行中暂停，人类可修改状态后继续 | Bisheng 多轮对话干预 | Graph 级有，Session 级无 |

> **v2.0 修正**：移除"反馈环"和"通用等待"模式。feedback 模式尚无业务需求，YAGNI 原则。通用等待已被审批门覆盖。

#### 2.4.3 评审结论

**根因**：HITL 能力分散在 Graph/TeamRun/Agent 三个层级，缺少统一的 `HumanLoopGate` 抽象。

**风险**：
- 新增 HITL 场景需在多个层级分别实现
- 不同层级的 HITL 行为不一致（超时策略、恢复机制）
- 无法在编排层面统一声明 HITL 需求

> **v2.0 补充**：HITL 的 `await_user_reply` 机制分散在 `ChatOrchestrator.makeAwaitReplyFunc`、`awaitMetaCache`、`resumeInFlight`、`team.Runner.SetAwaitHookProvider` 等多处。痛点 1 的 `AwaitCoordinator` 提取是痛点 4 的前置依赖。

---

### 痛点 5：多 Agent 编排模式单一

**严重度**：中 | **影响面**：Team 编排灵活性 | **竞品对标**：OpenAI Handoff + Agent-as-Tool 双模式

#### 2.5.1 当前编排模式

| 模式 | Graph 编译 | Native 回退 | 评估 |
|------|-----------|------------|------|
| sequential | pipelineTemplate | chainagent | 成熟 |
| parallel | parallelReviewTemplate | parallelagent | 成熟 |
| coordinator | dispatchTemplate | Swarm + MemberTool | 成熟 |
| critic_loop | reviewLoopTemplate | cycleagent | 升级函数较脆弱 |
| swarm | adaptiveTemplate（映射为 adaptive） | trpcteam.NewSwarm | 与 adaptive 无实质区别 |
| adaptive | adaptiveTemplate | trpcteam.NewSwarm | 成熟 |

#### 2.5.2 缺失的编排模式

| 模式 | 描述 | 竞品支持 | Aranea 现状 |
|------|------|---------|------------|
| **Handoff（交接）** | 控制权从一个 Agent 转移到另一个 | OpenAI Agents SDK | swarm/adaptive 已有类似能力，非显式抽象 |
| **Agent-as-Tool（Agent 即工具）** | 管理者调用专家获取结构化结果 | OpenAI Agents SDK | coordinator 模式有 MemberTool，但仅 Native 路径 |
| **Evaluator-Optimizer** | 执行+评估循环 | Anthropic 模式 | critic_loop 近似，但升级函数脆弱 |

> **v2.0 修正**：移除 Blackboard 模式（学术研究，无实际业务需求）。Handoff 本质是 swarm 的子集（swarm = handoff + 自主路由），应作为配置项而非新模式。

#### 2.5.3 其他差距

- **Native 路径已 Deprecated 但未移除**：增加维护负担，每次编排变更需同时修改 Graph 编译器和 Native 构建器
- **Swarm 与 Adaptive 无实质区别**：`normalizeCompileMode` 将 swarm 映射为 adaptive
- **跨 Team 协作缺失**：`AgentAllocator.AssignedType=team` 仅为接口预留
- **运行时动态编排缺失**：拓扑在编译时确定，运行时无法动态调整

#### 2.5.4 评审结论

**根因**：6 种编排模式覆盖了常见场景，但 Native/Graph 双引擎增加了维护复杂度，Swarm/Adaptive 模糊性增加认知负担。

**风险**：
- 新编排模式需同时修改 Graph 编译器和 Native 构建器
- Swarm/Adaptive 模糊性增加用户认知负担
- 跨 Team 协作需求无法满足

> **v2.0 补充**：Native 路径清理不应在阶段四，应提前至阶段二。理由：(1) 每次编排逻辑修改需双路径维护，成本高于新增模式；(2) 拆分 AgentBuildDirector 时可只保留 Graph 路径；(3) 避免为即将移除的代码编写测试。

---

## 三、整体性分析与优先级排序

### 3.1 依赖关系图

> **v2.0 修正**：原 v1.0 报告称"痛点 2 相对独立可并行推进"，实际事件持久化是痛点 1/3/4 的横切关注点。

```
痛点3(状态机) ←── 痛点1(上帝对象) ──→ 痛点2(事件持久化)
     ↑                  ↑                    ↑
     └──── 痛点4(HITL) ─┘                    │
              ↑                              │
              └──── 痛点5(编排模式) ──────────┘
```

**关键洞察**：
- 痛点 3（状态机）是痛点 1 和 4 的前置依赖——先定义状态机，才能正确拆分 Orchestrator 和设计 HITL
- 痛点 4（HITL）依赖痛点 1 的 AwaitCoordinator 提取——先统一 await 机制，再设计 HumanLoopGate
- 痛点 5（编排模式）依赖痛点 1 的 AgentBuildDirector 拆分——先拆分构建逻辑，再扩展模式
- **痛点 2（事件持久化）是横切关注点**——状态机转换事件、HITL 挂起/恢复事件、Orchestrator 生命周期事件都需要可靠投递

### 3.2 ROI 分析

| 痛点 | 实施成本 | 收益 | ROI | 建议优先级 |
|------|---------|------|-----|-----------|
| 痛点3：显式状态机 | 中（定义+迁移+测试） | 高（消除散落转换、支持可视化） | 高 | P0 |
| 痛点1：拆分Orchestrator | 高（5个子管理器+Wire重组） | 高（降低认知复杂度、独立测试） | 中高 | P1 |
| 痛点2：事件持久化 | 中（WAL+关键事件持久化） | 高（崩溃不丢事件、支持 Durable） | 高 | P0（与P1并行） |
| 痛点4：HITL一等公民 | 中（HumanLoopGate抽象+2种模式） | 中高（统一HITL、支持新场景） | 中高 | P2 |
| 痛点5：编排模式扩展 | 低（Handoff配置化+Agent-as-Tool补全） | 中（覆盖更多场景） | 中 | P3 |

---

## 四、最优解决方案

### 阶段一：基础加固（P0，1-2 迭代）

#### 4.1.1 显式状态机（痛点3）

**方案**：定义统一状态机接口 + `RunStateMachine` + `SessionRunPhaseMachine`。

**统一状态机接口**：

```go
// internal/biz/shared/state_machine.go
type StateMachine[S ~string, E ~string] interface {
    Transition(from S, event E) (S, error)
    CanTransition(from S, to S) bool
    ValidTargets(from S) []S
}
```

> **v2.0 修正**：原 v1.0 方案将 RunStateMachine 放在 `internal/runtime/turn/`，但 Run 状态是业务概念，其转换规则应在 biz 层定义。runtime/turn 包通过 biz 层接口调用。同时，Session/Team/TeamRun 的状态机应统一到此接口。

**RunStateMachine**：

```go
// internal/biz/run_state_machine.go
type RunState string

const (
    RunStateNone          RunState = ""
    RunStateRunning       RunState = "running"
    RunStateCompleted     RunState = "completed"
    RunStateFailed        RunState = "failed"
    RunStateCancelled     RunState = "cancelled"
    RunStateAwaitingUser  RunState = "awaiting_user"
)

type RunTransition struct {
    From   RunState
    Event  string
    To     RunState
    Guard  func(ctx context.Context) bool // 可选守卫条件
}

var runTransitions = []RunTransition{
    {RunStateNone, "start", RunStateRunning, nil},
    {RunStateRunning, "complete", RunStateCompleted, nil},
    {RunStateRunning, "fail", RunStateFailed, nil},
    {RunStateRunning, "cancel", RunStateCancelled, nil},
    {RunStateRunning, "await", RunStateAwaitingUser, nil},
    {RunStateAwaitingUser, "resume", RunStateRunning, nil},
    {RunStateAwaitingUser, "cancel", RunStateCancelled, nil},
}

type RunStateMachine struct {
    transitions map[RunState]map[string]RunTransition
}

func (sm *RunStateMachine) Transition(from RunState, event string) (RunState, error) {
    // 校验转换合法性，返回目标状态或错误
}

func (sm *RunStateMachine) CanTransition(from, to RunState) bool { ... }
func (sm *RunStateMachine) ValidTargets(from RunState) []RunState { ... }
```

**SessionRunPhaseMachine**（v2.0 新增）：

```go
// internal/biz/session_run_phase_machine.go
type SessionRunPhase string

const (
    PhaseInteractive SessionRunPhase = "interactive"
    PhaseEscalating  SessionRunPhase = "escalating"
    PhaseDurable     SessionRunPhase = "durable"
    PhaseCompleted   SessionRunPhase = "completed"
    PhaseFailed      SessionRunPhase = "failed"
)

var sessionRunPhaseTransitions = []SessionRunPhaseTransition{
    {PhaseInteractive, "escalate", PhaseEscalating, nil},
    {PhaseEscalating, "durable", PhaseDurable, nil},
    {PhaseInteractive, "complete", PhaseCompleted, nil},
    {PhaseInteractive, "fail", PhaseFailed, nil},
    {PhaseEscalating, "complete", PhaseCompleted, nil},
    {PhaseEscalating, "fail", PhaseFailed, nil},
    {PhaseDurable, "complete", PhaseCompleted, nil},
    {PhaseDurable, "fail", PhaseFailed, nil},
}
```

**迁移策略**：
1. 新建 `RunStateMachine` 和 `SessionRunPhaseMachine`，与现有 `SetStatus` 并行运行
2. 在 `setRunStatus` 中增加转换合法性校验（仅日志警告，不阻断）
3. 验证期（**2 个迭代**）后，非法转换升级为错误
4. 最终移除散落的 `SetStatus` 调用，统一走 `StateMachine.Transition`
5. 将 Team/TeamRun 的函数式校验迁移为 struct 实现，统一到 `StateMachine[S, E]` 接口

> **v2.0 修正**：验证期从 1 个迭代延长到 2 个迭代。现有代码中 `setRunStatus` 被调用的路径有 10+ 处，1 个迭代可能不够覆盖所有边界情况。

**跨实体状态关联**（v2.0 新增）：

定义 `StateMachineCoordinator` 接口，在 service 层实现跨实体状态协调：

```go
// internal/biz/shared/state_machine_coordinator.go
type StateMachineCoordinator interface {
    // OnRunTransition Run 状态变更后联动 Session 状态
    OnRunTransition(ctx context.Context, sessionID string, from, to RunState) error
    // OnTeamRunTransition TeamRun 状态变更后联动子 Run 状态
    OnTeamRunTransition(ctx context.Context, teamRunID string, from, to TeamRunState) error
}
```

**验证**：单元测试覆盖所有合法/非法转换；集成测试验证现有行为不变；跨实体关联测试验证级联行为。

#### 4.1.2 事件关键路径持久化（痛点2，短期方案）

**方案**：为 Critical 级低频事件增加 Write-Before-Publish（WBPF）机制。

> **v2.0 修正**：原 v1.0 方案对 criticalTypeSet 中全部 13 种事件走 WBPF，但 `StateDelta` 和 `TokenUsage` 是高频事件（单次 Turn 可产生数十到数百次），同步写 SQLite WAL 会显著增加延迟。WBPF 仅覆盖 Critical 级低频事件。

**事件分级与 WBPF 策略**：

| 级别 | 事件类型 | 可靠性保证 | 持久化 |
|------|---------|-----------|--------|
| Critical（WBPF） | ToolResult / Error / RunnerCompletion / Checkpoint | 先写后发 + 重试 | SQLite WAL |
| Important（BlockUpTo + 异步持久化） | StateDelta / TokenUsage / RunStatus / SessionStatusChanged / GraphNodeEnd / TeamRunFinished / UserFeedback | BlockUpTo + 异步持久化 | SQLite EventStore |
| Informational（尽力而为） | TextDelta / FlowLog / Log / MemberDelta | 尽力而为 | 不持久化 |

```go
// internal/event/wal.go
type EventWAL struct {
    db    *sql.DB
    lg    loggateway.Logger
}

// WriteBeforePublish: Critical 低频事件先写 WAL 再发布
func (w *EventWAL) WriteBeforePublish(ctx context.Context, env Envelope, publish func()) error {
    if !isCriticalWBPFType(env.Type) {
        publish() // Important/Informational 事件直接发布
        return nil
    }
    // 1. 写入 WAL 表（INSERT OR IGNORE 幂等）
    if err := w.appendWAL(ctx, env); err != nil {
        return err
    }
    // 2. 发布到 Bus
    publish()
    // 3. 同步标记 WAL 为已发布（v2.0 修正：原异步 go w.markPublished 违反红线 BC1，改为同步）
    w.markPublished(ctx, env.ID)
    return nil
}

// Recover: 进程启动时回放未发布的 WAL 条目（幂等）
func (w *EventWAL) Recover(ctx context.Context, bus Bus, store EventStore) {
    pending := w.loadUnpublished(ctx)
    for _, env := range pending {
        // 幂等校验：EventStore 已存在则跳过（v2.0 新增）
        if store.Exists(ctx, env.ID) {
            w.markPublished(ctx, env.ID)
            continue
        }
        bus.Publish(ctx, env)
        w.markPublished(ctx, env.ID)
    }
}
```

> **v2.0 修正**：
> - `go w.markPublished(env.ID)` 改为同步 `w.markPublished(ctx, env.ID)`，避免异步标记在崩溃时导致 WAL 条目被重复发布
> - Recover 增加幂等校验（基于 event ID 去重），避免 persistHandler 重复写入 EventStore
> - WAL 写入使用 `INSERT OR IGNORE`（基于 event ID 幂等）
> - Recover 延迟到 Bus 和所有 subscriber 就绪后执行

**SQLite WAL 表**：
```sql
CREATE TABLE IF NOT EXISTS event_wal (
    id TEXT PRIMARY KEY,
    envelope_json TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    published_at DATETIME,
    published INTEGER DEFAULT 0
);
CREATE INDEX idx_event_wal_unpublished ON event_wal(published, created_at);
```

**迁移策略**：
1. 新建 `EventWAL`，在 Bus 和所有 subscriber 就绪后调用 `Recover`
2. 修改 `Infra.Publish`，对 Critical WBPF 类型走 `WriteBeforePublish`
3. Important 级别事件保持现有 `BlockUpTo` + 异步持久化到 EventStore
4. Informational 级别事件行为不变
5. 添加 WAL 清理定时任务（7 天 TTL，与 EventStore 一致）

**验证**：模拟进程崩溃测试，验证 Critical 事件不丢失；Recover 幂等测试验证不重复。

---

### 阶段二：结构优化（P1，2-3 迭代）

#### 4.2.1 拆分 ChatOrchestrator（痛点1）

**方案**：按职责域提取 5 个子管理器，ChatOrchestrator 退化为纯协调者。

```
ChatOrchestrator（协调者，仅持有子管理器引用）
  ├── RunStatusTracker      — Run 状态管理
  ├── AwaitCoordinator      — Await/Resume 管理
  ├── PendingQueueManager   — Pending Queue 管理
  ├── SessionRunLifecycle   — Session Run 生命周期
  └── AgentBuildDirector    — Agent 构建+工具注入
```

> **v2.0 修正**：子管理器放在 `internal/service/` 下独立文件（如 `chat_orch_run_status.go`），而非 `internal/runtime/turn/`。理由：子管理器持有 `biz.SessionWriter`、`biz.ChatUsecase` 等 biz 层依赖，按项目分层规范 BA2，不应放入 runtime 包。

**提取顺序**（按依赖关系从低到高）：

1. **RunStatusTracker**：依赖 RunRegistry + Bus
   - 提取字段：`sessionRunBindings` sync.Map
   - 提取方法：`setRunStatus`、`persistRunStatus`、`hydrateRunStatusFromSession`、`publishRunStatus`、`GetRunStatus`
   - 与 RunStateMachine 集成：`Transition` 成功后自动 `publishRunStatus`
   - sync.Map `sessionRunBindings` 移入此管理器

2. **PendingQueueManager**：依赖 chatUC
   - 提取字段：`pendingMergeFollowup` sync.Map
   - 提取方法：`EnqueueUserMessage`、`DequeuePendingMessage`、`processPendingQueue`、`SetSessionPendingMergeFollowup`、`sessionPendingMergeFollowup`
   - sync.Map `pendingMergeFollowup` 移入此管理器

3. **AwaitCoordinator**：依赖 RunStatusTracker + chatUC
   - 提取字段：`awaitMetaCache`、`resumeInFlight` sync.Map
   - 提取方法：`makeAwaitReplyFunc`、`tryBeginResume`、`endResume`、`persistAwaitMarkers`、`canResumeAwait`、`RegisterAwaitChannel`、`DeleteAwaitChannel`、`LoadAwaitChannel`
   - sync.Map `awaitMetaCache`、`resumeInFlight` 移入此管理器

4. **SessionRunLifecycle**：依赖 RunStatusTracker + AwaitCoordinator + ChannelTurnJobDeps
   - 提取方法：`beginSessionRunLifecycle`、`finishSessionRunLifecycle`、`escalateSessionRunToDurable`
   - 需同时持有 `ChannelTurnJobUsecase` 和 `SessionRunUsecase`

5. **AgentBuildDirector**：依赖 RuntimeTooling
   - 先提取 `TRPCBuilderDeps` 为独立 struct（类似已有的 `RuntimeTooling` 模式），将 30+ 字段的依赖组装逻辑封装
   - 再将 `runSingleAgentViaTRPC` 从 571 行降至 ~200 行
   - **不拆为 Build→Invoke→Finalize 三阶段**（v2.0 修正）：Build 和 Invoke 共享 ctx/emitter/runID 等大量中间状态，强行拆分增加跨阶段参数传递复杂度。采用"提取构建器"策略

> **v2.0 修正**：
> - 提取顺序调整：PendingQueueManager 提前到第 2 位（低依赖），AwaitCoordinator 移到第 3 位（中依赖）
> - AgentBuildDirector 不拆为三阶段，改为提取 TRPCBuilderDeps 构建器
> - Wire 绑定收口在 `service.go` 的 ProviderSet 中，子管理器不各自 `wire.NewSet`

**Wire 重组**：
- 子管理器统一在 `service.go` ProviderSet 中注册
- `ChatOrchestratorDeps` 拆分为 `OrchCoreDeps` + 5 个子管理器 deps struct
- 每次提取一个子管理器后立即 `make wire && make build` 验证

**验证**：每个子管理器独立单测；集成测试验证 Orchestrator 行为不变。

#### 4.2.2 sync.Map 类型安全化

**方案**：使用泛型包装替代 `timestampedEntry{value: any}`。

```go
// internal/service/orch_typed_map.go
type TypedSyncMap[K comparable, V any] struct {
    mu   sync.Map
    ttl  time.Duration
}

func (m *TypedSyncMap[K, V]) Store(key K, value V) { ... }
func (m *TypedSyncMap[K, V]) Load(key K) (V, bool) { ... }
func (m *TypedSyncMap[K, V]) Delete(key K) { ... }
func (m *TypedSyncMap[K, V]) Sweep() { ... } // 清理过期条目
```

> **v2.0 修正**：放置位置从 `internal/runtime/turn/` 改为 `internal/service/`（与子管理器同层）。对于 `resumeInFlight` 的竞态问题，应改用 `context.WithTimeout` + `Done()` channel 替代时间戳清理，而非仅泛型化。

**迁移**：随子管理器提取同步替换，不单独做。

#### 4.2.3 SessionUsecase 拆分（v2.0 新增）

**方案**：将 SessionUsecase 的 25+ 字段拆分为独立 Usecase。

1. **提取 `SessionMetricsUsecase`**：将 `metricsDeltaMu`、`metricsDeltas`、`flushInterval`、`metricsUpdatedPublisher` 移入独立 struct
2. **提取 `SessionCompressionUsecase`**：将 `compressionUsecase` 从内嵌 Facade 提升为独立注入
3. SessionUsecase 字段数从 25+ 降至 ~15

#### 4.2.4 Native 路径清理（v2.0 从阶段四提前）

**方案**：移除 Native 路径，仅保留 Graph 编译路径。

- 移除 `BuildTRPCTeam`、`tryNativeFallback`、`envTeamNativeForced`
- 移除 `internal/team/trpc_build.go`
- 清理 Wire ProviderSet 中的 Native 相关注入
- **前置条件**：评估 Native 路径使用率（通过 metrics 采集），若 < 5% 则执行清理

> **v2.0 修正**：Native 清理从阶段四提前到阶段二。理由：(1) 每次编排逻辑修改需双路径维护；(2) 拆分 AgentBuildDirector 时可只保留 Graph 路径；(3) 阶段三所有新功能仅针对 Graph 路径实现，加速 Native 自然淘汰。

---

### 阶段三：能力增强（P2，3-5 迭代）

#### 4.3.1 HITL 一等公民（痛点4）

**方案**：定义 `HumanLoopGate` 统一抽象，覆盖 approval + checkpoint 两种模式。

> **v2.0 修正**：
> - 移除 feedback 模式（尚无业务需求，YAGNI）
> - `Request` 阻塞式接口改为 `Suspend/Resume` 两阶段接口（HTTP 请求模型下客户端不会保持连接等待人工审批）
> - `map[string]any` 改为具体类型（违反 BI5）

```go
// internal/biz/human_loop.go
type HumanLoopKind string

const (
    HumanLoopApproval   HumanLoopKind = "approval"   // 审批门
    HumanLoopCheckpoint HumanLoopKind = "checkpoint"  // 检查点干预
)

type ApprovalContext struct {
    ToolName string
    ToolArgs json.RawMessage
    Options  []string // 如 approve/reject
}

type CheckpointContext struct {
    NodeID    string
    StateKeys []string
}

type HumanLoopRequest struct {
    Kind       HumanLoopKind
    SessionID  string
    RunID      string
    Message    string
    Timeout    time.Duration
    Approval   *ApprovalContext   `json:",omitempty"`  // approval 模式专用
    Checkpoint *CheckpointContext `json:",omitempty"`  // checkpoint 模式专用
}

type HumanLoopResponse struct {
    Action        string          // 用户选择（approve/reject/modify）
    ModifiedState json.RawMessage // 修改后的状态（checkpoint 模式）
}

type HumanLoopGate interface {
    // Suspend 挂起当前执行，返回挂起凭证（非阻塞）
    Suspend(ctx context.Context, req HumanLoopRequest) (suspendToken string, err error)
    // Resume 恢复被挂起的执行（新的 HTTP 请求中调用）
    Resume(ctx context.Context, token string, resp HumanLoopResponse) error
    // Cancel 取消挂起的请求
    Cancel(ctx context.Context, token string) error
}
```

> **v2.0 修正**：`Suspend/Resume` 两阶段设计更符合现有 `await_user_reply` 机制（Agent Turn 内挂起 → 新 HTTP 请求恢复），而非原方案的阻塞式 `Request`。

**实现策略**：
1. `approval` 模式：复用现有 `await_user_reply` 机制，统一到 `HumanLoopGate.Suspend(kind=approval)`
2. `checkpoint` 模式：复用 Graph `InterruptBefore/After`，通过 `GraphInterruptAdapter` 实现 `HumanLoopGate`

**迁移**：
- 现有 `await_user_reply` → `HumanLoopGate.Suspend(kind=approval)`
- 现有 `tool_confirm` → `HumanLoopGate.Suspend(kind=approval)`
- Graph `InterruptBefore/After` → `GraphInterruptAdapter` 实现 `HumanLoopGate`

**前置依赖**：痛点 1 的 `AwaitCoordinator` 提取完成后，HumanLoopGate 的实现才能基于统一的 await 机制。

#### 4.3.2 事件持久化增强（痛点2，中期方案）

**方案**：增强 EventStore 查询能力 + Checkpoint 机制，不引入完整 CQRS。

> **v2.0 修正**：原 v1.0 方案引入 Event Sourcing + CQRS + Projection Builder，这是架构范式的根本性变更。当前系统不是 Event Sourcing 架构，引入 CQRS 投入产出比极低。降级为远期可选目标。

**实际实施**：
1. EventStore 增加 `session_id + run_id` 联合索引，支持时间范围查询
2. Checkpoint 机制：定期快照 + 增量事件，支持状态重建
3. 事件审计日志：合规需求，关键事件不可删除
4. 架构 Fitness Function 自动化（AS-FIT-01）

---

### 阶段四：生态扩展（P3，6+ 迭代）

#### 4.4.1 编排模式优化（痛点5）

**方案**：Handoff 作为 swarm 配置项、Agent-as-Tool 补全 Graph 路径。

> **v2.0 修正**：不新增 `ModeHandoff` 和 `ModeAgentAsTool` 枚举值。Handoff 是 swarm 的子集，应作为配置项。Agent-as-Tool 通过工具注入实现，无需修改 Graph 编译器。

**Handoff 策略化**：在现有 `ModeSwarm`/`ModeAdaptive` 中增加 `handoff_strategy` 配置：
- `implicit`（默认）：当前行为，Agent 自主路由
- `explicit`：Handoff 模式，控制权显式转移
- 支持 Handoff 策略参数：`max_handoffs`、`repetitive_detection`、`cross_request`

**Agent-as-Tool Graph 路径**：在 `internal/tools/subagent/` 中实现 Agent-as-Tool 工具（将 Agent 调用封装为工具），Graph 编译器通过工具注入方式使用，无需修改编译器本身。

**Swarm/Adaptive 合并**：合并 `ModeSwarm` 和 `ModeAdaptive` 为一种模式（保留 `adaptive` 名称，`swarm` 作为 alias），减少用户认知负担。

#### 4.4.2 跨 Team 协作

- 实现 `AgentAllocator.AssignedType=team` 的完整路径
- 支持 Team 间委派（Team A 委派子任务给 Team B）
- A2A 协议集成（基于现有 `pkg/trpc-agent-go/server/a2a/` 基础设施）

#### 4.4.3 远期可选：Event Sourcing + CQRS

> **v2.0 降级**：从阶段三移至远期可选。需先有 ADR 论证，且需满足以下前提：
> - 系统需要水平扩展（当前单机 SQLite 不需要）
> - 状态重建有明确业务需求
> - 持久化方案从 SQLite 迁移到支持更高写并发的存储

---

## 五、建设性架构评判标准

当前项目评判标准偏"禁止性红线"，缺少以下建设性标准。建议补充到 `project_rules.md` 和 `aranea-coding-guide`。

### 标准 1：ADR（架构决策记录）

**编号**：AS-ADR-01

**要求**：每个影响跨模块的架构决策必须记录 ADR。

**格式**（v2.0 轻量化）：
```markdown
# ADR-NN: <标题>

## 状态
提议 | 已接受 | 已废弃 | 已替代

## 背景
<为什么需要做决策>

## 决策
<做了什么决策>

## 后果
<决策的正面和负面影响>

## 替代方案（可选）
<考虑过但未选择的方案及原因>
```

> **v2.0 修正**：替代方案降为可选，降低 ADR 编写门槛。存放位置从 `docs/reports/` 改为 `docs/adr/`，编号 `ADR-001-xxx.md`。增加 ADR 编号分配机制（`docs/adr/index.md` 索引文件）。

**触发条件**：
- 新增模块或包
- 修改依赖方向
- 引入新框架/库
- 修改核心数据结构
- 性能关键路径的权衡决策
- 修改 AS 系列标准本身（v2.0 新增）

**落地路径**（v2.0 新增）：
1. 创建 `docs/adr/` 目录和 `index.md` 索引文件
2. 本文档的 5 大痛点决策各写一个 ADR（ADR-001 ~ ADR-005）
3. 代码审查清单增加"跨模块变更是否附 ADR"
4. 编号从 ADR-001 开始，顺序分配

**执行现状**：0 篇 ADR，完全未执行。

### 标准 2：认知复杂度量化

**编号**：AS-COG-01

**要求**：以下指标不得超过上限，超标必须拆分。

| 指标 | biz 层上限 | service 层上限 | data 层上限 | 检测方式 |
|------|-----------|---------------|-----------|---------|
| struct 注入字段数 | 10 | 15 | 8 | 代码审查 |
| 单方法行数 | 80 | 80 | 80 | linter（CS-B5） |
| 单方法圈复杂度 | 15 | 15 | 15 | linter（CS-B6） |
| biz 层依赖数（单 struct） | 8 | 12 | N/A | 代码审查 |
| sync.Map 数（单 struct） | 0 | 0 | 0 | 代码审查 |
| 文件总行数 | 500 | 500 | 500 | linter |
| 单文件导出类型数 | 15 | 15 | 15 | 代码审查 |

> **v2.0 修正**：
> - 分层设限：biz 层最窄（10 字段），service 层居中（15 字段），data 层最少（8 字段）
> - 包级导出类型数改为"单文件导出类型数 ≤ 15"，biz 包整体不受限
> - config struct（如 `AgentRuntimeSettings`）不适用字段数上限，但应拆分为子 struct 组合
> - 允许封装后的 typed sync.Map（如 `TypedSyncMap`），禁止裸 `sync.Map`

**超标处理**：
1. 标记 `// TECH-DEBT(COG): <指标>=<当前值>, 上限=<上限>`
2. 在下一个迭代规划中安排拆分
3. 不阻断当前开发，但禁止继续堆叠

**执行现状**：ChatOrchestrator 35 字段、SessionUsecase 25+ 字段、AgentRuntimeSettings 80+ 字段，均严重超标。

### 标准 3：状态机显式化要求

**编号**：AS-FSM-01

**要求**：任何实体拥有 >3 种状态时，必须定义显式状态机。

**定义位置**：与实体同包，文件名 `*_state_machine.go`

> **v2.0 补充**：新实体必须用 `*_state_machine.go`，现有实体保持原名但接口统一到 `StateMachine[S, E]`。

**必须包含**：
1. 状态枚举（const）
2. 合法转换表（var transitions）
3. 转换校验函数（`Transition(from, event) (to, error)`）
4. 可选守卫条件（`Guard func(ctx) bool`）
5. Mermaid 状态转换图（放在文件头注释中）（v2.0 新增）

**现有实体需补全**：
- Run 状态机（5 种状态）— 阶段一
- SessionRunPhase 状态机（5 种状态）— 阶段一（v2.0 新增）
- Session 状态机（已有，需统一到 `StateMachine[S, E]` 接口）
- TeamRun 状态机（6 种状态，需从函数式迁移为 struct）
- Team 状态机（7 种状态，需从函数式迁移为 struct）
- GraphExecution 状态机（5 种状态，需新建）

**跨实体状态关联**（v2.0 新增）：

有父子关系的实体，子实体状态转换必须满足父实体状态的约束（`ParentConstraint` 守卫条件），父实体状态转换必须级联校验子实体状态（`CascadeCheck` 守卫条件）。

**执行现状**：仅 Session 完全合规。Team/TeamRun 函数式（非 struct）。Run/SessionRunPhase/GraphExecution 无状态机。

### 标准 4：接口稳定性分级

**编号**：AS-STA-01

**要求**：biz 层 port 接口必须标注稳定性等级。

| 等级 | 标注 | 含义 | 变更规则 |
|------|------|------|---------|
| Stable | `// Stability:stable` | 生产依赖，不可破坏兼容 | 新增方法不需 ADR；修改/删除方法必须 ADR |
| Evolving | `// Stability:evolving` | 活跃开发中，可能变 | 破坏性变更需 ADR；新增方法不需 |
| Internal | `// Stability:internal` | 包内使用，不对外 | 自由变更，不需 ADR |

> **v2.0 修正**：明确了"变更"的边界——新增方法 vs 修改/删除方法的 ADR 要求不同。

**现有接口分级时间表**（v2.0 新增）：

| 阶段 | 接口 | 分级 | 备注 |
|------|------|------|------|
| P0 | `biz.AgentRepository` | Stable | 14+ 方法，标注前需先拆分子接口 |
| P0 | `biz.SessionTurnManager` | Stable | - |
| P1 | `biz.TeamReader` / `biz.TeamWriter` | Stable | - |
| P1 | `biz.GraphExecutor` | Evolving | - |
| P2 | `biz.HumanLoopGate`（新增） | Evolving | - |
| P2 | `biz.TaskPlannerPort` | Evolving | - |

> **v2.0 补充**：`AgentRepository`（14+ 方法）和 `SessionRepo`（17+ 方法）标注为 Stable 会锁定当前不合理的接口设计。应先拆分为子接口（`AgentReader`、`AgentWriter` 等），再标注 Stable。

**执行现状**：0 个接口有 Stability 标注，完全未执行。

### 标准 5：架构 Fitness Function

**编号**：AS-FIT-01

**要求**：以下架构不变量必须通过自动化测试验证。

| Fitness Function | 验证内容 | 实现方式 | 阶段 |
|-----------------|---------|---------|------|
| 依赖方向 | biz 不依赖 pkg/trpc-agent-go | `go vet` + import 检查 | P0 |
| 分层隔离 | service 不直接访问 data | import 检查 | P0 |
| 接口窄化 | biz port 接口方法 ≤ 5 | 反射检查 | P1 |
| 状态机覆盖 | >3 状态实体有显式状态机 | 测试枚举 | P2 |
| 认知复杂度 | struct 字段 ≤ 上限 | 静态分析 | P2 |

> **v2.0 修正**：分阶段实现，P0 只做最关键的两项（依赖方向 + 分层隔离）。

**实现路径**：

短期（P0）：Go test 实现，`make archlint` = `go test ./internal/archlint/ -count=1`

```go
// internal/archlint/archlint_test.go
func TestBizNotDependOnTrpcAgentGo(t *testing.T) { ... }
func TestServiceNotDirectlyAccessData(t *testing.T) { ... }
```

中期（P1）：增加接口窄化检查
长期（P2）：集成 CI + `golangci-lint` 自定义规则

**执行现状**：无任何自动化架构不变量测试，完全未执行。

### 标准 6：事件可靠性分级

**编号**：AS-EVT-01

**要求**：事件按业务关键性分级，不同级别有不同的可靠性保证。

| 级别 | 事件类型 | 可靠性保证 | 持久化 |
|------|---------|-----------|--------|
| Critical | ToolResult / Error / RunnerCompletion / Checkpoint | WBPF（先写后发）+ 重试 | SQLite WAL |
| Important | StateDelta / TokenUsage / RunStatus / SessionStatusChanged / GraphNodeEnd / TeamRunFinished / UserFeedback | BlockUpTo + 异步持久化 | SQLite EventStore |
| Informational | TextDelta / FlowLog / Log / MemberDelta | 尽力而为 | 不持久化 |

> **v2.0 修正**：
> - Critical 级别从原 6 种缩减为 4 种（移除 StateDelta 和 TokenUsage，降为 Important）
> - Important 级别增加 StateDelta / TokenUsage / UserFeedback
> - 与 EventWAL 方案对齐：Critical = WBPF，Important = BlockUpTo + 异步持久化

**检测方式**：代码审查时校验新增事件类型的分级是否正确。

**执行现状**：仅有 Critical/Normal 两级，无 WBPF/重试，部分分级与方案不一致。

---

## 六、实施路线图

> **v2.0 修正**：调整阶段内容和优先级

```
阶段一（P0，1-2 迭代）
├── RunStateMachine 定义 + 统一接口 + 迁移
├── SessionRunPhaseMachine 定义 + 迁移
├── EventWAL 实现（仅 Critical 4 种事件 WBPF）+ Recover
├── 事件分级标注（AS-EVT-01）
├── AS 标准落地：ADR 索引 + Stability 标注 + archlint P0
└── openspec/specs/ 基础结构创建（architecture-blueprint.md 初版）

阶段二（P1，2-3 迭代）
├── RunStatusTracker 提取
├── PendingQueueManager 提取
├── AwaitCoordinator 提取
├── SessionRunLifecycle 提取
├── AgentBuildDirector 提取（TRPCBuilderDeps 构建器）
├── sync.Map 泛型化
├── SessionUsecase 拆分
├── Native 路径清理（评估使用率后执行）
├── Team/TeamRun 状态机迁移为 struct
└── archlint P1（接口窄化检查）

阶段三（P2，3-5 迭代）
├── HumanLoopGate 统一抽象（approval + checkpoint 两种模式）
├── approval 模式实现（复用 await_user_reply）
├── checkpoint 模式实现（GraphInterruptAdapter）
├── EventStore 查询增强 + Checkpoint 机制
├── 架构 Fitness Function 自动化（AS-FIT-01 P2）
├── Swarm/Adaptive 合并
└── Handoff 策略化

阶段四（P3，6+ 迭代）
├── Agent-as-Tool Graph 路径支持
├── 跨 Team 协作 + A2A 协议集成
├── MCP 协议标准化
├── Checkpoint + 时间旅行增强
└── 远期可选：Event Sourcing + CQRS（需 ADR 论证）
```

---

## 七、结论

Aranea-Agents 的业务运行时架构在 Go Agent 平台领域处于中上水平，双框架分工、Wire DI、EventBus 优先级机制、Graph 编译引擎等设计优于多数 Python 竞品。但 5 大痛点——上帝对象、事件无持久化、隐式状态机、HITL 非一等公民、编排模式单一——限制了系统的可演进性和生产可靠性。

通过 4 阶段渐进式改进（基础加固→结构优化→能力增强→生态扩展），可以在不破坏现有行为的前提下系统性地解决这些问题。同时补充 6 项建设性架构评判标准（ADR、认知复杂度、状态机显式化、接口稳定性、Fitness Function、事件可靠性分级），将项目从"禁止性红线"模式升级为"建设性指引"模式。

**v2.0 核心修正总结**：

1. **事件持久化**从"独立并行"调整为"横切依赖"，WBPF 仅覆盖 Critical 低频事件
2. **HITL** 从三模式缩减为两模式（approval + checkpoint），接口从阻塞式改为 Suspend/Resume 两阶段
3. **Event Sourcing + CQRS** 降级为远期可选目标
4. **Native 路径清理**从阶段四提前到阶段二
5. **Handoff** 作为 swarm 配置项而非新模式
6. **状态机**统一泛型接口，补充 SessionRunPhase 和跨实体关联校验
7. **AS 标准**细化落地路径、分层上限、分级时间表
8. **SessionUsecase** 上帝对象纳入拆分计划
9. **openspec/specs/** 基础结构创建作为阶段一前置任务
