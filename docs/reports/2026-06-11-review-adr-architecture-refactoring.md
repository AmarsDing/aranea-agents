# ADR-01: ChatOrchestrator 拆分为子管理器 + 通用状态机 + 事件可靠性分级

## 状态：已接受

## 背景

ChatOrchestrator 是 service 层的核心编排器，承担了会话运行生命周期、等待/恢复协调、运行状态追踪、待处理队列管理、Agent 依赖构建等职责。随着业务增长，该 struct 出现以下问题：

1. **God Object**：字段数 ~30，4 个 `sync.Map`，认知复杂度远超 AS-COG-01 上限
2. **EventBus 无持久化**：Critical 事件（ToolResult/Error/RunnerCompletion/Checkpoint）丢失后无法恢复
3. **缺少显式状态机**：Run（5 种状态）、SessionRunPhase（5 种阶段）仅有字符串常量，无转换校验
4. **架构不变量无自动验证**：依赖方向、分层隔离等规则仅靠人工审查

## 决策

### D1: ChatOrchestrator 子管理器提取

将 ChatOrchestrator 的职责拆分为 5 个子管理器，每个子管理器定义窄接口。实际实现中，4 个子管理器进一步组合为 `chatRunManager` 接口，减少 ChatOrchestrator 字段数：

| 子管理器 | 子接口 | 方法数 | 职责 |
|---------|--------|--------|------|
| RunStatusTracker | RunStatusWriter(4) + RunStatusReader(2) + BindingManager(3) + AwaitMetaManager(5) + AwaitStateCleaner(2) + Sweep | 17 | 运行状态追踪、绑定管理、等待元数据、await 状态清理 |
| PendingQueueManager | PendingQueueWriter(3) + PendingQueueReader(3) + MergeFollowupManager(2) + Sweep | 9 | 待处理队列读写、合并后续管理 |
| AwaitCoordinator | AwaitResumeGuard(2) + AwaitChannelRegistry(4) + AwaitReplyBuilder(1) + AwaitResumption(5) + Sweep | 13 | 等待/恢复协调、通道注册、恢复发布 |
| SessionRunLifecycle | 4 方法 | 4 | 会话运行生命周期（开始/完成/升级/长任务配置） |
| AgentBuildDirector | 1 方法 | 1 | TRPCBuilderDeps 构建 |

**组合接口**：
- `chatRunManager`：组合 RunStatusTracker + PendingQueueManager + AwaitCoordinator + SessionRunLifecycle，作为 ChatOrchestrator 的单一 `runMgr` 字段
- `chatTurnLifecycle`：组合 sessionStateTransitor + turnRecorder + turnEventPublisher，作为 ChatOrchestrator 的单一 `turnLC` 字段
- `agentBuildDirector`：独立存在（仅在 BUILD 阶段使用），作为 `agentBuild` 字段

**ChatOrchestrator 当前结构**（12 个字段）：
- 6 个 deps 聚合（core / channelDeps / usageDeps / teamExecDeps / evoDeps / infraDeps）
- 4 个核心引用（runs / chatUC / turnLC / runMgr）
- 1 个 agentBuild
- 1 个 sweepStop

**sync.Map 消除**：引入 `TypedSyncMap[K, V]` 泛型包装（含 TTL 过期 + Sweep 清理），替代原始 `sync.Map` + `timestampedEntry{value: any}` 模式。ChatOrchestrator 的 sync.Map 数量从 4 降至 0，全部迁移到子管理器：

| TypedSyncMap 实例化 | 所在子管理器 | 原 sync.Map |
|---------------------|------------|------------|
| `TypedSyncMap[string, sessionRunTurnBinding]` | RunStatusTracker.bindings | sessionRunBindings |
| `TypedSyncMap[string, biz.ChatAwaitMeta]` | RunStatusTracker.awaitCache | awaitMetaCache |
| `TypedSyncMap[string, bool]` | PendingQueueManager.mergeFollowup | pendingMergeFollowup |
| `TypedSyncMap[string, struct{}]` | AwaitCoordinator.resumeInFlight | resumeInFlight |

**参数聚合**：超过 5 个参数的接口方法使用 Option struct 聚合（`SessionRunStartParams`、`AgentBuildParams`、`chatSessionRunLifecycleDeps`、`chatAwaitCoordinatorDeps`、`chatAgentBuildDirectorDeps`）。

**与原方案的偏差**：
- `processPendingQueue` 未提取到 PendingQueueManager（需要完整 turn 执行管道，仍留在 ChatOrchestrator）
- `BindingManager` 归入 RunStatusTracker 而非 SessionRunLifecycle（binding 是 run status 的关联数据）
- `AwaitMetaManager` 和 `AwaitStateCleaner` 归入 RunStatusTracker（await 元数据与 run 状态紧密关联）
- `ResolveChannelLongTaskConfig` 加入 SessionRunLifecycle（channel 长任务配置解析）
- `AgentBuildDirector` 使用 `customToolFunc` 回调模式注入自定义工具

### D2: 通用状态机框架

创建 `StateMachine[S ~string, E ~string]` 泛型接口（`Stability:stable`），支持：
- `Transition(from, event) (to, error)` — 带守卫条件的转换
- `CanTransition(from, to) bool` — 可转换性检查
- `ValidTargets(from) []S` — 合法目标状态枚举

实现 `GenericStateMachine[S, E]`，使用双索引（`fromEventIndex` + `fromToIndex`）O(1) 查找。构造后不可变，天然并发安全。

已迁移的状态机：

| 状态机 | 文件 | 状态数 | 事件数 | 转换规则 | 终态 |
|--------|------|--------|--------|---------|------|
| RunStateMachine | `run_state_machine.go` | 6 | 6 | 8 | Completed / Failed / Cancelled |
| SessionRunPhaseMachine | `session_run_phase_machine.go` | 6 | 5 | 11 | Completed / Failed / Cancelled |
| TeamRunStateMachine | `team_run_state_machine.go` | 6 | 6 | 10 | Success / Failed / Cancelled |
| TeamStateMachine | `team_state_machine.go` | 7+1 虚拟 | 7 | 12 | Archived |
| GraphExecStateMachine | `graph_execution_state_machine.go` | 5 | 5 | 7 | Completed / Failed / Cancelled |
| ChannelTurnJobStateMachine | `channel_turn_job_state_machine.go` | 8 | 11 | 15 | Completed / Failed / Timeout / Cancelled |
| EvolutionStateMachine | `evolution_state_machine.go` | 4 | 3 | 3 | Applied / Rejected / RolledBack |
| AgentStateMachine | `agent_state_machine.go` | 3 | 3 | 4 | Archived |

注：TeamStateMachine 中的 `TeamStateBlocked` 为虚拟状态，仅用于级联阻塞结果，不持久化，无转换规则。

### D3: 事件可靠性三级分级（AS-EVT-01）

| 级别 | 事件类型 | 可靠性保证 | 持久化 |
|------|---------|-----------|--------|
| Critical | ToolResult / Error / RunnerCompletion / Checkpoint | WBPF（先写后发）+ 重试 | SQLite WAL |
| Important | StateDelta / TokenUsage / RunStatus / SessionStatusChanged / GraphNodeEnd / TeamRunFinished / UserFeedback | BlockUpTo + 异步持久化 | SQLite EventStore |
| Informational | TextDelta / FlowLog / Log / MemberDelta | 尽力而为 | 不持久化 |

**EventWAL 实现**：
- Critical 事件先写入 `event_wal` SQLite 表，发布成功后**同步**标记 `published=1`（避免崩溃时 WAL 条目被重复发布）
- 启动时 `Recover` 未发布事件，使用 `EventStoreExistChecker`（仅含 `Exists` 方法）做幂等校验
- `PurgePublished` 清理已发布且超过 TTL（默认 7 天）的条目
- 定时清理任务 `EventWALCleanup`（1 小时间隔，可通过 `EVENT_WAL_CLEANUP_DISABLED` 禁用）

**事件分级定义**：`contract.ClassifyEventReliability` 为 single source of truth（`Stability:stable`），`event` 包有兼容包装层（`Stability:evolving`，标记 Deprecated），通过 type alias + delegation 委托到 contract 包。

**集成状态**：
- ✅ EventWAL 已集成到生产发布路径：`Infra.Publish` 对 Critical 事件走 `WriteBeforePublish`，WAL 不可用时降级为直接发布
- ✅ Wire 注入已包含 EventWAL provider：`InfraProviderSet` 包含 `ProvideEventWAL`
- ✅ 启动时 `Recover` 已接入：`app.go` 的 `BeforeStart` 中在 Bus 和 subscribers 就绪后调用 `WAL.Recover`
- ✅ `EventWALCleanup` 已接入 Wire 和启动生命周期：`AfterStart` 中启动定时清理
- ✅ Bus 层 `criticalTypeSet` 已替换为 `contract.RequiresBlockUpTo()`，与 AS-EVT-01 三级分类对齐
- ✅ biz 层 `isCriticalEnvelopeType` 已删除，替换为 `contract.RequiresBlockUpTo()`
- ✅ contract/event 两包重复定义已消除：event 包改为 type alias + delegation 兼容层

**遗留项**：
- `WAL.Recover` 的 `EventStoreExistChecker` 参数暂传 nil（subscriber 已有幂等处理），后续需接入 EventStore 适配器

### D4: 架构 Fitness Function（AS-FIT-01）

实现 5 个自动化测试（`internal/archlint/archlint_test.go`）：

| 测试 | 验证内容 | 失败行为 |
|------|---------|---------|
| TestBizNotDependOnTrpcAgentGo | biz 层不依赖 `trpc-agent-go` | Error（阻断） |
| TestServiceNotDirectlyAccessData | service 层不直接访问 data 层 | Error（阻断） |
| TestBizPortInterfaceMethodCount | biz port 接口方法 ≤ 5 | Log（警告） |
| TestStateMachineCoverage | >3 状态实体有显式状态机 | Error（阻断） |
| TestStructFieldCount | struct 字段 ≤ 15 | Log（警告） |

**状态机覆盖检查**已扩展到 7 个实体：Run、SessionRunPhase、TeamRun、GraphExecution、Team、ChannelTurnJob、Session。

**集成状态**：✅ Makefile 已添加 `archlint` target（`make archlint`）。

## 后果

### 正面
- ChatOrchestrator 字段从 ~30 降至 12，sync.Map 归零，认知复杂度大幅降低
- 状态转换有编译期校验，非法转换在运行时被拦截；8 个实体已有显式状态机
- Critical 事件 WBPF 机制已实现并集成，可保证至少一次投递
- 架构不变量有 5 个自动化测试守护

### 负面
- 子管理器增加了间接层，调试时需要跨文件跳转（chatRunManager 组合 4 个子管理器）
- EventWAL 增加了 SQLite 写入开销（仅 Critical 事件，已集成）
- 状态机泛型约束 `~string` 限制了未来扩展到整数状态枚举

## 替代方案

1. **不拆分，仅添加注释**：无法解决认知复杂度问题，违反 AS-COG-01
2. **使用有限状态机库（如 github.com/looplab/fsm）**：引入外部依赖，且不支持泛型守卫条件
3. **EventBus 全量持久化**：性能开销过大，TextDelta/FlowLog 等高频事件不适合持久化
4. **使用代码生成替代泛型状态机**：增加构建复杂度，泛型方案更简洁

## 代码核对记录（2026-06-12）

### D1 核对结果：全部一致

| 验证项 | 文档声称 | 代码实际 | 状态 |
|-------|---------|---------|------|
| ChatOrchestrator 字段数 | 12 | 12 | 一致 |
| 5 个子管理器接口 | 全部存在 | 全部存在 | 一致 |
| sync.Map 消除 | 已消除 | 已消除 | 一致 |
| TypedSyncMap[K,V] 泛型封装 | 已引入 | 已引入（含 TTL+Sweep） | 一致 |
| chatRunManager 组合接口 | 4 合 1 | 4 合 1 | 一致 |
| chatTurnLifecycle 组合接口 | 3 合 1 | 3 合 1 | 一致 |
| 4 个 TypedSyncMap 实例 | 分布在子管理器中 | 4 个全部存在 | 一致 |

### D2 核对结果：1 项数据过时已修正，2 项遗漏已补充

| 验证项 | 修正前 | 修正后 | 状态 |
|-------|-------|-------|------|
| SessionRunPhaseMachine | 5 状态/4 事件/8 转换 | 6 状态/5 事件/11 转换 | 已修正 |
| EvolutionStateMachine | 未记录 | 4 状态/3 事件/3 转换 | 已补充 |
| AgentStateMachine | 未记录 | 3 状态/3 事件/4 转换 | 已补充 |
| 其余 5 个状态机 | — | — | 一致 |

### D3 核对结果：5 项 TECH-DEBT 已全部修复

| # | TECH-DEBT | 修复前状态 | 修复后状态 | 修复方式 |
|---|-----------|-----------|-----------|---------|
| 1 | `Infra.Publish` 直接调用 `bus.Publish()`，未走 `WriteBeforePublish` | 仍存在 | ✅ 已修复 | 代码已更新：`Publish` 对 WAL 可用时走 `WriteBeforePublish` |
| 2 | Wire 注入中无 EventWAL provider | 仍存在 | ✅ 已修复 | `InfraProviderSet` 包含 `ProvideEventWAL`；`NewInfra` 接收 `*EventWAL` |
| 3 | Bus 层 `criticalTypeSet` 旧版 13 种类型未对齐 AS-EVT-01 | 仍存在 | ✅ 已修复 | 删除 `criticalTypeSet`，替换为 `contract.RequiresBlockUpTo()` |
| 4 | biz 层 `isCriticalEnvelopeType` 将 StateDelta/TokenUsage 归为 Critical | 仍存在 | ✅ 已修复 | 删除 `isCriticalEnvelopeType`，替换为 `contract.RequiresBlockUpTo()` |
| 5 | contract/event 两包事件可靠性定义重复 | 仍存在 | ✅ 已修复 | event 包改为 type alias + delegation 兼容层，标记 Deprecated |

额外修复：
- `EventWAL.Recover` 启动恢复已接入 `app.go` 的 `BeforeStart`
- `EventWALCleanup` 已接入 Wire 和 `AfterStart` 启动生命周期
- `RequiresBlockUpTo` 新增单元测试覆盖

### D4 核对结果：TECH-DEBT 已修复

| # | TECH-DEBT | 修复前状态 | 修复后状态 | 修复方式 |
|---|-----------|-----------|-----------|---------|
| 1 | Makefile 缺少 `archlint` target | 仍存在 | ✅ 已修复 | 添加 `make archlint` target |

### 剩余问题汇总

原 6 项 TECH-DEBT 已全部修复。当前仅剩 1 项改进项：

1. **`WAL.Recover` 的 `EventStoreExistChecker` 参数暂传 nil**：subscriber 已有幂等处理，后续需接入 EventStore 适配器做更精确的幂等校验
