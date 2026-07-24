# Aranea-Agents 模块开发交叉参考手册（完整版）

> **定位**：AI 开发某个模块时，快速定位所有关联模块、共享契约和变更影响面。
> **与蓝图的关系**：蓝图描述"模块是什么"，本手册描述"改模块 X 时必须注意谁"。
> **编码规范**：详见 SKILLs，本文聚焦**跨模块关联**。
> **与精简版的关系**：本文件是 `module-cross-reference.md` 的扩展版本，新增日志架构相关模块卡片（§1.12a–1.12h）。

---

## 使用方法

1. 找到你要开发的模块卡片
2. 检查 **上游依赖**：你调用了谁的接口？改了签名谁会断？
3. 检查 **下游影响**：谁依赖你的接口？改了谁会崩？
4. 检查 **共享契约**：你修改的类型/事件是否被其他模块消费？
5. 检查 **事件契约**：你新增/修改的事件类型是否在消费方处理？
6. 检查 **前端对应**：后端改动是否需要前端同步？

---

## 一、后端模块开发上下文卡片

### 1.1 Agent 构建 (`internal/agent/`)

**职责**：将 biz.Agent + 依赖构建为 trpcagent.Agent，创建 Runner，执行 Turn。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（Agent/Tool/Memory/Skill 类型）、`provider`（LLM 模型）、`tools`（工具装配）、`skill/trpc`（技能过滤）、`plugin/trpc`（插件回调）、`knowledge`（知识检索）、`event`（日志）、`session`（会话服务）、`memory/trpc`（记忆服务，含 `MemoryService trpcmemory.Service` 注入 `Service.Tools()` 统一路径） |
| **下游影响** | `service/chat`（ChatOrchestrator 调用 BuildTRPCAgent）、`team`（BuildTeamMemberAgents 调用 BuildTRPCAgent）、`a2a`（A2A invoker 构建 Agent） |
| **核心导出** | `BuildTRPCLLMAgent()`、`NewTRPCRunner()`、`RunTRPCUserTurn()`、`TRPCBuilderDeps`、`TRPCRunnerDeps` |
| **实现接口** | 无（直接被 Service 层调用，不实现 biz 端口） |
| **共享类型** | `TRPCBuilderDeps`（6 组子依赖 DTO，被 service/chat 和 team 共享；`TRPCMemoryKnowledgeDeps` 含 `MemoryService trpcmemory.Service`） |
| **事件生产** | 不直接生产事件（Runner 产出的事件由 Service 层投递到 EventBus） |
| **事件消费** | 无 |
| **数据库** | 无直接访问（通过 biz Repo 间接访问） |
| **前端对应** | ChatPage（对话执行）、AgentSettingsPage（Agent 配置影响构建） |

**⚠️ 开发注意**：
- 修改 `TRPCBuilderDeps` 结构体时，必须同步更新 `service/chat_wire.go` 的 `provideChatServiceDeps` 和 `team/trpc_build.go`
- 修改 Prompt 层级（L1-L4）时，影响所有 Agent 的系统提示词，需回归测试 Chat + Team
- 新增 `llmagent.Option` 时，检查 `BuildTRPCLLMAgent` 是否已传递该选项

---

### 1.2 工具装配 (`internal/tools/`)

**职责**：工具注册中心 + Assemble 装配。28 个注册工具 + ~37 个运行时注入工具 + AgentTool + MCP + Custom。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（Tool/Agent 类型）、`pkg/trpc-agent-go/tool`（框架工具 API） |
| **下游影响** | `agent`（BuildTRPCAgent 调用 Assemble 获取工具列表）、`service/chat`（工具策略解析）、`team`（Team 成员工具装配） |
| **核心导出** | `Registry()`、`Assemble()`、`AssemblyConfig`、`ToolRegistration`、`builtin_tools_seed.go` |
| **共享类型** | `AssemblyConfig`（被 agent 和 service/chat 共享，含 `MemoryTools []Tool` 字段）、`AssembledToolsets` |
| **事件生产** | 无 |
| **事件消费** | 无 |
| **数据库** | 无直接访问（工具配置通过 biz ToolUsecase 获取） |
| **前端对应** | ToolsPage（工具目录）、AgentSettingsPage（工具策略配置） |

**⚠️ 开发注意**：
- 新增工具必须先在 `Registry()` 注册 `ToolRegistration` + `builtin_tools_seed.go` 添加种子
- Chat 和 Team 共用同一 `BuildToolsets` 逻辑，新增工具必须验证两处生效
- 修改 `AssemblyConfig` 结构体时，同步更新 `tools/trpc/toolsets.go` 的适配层（含 `MemoryTools` 字段和 `ToolsetConfig.MemoryTools`）

---

### 1.3 LLM Provider (`internal/provider/`)

**职责**：LLM 模型工厂，将 biz Provider 配置转换为 trpcmodel.Model。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（LlmProviderModelUsecase/Repo 类型）、`event`（日志） |
| **下游影响** | `agent`（BuildTRPCAgent 调用 TRPCModelForProviderModel）、`runtime`（providers.go） |
| **核心导出** | `TRPCModelForProviderModel()`、`Catalog`、`WrapModelWithMetrics()` |
| **共享类型** | 无（返回框架 `trpcmodel.Model` 接口） |
| **事件生产** | 无 |
| **事件消费** | 无 |
| **数据库** | 无直接访问（通过 biz LlmProviderModelUsecase 获取配置） |
| **前端对应** | ResourceManagerPage（Provider/Model 管理） |

**⚠️ 开发注意**：
- 新增 Provider 类型时，需在 `trpc_llm.go` 的 `MapProviderType` 添加映射
- HA 策略（Failover/Hedge）配置在 `LlmProviderModelUsecase` 的 ProviderModelConfig 中
- Provider 连接问题不会导致编译错误，但会在运行时影响所有使用该 Provider 的 Agent

---

### 1.4 记忆服务 (`internal/memory/` + `internal/service/memory*.go` + `internal/data/memory_shim_*.go` + `internal/tools/working_memory/`)

**职责**：5 层记忆系统适配器，提供记忆 CRUD + 6 个框架记忆工具 + 5 个 working_memory 工具 + 自动提取 + Memory 管理 API 传输桥点。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（Memory 类型 + `MemoryDebugRecaller`/`MemoryFactIndexCounter` 端口）、`pkg/trpc-agent-go/memory`（框架记忆 API）、`data`（`memoryDebugRecallAdapter`/`memoryFactIndexCounterAdapter` + `memory_shim_*` L0–L4 Store 实现） |
| **下游影响** | `agent`（MemoryService.Tools() 注入记忆工具，统一路径：`Service.Tools()` → 过滤 → `AssemblyConfig.MemoryTools`）、`agent`（working_memory BeforeToolHook 注入 L1TaskWriter/L1FieldWriter/L1AdminReader）、`service/chat`（记忆管理 API）、`service/memory`（L4 级联管理 + Debug Recall + Worker Status） |
| **核心导出** | `memtrpc.NewMemoryService(...)`（L3FactReader/Writer + settingsLoader）、`Service.Tools()`、`Service.EnqueueAutoMemoryJob()`、`service.NewMemoryService()`（Admin API，含 `debugRecaller`/`factIndexCounter`）、`working_memory.ToolSet`/`Tools()`（5 个 L1 工具） |
| **共享类型** | `trpcmemory.Service` 接口（被 agent 和 service 共享）、`biz.RecallDebugRow`/`biz.RecallScoreBreakdown`（debug recall DTO）、`biz.L1TaskInsert`/`biz.L1FieldInsert`（L1 写入 DTO） |
| **事件生产** | 无直接生产（记忆提取通过 EventBus 异步触发） |
| **事件消费** | 记忆提取 Worker 消费 `runner_completion` 事件 |
| **数据库** | SQLite（memory_facts/memory_entities/memory_l4_graph/memory_episodes/memory_l1_tasks/memory_l1_fields/memory_l1_field_history）+ PostgreSQL（embedding 向量） |
| **前端对应** | MemoryCenterPage（5 层记忆浏览）、AgentSettingsPage（记忆策略配置） |

**⚠️ 开发注意**：
- 记忆写入必须经 broker/async 异步写（红线 #3），禁止在 plugin 回调中直接写库
- 记忆工具通过 `Service.Tools()` 注入（统一路径），不手动构造，不使用 `memorytool.DefaultTools()`
- **working_memory 工具**通过 `BeforeToolHook` 注入 L1 依赖（L1TaskWriter/L1FieldWriter/L1AdminReader/sessionID/agentID），不在工具构造时传入
- 修改记忆层级结构时，需同步更新前端 MemoryCenterPage 的 5 个 Tab
- **Service 层禁止直接依赖 data 层 store 具体类型**（原 `sessionmemory.Store` 已折叠为 `internal/data/memory_shim_*.go`），需通过 biz 端口接口（`MemoryDebugRecaller`/`MemoryFactIndexCounter`/`L3FactWriter` 等）+ data 层适配器桥接
- Memory Admin RPC 须经 `authorizeMemoryScope`（`internal/service/memory_scope.go`）做 scope/workspace ACL；trpc `Add/Update/Delete/Clear` 须经 `assertL3WriteAllowed` 尊重 `WriteL3Facts`
- `L4CascadeUsecase` 构造函数接收 4 个子接口（`CascadeProposalStore`/`CascadeGraphReader`/`CascadeFactMutator`/`CascadeSagaStore`）+ `L4EntityWriter`，不使用聚合接口 `CascadeGraphStore`（已 Deprecated）
- `SessionAdminStore` 是向后兼容的组合接口（38 方法），新代码应依赖细粒度子接口（`L1TaskWriter`/`L1FieldWriter`/`L1AdminReader`/`L2ConsolidationStore`/`L3ConflictStore`/`PIIReviewStore`/`L4EntityStore`/`L4EvolutionStore` 等）
- 新增 biz 端口接口时，需同步创建 data 层适配器 + 更新 `cmd/admin/wire.go` 绑定
- L1→L2 桥接：`EndL1Task` 自动归档 + 创建 L2 Episode（`archiveAndCreateEpisode`），L1 Archive Worker 定时扫描空闲任务
- L2 Consolidation：Episode 有 pending/consolidated 状态，`MemoryL2ConsolidateWorker` 负责 pending→consolidated 转换
- L3 冲突检测：`DetectFactConflicts` 启发式否定词检测 + `IncrementConflictCount`，best-effort 日志
- L3 PII 审核：`PIIReviewStore` 提供 list/approve/reject API

---

### 1.5 会话存储 (`internal/session/`)

**职责**：适配 trpcsession.Service，提供会话快照读写。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（Session 类型）、`pkg/trpc-agent-go/session`（框架会话 API） |
| **下游影响** | `agent`（NewTRPCRunner 注入 SessionService）、`service/chat`（会话持久化） |
| **核心导出** | `SQLiteSessionService`、`Service` 接口实现 |
| **共享类型** | `trpcsession.Service` 接口 |
| **事件生产** | 无 |
| **事件消费** | 无 |
| **数据库** | SQLite（sessions/messages/session_turns/session_runs） |
| **前端对应** | SessionsPage、ChatPage（会话列表 + 消息展示） |

---

### 1.5b 会话状态监控 (`internal/biz/session/status*.go` + `internal/service/session_status_guard.go`)

**职责**：Session 执行状态管理——5 态状态机、删除保护、WS 实时推送、优雅退出/异常恢复。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz/session`（SessionUsecase、SessionRepo）、`event`（ActivityEvent 发布）、`loggateway`（日志） |
| **下游影响** | `service/chat`（ChatOrchestrator 调用 TransitionStatus）、`service/team`（TeamTurnHooks 调用 TransitionStatus）、`cmd/admin/wire.go`（SessionStatusGuard 注册到 Kratos 生命周期） |
| **核心导出** | `SessionStatus`/`SessionStatusReason` 常量、`IsProtectedStatus`、`SessionStatusMachine`、`SessionStatusPublisher`（端口接口）、`SessionStatusTransitioner`（端口接口）、`SessionStatusGuard` |
| **共享类型** | `SessionStatus`（5 枚举值）、`SessionStatusReason`（11 枚举值） |
| **事件生产** | `session.status_changed`（通过 `SessionStatusPublisher` → ActivityEvent / WS） |
| **事件消费** | 无 |
| **数据库** | SQLite sessions 表（status/status_reason/status_changed_at 三列）；**生命周期由 `archived_at`/`deleted_at` 时间戳判断，status 列仅存执行状态** |
| **前端对应** | `SessionStatusBadge.vue`（5 种状态徽章）、`sessionSync.ts`（status_changed 变体，WS → emitSessionMutation → sessionSync 总线）、`useChatInboundSync.ts`（session.status_changed → emitSessionMutation） |

---

### 1.6 Chat 服务 (`internal/service/chat*.go`)

**职责**：Chat 传输桥点 + Runner 装配入口。实现所有 Turn 相关端口接口。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（全部 Turn/Session/Agent/Channel 端口接口 + `SeedVersionRepo`）、`agent`（BuildTRPCAgent/NewTRPCRunner）、`tools`（Assemble）、`provider`（Model）、`runtime`（TurnDeps）、`team`（TeamOrchestrationDeps）、`event`（Infra）、`memory`（MemoryService，`Service.Tools()` → 过滤 → `AssemblyConfig.MemoryTools`）、`session`（SessionService）、`knowledge`（检索）、`plugin/trpc`（Manager）、`skill/trpc`（Filter） |
| **下游影响** | **几乎所有模块**：Channel/Cron/A2A/WS/DurableWorker 通过端口接口依赖 ChatService |
| **核心导出** | `ChatService`（实现 7 个 biz 端口接口）、`ChatOrchestrator`（实现 `biz.TurnExecutor`） |
| **实现接口** | `NativeTurnGateway`、`TurnExecutorGateway`、`TurnRunControlGateway`、`TurnGateway`、`TurnControlGateway`、`DurableResumeGateway`、`A2ARunnerFactory`、`SessionRunDurableEscalator`（L2 关机批量升级 durable）、`TaskResumer`（WS 本地，L3 中断任务续跑） |
| **共享类型** | `TurnInput`（被 Channel/Cron/A2A/WS 共享的传输中立输入） |
| **事件生产** | **全部聊天事件**：text_delta、tool_call、tool_result、runner_completion、context_usage、error、run_status 等 |
| **事件消费** | 会话投影（SessionProjectionAdapter 消费事件持久化消息） |
| **数据库** | 无直接访问（通过 biz Usecase 间接访问） |
| **前端对应** | ChatPage（对话界面 + WS 实时流） |

**⚠️ 开发注意**：
- **最关键的模块**：修改 ChatService 的任何方法签名，可能影响 Channel/Cron/A2A/WS/DurableWorker
- 新增 Turn 入口点时，必须同步更新 `TurnEntryPointConfig` 和 `ChatOrchestrator.ExecuteTurn` 的准入逻辑
- 修改 `TurnInput` 结构体时，所有调用方（Channel/Cron/A2A/WS）都需要同步更新
- 修改 Activity / Monitor 事件形状时，同步前端 `realtime/`（ActivityEvent / MonitorEvent 消费路径）与 `features/chat/`；legacy Envelope 类型已删除（ADR-03）
- 崩溃恢复三层机制：L1 `V2RecoveryRepo.FailOrphanedInFlight`（task→interrupted，其余→failed）、L2 `EscalateAllActiveToDurable`（关机批量升级 durable，SessionStatusGuard 调用）、L3 WS 上行 `resume_task` → `ResumeInterruptedTask`（CAS interrupted→running + 轨迹重跑）；新增终态事件必须走 `CompleteTaskTerminal`（版本以 DB 为准），详见 [1-chat.design.md](./1-chat.design.md) §B.10.16
- 需求澄清门（Clarification Gate，§B.10.18）：Intent Pass 后 `chat_clarify_gate.go` 判定阻塞性歧义 → 发布 orphan clarify Step（awaiting_input，信封含 `original_input`）并挂起 turn（`awaiting_confirmation(reason=clarification)`）；提交端点 `SubmitClarification`（CAS 409）→ `resumeTurnWithClarification` 同 turn 续跑（`resolveResumeInput`：内存 pending 优先，缺失从信封 `original_input` 惰性重建）；自由回复等价路径由 `Execute` 统一入口 `resolveClarificationFreeText` 拦截（回写 `free_text` + 按推荐填充 + 输入重写）；开关 `clarification_enabled` 持久化于 `agent_runtime_settings`（迁移 20261108）；orphan Step 前端由 `TaskCard.vue` 渲染 `ClarifyBlock.vue`

---

### 1.7 Channel 渠道 (`internal/channel/` + `internal/service/channel*.go`)

**职责**：12+ IM 平台适配 + 入站消息处理 + 出站消息投递。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（Channel 类型、NativeTurnGateway/TurnControlGateway 端口、GraphExecutor 端口、CronTriggerGateway 端口）、`event`（Infra） |
| **下游影响** | 无（Channel 是终端消费者，不被其他模块依赖） |
| **核心导出** | `Runner`/`OutboundText`/`InboundHandler` 接口、各平台适配器 |
| **消费接口** | `NativeTurnGateway`（执行 Turn）、`TurnControlGateway`（卡片操作）、`GraphExecutor`（Graph 执行）、`CronTriggerGateway`（触发定时任务） |
| **共享类型** | `InboundEvent`、`OutboundMessage`、`port.Meta`、`biz.TurnInput`（入站 Turn 输入，Phase K 起 ChannelIngress 直接使用） |
| **事件生产** | `channel_inbound`、`channel_outbound`、`channel_delivery` |
| **事件消费** | 无 |
| **数据库** | 通过 biz ChannelUsecase 访问（channels/channel_credentials/channel_deliveries/channel_turn_jobs/channel_peer_sessions/channel_inbound_receipts）；Service 层不直接持有任何 Repo 接口（Phase K 修复 K-09；Phase L 修复 J-10） |
| **前端对应** | ChannelsPage（渠道管理） |

**⚠️ 开发注意**：
- 新增平台适配器时，在 `channel/all/all.go` 注册，实现 `Runner` + `OutboundText` 接口
- Channel 不持有 `*ChatService` 具体类型，通过 `biz.NativeTurnGateway` 端口交互
- ChannelIngress 不 import proto 包（`chatv1`/`cronv1`），入站 Turn 输入使用 `biz.TurnInput`（Phase K 修复 K-06）
- `ChannelService`/`ChannelIngress` 不直接持有任何 biz Repo 接口（`ChannelPeerSessionRepo`/`ChannelInboundReceiptRepo`/`AgentRepository`/`TeamRepository`），全部通过 `ChannelUsecase` Facade 方法访问（Phase K 修复 K-09；Phase L 修复 J-10）
- 入站消息处理链：`ProcessInbound` → 路由匹配 → `ExecuteTurn` → 事件流 → 出站投递

---

### 1.8 Team 多 Agent (`internal/team/`)

**职责**：Team 编排，6 种模式（Sequential/Parallel/Coordinator/CriticLoop/Swarm/Adaptive）。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（Team 类型、AgentIDExistenceChecker 端口）、`agent`（BuildTeamMemberAgents）、`provider`（LLM 模型）、`tools`（工具装配）、`skill/trpc`（技能过滤）、`event`（日志）、`graph/trpc`（Graph 编译路径） |
| **下游影响** | `service/team`（TeamService 调用 Team Runner） |
| **核心导出** | `BuildWorkflowRoot()`、`BuildTeamMemberAgents()`、`BuildTRPCTeam()`（Deprecated 回退） |
| **消费接口** | `AgentIDExistenceChecker`（验证成员 Agent 存在，Wire 绑定到 AgentRepository） |
| **共享类型** | `TeamDefinition`、`TeamRunResult` |
| **事件生产** | `team_run_started`、`team_run_finished`、`team_run_failed`、`team_step_started`、`team_step_finished`、`member_message_start/delta/done`、`team_summary` |
| **事件消费** | 无 |
| **数据库** | 通过 biz TeamUsecase 访问（teams/team_runs/team_run_steps） |
| **前端对应** | TeamsPage、TeamOrchestratePage、TeamRunObservatoryPage |

**⚠️ 开发注意**：
- 当前默认使用 **GraphAgent 编译路径**，原生路径为紧急回退
- 修改 Team 编排逻辑时，需同时验证 Graph 编译路径和原生回退路径
- Team 事件类型（team_run_*、member_message_*）被前端 TeamsPage 和 MonitorPage 消费

---

### 1.9 Graph 图编排 (`internal/graph/`)

**职责**：可视化图定义 + 执行引擎 + HITL + 检查点 + 时间旅行 + 模板系统。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（Graph 类型、GraphBuildConfig/GraphRuntime 端口）、`agent`（Agent 节点构建）、`event`（事件桥接）、`trpc-agent-go/graph`（框架 Graph API） |
| **下游影响** | `service/graph`（GraphService 调用 Builder）、`team`（Graph 编译路径）、`biz/graph.go`（GraphUsecase） |
| **核心导出** | `Builder`、`BuildAndRun()`、`BuildRuntime()`、`Visualize()`、`Validate()`、`ListTemplates()` |
| **实现接口** | `biz.GraphBuilderFactory`（被 GraphUsecase 和 Team 消费） |
| **共享类型** | `GraphDefinition`、`GraphExecution`、`NodeDef`、`EdgeDef`、`ConditionalEdgeDef`、`StateFieldDef`、`Task`、`CheckpointInfo`、`GraphBuildConfig`、`GraphValidationResult` |
| **事件生产** | `graph_node_start`、`graph_node_end`、`graph_node_error`、`graph_step`、`graph_execution_done`、`graph_task_status`、`checkpoint` |
| **事件消费** | 无 |
| **数据库** | 通过 biz GraphUsecase 访问（graph_executions/graph_tasks/graph_task_events） |
| **前端对应** | GraphsPage、GraphEditorPage、GraphRunPage、GraphExecutionsPage |
| **测试覆盖** | biz 层 47 个测试用例（R15-4）：ShouldCreateTaskForNode/ShouldCreateTeamGraphTaskNode/GraphTaskInputFromNode/BuildConfigFromGraphDefinition/compactNodesForVersion/ReadUserTemplateMeta/WriteUserTemplateMeta/upsertGraphStep/evictIfNeeded/ApplyFailurePolicy/ApplyCircuitBreakerPolicy/FinalizeGraphFailurePolicy/parallelBranchNodeIDs/normalizeFailureDefault；service 层和 adapter 层待补 |

**⚠️ 开发注意**：
- Graph 执行引擎同时被 **GraphService**（直接执行）和 **Team**（编译路径）消费
- 修改节点类型时，需同步更新前端 `GraphEditorCanvas` 的节点组件
- HITL 任务节点产生的 `Task` 被前端 `GraphRunPage` 消费
- 前端实时校验（`useGraphLocalValidation`）8 种规则与后端校验合并去重
- `GraphVariablePicker` 支持 `{{nodeId.field}}` 变量引用，修改 `NodeDef` 字段时需同步 VariablePicker

---

### 1.10 Cron 定时任务 (`internal/cronrunner/`)

**职责**：Cron 调度 + 定时触发 Agent Turn。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（Cron 类型、NativeTurnGateway 端口）、`event`（日志） |
| **下游影响** | 无（Cron 是终端消费者） |
| **消费接口** | `NativeTurnGateway`（执行定时 Turn） |
| **共享类型** | 无 |
| **事件生产** | `cron_task_triggered`、`cron_task_completed`、`cron_task_failed` |
| **事件消费** | 无 |
| **数据库** | 通过 biz CronUsecase 访问（cron_tasks/cron_task_runs） |
| **前端对应** | CronTasksPage、CronRunsPage |

---

### 1.11 A2A 协议 (`internal/a2a/`)

**职责**：Agent-to-Agent 通信协议（Agent Card、远程调用、Graph 恢复）。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（A2A 类型、A2ARunnerFactory 端口）、`agent`（Agent 构建）、`graph`（Graph 恢复）、`event`（日志） |
| **下游影响** | `service/a2a`（A2AService 调用 A2A invoker） |
| **消费接口** | `biz.A2ARunnerFactory`（构建 A2A Runner，Wire 绑定到 ChatService） |
| **共享类型** | `AgentCard`、`TaskState`、`Message`（A2A 协议类型） |
| **事件生产** | 无直接生产（通过 A2ARunnerFactory 间接使用 Chat 事件流） |
| **事件消费** | 无 |
| **数据库** | 通过 biz A2AUsecase 访问（a2a_endpoints） |
| **前端对应** | A2APage |

---

### 1.12 事件系统 (`internal/event/` + `internal/biz` EventBus)

**职责**：聊天用 **v2 `biz.EventBus`**（Task/Turn/Step/`system.*` → WS `v2_event`）；监控用 **MonitorEventBus**。v1 ActivityEventBus / ActivityBridge 生产路径已退役（2026-07-16）。

| 维度 | 内容 |
|------|------|
| **上游依赖** | 无（基础设施层，不依赖业务模块） |
| **下游影响** | chat/channel/team/graph/monitor/memory/plugin 依赖 v2 EventBus 和/或 MonitorEventBus |
| **核心导出** | `Infra`（MonitorEventBus）、`biz.EventBus` / EventKind、`MonitorBus`（contract）、`MonitorEvent`、`FlowLog`、`loggateway.Logger` |
| **共享类型** | v2 Event（Task/Turn/Step/Team/Graph/system.*）、`MonitorEvent` |
| **事件生产** | 不生产业务事件（只提供基础设施；投影在 `agent/v2`） |
| **事件消费** | Bus 是传输层；WS 经 `ws_v2_subscriber` 推送 |
| **数据库** | FlowLogRepo；v2 实体表（tasks/turns/steps/…）；`activities` 已 DROP（20261012）；FE 历史走 listStepsV2 |
| **前端对应** | ChatPage（`v2_event` → activityV2Store）、MonitorPage（`monitor_event`） |

**⚠️ 开发注意**：
- 新增聊天事件用 typed v2 EventKind，禁止再引入 `ActivityBridgeEvent` / `activity_event`
- 监控事件走 `MonitorEvent`，与聊天总线分离
- 修改 Sequencer 投递策略时，回归 WS 实时性与终态 WBPF/outbox
- 详见 [1-chat.design.md](./1-chat.design.md) §三、[34-event-system.development.md](./34-event-system.development.md)

---

### 1.12a FlowTracker (`internal/event/flow_tracker.go`)

**职责**：Flow Log 核心发射器，持有 FlowContext + SpanCollector + UsageAggregator，提供 LogStart/LogDone/LogError 等方法。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `event`（Infra 双总线、Buffer、TraceContext）、`event`（FlowContext、SpanCollector、UsageAggregator） |
| **下游影响** | `service/chat`（ChatOrchestrator 创建 FlowTracker 记录 Turn 生命周期）、所有使用 Flow Log 的模块 |
| **核心导出** | `NewFlowTracker()`、`LogStart()`、`LogDone()`、`LogSkip()`、`LogWarn()`、`LogError()`、`LogCritical()`、`Log()`、`FinishRoot()`、`SetOtelRefs()`、`CompleteToolCall()`、`SyncOtelSpanIDs()`、`MetadataJSON()`、`TraceID()`、`RunID()`、`SpanCollector()`、`UsageAggregator()`、`FlowContextState()` |
| **实现接口** | 无（直接被 Service 层使用） |
| **共享类型** | `Pair`（键值对，用于 extra 参数）、`FlowPhase`/`FlowSeverity` 常量 |
| **事件生产** | `EnvelopeTypeFlowLog`（通过 `emit` → Infra.Publish → MonitorBus） |
| **事件消费** | 无 |
| **数据库** | 无直接访问（FlowLog 条目通过 Buffer → FlowLogRepo 持久化） |
| **前端对应** | MonitorPage（Flow Log 展示） |

**⚠️ 开发注意**：
- `FlowTracker` 是 `FlowLog` 的替代品（旧 `FlowLog` 为全局函数式 API，`FlowTracker` 为实例化 API）
- ADR-03 Phase 5 后：`FlowTracker.emit` 直接发布到 `MonitorEventBus`（`contract.MonitorEvent`），不再走 `Infra.Publish` 路由表；`LogError` 不再向 SessionBus 发布 `EnvelopeTypeError`（`shouldPublishFlowChatError`/`flowStepsSkipChatError` 死代码已删除）
- 修改 `emit` 方法签名或 `FlowLogEntry` 结构时，需同步 MonitorPage 的 Flow Log 展示

---

### 1.12b SpanCollector (`internal/event/span_collector.go`)

**职责**：管理 span 树（chat.turn → llm.call / tool.call），为 Usage 元数据提供结构化瀑布图。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `event`（SpanContext、UsageContext、TraceContext） |
| **下游影响** | `FlowTracker`（持有 SpanCollector 实例）、`UsageAggregator`（通过 SpanCollector 记录 LLM/Tool span） |
| **核心导出** | `NewSpanCollector()`、`FinishRoot()`、`CompleteToolCall()`、`OpenToolSpan()`、`HasToolSpan()`、`StartLLMSpan()`、`SyncOtelSpanIDs()`、`MetadataJSON()`、`Spans()` |
| **实现接口** | 无 |
| **共享类型** | `OtelSpanIDSource` 接口（由 OTel hook 实现，提供 LLMSpanOtelID/ToolSpanOtelID） |
| **事件生产** | 无直接生产（span 数据通过 `MetadataJSON()` 供 Usage 记录消费） |
| **事件消费** | 无 |
| **数据库** | 无直接访问（span 数据嵌入 usage.metadata_json） |
| **前端对应** | 无独立前端（span 数据通过 UsageEventsPage 间接展示） |

**⚠️ 开发注意**：
- `StartLLMSpan` 支持合并 token 计数到已打开的 LLM span（`MergeLLMSpanTokens`），避免多轮 LLM 调用产生冗余 span
- `OpenToolSpan`/`HasToolSpan`/`TakeToolSpan` 管理 tool.call span 的生命周期
- `SyncOtelSpanIDs` 将 OTel span ID 关联到对应的 span 行，需 OTel hook 实现 `OtelSpanIDSource` 接口

---

### 1.12c UsageAggregator (`internal/event/usage_aggregator.go`)

**职责**：观察框架事件流，聚合 LLM token 用量和工具调用 span，为 Usage 记录提供元数据。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `event`（SpanCollector、UsageContext、TraceContext）、`trpc-agent-go/event`（框架 Event 类型） |
| **下游影响** | `FlowTracker`（持有 UsageAggregator 实例）、`service/chat`（通过 FlowTracker 间接使用） |
| **核心导出** | `NewUsageAggregator()`、`ObserveFrameworkEvent()`、`SetOtelRefs()`、`SyncOtelSpanIDs()`、`MetadataJSON()` |
| **实现接口** | 无 |
| **共享类型** | 无（消费框架 `trpcevent.Event`，产出嵌入 `metadata_json`） |
| **事件生产** | 无直接生产（聚合数据通过 `MetadataJSON()` 供 Usage 记录消费） |
| **事件消费** | 框架事件流（`ObserveFrameworkEvent` 消费 `trpcevent.Event`） |
| **数据库** | 无直接访问（聚合数据嵌入 usage.metadata_json） |
| **前端对应** | UsageEventsPage（usage 记录展示）；OverviewPage 聚合面消费同一 ledger 的汇总（非本组件直连） |

**⚠️ 开发注意**：
- `ObserveFrameworkEvent` 是框架事件流的核心消费点，从 `ev.Response.Usage` 提取 token 计数，从 `ev.Response.GetToolCallIDs` 提取工具调用
- 修改框架 Event 结构时，需检查 `ObserveFrameworkEvent` 是否仍能正确提取 usage 数据
- `ToolNameFromResponse` 辅助函数从框架 Response 中提取工具名称
- 概览页（`/overview`）由 UsageService + MonitorService + Agent/Team/Platform 多源拼装，详见 `docs/reports/2026-07-16-review-nav-pages-fullstack-audit.md`

---

### 1.12d RuntimeLogAdapter (`internal/adapter/runtime_log.go`)

**职责**：将 trpc-agent-go 运行时日志桥接到 loggateway Pipeline，实现 `agentlog.Logger` 接口。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `pkg/loggateway`（Logger 接口）、`trpc-agent-go/log`（agentlog.Logger 接口）、`zap`（Fatal 日志专用） |
| **下游影响** | `cmd/admin/wire.go`（Wire 注入 RuntimeLogAdapter 到框架运行时） |
| **核心导出** | `NewRuntimeLogAdapter()`、`With()`（immutable 预设字段） |
| **实现接口** | `agentlog.Logger`（编译期检查 `var _ agentlog.Logger = (*RuntimeLogAdapter)(nil)`） |
| **共享类型** | 无 |
| **事件生产** | 无直接生产（日志通过 loggateway Pipeline 异步投递到 Sink） |
| **事件消费** | 无 |
| **数据库** | 无直接访问（日志通过 Pipeline → SinkGroup → Sink 写入） |
| **前端对应** | 无（后端透明，日志通过 MonitorPage 间接展示） |

**⚠️ 开发注意**：
- `Fatal`/`Fatalf` **不走 Pipeline**：直接写 stderr + zap.Logger + `os.Exit(1)`，确保致命错误不丢失
- `With()` 返回新实例（immutable 模式），不修改原始 adapter
- 框架运行时日志通过此适配器自动进入 loggateway Pipeline，无需额外处理
- 修改 `agentlog.Logger` 接口时，需同步更新此适配器

---

### 1.12e SinkGroup (`pkg/logpipeline/sink_group.go`)

**职责**：为 Sink 提供独立 goroutine + channel 缓冲，隔离慢 Sink 对 Pipeline 的影响。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `pkg/logpipeline`（Sink 接口、LogEntry 类型）、`pkg/safego`（goroutine 安全） |
| **下游影响** | `pkg/loggateway`（Pipeline 通过 SinkGroup 管理多个 Sink） |
| **核心导出** | `NewSinkGroup()`、`Emit()`、`Close()`、`Flush()`、`Stats()`、`DropPolicy`（DropNewest/DropBlock） |
| **实现接口** | 无（被 Pipeline 内部使用） |
| **共享类型** | `SinkGroupStats`（Name/Dropped/ChanLen/ChanCap）、`DropPolicy` 枚举 |
| **事件生产** | 无 |
| **事件消费** | 无 |
| **数据库** | 无直接访问（Sink 负责持久化） |
| **前端对应** | 无（后端基础设施） |

**⚠️ 开发注意**：
- `DropNewest`（默认）：缓冲区满时丢弃最新条目，不阻塞调用方
- `DropBlock`：缓冲区满时阻塞调用方，直到有空间
- `Close()` 会取消 context + 关闭 channel + 等待 goroutine 退出 + 关闭 Sink
- Sink.Write panic 会被 recover，不影响 SinkGroup 主循环
- 修改 `Sink` 接口时，需检查所有 SinkGroup 消费方

---

### 1.12f FlowContext (`internal/event/flow_context_state.go`)

**职责**：Flow 步骤计时状态，记录每个 stepID 的开始时间，提供耗时计算。

| 维度 | 内容 |
|------|------|
| **上游依赖** | 无（纯状态对象，仅依赖 stdlib） |
| **下游影响** | `FlowTracker`（持有 FlowContext 实例，调用 RecordStart/TakeTiming） |
| **核心导出** | `NewFlowContext()`、`RecordStart()`、`TakeTiming()` |
| **实现接口** | 无 |
| **共享类型** | `FlowTiming`（DurationMS 字段） |
| **事件生产** | 无 |
| **事件消费** | 无 |
| **数据库** | 无 |
| **前端对应** | 无（timing 数据嵌入 FlowLog Envelope 的 metadata） |

**⚠️ 开发注意**：
- `TakeTiming` 是破坏性读取：返回计时后删除对应 stepID，同一 stepID 只能 Take 一次
- 内部使用 `sync.Mutex` 保护 `timers` map，并发安全
- 修改 `FlowTiming` 结构时，需同步 FlowLog Envelope 的 metadata 解析

---

### 1.12g SpanContext (`internal/event/span_context.go`)

**职责**：Span 树的底层数据结构，管理 span 列表、活跃 span、工具 span 映射、LLM span 合并。

| 维度 | 内容 |
|------|------|
| **上游依赖** | 无（纯数据结构，仅依赖 stdlib） |
| **下游影响** | `SpanCollector`（持有 SpanContext 实例） |
| **核心导出** | `NewSpanContext()`、`StartSpan()`、`EndSpan()`、`EndSpanWithDuration()`、`FinishRoot()`、`OpenToolSpan()`/`TakeToolSpan()`/`HasToolSpan()`、`SetOpenLLMSpan()`/`MergeLLMSpanTokens()`、`RootID()`、`Spans()`、`IterateSpans()` |
| **实现接口** | 无 |
| **共享类型** | 无（span 行为 `map[string]any`，由 SpanCollector 消费） |
| **事件生产** | 无 |
| **事件消费** | 无 |
| **数据库** | 无 |
| **前端对应** | 无（span 数据嵌入 usage.metadata_json） |

**⚠️ 开发注意**：
- 内部使用 `sync.Mutex` 保护所有状态，并发安全
- `IterateSpans` 在锁内调用回调函数，回调可直接修改 span map（非拷贝）
- `rootID` 在首次 `StartSpan("chat.turn", ...)` 时设置
- `openTools` map 维护 `tool_call_id → span_id` 映射，用于工具调用 span 的生命周期管理

---

### 1.12h UsageContext (`internal/event/usage_context.go`)

**职责**：Usage 元数据状态，存储 OTel 关联 ID 和 Turn 开始时间。

| 维度 | 内容 |
|------|------|
| **上游依赖** | 无（纯状态对象，仅依赖 stdlib） |
| **下游影响** | `SpanCollector`（读取 TurnStart/OtelRootID）、`UsageAggregator`（写入 OtelRefs） |
| **核心导出** | `NewUsageContext()`、`SetOtelRefs()`、`OtelTraceID()`、`OtelRootID()`、`TurnStart()` |
| **实现接口** | 无 |
| **共享类型** | 无 |
| **事件生产** | 无 |
| **事件消费** | 无 |
| **数据库** | 无 |
| **前端对应** | 无 |

**⚠️ 开发注意**：
- 内部使用 `sync.Mutex` 保护所有字段，并发安全
- `SetOtelRefs` 存储 OTel trace/span ID，用于 usage.metadata_json 中的关联
- `TurnStart` 在构造时设置为 `time.Now()`，作为 span 瀑布图的时间基准

---

### 1.13 知识库 (`internal/knowledge/`)

**职责**：统一摄取管线（上传 → Extractor 模态路由（文本/多模态）→ LLM 整理为 Markdown → 分块 → Embedding → pgvector → 检索）。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（Knowledge 类型 + LLMCaller）、`provider`（Embedding/LLM 模型） |
| **下游影响** | `agent`（知识注入 Prompt L4 层）、`service/knowledge`（Knowledge API） |
| **核心导出** | `Ingest()`、`Retriever`、`Chunker`、`ExtractorRegistry`、`MarkdownOrganizer`（Phase 8/9 计划新增，见 37-knowledge.development.md） |
| **共享类型** | `Chunk`、`RetrievalResult` |
| **事件生产** | `knowledge_ingest` |
| **事件消费** | 无 |
| **数据库** | 通过 biz KnowledgeUsecase 访问（knowledge_collections/knowledge_documents + pgvector chunks） |
| **前端对应** | KnowledgePage（含拖拽上传区/上传队列/MD 预览，Phase 8 计划新增） |

---

### 1.14 Plugin 插件 (`internal/plugin/`)

**职责**：Plugin 生命周期管理 + 回调链（audit/modify/notify）+ 费用守卫。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（Plugin 类型）、`event`（日志）、`trpc-agent-go/plugin`（框架 Plugin API） |
| **下游影响** | `agent`（PluginManager 注入到 Runner）、`service/plugin`（Plugin API） |
| **核心导出** | `Manager`、`Registry()`、`CostGuard` |
| **共享类型** | `PluginConfig`、`HookResult` |
| **事件生产** | `plugin_hook_audit`、`plugin_hook_modify`、`plugin_hook_notify` |
| **事件消费** | 无 |
| **数据库** | 通过 biz PluginUsecase 访问（plugins/plugin_runs） |
| **前端对应** | PluginsPage、PluginRunsPage |

---

### 1.15 评估系统 (`internal/evaluation/`)

**职责**：LLM Judge 评估框架（数据集 → 运行 → 评分 → 统计）。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（Evaluation 类型）、`agent`（构建评估 Agent） |
| **下游影响** | `service/evaluation`（Evaluation API） |
| **核心导出** | `Runner`、`Scores`、`LLMJudge` |
| **共享类型** | `EvaluationRun`、`EvaluationResult` |
| **事件生产** | 无 |
| **事件消费** | 无 |
| **数据库** | 通过 biz EvalUsecase 访问 |
| **前端对应** | EvaluationPage |

---

### 1.16 Monitor 监控 (`internal/service/monitor*.go` + `internal/biz/monitor/`)

**职责**：Flow Log、Trace 投影、告警规则评估、通知。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（Monitor 类型）、`event`（MonitorBus 消费） |
| **下游影响** | 无（Monitor 是终端消费者） |
| **核心导出** | `MonitorUsecase`、`AlertEvaluator`、`AlertNotifier` |
| **共享类型** | `FlowLogEntry`、`AlertRule`、`TraceEvent` |
| **事件生产** | `alert_notify` |
| **事件消费** | 消费 MonitorBus 的 `flow_log`、`log` 事件；消费 `alert_notify` 触发通知 |
| **数据库** | 通过 biz MonitorUsecase 访问（monitor_events/monitor_traces/monitor_alert_rules） |
| **前端对应** | MonitorPage（日志 + 告警 + Trace） |

---

### 1.17 学习闭环 (`internal/biz/learning_loop.go` + `internal/service/learning_loop.go` + `internal/data/learning_loop.go`)

**职责**：Observation → Pattern → Proposal → Validation → Registration 完整学习闭环，让 Agent 从经验中学习。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（Observation/Pattern/Proposal 类型、ObservationReadWriter/PatternReadWriter/ProposalReadWriter 端口）、`biz/evolution.go`（EvolutionUsecase） |
| **下游影响** | `service/learning_loop`（LearningLoopService 调用 LearningLoopUsecase） |
| **核心导出** | `LearningLoopUsecase`、`Observation`/`Pattern`/`KnowledgeProposal` 领域模型 |
| **实现接口** | 无（通过 Service 层暴露 HTTP/gRPC API） |
| **共享类型** | `Observation`（tool_call/feedback/memory_hit/memory_miss）、`Pattern`（detected/confirmed/dismissed）、`KnowledgeProposal`（draft/pending/approved/rejected/applied） |
| **事件生产** | 无直接生产（闭环由 API 触发或手动 RunLoop） |
| **事件消费** | 无直接消费（未来可消费 `runner_completion` 事件自动触发闭环） |
| **数据库** | SQLite（learning_observations/learning_patterns/learning_proposals，原生 SQL DDL） |
| **前端对应** | AgentLearningLoopPanel（Agent 详情页"学习闭环"Tab）、LearningLoopOverview/LearningPatternList/LearningProposalList 组件 |

**⚠️ 开发注意**：
- 学习闭环与 EvolutionUsecase 协作：EvolutionUsecase 提供进化指标，LearningLoopUsecase 提供模式识别和知识注册
- 修改 `Observation`/`Pattern`/`KnowledgeProposal` 类型时，需同步更新前端 `api.learning.ts` 类型定义
- 前端数据流：`api.learning.ts` → `useAgentLearningLoopPanel` composable → `AgentLearningLoopPanel.vue` + 子组件

---

### 1.18 技能自创建 (`internal/biz/skill_evolution.go` + `internal/service/skill_evolution.go` + `internal/data/skill_evolution.go`)

**职责**：检测 Agent 重复工具调用模式，自动提议创建新 Skill，经审批后注册到 Skill 仓库。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（SkillProposal 类型、SkillProposalReadWriter/SkillAutoCreator/SkillRegistrationPort 端口、AgentRepository、PatternReader） |
| **下游影响** | `service/skill_evolution`（SkillEvolutionService 调用 SkillEvolutionUsecase）、`skill`（注册新 Skill） |
| **核心导出** | `SkillEvolutionUsecase`、`SkillProposal` 领域模型、`DetectAndPropose`/`ApproveProposal`/`RejectProposal`/`RegisterApproved` 方法 |
| **实现接口** | `SkillAutoCreator`（LLM 生成 SKILL.md）、`SkillRegistrationPort`（注册 Skill 到仓库） |
| **共享类型** | `SkillProposal`（pending/approved/rejected/registered/expired）、`ToolCallRecord` |
| **事件生产** | 无直接生产（由 Cron 定时任务或 API 触发） |
| **事件消费** | 无直接消费 |
| **数据库** | SQLite（skill_proposals，原生 SQL DDL） |
| **前端对应** | 待集成（后端 API 已就绪，前端 Skill 进化管理界面待开发） |

**⚠️ 开发注意**：
- `SkillAutoCreator` 接口由 `internal/skill/auto_creator.go` 实现，调用 LLM 生成 SKILL.md
- `SkillRegistrationPort` 接口由 SkillUsecase 适配，将审批通过的 Proposal 注册为正式 Skill
- 定时检测通过 `internal/cronrunner/jobs/skill_evolution.go` 触发
- 前端集成时需在 SkillsPage 或 AgentSettingsPage 新增"技能提议"管理界面

---

### 1.19 技能管家工具 (`internal/tools/skills_butler/`)

**职责**：4 个技能管家核心工具（evolve_skill/optimize_skill/recommend_skills/analyze_skill_usage），让 `__skills__` Agent 主动进化自身技能。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（SkillUsecase/SkillQueryReader/EvolutionMetricsRepo/ToolInvocationReader 端口）、`provider`（LLM 模型，evolve_skill/optimize_skill 需调用 LLM） |
| **下游影响** | `agent`（ChatOrchestrator.skillsButlerTools() 注入到 `__skills__` Agent）、`service/chat`（工具装配） |
| **核心导出** | `RegisterAll(deps Deps) []trpctool.Tool`、`IsSkillsButlerAllowed(agentKey) bool`、4 个工具函数 |
| **实现接口** | 无（工具注册到 tools 包，通过 Assemble 装配） |
| **共享类型** | `Deps`（端口依赖聚合）、`EvolveSkillInput/Output`、`OptimizeSkillInput/Output`、`RecommendSkillsInput/Output`、`AnalyzeSkillUsageInput/Output` |
| **事件生产** | 无直接生产（工具执行结果通过 tool_result 事件返回） |
| **事件消费** | 无 |
| **数据库** | 无直接访问（通过 biz Usecase/Repo 间接访问） |
| **前端对应** | 待集成（工具在 `__skills__` Agent 会话中使用，前端无需独立页面） |

**⚠️ 开发注意**：
- `IsSkillsButlerAllowed` 仅对 `__skills__` Agent 返回 true，其他 Agent 不注入这些工具
- `evolve_skill` 和 `optimize_skill` 需调用 LLM 分析失败模式和生成优化方案，需确保 Provider 可用
- 修改 `Deps` 结构体时，需同步更新 `service/chat` 的 `skillsButlerTools()` 适配器

---

### 1.20 Skill 渐进加载 (`internal/biz/skill_load_mode.go` + `internal/agent/skill_guidance_inject.go` + `internal/agent/trpc_build.go` + `internal/agent/prompt_mode.go`)

**职责**：将 Skill Prompt 注入策略从 Eager 全量注入改为 3 阶段渐进加载（L0 manifest → L1 body → L2 refs），通过 `skill_load_mode=progressive` 配置切换。

**关键文件**：
- `internal/biz/skill_load_mode.go`（模式常量 + 判断函数）
- `internal/biz/agent_settings.go`（GetSkillLoadMode）
- `internal/agent/skill_guidance_inject.go`（progressive guidance hook）
- `internal/agent/trpc_build.go`（构建时注入 RoutedSkills 选项）
- `internal/agent/prompt_mode.go`（Prompt 模式适配）
- `pkg/trpc-agent-go/internal/flow/processor/skills.go`（processor 层 SkillLoadModeProgressive + RoutedSkillsStateKey + WithRoutedSkillsResolver）
- `pkg/trpc-agent-go/agent/llmagent/option.go`（导出 SkillLoadModeProgressive + RoutedSkillsStateKey + WithSkillsRoutedSkillsResolver）
- `pkg/trpc-agent-go/agent/llmagent/llm_agent.go`（选项消费）
- `api/kratos/agent/v1/agent.proto`（skill_load_mode 枚举注释）

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（Agent 类型、SkillLoadMode 配置、`GetSkillLoadMode`）、`pkg/trpc-agent-go/processor/skills`（SkillsRequestProcessor）、`pkg/trpc-agent-go/agent/llmagent`（WithSkillsRoutedSkills 选项） |
| **下游影响** | `agent`（BuildTRPCAgent 根据 skill_load_mode 选择注入策略）、所有使用 Skill 的 Agent |
| **核心导出** | `biz.IsProgressiveSkillLoad(mode) bool`、`biz.SkillLoadModeProgressive = "progressive"`、`processor.SkillLoadModeProgressive = "progressive"`、`llmagent.SkillLoadModeProgressive`（re-export from processor）、`processor.RoutedSkillsStateKey = "processor:skills:routed"`（invocation state key，期望值类型 `[]string`）、`llmagent.RoutedSkillsStateKey`（re-export from processor）、`processor.WithRoutedSkills(names []string)`、`processor.WithRoutedSkillsResolver(func(*agent.Invocation) []string)`、`llmagent.WithSkillsRoutedSkills(names []string)`（优先级：`WithSkillsRoutedSkillsResolver > WithSkillsRoutedSkills > RoutedSkillsStateKey`）、`llmagent.WithSkillsRoutedSkillsResolver(func(*agent.Invocation) []string)`（优先级同上）、`processor.writeSkillOverviewLines`（helper method，处理 `[routed]` 标记渲染）、`newProgressiveSkillGuidanceHook` |
| **实现接口** | 无（修改现有 guidance hook 行为，新增 `newProgressiveSkillGuidanceHook`） |
| **共享类型** | 无新增（复用现有 `skill_load_mode` 字段） |
| **事件生产** | 无 |
| **事件消费** | 无 |
| **数据库** | 无（配置存储在 Agent 的 runtime_settings 中） |
| **前端对应** | AgentSettingsPage（skill_load_mode 选项，需新增 `progressive` 选项） |

**⚠️ 开发注意**：
- progressive 模式下 `newProgressiveSkillGuidanceHook` 替代原有 `newSkillGuidanceBeforeHook`，返回 nil（不注入 guidance），LLM 必须通过 `skill_load` 工具按需获取 Skill 正文
- progressive 模式自动启用 `WithSkillsLoadedContentInToolResults(true)`，避免 loaded body 再次注入 system prompt
- `injectOverview` 已简化为单一线性流程：写 protocol（如有）→ header → roots → skill lines → capability+tooling guidance（如无 protocol），不再有双分支
- `writeSkillOverviewLines` helper method 统一处理 skill 行渲染，包括 `[routed]` 标记
- `ResolveFlags` 降级时通过 `log.Warnf` 记录警告（而非静默降级）
- `RoutedSkillsStateKey` godoc 标注期望值类型：`[]string`
- `WithSkillsRoutedSkills` 和 `WithSkillsRoutedSkillsResolver` godoc 标注优先级：`WithSkillsRoutedSkillsResolver > WithSkillsRoutedSkills > RoutedSkillsStateKey`
- **Routed Skills 解析优先级**：`routedSkillsResolver`（最高）> 静态 `routedSkills` 列表 > `RoutedSkillsStateKey` 从 invocation state 读取（fallback）
- **`RoutedSkillsStateKey`** 是 progressive guidance hook 与 processor 之间的集成点：hook 在 `BeforeModel` 中写入 routed slugs（`inv.SetState(trpcllmagent.RoutedSkillsStateKey, result.Slugs)`），processor 在下一轮 `ProcessRequest` 中读取
- `llmagent.SkillLoadModeProgressive` 和 `llmagent.RoutedSkillsStateKey` 在 `pkg/trpc-agent-go/agent/llmagent/option.go` 中从 processor re-export
- `processor.WithRoutedSkillsResolver(func(*agent.Invocation) []string)` 支持动态解析 routed skills（优先级最高），`llmagent.WithSkillsRoutedSkillsResolver` 为其 llmagent 层包装
- 修改 `skill_load_mode` 枚举时，需同步更新 `api/kratos/agent/v1/agent.proto` 注释
- `SkillLoadModeProgressive` 常量在 `biz`、`processor`、`llmagent` 三层有定义/re-export，修改时需同步

---

### 1.21 Spirit 动态编排 (`internal/biz/spirit_team_usecase.go` + `internal/biz/spirit_synthesis.go` + `internal/biz/spirit_task_dag.go`)

**职责**：动态组装 Team 并行执行任务，综合结果。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（TeamStarterPort 端口、TaskDAG、SynthesisEngine）、`team`（Team 运行） |
| **下游影响** | `service/spirit_synthesis`（SpiritSynthesisService 调用 SpiritTeamUsecase） |
| **核心导出** | `SpiritTeamUsecase`、`SynthesisEngine`（template/prompt/hybrid 三策略）、`TaskDAG`（依赖验证/环检测/拓扑排序） |
| **实现接口** | `biz.TeamStarterPort`（Wire 绑定到 TeamStarter） |
| **共享类型** | `SpiritTeamConfig`、`SynthesisResult`、`TaskDAG`、`DAGNode` |
| **事件生产** | `spirit_team_assembled`、`spirit_team_completed`、`spirit_team_failed`、`spirit_team_progress`、`spirit_teams_all_completed`、`spirit_synthesis_completed` |
| **事件消费** | 无 |
| **数据库** | 无直接访问（通过 biz Usecase 间接访问） |
| **前端对应** | SpiritEntry/TeamAssemblyCard/TeamProgressCard/TaskExecutionPanel 组件；执行总结报告经 `NoticeBlock` 分支渲染 `ExecutionReportCard`（2026-07-22 P-REPORT，取代未接入的 SynthesisResultCard） |

**⚠️ 开发注意**：
- Spirit 模式是 Team 的上层编排，不替代 Team，而是动态创建和调度多个 Team
- 修改 `TeamStarterPort` 接口时，需同步更新 `service/team.go` 的 `TeamStarter` 实现
- 6 种 Spirit EnvelopeType 被前端 `useSpiritTeamStore` 和 `useOrchestrationStore` 消费
- **执行总结报告（B.10.17）**：`synthesisEventPublisher` 将报告以 `StepCreatedEvent`（Kind=notice，`NoticeType=synthesis_completed`，Content=ExecutionReportEnvelope JSON）持久化；存在 cancelled 团队时跳过发布；LLM 结论失败时 `degraded=true` 保留结构化板块；前端 `parseExecutionReport` 解析失败回落默认 notice

---

### 1.22 MCP 服务器管理 (`internal/mcp/`)

**职责**：MCP 服务器全生命周期管理——配置解析、健康检查、告警发布、探针策略。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（MCPServerUsecase/Repo）、`event`（告警发布） |
| **下游影响** | `service/mcp_server`（MCPServerService 调用健康检查）、`tools`（MCP ToolSet 连接） |
| **核心导出** | `config.ParseServerConfigJSON`、`health.Runner`、`alert.Publisher`、`classify.IsMCPToolInvocation`、`probe.Prober` |
| **共享类型** | `ServerConfig`、`TransportConfig`、`TestResult` |
| **事件生产** | `mcp.session.reconnect`、`mcp.health.alert` |
| **事件消费** | 无 |
| **数据库** | 通过 biz MCPServerUsecase 访问（mcp_servers/mcp_user_credentials） |
| **前端对应** | McpServersPage |

---

### 1.23 出站消息路由 (`internal/outbound/`)

**职责**：统一的出站消息路由层，解耦渠道适配器与消息发送逻辑。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `channel`（各平台 OutboundText 适配器）、`service/chat`（SessionResolver 注册、RuntimeState 注入）、`cmd/admin/wire.go`（provideOutboundRouter） |
| **下游影响** | `tools`（message 工具通过 OutboundRouter 发送）、`tools/subagent`（完成通知闭环）、`channel`（出站投递）、`channel/runtime`（Manager 集成） |
| **核心导出** | `Router`、`TextSender`/`MessageSender` 接口、`WrapOutboundText`、`MessageTool`、`ResolveTarget`、`RuntimeStateForTarget`、`RegisterSessionResolver`、`RegisterFromInboundEvent` |
| **共享类型** | `DeliveryTarget`、`OutboundMessage`、`OutboundFile` |
| **事件生产** | 无直接生产（通过 channel 适配器间接发送） |
| **事件消费** | 无 |
| **数据库** | 无直接访问 |
| **前端对应** | ChannelsPage（渠道配置影响路由） |

---

### 1.24 模型注册表 (`internal/modelregistry/`)

**职责**：LLM 模型目录的文件系统存储、远程同步、定价应用、Provider 迁移。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（ModelRegistryUsecase）、`provider`（Provider 信息） |
| **下游影响** | `service/model_catalog`（ModelCatalogService 调用 Store）、`tools/modelsync`（模型同步工具） |
| **核心导出** | `Store`、`Syncer`、`Applier`、`ApplyBackend`、`ProviderMigrationRule` |
| **共享类型** | `Directory`、`Policy`、`SyncLog`、`ProviderMigrationRule` |
| **事件生产** | 无 |
| **事件消费** | 无 |
| **数据库** | 通过 biz LlmProviderModelUsecase 间接访问 |
| **前端对应** | ResourceManagerPage |

---

### 1.25 Agent 演化 (`internal/biz/evolution.go` + `internal/biz/evolution_scan.go`)

**职责**：Agent 演化闭环——指标采集、建议生成/应用/拒绝、自动扫描。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（EvolutionMetricsRepo/EvolutionSuggestionRepo 端口、AgentRepository） |
| **下游影响** | `service/chat`（ChatOrchestrator 调用演化指标记录） |
| **核心导出** | `EvolutionUsecase`、`EvolutionMetrics`、`EvolutionSuggestion` |
| **共享类型** | `EvolutionMetrics`（工具成功率/检索质量）、`EvolutionSuggestion`（persona/prompt/skill 类型） |
| **事件生产** | 无直接生产 |
| **事件消费** | 无 |
| **数据库** | SQLite（evolution_metrics/evolution_suggestions，原生 SQL DDL） |
| **前端对应** | AgentEvolutionPanel（Agent 详情页"演化"Tab） |

---

### 1.26 工作空间 (`internal/workspace/`)

**职责**：多租户 workspace ID 的 context 传播。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `pkg/auth`（JWT `workspace_id` claim）；`server/middleware.WorkspaceFilter` |
| **下游影响** | `data`（Ent hooks 过滤）、`server`（中间件注入）、`biz`/`service`（AssertWorkspace） |
| **核心导出** | `WithContext`、`FromContext`、`IDFromContext`、`WithSystemWorkspace`、`IsSystem`、`AssertWorkspace` |
| **共享类型** | `SystemWorkspaceID = "__system__"`、`DefaultWorkspaceID = "default"` |
| **事件生产** | 无 |
| **事件消费** | 无 |
| **数据库** | `admins.workspace_id`（B-01 P2-A：1:1 admin→workspace）；其余实体逐步补 `workspace_id` |
| **前端对应** | 无（后端透明；Header `X-Workspace-ID` 对已登录用户不可伪造） |

**⚠️ 开发注意（B-01）**：已认证请求的 workspace **只**来自 JWT；与 Header/Query 不一致时返回 403。Admin 切租户需换带目标 `workspace_id` 的会话，不能靠 Header。

---

### 1.27 Agent 资源共享 / M71 (`internal/biz/resource_access.go` + `dept_mailbox.go` + `session_search.go` + `internal/service/member_fs.go` + `mailbox_waker.go` + `internal/tools/{memberfs,deptmail,sessionaccess}/`)

**职责**：受控资源访问层——memberfs（部门主管只读员工工作目录）+ deptmail（主管间信箱 + Turn 唤醒）+ sessionaccess（精灵只读检索会话内容）。统一 权限校验 → 范围解析 → 审计落库（fail-closed）。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（AgentRepository/OrganizationReader/DeptTeamLister/SessionReader+Writer）、`internal/workspace`（租户目录解析）、`data`（DeptLeadMessageRepo/ResourceAccessAuditRepo/GlobalMessageSearchRepo） |
| **下游影响** | `service/chat_orchestrator.go`（CustomToolFunc 按 agent 身份装配 10 个工具）；`service/chat_wire.go`（MailboxWaker.SetTurnGateway setter 注入破环） |
| **核心导出** | `ResourceAccessUsecase`（ListMemberFiles/ReadMemberFile/SearchMemberFiles）、`DeptMailboxUsecase`（Send/ListInbox/Read/Reply + 5min 唤醒防抖）、`SessionSearchUsecase`（SearchMessages/ListAgentSessions/ReadSessionHistory + 20/min 令牌桶）、`IsDeptLeadAgent`、`MailboxWaker` 端口 |
| **共享类型** | `DeptLeadMessage`、`AuditEntry`（Result: allowed/denied；Relation: org_home/team_owner；ActorRole: dept_lead/spirit）、`GlobalMessageHit`、`FileEntry`、`SessionMeta`/`SessionMessageView` |
| **事件生产** | 唤醒经 `TurnExecutorGateway.ExecuteTurn` → 复用 Turn 事件流 |
| **事件消费** | 无 |
| **数据库** | SQLite（dept_lead_messages / resource_access_audits，Ent Schema + Indexes()）；全局检索走 steps_v2 `content LIKE`（messages_fts 已于 20260902 移除） |
| **前端对应** | 无独立 UI；唤醒 Turn 与工具调用以常规 chat activity 呈现在主管/精灵会话时间线 |

**⚠️ 开发注意（M71）**：
1. 权限 fail-closed：审计写失败 = 访问拒绝；Auditor 为 nil = 拒绝。
2. dept_lead 身份双判：`AgentVariant=="dept_lead"` 或 AgentKey `__dept_lead_*__` 前后缀；借调可见性经 `Team.CrossDeptMemberIDs`（archived/deleted team 不算）。
3. 路径安全：service 层 `secureJoin`（拒绝对路径/`..`/符号链接逃逸）+ 二进制嗅探 + UTF-8 校验 + 200KB 截断；隐藏文件与符号链接不进 List/Search。
4. 唤醒防抖键为「发送方→接收方」对；唤醒失败不阻塞消息落库（NFR-05）。
5. Wire 环规避：`MailboxWaker` 不接收 TurnExecutorGateway 构造参数，由 `ProvideChatService` 后调 `SetTurnGateway`（同 TeamStarter 模式）。

---

## 二、前端模块开发上下文卡片

### 2.1 Chat 域（最复杂的前端域）

**涉及文件**：`features/chat/api.ts`、`stores/chat/`（含 `activityV2Store`）、`components/chat/v2/`、`realtime/`

| 维度 | 内容 |
|------|------|
| **Store 拆分** | `useChatSessionStore`、`useChatMessageStore`（revision/pending 等非主渲染）、`useChatRuntimeStore`、`useChatActivityStore`（**主渲染**）、`useChatConversationStore` |
| **跨 Store 通信** | `sessionSync` 事件总线；AppStore → ChatSessionStore |
| **WS 事件消费** | **仅** `v2_event`：`task.*` / `turn.*` / `step.*` / `team_*` / `graph_*` / `system.*` |
| **后端对应** | ChatService（HTTP + WS `/v1/ws`）+ v2 REST hydrate |
| **共享类型** | `v2Types.ts`（Task/Turn/Step…）、`RunStatus`、`ChatOption` |

**⚠️ 开发注意**：
- 聊天渲染只走 SessionPanelV2；禁止 legacy message / activity_event 时间线回落
- 排序用 `Seq` + 时间戳，禁止 `turn_index` 推算
- 展示组件经 `useActivityQueries`，禁止直连 Pinia/API

---

### 2.2 Agent 域

**涉及文件**：`features/agents/api.ts`、`stores/agents/`（3 个 Store）、`components/agents/`（25 组件）

| 维度 | 内容 |
|------|------|
| **Store 拆分** | `useAppStore`（全局 Agent 列表 + 选中）、`useAgentsPageStore`（列表页筛选）、`useAgentDetailStore`（设置页详情） |
| **跨 Store 通信** | AppStore → ChatSessionStore/ChatMessageStore（Agent 切换重置）、AgentsPageStore → AppStore（upsertAgent） |
| **后端对应** | AgentService（CRUD）+ ChatService（对话） |
| **共享类型** | `Agent`、`AgentRuntimeSettings`、`AgentPromptFile` |

---

### 2.3 Graph 域

**涉及文件**：`features/graph/api.ts`、`stores/graph/`、`components/graph/`（22 组件）

| 维度 | 内容 |
|------|------|
| **Store** | `useGraphStore`（图列表 + 编辑 + 执行 + 检查点 + 任务 + 模板） |
| **Composable** | `useGraphEditorPage`（编辑器编排 + 合并校验）、`useGraphRunPage`（运行态编排）、`useGraphExecutionsPage`（执行历史 + 服务端过滤）、`useGraphLocalValidation`（8 种前端实时校验，区分 error/warning）、`useGraphUndoRedo`（22 种命令双栈撤销重做）、`useSnapGuide`（节点拖拽对齐）、`useConditionalRoutes`（条件边路由）、`useGraphExecutionStream`（WS 事件流）、`useGraphRunTasks`（任务看板）、`useGraphRunHitl`（HITL 交互）、`useGraphTimeTravel`（检查点时间旅行） |
| **WS 事件消费** | graph_node_start/end/error、graph_step、graph_execution_done、graph_task_status、checkpoint |
| **后端对应** | GraphService（CRUD + 执行 + 任务 + 检查点 + 模板） |
| **共享类型** | `GraphDefinition`、`GraphExecution`、`NodeDef`、`EdgeDef`、`ConditionalEdgeDef`、`StateFieldDef`、`Task`、`CheckpointInfo`、`ValidationError`、`ValidationWarning` |

**⚠️ 开发注意**：
- Graph 编辑器使用 Vue Flow 库，节点组件在 `components/graph/`
- 修改后端 `NodeDef` 类型时，需同步前端节点组件的 props
- 前端实时校验（`useGraphLocalValidation`）与后端校验结果在 `useGraphEditorPage` 中合并去重（key=`code:nodeId:field`）
- `GraphVariablePicker` 支持 `{{nodeId.field}}` 变量引用插入，集成在 `GraphPropertyPanel` instruction 字段
- `GraphCheckpointPanel` 提供状态快照预览 + 回退确认，通过 emit 上抛 restore 操作

---

### 2.4 Team 域

**涉及文件**：`features/teams/api.ts`、`stores/teams/`、`components/teams/`（9 组件）

| 维度 | 内容 |
|------|------|
| **Store 拆分** | `useTeamsStore`（列表）、`useTeamsPageStore`（页面状态） |
| **WS 事件消费** | team_run_started/finished/failed、team_step_started/finished、member_message_*、team_summary |
| **后端对应** | TeamService（CRUD + 运行） |
| **共享类型** | `Team`、`TeamRun`、`TeamRunStep` |

---

### 2.5 Monitor 域

**涉及文件**：`features/monitor/api.ts`、`stores/monitor/`、`components/monitor/`（17 组件）

| 维度 | 内容 |
|------|------|
| **Store** | `useMonitorStore`（日志 + 告警 + Trace + 运行时指标） |
| **WS 事件消费** | flow_log、log、alert_notify、mcp_health_alert（通过 globalWsHub session_id=*） |
| **后端对应** | MonitorService + EventService |
| **跨 Store 依赖** | `listChannels`（从 channels/api 获取告警渠道选项） |

---

### 2.6 实时通信层 (`realtime/`)

**涉及文件**：`ws-transport.ts`、`globalWsHub.ts`、Activity/Monitor 流消费 hooks（legacy `useEnvelopeStream` / `envelope.ts` 已按 ADR-03 退役或收窄）

| 维度 | 内容 |
|------|------|
| **核心导出** | WS 传输、全局 hub、`v2_event` / `monitor_event` 分发 |
| **消费方** | Chat（v2 会话流）、Monitor（日志流）、Teams/Graph/Orchestration（typed v2 或 system.notice） |
| **后端对应** | WSServer + `biz.EventBus`（v2）+ `contract.MonitorEventBus` |

**⚠️ 开发注意**：
- 聊天业务事件走 typed v2 EventKind；监控走 `MonitorEvent`。禁止新增 `activity_event` / bridge。
- `globalWsHub` 引用计数：`acquireGlobalWsConsumer`/`releaseGlobalWsConsumer` 必须配对
- WS 重连 hydrate 用 v2 REST（`/v2/sessions/...`），不走 EventBuffer replay

---

## 三、跨模块变更影响速查表

### 3.1 修改 biz 层端口接口时

| 修改的接口 | 影响的模块 | 需要同步更新 |
|-----------|-----------|-------------|
| `TurnExecutorGateway` | WSServer | `internal/server/ws.go` 本地 `WSTurnExecutor` 类型 + `cmd/admin/wire.go` `wsTurnExecutorAdapter` 适配 |
| `TurnRunControlGateway` | DurableWorker | `internal/service/session_run_durable_worker.go` + `cmd/admin/wire.go` |
| `TurnControlGateway` | ChannelIngress | `internal/service/channel_ingress.go` |
| `NativeTurnGateway` | Channel/Cron | `internal/service/channel_ingress.go` + `internal/service/cron.go` |
| `DurableResumeGateway` | DurableWorker | `internal/service/session_run_durable_worker.go` |
| `A2ARunnerFactory` | A2AService | `internal/service/a2a_endpoint.go` |
| `CronTriggerGateway` | ChannelIngress | `internal/service/channel_ingress.go` |
| `GraphExecutor` | Channel/Cron | `internal/service/channel_ingress.go` + `internal/service/cron.go` |
| `GraphBuilderFactory` | GraphUsecase + Team | `internal/biz/graph.go` + `internal/team/` |
| `AgentIDExistenceChecker` | TeamUsecase | `internal/biz/team_usecase.go` |
| `MemoryDebugRecaller` | MemoryService | `internal/service/memory_recall.go` + `internal/data/memory_debug_recall.go` 适配器 + `cmd/admin/wire.go` |
| `MemoryFactIndexCounter` | MemoryService | `internal/service/memory_recall.go` + `internal/data/memory_debug_recall.go` 适配器 + `cmd/admin/wire.go` |
| `CascadeProposalStore`/`CascadeGraphReader`/`CascadeFactMutator`/`CascadeSagaStore` | L4CascadeUsecase | `internal/biz/memory_l4_cascade.go` + `cmd/admin/wire.go` |
| `L4EntityWriter` | L4CascadeUsecase | `internal/biz/memory_l4_cascade.go` + `cmd/admin/wire.go` |
| `SessionStatusPublisher` | SessionUsecase | `internal/service/run_status_publish.go`（sessionStatusPublisher 适配器）+ `cmd/admin/wire.go`（WireSessionStatusPublisher） |
| `SessionStatusTransitioner` | SessionUsecase | `internal/biz/session/usecase.go`（SessionUsecase 自身实现）+ `internal/service/chat_orchestrator*.go`/`team_turn_hooks.go`（调用方） |
| `SeedVersionRepo` | IndustryAgentSeed | `internal/biz/seed_version.go`（接口定义）+ `internal/data/seed_version_repo.go`（实现）+ `cmd/admin/wire.go`（绑定） |
| `TeamStarterPort` | SpiritTeamUsecase | `internal/service/team.go`（TeamStarter 实现）+ `cmd/admin/wire.go` |
| `ToolResultGate` | ChatOrchestrator | `internal/service/chat_orchestrator*.go` + `internal/biz/tool_result_gate.go` |
| `MemoryTextExtractor` | MemoryService | `internal/service/memory_recall.go`（MemoryLLMExtractor）+ `cmd/admin/wire.go` |
| `SkillEmbedder` | KnowledgeService | `internal/knowledge/embedder.go` + `cmd/admin/wire.go` |

### 3.2 修改共享类型时

| 修改的类型 | 定义位置 | 影响的模块 |
|-----------|---------|-----------|
| `TurnInput` | `biz/turn_input.go` | ChatService、ChannelIngress、CronService、A2AService、WSServer（通过 `WSTurnInput` 本地类型 + adapter 转换） |
| `TurnResult` | `biz/turn_input.go` | ChatOrchestrator、ChannelIngress |
| `NativeTurnResult` | `biz/native_turn_result.go` | ChatOrchestrator、ChannelIngress |
| ~~`Envelope` / `EnvelopeType`~~ | legacy（ADR-03 后收窄） | 新代码用 `ActivityEvent` / `MonitorEvent`；前端勿再扩 `EnvelopeType` |
| `Agent` | `biz/agent_types.go` | AgentUsecase、ChatOrchestrator、Team、A2A、前端 |
| `Tool` | `biz/tool/tool.go` | ToolUsecase、ChatOrchestrator、前端 |
| `GraphDefinition` | `biz/graph.go` | GraphUsecase、GraphService、Team、前端 |
| `SessionStatus` | `biz/session/status.go` | SessionUsecase、SessionRepo、ChatOrchestrator、TeamTurnHooks、SessionStatusGuard、前端 `types.ts` + `SessionStatusBadge.vue` |
| `SessionStatusReason` | `biz/session/status.go` | SessionUsecase、ChatOrchestrator、TeamTurnHooks、SessionStatusGuard、前端 `SessionStatusBadge.vue` |
| `TeamDefinition` | `biz/team_types.go` | TeamUsecase、TeamService、前端 |
| `RecallDebugRow`/`RecallScoreBreakdown` | `biz/memory_debug_recall.go` | MemoryService、data 适配器、前端（debug recall 面板） |

### 3.3 修改事件类型时

| 新增/修改的事件 | 生产者 | 消费者 | 前端处理 |
|---------------|--------|--------|---------|
| `text_delta` | ChatOrchestrator | WS 推送、SessionProjection | ChatMessageStore 追加流式文本 |
| `tool_call` / `tool_result` | ChatOrchestrator | WS 推送、SessionProjection | ChatMessageStore 显示工具步骤 |
| `runner_completion` | ChatOrchestrator | WS 推送、MemoryWorker | ChatRuntimeStore 触发 loadMessages |
| `graph_node_*` | Graph Builder | WS 推送 | GraphStore 更新节点状态 |
| `team_run_*` | Team Runner | WS 推送 | TeamsStore 更新运行状态 |
| `alert_notify` | Monitor | WS 推送 + Channel 通知 | InboundNotificationStore |
| `flow_log` | 任意模块 | MonitorBus → FlowLogRepo | MonitorPage 日志展示 |
| `session.status_changed` | SessionStatusPublisher（service 层） | WS 推送 | SessionStore.patchSessionStatus + SessionStatusBadge |

### 3.4 修改数据库 Schema 时

| 修改的表 | Ent Schema | 影响的 Repo | 影响的 Usecase | 影响的前端 |
|---------|-----------|------------|---------------|-----------|
| agents | ✅ | AgentRepo | AgentUsecase | AgentsPage |
| sessions | ✅ | SessionRepo | SessionUsecase | ChatPage/SessionsPage（+ SessionStatusBadge） |
| messages | ✅ | SessionRepo | SessionUsecase | ChatPage |
| teams | ✅ | TeamRepo | TeamUsecase | TeamsPage |
| tools | ✅ | ToolRepo | ToolUsecase | ToolsPage |
| channels | ✅ | ChannelRepo | ChannelUsecase | ChannelsPage |
| cron_tasks | ✅ | CronRepo | CronUsecase | CronTasksPage |
| hooks | ✅ | HookRepo | HookUsecase | HooksPage |
| plugins | ✅ | PluginRepo | PluginUsecase | PluginsPage |
| memory_* | 原生 SQL | MemoryRepo | MemoryUsecase | MemoryCenterPage |
| graph_* | 原生 SQL | GraphRepo | GraphUsecase | GraphsPage |
| flow_log_events | 原生 SQL | FlowLogRepo | MonitorUsecase | MonitorPage |
| learning_observations / learning_patterns / learning_proposals | 原生 SQL | LearningLoopRepo | LearningLoopUsecase | AgentSettingsPage（学习闭环 Tab） |
| skill_proposals | 原生 SQL | SkillEvolutionRepo | SkillEvolutionUsecase | 待集成 |

---

## 四、前后端对齐速查

| 后端 Service | Proto 包 | 前端 Service 工厂 | 前端 Store | 前端页面 |
|-------------|---------|-----------------|-----------|---------|
| ChatService | chat/v1 | createChatService | useChatSessionStore + useChatMessageStore + useChatRuntimeStore + useChatConversationStore | ChatPage |
| AgentService | agent/v1 | createAgentService | useAppStore + useAgentsPageStore + useAgentDetailStore | AgentsPage + AgentSettingsPage；OverviewPage（Hero active/total 经 ListAgents.total） |
| TeamService | team/v1 | createTeamService | useTeamsStore + useTeamsPageStore | TeamsPage + TeamOrchestratePage；OverviewPage（Team 计数，当前全量 ListTeams.length） |
| GraphService | graph/v1 | createGraphService | useGraphStore | GraphsPage + GraphEditorPage + GraphRunPage + GraphExecutionsPage |
| SessionService | session/v1 | createSessionService | useSessionStore | SessionsPage |
| ChannelService | channel/v1 | createChannelService | useChannelsStore | ChannelsPage |
| ToolService | tool/v1 | createToolService | useToolsStore + useToolDetailStore + useToolEditorStore | ToolsPage |
| CronService | cron/v1 | createCronService | useCronStore | CronTasksPage |
| HookService | hook/v1 | createHookService | useHooksStore | HooksPage |
| PluginService | plugin/v1 | createPluginService | usePluginsStore | PluginsPage |
| SkillService | skill/v1 | createSkillService | useSkillsStore | SkillsPage |
| MemoryService | memory/v1 | createMemoryService | useMemoryStore | MemoryCenterPage |
| KnowledgeService | knowledge/v1 | createKnowledgeService | useKnowledgeStore | KnowledgePage |
| MonitorService | monitor/v1 | createMonitorService | useMonitorStore | MonitorPage（权威运行时面）；OverviewPage（RunnerMetrics 快捷条，同 GetRunnerMetrics） |
| A2AService | a2a/v1 | createA2AService | useA2AStore | A2APage |
| LlmProviderModelService | llm_provider_model/v1 | createLlmProviderModelService | usePlatformStore | ResourceManagerPage；OverviewPage（Provider 目录健康） |
| UsageService | usage/v1 | createUsageService | useUsageStore | OverviewPage（GetUsageOverview / trends / all-models-breakdown 主表面）；UsageEventsPage（事件账本） |
| EvaluationService | evaluation/v1 | createEvaluationService | useEvaluationStore | EvaluationPage |
| EcosystemService | ecosystem/v1 | createEcosystemService | useEcosystemStore | EcosystemPage |
| ArtifactService | artifact/v1 | createArtifactService | useArtifactStore | ArtifactsPage |
| MCPServerService | mcp_server/v1 | createMCPServerService | useMcpStore | McpServersPage |
| SystemSettingService | system_setting/v1 | createSystemSettingService | useSystemSettingsStore | SystemSettingsPage |
| ModelCatalogService | model_catalog/v1 | createModelCatalogService | usePlatformStore | ResourceManagerPage |
| AdminService | admin/v1 | createAdminService | useAuthStore | LoginPage |
| LearningLoopService | learning_loop/v1 | createLearningLoopService | useLearningLoopStore + useAgentDetailStore（学习闭环 Tab） | AgentSettingsPage（学习闭环 Tab） |
| SkillEvolutionService | skill_evolution/v1 | createSkillEvolutionService | 待集成 | 待集成 |
| SpiritSynthesisService | spirit/v1 | createSpiritService | useSpiritTeamStore | — |
| TaxonomyService | taxonomy/v1 | createTaxonomyService | — | — |
| FlowLogService | monitor/v1 | createMonitorService | useMonitorStore | MonitorPage |
| CodeExecutorService | monitor/v1 | createMonitorService | useMonitorStore | MonitorPage |
| OpenAICompatService | — | — | — | — |
| PersistentTurnService | — | — | — | — |

> **Overview（`/overview`）多源拼装**：主表面为 `UsageService`（概览/趋势/全模型分解）；辅助 KPI 来自 `AgentService` / `TeamService` / `LlmProviderModelService` / Organization 树；Runner 面板复用 `MonitorService.GetRunnerMetrics`。权威运维面仍为 `MonitorPage`。全页评审见 `docs/reports/2026-07-16-review-nav-pages-fullstack-audit.md`。

---

## 五、开发场景速查

### 场景 1：新增一个 LLM Provider

| 步骤 | 模块 | 文件 |
|------|------|------|
| 1. 添加 Provider 类型映射 | provider | `internal/provider/trpc_llm.go` MapProviderType |
| 2. 实现框架 Model 接口 | trpc-agent-go | `pkg/trpc-agent-go/model/<provider>/` |
| 3. 添加 HA 选项 | provider | `internal/provider/trpc_llm.go` buildProviderOptions |
| 4. 前端添加选项 | 前端 | `web/src/config/chatOptions.ts` CHAT_MODEL_PROVIDER_OPTIONS |
| 5. 验证 | 全栈 | 后端 `go build` + 前端 `pnpm build` |

**关联模块**：provider → agent → service/chat → 前端 ChatPage

### 场景 2：新增一个内置工具

| 步骤 | 模块 | 文件 |
|------|------|------|
| 1. 注册 ToolRegistration | tools | `internal/tools/registry.go` Registry() |
| 2. 添加种子数据 | tools | `internal/tools/builtin_tools_seed.go` |
| 3. 实现 Assemble 覆盖（如需配置） | tools | `internal/tools/toolset.go` Assemble() |
| 4. 前端工具目录展示 | 前端 | ToolsPage 自动展示（从后端拉取） |
| 5. 验证 Chat + Team | 全栈 | 两种编排模式都使用 BuildToolsets |

**关联模块**：tools → agent → service/chat + team → 前端 ToolsPage/AgentSettingsPage

### 场景 3：新增一个 Channel 平台

| 步骤 | 模块 | 文件 |
|------|------|------|
| 1. 实现适配器 | channel | `internal/channel/<platform>/`（Runner + OutboundText） |
| 2. 注册到 All | channel | `internal/channel/all/all.go` |
| 3. 添加渠道 Catalog 图标 | biz | `internal/biz/channelicons/` |
| 4. 前端渠道类型选项 | 前端 | ChannelsPage 自动展示（从后端拉取 Catalog） |
| 5. 验证入站+出站 | 全栈 | Webhook → ProcessInbound → ExecuteTurn → SendText |

**关联模块**：channel → biz(NativeTurnGateway) → service/chat → event → 前端 ChannelsPage

### 场景 4：新增一个 Envelope 事件类型

| 步骤 | 模块 | 文件 |
|------|------|------|
| 1. 添加 EnvelopeType 常量 | event | `internal/event/envelope.go` |
| 2. 生产者发布事件 | 对应 Service | 在事件流循环中 Infra.Publish() |
| 3. 消费者处理事件 | 对应 Consumer | Bus.Subscribe() 或 WS 推送 |
| 4. 前端类型定义 | 前端 | `web/src/realtime/envelope.ts` |
| 5. 前端事件处理 | 前端 | 对应 Store/Composable 的 dispatcher.onType() |

**关联模块**：event → 生产者 Service → 消费者 → 前端 realtime → 前端 Store

### 场景 5：修改 TurnInput 结构体

| 步骤 | 模块 | 文件 |
|------|------|------|
| 1. 修改 biz 类型定义 | biz | `internal/biz/turn_input.go` TurnInput |
| 2. 更新所有调用方 | service | ChatService、ChannelIngress、CronService、A2AService |
| 3. 更新 WS 协议 | server | `internal/server/ws.go` 本地 `WSTurnInput`/`WSTurnOptions` 类型 + `cmd/admin/wire.go` `wsTurnExecutorAdapter` |
| 4. 更新前端 WS 发送 | 前端 | ChatComposer 的 WS message 构建 |
| 5. 验证所有入口 | 全栈 | HTTP Chat + WS Chat + Channel + Cron + A2A |

**关联模块**：biz → service/chat + channel + cron + a2a + server/ws → 前端 ChatPage

### 场景 6：修改 Session 状态枚举

| 步骤 | 模块 | 文件 |
|------|------|------|
| 1. 修改 SessionStatus/SessionStatusReason 常量 | biz | `internal/biz/session/status.go` |
| 2. 更新状态机合法转换 | biz | `internal/biz/session/status_machine.go` + `_test.go` |
| 3. 更新 IsProtectedStatus | biz | `internal/biz/session/status.go` |
| 4. 更新 SessionUsecase 转换逻辑 | biz | `internal/biz/session/usecase.go` + `recovery.go` |
| 5. 更新 SessionRepo 状态查询/写入 | data | `internal/data/session_repo.go` + `session_repo_batch.go` |
| 6. 更新 Ent Schema 默认值/字段 | data | `internal/data/ent/schema/session.go` → `go generate` |
| 7. 更新 ChatOrchestrator 调用点 | service | `internal/service/chat_orchestrator*.go` |
| 8. 更新 TeamTurnHooks 调用点 | service | `internal/service/team_turn_hooks.go` |
| 9. 更新 SessionStatusGuard | service | `internal/service/session_status_guard.go` |
| 10. 更新 SessionStatusPublisher | service | `internal/service/run_status_publish.go` |
| 11. 更新 Proto 定义 | api | `api/kratos/session/v1/session.proto` → `make api` |
| 12. 更新前端类型 | 前端 | `web/src/features/session/types.ts` |
| 13. 更新 SessionStatusBadge | 前端 | `web/src/components/sessions/SessionStatusBadge.vue` |
| 14. 更新删除保护 UI | 前端 | `web/src/components/sessions/SessionsTableSection.vue` |
| 15. 验证全链路 | 全栈 | 状态机测试 + 编译 + 前端 lint + E2E |

**关联模块**：biz/session → data → service/chat + team → event → api → 前端 types + SessionStatusBadge + sessionSync
