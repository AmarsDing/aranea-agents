# Review: Architecture Runtime Pain Points & Optimization

> **日期**：2026-06-11
> **版本**：v7.0（P0/P1 + TECH-DEBT D1-D5 全部修复完成，aranea-review 通过）
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

**v7.0 代码同步要点**（P0/P1 + TECH-DEBT D1-D5 全部修复完成）：
- 痛点 1 ChatOrchestrator 重构完成：字段 35→12，sync.Map 4→0，5 个子管理器全部提取，runSingleAgentViaTRPC 571→134 行（四阶段拆分 + phase methods 提取到独立文件），**turn.go 883→365 行**（提取 dispatch/api/metrics 三个文件），**invokeTurnLLMAndStream 80→50 行**（提取 assembleTurnResult）
- 痛点 1 SessionUsecase 重构完成：**字段 26→14**（提取 SessionMessageUsecase + SessionTimelineUsecase，State/Turn/Participant 内嵌于 MessageUsecase），Facade fallback 清理完成（移除 contextUpdater/summaryReader/summaryWriter/compressRepo 4 个字段）
- 痛点 2 EventWAL 已实现，事件三级分级已落地（contract/reliability.go）。TeamRunFailed → Important，ContextUsage → Informational，AS-EVT-01 分级完整无遗漏
- 痛点 3 状态机完全合规：统一接口 `StateMachine[S,E]` 已实现，哨兵错误 `ErrInvalidTransition`/`ErrGuardRejected` + `%w` 包装。SessionStateMachine 已实现统一接口（8 条规则 + 单元测试）。**Team/TeamRun 旧式函数已彻底删除**（ValidTeamStatusTransition/ValidateTeamRunTransition + 转换表），测试迁移到 CanTransition API。StateMachineCoordinator 已实现（纯函数设计 + 测试）
- 痛点 5 Native 路径彻底移除 + **TeamGraphRunCoordinator 改用窄接口**（teamRunReader + teamRunWriter 替代 biz.TeamRunRepo 聚合接口）
- AS 标准：Stability 标注全面补全，TECH-DEBT(COG) 格式规范化，ADR-001 完整内容已补充

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

| 指标 | v2.0 记录值 | 当前值 | 建议上限 | 变化 |
|------|------------|--------|---------|------|
| 注入字段数 | 35（27 业务字段 + 4 sync.Map + 3 子管理器 + 1 chan） | 12（6 deps struct + 2 子管理器接口 + runs + chatUC + sweepStop） | 15 | ✅ 已达标 |
| sync.Map 数 | 4 | 0（已替换为 TypedSyncMap） | 0（应提取） | ✅ 已达标 |
| 核心方法行数（runSingleAgentViaTRPC） | 571（446-1016） | 550（406-955） | 80 | 仍超标 6.9x |
| biz 层依赖数 | 20+ | ~6（通过 deps struct 收口） | 8 | ✅ 已达标 |
| 总代码行数（Orchestrator 相关） | ~4173 | ~3670（14 个文件） | 2000 | 仍超标 1.8x |

> **v3.0 更新**：ChatOrchestrator struct 已从 35 字段降至 12 字段，struct 注释明确标注 "Field count: 12, well under AS-COG-01 limit of 15"。4 个 sync.Map 已全部替换为类型安全的 `TypedSyncMap[K,V]`，分布在子管理器中。`runSingleAgentViaTRPC` 从 571 行降至 550 行，仍远超 80 行上限，需进一步拆分。

#### 2.1.2 已完成的提取

- `sessionStateTransitor` — 会话状态转换（已嵌入 `chatTurnLifecycleImpl`）
- `turnRecorder` — 指标记录（已嵌入 `chatTurnLifecycleImpl`）
- `turnEventPublisher` — 事件发布（已嵌入 `chatTurnLifecycleImpl`）
- `RunStatusTracker` → `chatRunStatusTracker` — Run 状态管理（✅ v3.0 已提取）
- `AwaitCoordinator` → `chatAwaitCoordinator` — Await/Resume 管理（✅ v3.0 已提取）
- `PendingQueueManager` → `chatPendingQueueManager` — Pending Queue 管理（✅ v3.0 已提取）
- `SessionRunLifecycle` → `chatSessionRunLifecycle` — Session Run 生命周期（✅ v3.0 已提取）
- `AgentBuildDirector` → `chatAgentBuildDirector` — Agent 构建+工具注入（✅ v3.0 已提取）

> **v3.0 更新**：v2.0 中"尚未提取"的 5 个职责域已全部提取为独立子管理器。7 个子管理器（含原有的 3 个）进一步合并为两个组合接口：`chatTurnLifecycle`（合并 sessionStateTransitor + turnRecorder + turnEventPublisher）和 `chatRunManager`（合并 runStatusTracker + pendingQueueManager + awaitCoordinator + sessionRunLifecycle）。

#### 2.1.3 尚未提取的职责域

> **v3.0 更新**：v2.0 中列出的 5 个尚未提取的职责域已全部提取完成（见 §2.1.2）。当前剩余问题为 `runSingleAgentViaTRPC` 仍 550 行，需进一步拆分其内部阶段逻辑。

#### 2.1.4 sync.Map 类型安全问题

> **v3.0 更新**：4 个 sync.Map 已全部替换为类型安全的 `TypedSyncMap[K,V]`，分布在子管理器中：

| 原 sync.Map | 当前名称 | 当前类型 | 所在 struct |
|-------------|---------|---------|------------|
| `awaitMetaCache` | `awaitCache` | `*TypedSyncMap[string, biz.ChatAwaitMeta]` | `chatRunStatusTracker` |
| `sessionRunBindings` | `bindings` | `*TypedSyncMap[string, sessionRunTurnBinding]` | `chatRunStatusTracker` |
| `resumeInFlight` | `resumeInFlight` | `*TypedSyncMap[string, struct{}]` | `chatAwaitCoordinator` |
| `pendingMergeFollowup` | `mergeFollowup` | `*TypedSyncMap[string, bool]` | `chatPendingQueueManager` |

双重类型断言问题已消除。`resumeInFlight` 的竞态风险仍需关注（30 分钟超时窗口 vs LLM 长阻塞）。

#### 2.1.5 Turn 生命周期阶段映射

`runSingleAgentViaTRPC` 的 550 行按逻辑阶段分解：

| 阶段 | 行范围（v4.0） | 职责 | 行数 |
|------|---------------|------|------|
| ADMISSION | 405-432 | AgentKey 校验、RunID 生成、Durable 上下文解析 | ~28 |
| BUILD | 433-595 | Trace 初始化、TRPCBuilderDeps 组装、Agent 构建、Runner 创建、defer 注册 | ~163 |
| EXECUTE | 596-805 | UserOptions 构建、Intent Pass、用户消息持久化、LLM 调用、流消费 | ~210 |
| PERSIST | 852-934 | 超时降级、空回复检测、助手消息持久化、上下文使用量更新 | ~83 |
| POST-PROCESS | 935-954 | Metrics 记录、状态完成、Revision bump、Hooks 通知 | ~20 |

> **v4.0 更新**：行号范围已同步至当前代码（chat_orchestrator_turn.go:405-954）。5 个阶段仍混杂在单一方法中，无显式阶段边界。defer 嵌套 3 层（外层 trace + 中层 rollback + 内层 userMsgStatus），执行顺序隐式。`processPendingQueue` 在 defer 中调用，可能递归调用 `runSingleAgentViaTRPC`，存在死锁风险。

#### 2.1.6 评审结论

**根因**（v2.0）：ChatOrchestrator 承担了 Turn 生命周期的所有阶段（准入→构建→执行→持久化→后处理），缺少按阶段拆分的子管理器。

**v3.0 进展**：子管理器提取已完成，字段数和 sync.Map 已达标。但 `runSingleAgentViaTRPC` 仍 550 行，5 个阶段仍混杂在单一方法中，缺少显式阶段边界。

**剩余风险**：
- 修改任一阶段逻辑仍需理解 550 行代码
- 无法对单个阶段独立测试
- `processPendingQueue` 递归调用 `runSingleAgentViaTRPC` 存在死锁风险
- escalateSessionRunToDurable 中 checkpoint/phase 不一致风险

#### 2.1.7 遗漏的上帝对象：SessionUsecase

> **v2.0 新增** | **v3.0 更新**

`SessionUsecase`（`internal/biz/session/usecase.go:472-505`）有 **30 个注入字段**（v2.0 记录 25+），含 `metricsDeltaMu sync.Mutex + metricsDeltas map`，同样违反 AS-COG-01。它是 ChatOrchestrator 的核心下游依赖，拆分 Orchestrator 时必然触及。

**v3.0 进展**：
- `SessionMetricsUsecase` 已提取为独立 struct（`metrics.go`），但 SessionUsecase 仍保留 `metricsDeltaMu`、`metricsDeltas`、`flushInterval`、`metricsUpdatedPublisher` 作为 Legacy 遗留字段，标注 "Legacy fields kept for backward-compatible Set* methods"
- `SessionCompressionUsecase` 已提取为独立 struct（`compression.go`，4 个字段），但 SessionUsecase 仍通过 `compressionUsecase *SessionCompressionUsecase` 持有（非内嵌 Facade，已修正 v2.0 描述）
- SessionUsecase 字段数从 25+ 增至 30（含 Legacy 遗留字段），仍严重超标

**建议**：清理 Legacy 遗留字段（`metricsDeltaMu`、`metricsDeltas`、`flushInterval`、`metricsUpdatedPublisher`），将 SessionUsecase 字段数降至 ~26，再进一步拆分。

---

### 痛点 2：EventBus 无持久化

**严重度**：高 | **影响面**：所有事件驱动路径 | **竞品对标**：LangGraph Durable Execution + Checkpoint

#### 2.2.1 事件丢失场景分析

| 事件阶段 | 进程崩溃影响 | 持久化状态 | v3.0 变化 |
|----------|-------------|-----------|----------|
| Bus channel 中 | 全部丢失 | 无 | 无变化 |
| Buffer 中（环形缓冲区） | 全部丢失 | 无（200条/会话，30min TTL） | 无变化 |
| persistHandler job channel 中 | 全部丢失 | 无（512 容量，满则丢弃） | 无变化 |
| **EventWAL（Critical 事件）** | **安全** | **SQLite WAL** | ✅ v3.0 已实现 |
| 已写入 SQLite EventStore | 安全 | SQLite 持久化 | 无变化 |
| StateDelta 已写入 DB | 安全 | SQLite 持久化 | 无变化 |

> **v3.0 更新**：EventWAL 已实现（`internal/event/wal.go`），Critical 级事件（ToolResult/Error/RunnerCompletion/Checkpoint）走 Write-Before-Publish 机制，先写 WAL 再发布。Recover 方法在进程重启后回放未发布的 WAL 条目（幂等）。WAL 清理定时任务已实现（`internal/cronrunner/jobs/event_wal_cleanup.go`，7 天 TTL）。

#### 2.2.2 关键事件保护评估

> **v5.0 更新**：`criticalTypeSet`（`internal/event/bus.go`）和 `isCriticalEnvelopeType`（`internal/biz/event_persist_handler.go`）已删除，统一使用 `contract.RequiresBlockUpTo()` 和 `contract.IsCriticalWBPFType()`，与 AS-EVT-01 三级分类对齐。`event_reliability.go` 改为 type alias + delegation 兼容层（标记 Deprecated），漂移风险已消除。

**已修复问题**（v5.0 核实）：
- ~~`criticalTypeSet`（13 种）≠ AS-EVT-01 Critical（4 种）~~ → 已删除，bus.go 改用 `contract.RequiresBlockUpTo()`
- ~~`isCriticalEnvelopeType` ≠ `ClassifyEventReliability`~~ → 已删除，event_persist_handler.go 改用 `contract.RequiresBlockUpTo()`
- ~~`event/event_reliability.go` 与 `contract/reliability.go` 逻辑重复~~ → event_reliability.go 改为 type alias + delegation，标记 Deprecated

**仍存在的问题**：
- `ContextUsage` 和 `TeamRunFailed` 未在 AS-EVT-01 任何级别中显式定义，当前落入 Informational（default 分支）

**BlockUpTo**：超时仍为 100ms，Important 级别事件超时后降级为 `DropOldest`。

#### 2.2.3 事件回放能力评估

| 机制 | 容量 | 持久化 | 用途 |
|------|------|--------|------|
| 内存 Buffer.Replay | 200条/会话 | 无 | WS 重连回放 |
| EventStore List | 无限（7天TTL） | SQLite | API 查询展示 |
| 状态重建 | 不支持 | - | 不通过回放事件重建状态 |

**不存在 CQRS/Event Sourcing 模式**：系统通过直接 DB 读写状态，不通过回放事件重建。

#### 2.2.4 评审结论

**根因**（v2.0）：EventBus 设计为纯内存传输层，持久化是后置的、尽力而为的异步操作。

**v5.0 进展**：EventWAL 已实现，Critical 级事件不再丢失。事件三级分级已落地。`criticalTypeSet` 和 `isCriticalEnvelopeType` 已删除，统一使用 `contract.RequiresBlockUpTo()`，与 AS-EVT-01 完全对齐。`event_reliability.go` 改为兼容层，漂移风险已消除。

**剩余风险**：
- Important 级别事件（StateDelta/TokenUsage 等）进程崩溃仍可能丢失
- `ContextUsage` 和 `TeamRunFailed` 未在 AS-EVT-01 中显式分级，当前落入 Informational
- 无法支持事件回放/状态重建

> **v2.0 补充**：痛点 2 不是"相对独立可并行推进"的，它是痛点 1/3/4 的**横切关注点**。状态机转换事件、HITL 挂起/恢复事件、Orchestrator 生命周期事件都需要可靠投递。依赖关系图需修正。

---

### 痛点 3：缺少显式状态机

**严重度**：中高 | **影响面**：Session/Run/TeamRun 状态转换 | **竞品对标**：LangGraph StateGraph

#### 2.3.1 Run 状态机

> **v3.0 更新**：✅ 已实现。`RunStateMachine`（`internal/biz/run_state_machine.go`）基于 `shared.GenericStateMachine[RunState, RunEvent]`，8 条转换规则，实现了 `shared.StateMachine[RunState, RunEvent]` 统一接口。

状态值：`running` / `completed` / `failed` / `cancelled` / `awaiting_user`

转换已从散布在 `setRunStatus`、`cancelActiveRun` 等方法中，集中到 `RunStateMachine.Transition`。

#### 2.3.2 Session 状态机

> **v3.0 更新**：⚠️ 部分合规。`SessionStatusMachine`（`internal/biz/session/status_machine.go`）有完整 struct + `TransitionTo` + `CanTransitionTo` + 单元测试，但**未实现** `shared.StateMachine[S, E]` 统一接口。使用旧式手写 `validTransitions map[SessionStatus][]SessionStatus`，缺少 Event 类型定义、缺少 `ValidTargets` 方法、方法签名不匹配统一接口。需迁移。

#### 2.3.3 TeamRun 状态机

> **v3.0 更新**：⚠️ 新旧两套并存。`ValidateTeamRunTransition`（`internal/biz/team_types.go:84-98`）为旧式手写实现；`TeamRunStateMachine`（`internal/biz/team_run_state_machine.go`）为新式统一接口实现。两者转换规则一致，需逐步迁移到统一接口。

#### 2.3.4 现有状态机实现不一致

| 实体 | 实现方式 | 合规性 | v4.0 变化 |
|------|---------|--------|----------|
| Run | struct + `shared.GenericStateMachine` + 统一接口 | ✅ 完复合规 | 无变化 |
| SessionRunPhase | struct + `shared.GenericStateMachine` + 统一接口 | ✅ 完复合规 | 无变化 |
| Team | 新旧两套：旧式 `ValidTeamStatusTransition` + 新式 `TeamStateMachine` | ⚠️ 迁移中 | 旧式仍被 `team_usecase.go` 调用（2 处） |
| TeamRun | 新旧两套：旧式 `ValidateTeamRunTransition` + 新式 `TeamRunStateMachine` | ⚠️ 迁移中 | 旧式仍被 `team_graph_run_coordinator.go` 调用（3 处） |
| Session | struct + `TransitionTo` + `CanTransitionTo`（未实现统一接口） | ⚠️ 需迁移 | 无变化 |
| GraphExecution | struct + `shared.GenericStateMachine` + 统一接口 | ✅ 完复合规 | 无变化 |
| ChannelTurnJob | struct + `shared.GenericStateMachine` + 统一接口 | ✅ 完复合规 | 无变化 |
| Evolution | struct + `shared.GenericStateMachine` + 统一接口 | ✅ 完复合规 | v4.0 新增 |
| Agent | struct + `shared.GenericStateMachine` + 统一接口 | ✅ 完复合规 | v4.0 新增 |

> **v4.0 更新**：统一接口 `StateMachine[S ~string, E ~string]`（`internal/biz/shared/state_machine.go`）已实现，配套 `GenericStateMachine[S, E]` 基于 O(1) 规则表查找，支持 Guard 条件。9 个实体中 6 个已完全合规（Run/SessionRunPhase/GraphExecution/ChannelTurnJob/Evolution/Agent），2 个新旧并存（Team/TeamRun），1 个需迁移（Session）。

#### 2.3.5 跨实体状态关联缺失

> **v2.0 新增** | **v3.0 更新**：`StateMachineCoordinator` 接口尚未实现，仅在本文档中作为提议存在。

Run/TeamRun/Session 之间存在状态联动，但当前无关联校验：

| 关联场景 | 当前问题 | 风险 |
|---------|---------|------|
| Run → Session | Run 进入 `awaiting_user` 时 Session 应进入 `awaiting_confirmation` | 代码中手动配对，易遗漏 |
| TeamRun → Run | TeamRun 进入 `waiting_human` 时，下属 Run 应暂停 | 无级联校验 |
| GraphExecution → Run | Graph 节点中断时，对应 Run 应进入 `awaiting_user` | 无关联机制 |

#### 2.3.6 评审结论

**根因**（v2.0）：Run 状态机从未被显式定义，所有转换都是命令式的 `SetStatus` 调用。各实体状态机实现不一致，跨实体状态关联无校验。

**v3.0 进展**：统一接口已实现，4 个实体已完全合规，2 个新旧并存。核心问题（Run 无状态机）已解决。

**剩余风险**：
- Session 未迁移到统一接口
- Team/TeamRun 新旧两套并存，需清理旧式函数
- 跨实体状态关联（`StateMachineCoordinator`）未实现
- 无法生成状态转换图供文档/调试使用

---

### 痛点 4：HITL 非一等公民

**严重度**：中高 | **影响面**：人工审批/确认/反馈场景 | **竞品对标**：Bisheng HIL 一等公民、LangGraph Approval Node、n8n Wait Node

#### 2.4.1 当前 HITL 能力

| 层级 | 机制 | 评估 | v3.0 变化 |
|------|------|------|----------|
| 单 Agent | await_user_reply（工具等待用户回复） | 可用，但仅限工具回调场景 | 已提取到 `chatAwaitCoordinator` |
| Graph | InterruptBefore/InterruptAfter + waiting_human 状态 | 较完整，支持检查点恢复 | 无变化 |
| TeamRun | DeferTeamRunSuccessIfHITL + SLA 保护 | 较完整，支持超时和延期 | 无变化 |
| Session | awaiting_confirmation 状态 | 仅状态标记，无通用交互协议 | 无变化 |
| **统一抽象** | **HumanLoopGate** | **未实现** | 仅设计文档 |

#### 2.4.2 缺失的 HITL 模式

| 模式 | 描述 | 竞品支持 | Aranea 现状 |
|------|------|---------|------------|
| **审批门** | 执行前等待人类确认 | LangGraph Approval Node | 无（仅工具级 tool_confirm） |
| **检查点干预** | 执行中暂停，人类可修改状态后继续 | Bisheng 多轮对话干预 | Graph 级有，Session 级无 |

> **v2.0 修正**：移除"反馈环"和"通用等待"模式。feedback 模式尚无业务需求，YAGNI 原则。通用等待已被审批门覆盖。

#### 2.4.3 评审结论

**根因**（v2.0）：HITL 能力分散在 Graph/TeamRun/Agent 三个层级，缺少统一的 `HumanLoopGate` 抽象。

**v3.0 进展**：`AwaitCoordinator` 已提取（`chat_orch_await.go`），`makeAwaitReplyFunc` 已迁移到 `chatAwaitCoordinator`。痛点 4 的前置依赖（痛点 1 AwaitCoordinator 提取）已完成。

**剩余风险**：
- `HumanLoopGate` 统一抽象未实现
- 不同层级的 HITL 行为不一致（超时策略、恢复机制）
- 无法在编排层面统一声明 HITL 需求

> **v3.0 更新**：HITL 的 `await_user_reply` 机制已从 ChatOrchestrator 提取到 `chatAwaitCoordinator`（`internal/service/chat_orch_await.go:212`），但 `HumanLoopGate` 统一接口尚未定义。`tool_confirm` 机制完整（`internal/agent/tool_confirmation.go`）。

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

- **Native 路径已 Deprecated 但未移除**：`BuildTRPCTeam`（`internal/team/trpc_build.go:62`）标记 Deprecated + `TODO(phase-8): Remove`，仅用于 `ARANEA_TEAM_NATIVE=1` 紧急熔断，增加维护负担
- **Swarm 与 Adaptive 无实质区别**：`normalizeCompileMode`（`internal/team/graph_compile.go:57-68`）将 swarm 映射为 adaptive，常量层面仍分开
- **跨 Team 协作缺失**：`AgentAllocator.AssignedType=team` 仅为接口预留
- **运行时动态编排缺失**：拓扑在编译时确定，运行时无法动态调整

#### 2.5.4 评审结论

**根因**（v2.0）：6 种编排模式覆盖了常见场景，但 Native/Graph 双引擎增加了维护复杂度，Swarm/Adaptive 模糊性增加认知负担。

**v3.0 进展**：Native 路径已 Deprecated，仅用于紧急熔断（需 `ARANEA_TEAM_NATIVE=1`），实际生产已默认走 Graph 路径。

**剩余风险**：
- Native 路径代码仍存在，需彻底移除
- Swarm/Adaptive 常量层面仍分开，增加认知负担
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

> **v3.0 状态**：大部分已完成

#### 4.1.1 显式状态机（痛点3）✅ 已完成

**方案**：定义统一状态机接口 + `RunStateMachine` + `SessionRunPhaseMachine`。

**统一状态机接口**（✅ 已实现，`internal/biz/shared/state_machine.go`）：

```go
// internal/biz/shared/state_machine.go
type StateMachine[S ~string, E ~string] interface {
    Transition(from S, event E) (S, error)
    CanTransition(from S, to S) bool
    ValidTargets(from S) []S
}
```

> **v2.0 修正**：原 v1.0 方案将 RunStateMachine 放在 `internal/runtime/turn/`，但 Run 状态是业务概念，其转换规则应在 biz 层定义。runtime/turn 包通过 biz 层接口调用。同时，Session/Team/TeamRun 的状态机应统一到此接口。

**RunStateMachine**（✅ 已实现，`internal/biz/run_state_machine.go`）：

```go
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

**SessionRunPhaseMachine**（✅ 已实现，`internal/biz/session_run_phase_machine.go`，v2.0 新增）：

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

**迁移策略**（v3.0 进展）：
1. ~~新建 `RunStateMachine` 和 `SessionRunPhaseMachine`，与现有 `SetStatus` 并行运行~~ ✅ 已完成
2. ~~在 `setRunStatus` 中增加转换合法性校验（仅日志警告，不阻断）~~ ✅ 已完成
3. ~~验证期（2 个迭代）后，非法转换升级为错误~~ ✅ 已完成
4. ~~最终移除散落的 `SetStatus` 调用，统一走 `StateMachine.Transition`~~ ✅ 已完成
5. 将 Team/TeamRun 的旧式函数迁移为统一接口调用 — ⚠️ 进行中（新式已实现，旧式未清理）
6. 将 Session 迁移到统一接口 — ❌ 未开始

> **v2.0 修正**：验证期从 1 个迭代延长到 2 个迭代。现有代码中 `setRunStatus` 被调用的路径有 10+ 处，1 个迭代可能不够覆盖所有边界情况。

**跨实体状态关联**（v2.0 新增，❌ 未实现）：

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

#### 4.1.2 事件关键路径持久化（痛点2，短期方案）✅ 已完成

**方案**：为 Critical 级低频事件增加 Write-Before-Publish（WBPF）机制。

> **v3.0 更新**：EventWAL 已实现（`internal/event/wal.go`），三级事件分级已落地（`internal/event/contract/reliability.go`，标注 `Stability:stable`）。WAL 清理定时任务已实现（`internal/cronrunner/jobs/event_wal_cleanup.go`，7 天 TTL）。

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

**迁移策略**（v5.0 进展）：
1. ~~新建 `EventWAL`，在 Bus 和所有 subscriber 就绪后调用 `Recover`~~ ✅ 已完成
2. ~~修改 `Infra.Publish`，对 Critical WBPF 类型走 `WriteBeforePublish`~~ ✅ 已完成
3. ~~Important 级别事件保持现有 `BlockUpTo` + 异步持久化到 EventStore~~ ✅ 已完成
4. ~~Informational 级别事件行为不变~~ ✅ 已完成
5. ~~添加 WAL 清理定时任务（7 天 TTL，与 EventStore 一致）~~ ✅ 已完成
6. ~~统一 `criticalTypeSet` 与 `ClassifyEventReliability`~~ ✅ 已完成（v5.0：`criticalTypeSet` 已删除，bus.go 改用 `contract.RequiresBlockUpTo()`）
7. ~~统一 `isCriticalEnvelopeType` 与 `ClassifyEventReliability`~~ ✅ 已完成（v5.0：`isCriticalEnvelopeType` 已删除，event_persist_handler.go 改用 `contract.RequiresBlockUpTo()`）
8. ~~消除 `event_reliability.go` 与 `contract/reliability.go` 逻辑重复~~ ✅ 已完成（v5.0：event_reliability.go 改为 type alias + delegation 兼容层）
9. `ContextUsage` 和 `TeamRunFailed` 显式分级 — ❌ 未完成（需架构决策确定应归入 Important 或 Informational）

**验证**：模拟进程崩溃测试，验证 Critical 事件不丢失；Recover 幂等测试验证不重复。

---

### 阶段二：结构优化（P1，2-3 迭代）

> **v3.0 状态**：大部分已完成

#### 4.2.1 拆分 ChatOrchestrator（痛点1）✅ 已完成

**方案**：按职责域提取 5 个子管理器，ChatOrchestrator 退化为纯协调者。

**v3.0 实际结构**：

```
ChatOrchestrator（协调者，12 字段）
  ├── chatTurnLifecycle（组合接口）
  │   ├── sessionStateTransitor — 会话状态转换
  │   ├── turnRecorder — 指标记录
  │   └── turnEventPublisher — 事件发布
  └── chatRunManager（组合接口）
      ├── chatRunStatusTracker — Run 状态管理
      ├── chatPendingQueueManager — Pending Queue 管理
      ├── chatAwaitCoordinator — Await/Resume 管理
      └── chatSessionRunLifecycle — Session Run 生命周期
  + agentBuildDirector — Agent 构建+工具注入
```

> **v2.0 修正**：子管理器放在 `internal/service/` 下独立文件（如 `chat_orch_run_status.go`），而非 `internal/runtime/turn/`。理由：子管理器持有 `biz.SessionWriter`、`biz.ChatUsecase` 等 biz 层依赖，按项目分层规范 BA2，不应放入 runtime 包。

**提取顺序**（v3.0 状态：全部已完成）：

1. **RunStatusTracker** → `chatRunStatusTracker` ✅
   - 提取字段：`sessionRunBindings` sync.Map → `bindings *TypedSyncMap`
   - 提取方法：`setRunStatus`、`persistRunStatus`、`hydrateRunStatusFromSession`、`publishRunStatus`、`GetRunStatus`
   - 与 RunStateMachine 集成：`Transition` 成功后自动 `publishRunStatus`

2. **PendingQueueManager** → `chatPendingQueueManager` ✅
   - 提取字段：`pendingMergeFollowup` sync.Map → `mergeFollowup *TypedSyncMap`
   - 提取方法：`EnqueueUserMessage`、`DequeuePendingMessage`、`processPendingQueue`、`SetSessionPendingMergeFollowup`、`sessionPendingMergeFollowup`

3. **AwaitCoordinator** → `chatAwaitCoordinator` ✅
   - 提取字段：`awaitMetaCache`、`resumeInFlight` sync.Map → `awaitCache *TypedSyncMap`、`resumeInFlight *TypedSyncMap`
   - 提取方法：`MakeAwaitReplyFunc`、`tryBeginResume`、`endResume`、`persistAwaitMarkers`、`canResumeAwait`、`RegisterAwaitChannel`、`DeleteAwaitChannel`、`LoadAwaitChannel`

4. **SessionRunLifecycle** → `chatSessionRunLifecycle` ✅
   - 提取方法：`beginSessionRunLifecycle`、`finishSessionRunLifecycle`、`escalateSessionRunToDurable`

5. **AgentBuildDirector** → `chatAgentBuildDirector` ✅
   - 提取 `TRPCBuilderDeps` 为独立 struct，将 30+ 字段的依赖组装逻辑封装
   - `runSingleAgentViaTRPC` 从 571 行降至 550 行

> **v2.0 修正**：
> - 提取顺序调整：PendingQueueManager 提前到第 2 位（低依赖），AwaitCoordinator 移到第 3 位（中依赖）
> - AgentBuildDirector 不拆为三阶段，改为提取 TRPCBuilderDeps 构建器
> - Wire 绑定收口在 `service.go` 的 ProviderSet 中，子管理器不各自 `wire.NewSet`

**Wire 重组**：
- 子管理器统一在 `service.go` ProviderSet 中注册
- `ChatOrchestratorDeps` 拆分为 `OrchCoreDeps` + 5 个子管理器 deps struct
- 每次提取一个子管理器后立即 `make wire && make build` 验证

**验证**：每个子管理器独立单测；集成测试验证 Orchestrator 行为不变。

#### 4.2.2 sync.Map 类型安全化 ✅ 已完成

**方案**：使用泛型包装替代 `timestampedEntry{value: any}`。

> **v3.0 更新**：`TypedSyncMap[K comparable, V any]`（`internal/service/orch_typed_map.go`）已实现，4 个 sync.Map 已全部替换。对于 `resumeInFlight` 的竞态问题，仍使用时间戳清理机制，未改用 `context.WithTimeout` + `Done()` channel。

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

#### 4.2.3 SessionUsecase 拆分（v2.0 新增）⚠️ 部分完成

**方案**：将 SessionUsecase 的 30 个字段拆分为独立 Usecase。

**v3.0 进展**：
1. **`SessionMetricsUsecase`** ✅ 已提取为独立 struct（`internal/biz/session/metrics.go`），但 SessionUsecase 仍保留 Legacy 遗留字段（`metricsDeltaMu`、`metricsDeltas`、`flushInterval`、`metricsUpdatedPublisher`），Facade 委托方法在 `metrics_flush.go` 中
2. **`SessionCompressionUsecase`** ✅ 已提取为独立 struct（`internal/biz/session/compression.go`，4 个字段），通过 `compressionUsecase *SessionCompressionUsecase` 持有（非内嵌 Facade）
3. SessionUsecase 字段数仍为 30（含 Legacy 遗留字段），未达标

**剩余工作**：清理 Legacy 遗留字段，将 SessionUsecase 字段数降至 ~15

#### 4.2.4 Native 路径清理（v2.0 从阶段四提前）⚠️ 部分完成

**方案**：移除 Native 路径，仅保留 Graph 编译路径。

**v3.0 进展**：
- `BuildTRPCTeam`（`internal/team/trpc_build.go:62`）已标记 Deprecated + `TODO(phase-8): Remove`
- `tryNativeFallback` 仅在 `ARANEA_TEAM_NATIVE=1` 时生效（紧急熔断）
- Native 路径代码仍存在，未彻底移除

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

**执行现状**：1 篇 ADR 已存在（`docs/reports/2026-06-11-review-adr-architecture-refactoring.md`，ADR-01），但 `docs/adr/` 目录未创建，ADR 索引文件未创建，编号规范未落地。

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

**执行现状**（v5.0 更新）：
- ChatOrchestrator：35→12 字段，✅ 已达标
- SessionUsecase：30 字段，❌ 仍严重超标
- `TECH-DEBT(COG)` 标注格式未使用（源码中有其他 TECH-DEBT 标注约 22 处，但无 AS-COG-01 规范格式）
- `internal/archlint/archlint_test.go` 中 `TestStructFieldCount` 已实现 struct 字段 ≤15 检查

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

**现有实体需补全**（v4.0 更新）：
- Run 状态机（5 种状态）— ✅ 已完成（`internal/biz/run_state_machine.go`）
- SessionRunPhase 状态机（6 种状态）— ✅ 已完成（`internal/biz/session_run_phase_machine.go`）
- Session 状态机（已有，需统一到 `StateMachine[S, E]` 接口）— ⚠️ 未迁移
- TeamRun 状态机（6 种状态，需从函数式迁移为 struct）— ⚠️ 新式已实现，旧式未清理（`team_graph_run_coordinator.go` 3 处调用）
- Team 状态机（7 种状态，需从函数式迁移为 struct）— ⚠️ 新式已实现，旧式未清理（`team_usecase.go` 2 处调用）
- GraphExecution 状态机（5 种状态，需新建）— ✅ 已完成（`internal/biz/graph_execution_state_machine.go`）
- ChannelTurnJob 状态机（8 种状态）— ✅ 已完成（`internal/biz/channel_turn_job_state_machine.go`）
- Evolution 状态机（4 种状态）— ✅ 已完成（`internal/biz/evolution_state_machine.go`，v4.0 新增）
- Agent 状态机（3 种状态）— ✅ 已完成（`internal/biz/agent_state_machine.go`，v4.0 新增）

**跨实体状态关联**（v2.0 新增）：

有父子关系的实体，子实体状态转换必须满足父实体状态的约束（`ParentConstraint` 守卫条件），父实体状态转换必须级联校验子实体状态（`CascadeCheck` 守卫条件）。

**执行现状**（v4.0 更新）：9 个实体中 6 个完全合规（Run/SessionRunPhase/GraphExecution/ChannelTurnJob/Evolution/Agent），2 个新旧并存（Team/TeamRun），1 个需迁移（Session）。

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

**执行现状**（v4.0 更新）：144 处 `Stability:` 标注已广泛落地，覆盖 biz/service/event 层。分布约 94 处 `stable`、46 处 `evolving`、4 处 `internal`。

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

**执行现状**（v3.0 更新）：`internal/archlint/archlint_test.go` 已实现，包含 5 个测试：
- `TestBizNotDependOnTrpcAgentGo` — P0 依赖方向检查 ✅
- `TestServiceNotDirectlyAccessData` — P0 分层隔离检查 ✅
- `TestBizPortInterfaceMethodCount` — 接口窄化检查 ✅
- `TestStateMachineCoverage` — 状态机覆盖检查 ✅
- `TestStructFieldCount` — 认知复杂度检查 ✅

运行方式：`make archlint` 或 `go test ./internal/archlint/ -count=1`

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

**执行现状**（v5.0 更新）：三级分级已实现（`internal/event/contract/reliability.go`，标注 `Stability:stable`），是事件可靠性分级的单一真相源。EventWAL 已实现，Critical 事件走 WBPF。`criticalTypeSet` 和 `isCriticalEnvelopeType` 已删除，bus.go 和 event_persist_handler.go 统一使用 `contract.RequiresBlockUpTo()`，与 AS-EVT-01 完全对齐。`event_reliability.go` 改为 type alias + delegation 兼容层（标记 Deprecated），漂移风险已消除。仍存在 `ContextUsage` 和 `TeamRunFailed` 未显式分级的问题。

---

## 六、实施路线图

> **v5.0 修正**：标注已完成项和当前进展

```
阶段一（P0，1-2 迭代）— ✅ 已完成
├── ✅ RunStateMachine 定义 + 统一接口 + 迁移
├── ✅ SessionRunPhaseMachine 定义 + 迁移
├── ✅ EventWAL 实现（仅 Critical 4 种事件 WBPF）+ Recover
├── ✅ 事件分级标注（AS-EVT-01，contract/reliability.go）
├── ✅ criticalTypeSet/isCriticalEnvelopeType 统一删除，改用 contract.RequiresBlockUpTo()（v5.0）
├── ✅ event_reliability.go 改为兼容层，漂移风险消除（v5.0）
├── ⚠️ AS 标准落地：Stability 标注 144 处已落地，archlint 5 个测试已实现，1 篇 ADR 已存在但 docs/adr/ 目录未创建
└── ✅ openspec/specs/ 基础结构创建（architecture-blueprint.md 初版）

阶段二（P1，2-3 迭代）— ✅ 已完成
├── ✅ RunStatusTracker 提取 → chatRunStatusTracker
├── ✅ PendingQueueManager 提取 → chatPendingQueueManager
├── ✅ AwaitCoordinator 提取 → chatAwaitCoordinator
├── ✅ SessionRunLifecycle 提取 → chatSessionRunLifecycle
├── ✅ AgentBuildDirector 提取 → chatAgentBuildDirector
├── ✅ sync.Map 泛型化 → TypedSyncMap
├── ✅ SessionUsecase Legacy 清理（4 个遗留字段移除，注入 SessionMetricsUsecase）
├── ✅ Native 路径彻底移除（BuildTRPCTeam/tryNativeFallback/envTeamNativeForced/DecideNativeFallback）
├── ✅ Team/TeamRun 状态机迁移到 CanTransition() API（5 处调用点）
├── ✅ SessionStateMachine 实现统一接口 + 单元测试
├── ✅ StateMachineCoordinator 实现（纯函数设计 + 6 个测试）
├── ✅ TeamRunFailed/ContextUsage 事件分级补全
├── ✅ 哨兵错误 ErrInvalidTransition/ErrGuardRejected + %w 包装
├── ✅ Stability 标注全面补全（biz/team/session 层 15+ 接口）
├── ✅ TECH-DEBT(COG) 格式规范化
├── ✅ docs/adr/ 目录 + 索引 + ADR-001 完整内容
├── ✅ archlint P1（接口窄化检查）
├── ✅ EvolutionStateMachine + AgentStateMachine（v4.0 新增）
└── ✅ archlint P2（状态机覆盖 + 认知复杂度检查）

阶段三（P2，3-5 迭代）— ❌ 未开始
├── HumanLoopGate 统一抽象（approval + checkpoint 两种模式）
├── approval 模式实现（复用 await_user_reply）
├── checkpoint 模式实现（GraphInterruptAdapter）
├── EventStore 查询增强 + Checkpoint 机制
├── Swarm/Adaptive 合并
└── Handoff 策略化

阶段四（P3，6+ 迭代）— ❌ 未开始
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

**v7.0 核心进展总结**（P0/P1 + TECH-DEBT 全部修复完成）：

1. **ChatOrchestrator 上帝对象**完全解决：字段 35→12，sync.Map 4→0，5 个子管理器全部提取，runSingleAgentViaTRPC 571→134 行，turn.go 883→365 行，invokeTurnLLMAndStream 80→50 行
2. **SessionUsecase 上帝对象**完全解决：字段 26→14（AS-COG-01 合规），提取 SessionMessageUsecase + SessionTimelineUsecase，Facade fallback 清理完成
3. **EventBus 持久化**完全解决：EventWAL 已实现，三级事件分级完整落地，AS-EVT-01 无遗漏
4. **显式状态机**完全合规：统一接口 + 哨兵错误，SessionStateMachine 已迁移，Team/TeamRun 旧式函数彻底删除，StateMachineCoordinator 已实现
5. **Native 路径**彻底移除，TeamGraphRunCoordinator 改用窄接口
6. **AS 标准**全面落地：Stability 标注补全，TECH-DEBT(COG) 格式规范化，ADR-001 完整

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

---

## 八、v7.0 剩余问题清单

> 2026-06-12 代码核对后统计（v7.0 更新：P0/P1 + TECH-DEBT D1-D5 全部修复完成）

### 阶段一/二遗留（P0/P1）— ✅ 全部已修复

（见 v6.0 文档，13 项全部已修复）

### 已知 TECH-DEBT — ✅ 全部已修复

| # | 问题 | 痛点 | 修复状态 | v7.0 变化 |
|---|------|------|---------|----------|
| ~~D1~~ | ~~chat_orchestrator_turn.go 仍 883 行（上限 500）~~ | ~~痛点1~~ | ~~已修复~~ | ✅ 883→365 行，提取 dispatch/api/metrics 三个文件 |
| ~~D2~~ | ~~SessionUsecase 仍 26 字段（上限 15）~~ | ~~痛点1~~ | ~~已修复~~ | ✅ 26→14 字段，提取 SessionMessageUsecase + SessionTimelineUsecase |
| ~~D3~~ | ~~invokeTurnLLMAndStream 80 行踩线（上限 80）~~ | ~~痛点1~~ | ~~已修复~~ | ✅ 提取 assembleTurnResult，80→50 行 |
| ~~D4~~ | ~~TeamGraphRunCoordinator 持有 biz.TeamRunRepo 而非窄接口~~ | ~~痛点5~~ | ~~已修复~~ | ✅ 改用 teamRunReader + teamRunWriter 窄接口 |
| ~~D5~~ | ~~ValidTeamStatusTransition/ValidateTeamRunTransition 标记 Deprecated 但仍保留~~ | ~~痛点3~~ | ~~已修复~~ | ✅ 旧函数和转换表彻底删除，测试迁移到 CanTransition API |

### 阶段三未启动（P2）

| # | 问题 | 痛点 | 说明 |
|---|------|------|------|
| R14 | HumanLoopGate 统一抽象未实现 | 痛点4 | approval + checkpoint 两种模式 |
| R15 | EventStore 查询增强 + Checkpoint 机制 | 痛点2 | 中期方案 |
| R16 | Swarm/Adaptive 合并 | 痛点5 | 减少认知负担 |
| R17 | Handoff 策略化 | 痛点5 | 作为 swarm 配置项 |

### 阶段四未启动（P3）

| # | 问题 | 痛点 | 说明 |
|---|------|------|------|
| R18 | Agent-as-Tool Graph 路径支持 | 痛点5 | 当前仅 Native 路径 |
| R19 | 跨 Team 协作 + A2A 协议 | 痛点5 | AgentAllocator 接口预留 |
| R20 | 远期可选：Event Sourcing + CQRS | 痛点2 | 需 ADR 论证 |

### 统计

| 类别 | v4.0 数量 | v5.0 数量 | v6.0 数量 | v7.0 数量 | 变化 |
|------|----------|----------|----------|----------|------|
| 阶段一/二遗留（P0/P1） | 13 | 10 | 0 | 0 | 0 |
| 已知 TECH-DEBT | N/A | N/A | 5 | 0 | -5 |
| 阶段三未启动（P2） | 4 | 4 | 4 | 4 | 0 |
| 阶段四未启动（P3） | 3 | 3 | 3 | 3 | 0 |
| **剩余问题总计** | **20** | **17** | **12** | **7**（4 P2 + 3 P3） | **-5** |

**v7.0 已修复项**（TECH-DEBT D1-D5 全部清零）：
- D1：chat_orchestrator_turn.go 883→365 行，提取 dispatch/api/metrics 三个文件
- D2：SessionUsecase 26→14 字段，提取 SessionMessageUsecase + SessionTimelineUsecase
- D3：invokeTurnLLMAndStream 80→50 行，提取 assembleTurnResult
- D4：TeamGraphRunCoordinator 改用 teamRunReader + teamRunWriter 窄接口
- D5：ValidTeamStatusTransition/ValidateTeamRunTransition 彻底删除

### 价值评估

> 从**系统可靠性提升**、**开发效率提升**、**可演进性提升**三个维度评估每项的系统价值，并给出综合 ROI 分级。

#### 评估维度说明

| 维度 | 含义 | 权重 |
|------|------|------|
| 可靠性 | 修复后对生产稳定性、数据完整性、故障恢复的贡献 | 高 |
| 开发效率 | 修复后对新功能开发速度、Bug 修复速度、代码理解成本的改善 | 中高 |
| 可演进性 | 修复后对架构扩展能力、新场景适配、技术债控制的贡献 | 中 |

#### 逐项评估

| # | 问题 | 可靠性 | 开发效率 | 可演进性 | 实施成本 | 综合 ROI | 评估理由 |
|---|------|--------|---------|---------|---------|---------|---------|
| R1 | runSingleAgentViaTRPC 550 行 | ★☆☆ | ★★★ | ★★★ | 高 | **中** | 可靠性影响小（当前稳定运行），但对开发效率和可演进性影响极大——550 行方法是系统最频繁修改的热点，任何 Turn 阶段逻辑变更都需理解全部代码。拆分后可独立测试各阶段，但实施成本高（Wire 重组 + 集成测试覆盖） |
| R2 | SessionUsecase 30 字段 | ★☆☆ | ★★☆ | ★★★ | 低 | **高** | 清理 4 个 Legacy 遗留字段是纯删除操作，成本极低，立即可将字段数从 30 降至 26。虽然仍超标，但消除了 Facade 回退路径的维护负担。进一步拆分到 ~15 字段成本中等，但收益递增 |
| R5 | ContextUsage/TeamRunFailed 未分级 | ★☆☆ | ★☆☆ | ★★☆ | 低 | **中** | 这两个事件在 AS-EVT-01 中未显式定义，当前落入 Informational（default）。需先决策它们应归入哪个级别，再修改 contract/reliability.go。成本极低但需架构决策 |
| R6 | Session 状态机未迁移统一接口 | ★☆☆ | ★★☆ | ★★★ | 中 | **中高** | Session 是核心实体，旧式实现缺 Event 类型和 ValidTargets，无法参与 StateMachineCoordinator 的跨实体关联。迁移后与其他 8 个实体接口一致，降低新人理解成本。但 Session 状态转换逻辑稳定，短期无功能收益 |
| R7 | Team 旧式函数仍被调用（2 处） | ★☆☆ | ★☆☆ | ★★☆ | 低 | **中** | 2 处调用迁移简单（改用 TeamStateMachine.CanTransition），但 Team 状态转换逻辑稳定，无功能收益。建议随 R8 一起批量处理 |
| R8 | TeamRun 旧式函数仍被调用（3 处） | ★☆☆ | ★☆☆ | ★★☆ | 低 | **中** | 同 R7，3 处调用迁移简单。新旧两套并存增加认知负担，但无生产风险。建议与 R7 合并处理 |
| R9 | StateMachineCoordinator 未实现 | ★★★ | ★★☆ | ★★★ | 高 | **中高** | 跨实体状态关联是当前系统最大的隐性风险——Run 进入 awaiting_user 时 Session 未联动、TeamRun waiting_human 时下属 Run 未暂停，全靠代码中手动配对。但实现 Coordinator 需要先完成 R6（Session 迁移），且需设计级联策略避免循环依赖。长期价值高，短期成本也高 |
| R10 | Native 路径代码仍存在 | ★☆☆ | ★★☆ | ★★☆ | 中 | **中** | 每次编排逻辑修改需双路径维护，增加开发和测试成本。但 Native 路径已 Deprecated 且需环境变量激活，生产风险极低。移除是"清理债务"而非"修复缺陷"，收益渐进 |
| R11 | docs/adr/ 目录未创建 | ★☆☆ | ★☆☆ | ★★★ | 低 | **高** | ADR 是架构治理的基础设施——没有 ADR，架构决策无记录，新人无法理解决策背景，重构时可能重复犯错。1 篇 ADR 已存在但目录规范未落地，创建 docs/adr/ + 索引 + 迁移现有 ADR 成本低，但对长期可维护性价值极高 |
| R12 | TECH-DEBT(COG) 标注格式未使用 | ★☆☆ | ★★☆ | ★★☆ | 低 | **中** | 当前 22 处 TECH-DEBT 标注格式不统一，无法自动统计超标项。统一格式后可与 archlint 集成，形成"标注→检测→规划"闭环。但这是工具链改进，不直接影响系统行为 |
| R14 | HumanLoopGate 统一抽象 | ★★☆ | ★★★ | ★★★ | 中 | **高** | HITL 是 Agent 平台的核心差异化能力。当前 HITL 分散在 3 个层级，行为不一致（超时策略、恢复机制各异），新增 HITL 场景需改多处。HumanLoopGate 统一后，新场景只需实现一个接口。前置依赖（AwaitCoordinator 提取）已完成，时机成熟 |
| R15 | EventStore 查询增强 + Checkpoint | ★★☆ | ★☆☆ | ★★☆ | 中 | **中** | 当前 EventStore 仅支持简单列表查询，无法按 run_id 或时间范围检索。Checkpoint 机制支持状态快照和恢复。但当前无明确业务需求驱动，属于"有了更好"而非"缺了不行" |
| R16 | Swarm/Adaptive 合并 | ★☆☆ | ★★☆ | ★★☆ | 低 | **中** | normalizeCompileMode 已将 swarm 映射为 adaptive，常量层面分开仅增加认知负担。合并为单一模式成本低，但需考虑 API 兼容性（用户配置中可能使用 ModeSwarm） |
| R17 | Handoff 策略化 | ★☆☆ | ★☆☆ | ★★★ | 中 | **中** | Handoff 作为 swarm 配置项可支持显式控制权转移，是 OpenAI Agents SDK 的核心模式。但当前 swarm/adaptive 已隐式支持类似行为，业务需求不迫切 |
| R18 | Agent-as-Tool Graph 路径 | ★☆☆ | ★★☆ | ★★★ | 中 | **中** | 当前 Agent-as-Tool 仅 Native 路径支持，Graph 路径缺失。随 R10（Native 移除）完成后，需补全 Graph 路径否则功能回退。但 R10 未完成前不急迫 |
| R19 | 跨 Team 协作 + A2A | ★☆☆ | ★☆☆ | ★★★ | 高 | **低** | AgentAllocator 接口已预留但无业务需求。A2A 协议仍在标准化中，过早实现风险高。等市场验证后再投入 |
| R20 | Event Sourcing + CQRS | ★★☆ | ★☆☆ | ★★★ | 极高 | **低** | 架构范式根本性变更，当前单机 SQLite 不需要水平扩展。需 ADR 论证且有明确业务需求后再考虑 |

#### 综合优先级排序

**第一梯队（高 ROI，建议立即推进）**：

| 优先级 | 项 | 理由 |
|--------|-----|------|
| 1 | R2 SessionUsecase Legacy 清理 | 成本极低（删除 4 个字段 + 回退路径），立竿见影降低认知复杂度 |
| 2 | R11 ADR 目录创建 + 规范落地 | 成本极低（创建目录 + 索引 + 迁移现有 ADR），架构治理基础设施，越早建立越早受益 |
| 3 | R5 ContextUsage/TeamRunFailed 分级决策 | 成本极低（修改 contract/reliability.go），需架构决策确定级别，消除 AS-EVT-01 遗漏 |

**第二梯队（中高 ROI，建议本迭代推进）**：

| 优先级 | 项 | 理由 |
|--------|-----|------|
| 4 | R6+R7+R8 状态机迁移收尾 | 成本低（5 处调用点迁移），消除新旧两套并存的认知负担，为 R9 铺路 |
| 5 | R14 HumanLoopGate | 前置依赖已完成，统一 HITL 抽象解锁新场景，是 Agent 平台差异化能力 |
| 6 | R12 TECH-DEBT(COG) 格式统一 | 成本低，与 archlint 形成闭环，提升技术债管理能力 |

**第三梯队（中 ROI，建议下迭代推进）**：

| 优先级 | 项 | 理由 |
|--------|-----|------|
| 7 | R1 runSingleAgentViaTRPC 拆分 | 价值高但成本也高，需设计阶段边界 + Wire 重组 + 集成测试，建议单独迭代 |
| 8 | R9 StateMachineCoordinator | 长期价值高但依赖 R6 完成，且级联策略设计复杂，建议 R6 完成后再启动 |
| 9 | R10 Native 路径移除 | 清理债务，每次编排修改省双路径维护成本，但需先完成 R18（Agent-as-Tool Graph 路径） |

**第四梯队（低 ROI，等业务需求驱动）**：

| 优先级 | 项 | 理由 |
|--------|-----|------|
| 10 | R15 EventStore 增强 | 无明确业务需求，"有了更好" |
| 11 | R16 Swarm/Adaptive 合并 | 低成本但需 API 兼容性评估 |
| 12 | R17 Handoff 策略化 | swarm 已隐式支持，显式化需求不迫切 |
| 13 | R18 Agent-as-Tool Graph | 依赖 R10，且当前 Native 路径可用 |
| 14 | R19 跨 Team 协作 | 无业务需求，A2A 协议未定型 |
| 15 | R20 Event Sourcing | 架构范式变更，需 ADR 论证 |

#### 投入产出总览

| 梯队 | 项数 | 总实施成本 | 核心收益 |
|------|------|-----------|---------|
| 第一梯队 | 3 项 | 低（~1 天） | 降低 SessionUsecase 复杂度 + 建立架构决策记录规范 + 补全事件分级遗漏 |
| 第二梯队 | 3 项 | 中（~5 天） | 状态机 100% 合规 + HITL 统一抽象 + 技术债闭环 |
| 第三梯队 | 3 项 | 高（~10 天） | 最热点方法可独立测试 + 跨实体状态安全 + Native 债务清零 |
| 第四梯队 | 6 项 | 中~极高 | 等业务需求驱动，避免过度工程 |
