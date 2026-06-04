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
- HA 策略（Failover/Hedge）配置在 `LlmProviderModelUsecase` 的 CatalogConfig 中
- Provider 连接问题不会导致编译错误，但会在运行时影响所有使用该 Provider 的 Agent

---

### 1.4 记忆服务 (`internal/memory/` + `internal/service/memory*.go` + `internal/data/sessionmemory/` + `internal/tools/working_memory/`)

**职责**：5 层记忆系统适配器，提供记忆 CRUD + 6 个框架记忆工具 + 5 个 working_memory 工具 + 自动提取 + Memory 管理 API 传输桥点。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（Memory 类型 + `MemoryDebugRecaller`/`MemoryFactIndexCounter` 端口）、`pkg/trpc-agent-go/memory`（框架记忆 API）、`data`（`memoryDebugRecallAdapter`/`memoryFactIndexCounterAdapter` 适配器）、`data/sessionmemory`（L0-L4 Store 实现） |
| **下游影响** | `agent`（MemoryService.Tools() 注入记忆工具，统一路径：`Service.Tools()` → 过滤 → `AssemblyConfig.MemoryTools`）、`agent`（working_memory BeforeToolHook 注入 L1TaskWriter/L1FieldWriter/L1AdminReader）、`service/chat`（记忆管理 API）、`service/memory`（L4 级联管理 + Debug Recall + Worker Status） |
| **核心导出** | `NewSQLiteMemoryService()`、`Service.Tools()`、`Service.EnqueueAutoMemoryJob()`、`NewMemoryService()`（含 `debugRecaller`/`factIndexCounter` biz 端口）、`working_memory.ToolSet`/`Tools()`（5 个 L1 工具） |
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
- **Service 层禁止直接依赖 `*sessionmemory.Store`**（data 层类型），需通过 biz 端口接口（`MemoryDebugRecaller`/`MemoryFactIndexCounter`）+ data 层适配器桥接
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
| **上游依赖** | `biz/session`（SessionUsecase、SessionRepo）、`event`（Envelope 发布）、`loggateway`（日志） |
| **下游影响** | `service/chat`（ChatOrchestrator 调用 TransitionStatus）、`service/team`（TeamTurnHooks 调用 TransitionStatus）、`cmd/admin/wire.go`（SessionStatusGuard 注册到 Kratos 生命周期） |
| **核心导出** | `SessionStatus`/`SessionStatusReason` 常量、`IsProtectedStatus`、`SessionStatusMachine`、`SessionStatusPublisher`（端口接口）、`SessionStatusTransitioner`（端口接口）、`SessionStatusGuard` |
| **共享类型** | `SessionStatus`（5 枚举值）、`SessionStatusReason`（11 枚举值） |
| **事件生产** | `session.status_changed`（通过 `SessionStatusPublisher` → WS Envelope） |
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
| **实现接口** | `NativeTurnGateway`、`TurnExecutorGateway`、`TurnRunControlGateway`、`TurnGateway`、`TurnControlGateway`、`DurableResumeGateway`、`A2ARunnerFactory` |
| **共享类型** | `TurnInput`（被 Channel/Cron/A2A/WS 共享的传输中立输入） |
| **事件生产** | **全部聊天事件**：text_delta、tool_call、tool_result、runner_completion、context_usage、error、run_status 等 |
| **事件消费** | 会话投影（SessionProjectionAdapter 消费事件持久化消息） |
| **数据库** | 无直接访问（通过 biz Usecase 间接访问） |
| **前端对应** | ChatPage（对话界面 + WS 实时流） |

**⚠️ 开发注意**：
- **最关键的模块**：修改 ChatService 的任何方法签名，可能影响 Channel/Cron/A2A/WS/DurableWorker
- 新增 Turn 入口点时，必须同步更新 `TurnEntryPointConfig` 和 `ChatOrchestrator.ExecuteTurn` 的准入逻辑
- 修改 `TurnInput` 结构体时，所有调用方（Channel/Cron/A2A/WS）都需要同步更新
- 修改事件类型时，前端 `realtime/envelope.ts` 和 `features/chat/` 的流处理器需要同步

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

### 1.12 事件系统 (`internal/event/`)

**职责**：双总线（SessionBus + MonitorBus）发布/订阅，事件缓冲，Flow Log。

| 维度 | 内容 |
|------|------|
| **上游依赖** | 无（基础设施层，不依赖业务模块） |
| **下游影响** | **所有模块**：chat/channel/team/graph/monitor/memory/plugin 都依赖 EventBus |
| **核心导出** | `Infra`（双总线）、`Bus` 接口、`Buffer`、`Envelope`、`EnvelopeType`（30+ 事件类型常量）、`FlowLog`、`SysLog*`/`SessionSysLog*`（⚠️ 已废弃，迁移至 `pkg/loggateway.Logger`） |
| **共享类型** | `Envelope`（所有事件消费者共享）、`EnvelopeType`（事件类型枚举） |
| **事件生产** | 不生产业务事件（只提供基础设施） |
| **事件消费** | 不消费业务事件（Bus 本身是传输层） |
| **数据库** | FlowLogRepo（flow_log_events 表） |
| **前端对应** | MonitorPage（Flow Log 展示）、ChatPage（WS 事件流） |

**⚠️ 开发注意**：
- 新增 `EnvelopeType` 常量时，必须同步更新前端 `realtime/envelope.ts` 的类型定义
- 修改 `Envelope` 结构体时，影响所有事件消费者（chat/session/monitor/plugin）
- 修改 Bus 投递策略时，需回归测试 WS 推送的实时性

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
- `LogError` 会自动判断是否向 SessionBus 发布 `EnvelopeTypeError`（`shouldPublishFlowChatError` 白名单过滤 monitor-only 错误）
- `flowStepsSkipChatError` 列表中的 stepID 不会作为 chat toast 展示
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
| **前端对应** | UsageEventsPage（usage 记录展示） |

**⚠️ 开发注意**：
- `ObserveFrameworkEvent` 是框架事件流的核心消费点，从 `ev.Response.Usage` 提取 token 计数，从 `ev.Response.GetToolCallIDs` 提取工具调用
- 修改框架 Event 结构时，需检查 `ObserveFrameworkEvent` 是否仍能正确提取 usage 数据
- `ToolNameFromResponse` 辅助函数从框架 Response 中提取工具名称

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

**职责**：文档摄入管线（上传 → OCR → 分块 → Embedding → pgvector → 检索）。

| 维度 | 内容 |
|------|------|
| **上游依赖** | `biz`（Knowledge 类型）、`provider`（Embedding 模型） |
| **下游影响** | `agent`（知识注入 Prompt L4 层）、`service/knowledge`（Knowledge API） |
| **核心导出** | `Ingest()`、`Retriever`、`Chunker` |
| **共享类型** | `Chunk`、`RetrievalResult` |
| **事件生产** | `knowledge_ingest` |
| **事件消费** | 无 |
| **数据库** | 通过 biz KnowledgeUsecase 访问（knowledge_bases/knowledge_documents + pgvector chunks） |
| **前端对应** | KnowledgePage |

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
| **前端对应** | SpiritEntry/SynthesisResultCard/TeamAssemblyCard/TeamProgressCard/TaskExecutionPanel 组件 |

**⚠️ 开发注意**：
- Spirit 模式是 Team 的上层编排，不替代 Team，而是动态创建和调度多个 Team
- 修改 `TeamStarterPort` 接口时，需同步更新 `service/team.go` 的 `TeamStarter` 实现
- 6 种 Spirit EnvelopeType 被前端 `useSpiritTeamStore` 和 `useOrchestrationStore` 消费

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
| **上游依赖** | 无（基础设施） |
| **下游影响** | `data`（Ent hooks 过滤）、`server`（中间件注入）、`biz`（后台任务传播） |
| **核心导出** | `WithContext`、`FromContext`、`IDFromContext`、`WithSystemWorkspace`、`IsSystem` |
| **共享类型** | `SystemWorkspaceID = "__system__"`、`DefaultWorkspaceID = "default"` |
| **事件生产** | 无 |
| **事件消费** | 无 |
| **数据库** | 无 |
| **前端对应** | 无（后端透明） |

---

### 1.27 Auto Fix Engine (`.github/workflows/auto-fix.yml` + `.auto-fix/`)

**职责**：CI 失败自动检测 → LLM 诊断 → 修复生成 → 验证 → PR 创建，形成失败自动修复闭环。

| 维度 | 内容 |
|------|------|
| **上游依赖** | CI Pipeline（`workflow_run` 失败事件触发）、Lint System（lint 规则 + fix 命令参考，用于生成修复代码） |
| **下游影响** | `.auto-fix/` 目录（失败模式知识库，存储历史失败模式与修复策略）、`.github/workflows/auto-fix.yml`（工作流定义） |
| **核心导出** | 自动修复 PR（含修复代码 + 验证结果）、失败模式知识库条目 |
| **实现接口** | 无（GitHub Actions 工作流，非 Go 代码） |
| **共享类型** | `.auto-fix/patterns.json`（失败模式知识库格式：pattern/solution/confidence） |
| **事件生产** | `workflow_run`（修复 PR 的 CI 运行结果） |
| **事件消费** | CI Pipeline 的 `workflow_run` 事件（失败时触发） |
| **数据库** | 无（知识库存储在 `.auto-fix/` 目录的 JSON 文件中） |
| **前端对应** | 无（CI/CD 基础设施，通过 GitHub PR 界面交互） |

**⚠️ 开发注意**：
- 修复 PR 必须通过完整 CI 才能合并，避免引入新问题
- 单次修复超时 10 分钟，防止 LLM 陷入无限循环
- 每日最多 5 次自动修复，避免频繁创建低质量 PR
- 新增失败模式时，需在 `.auto-fix/patterns.json` 中添加模式条目（pattern + solution + confidence）
- 修改 CI Pipeline Job 名称时，需同步更新 auto-fix 的失败检测逻辑

---

### 1.28 Auto Release Pipeline (`.github/workflows/release.yml` + `.goreleaser.yml`)

**职责**：GoReleaser 驱动的自动化构建/发布/变更日志生成流水线。

| 维度 | 内容 |
|------|------|
| **上游依赖** | CI Pipeline（CI 门禁通过后才允许发布，通过 `workflow_run` 事件触发） |
| **下游影响** | `.goreleaser.yml`（GoReleaser 发布配置：构建目标/镜像/Changelog）、`.github/workflows/release.yml`（发布工作流定义）、`Dockerfile`（容器构建定义） |
| **核心导出** | GitHub Release（含多平台二进制 + Docker 镜像 + Changelog） |
| **实现接口** | 无（GitHub Actions 工作流） |
| **共享类型** | `.goreleaser.yml` 配置格式、Conventional Commits（Changelog 自动生成的 commit 规范） |
| **事件生产** | `release`（GitHub Release 发布事件，被 Iteration Dashboard 消费） |
| **事件消费** | CI Pipeline 的 `workflow_run` 成功事件（门禁通过） |
| **数据库** | 无 |
| **前端对应** | 无（通过 GitHub Releases 页面交互） |

**⚠️ 开发注意**：
- 发布前必须通过完整 CI + Staging 冒烟测试
- Tag 格式必须为 `v*`（如 `v1.2.3`），GoReleaser 根据 Tag 触发
- Changelog 从 Conventional Commits 自动生成，commit message 必须符合规范
- 修改 `.goreleaser.yml` 时，需验证 `goreleaser check` 通过
- 修改 `Dockerfile` 时，需同步验证多架构构建（linux/amd64 + linux/arm64）

---

### 1.29 Doc Sync Engine (`.github/workflows/doc-sync.yml` + `openspec/changelog/`)

**职责**：代码变更 → 影响分析 → 文档自动更新 → PR 创建，保持文档与代码同步。

| 维度 | 内容 |
|------|------|
| **上游依赖** | CI Pipeline（PR 合并事件触发，`push` to main/develop） |
| **下游影响** | `.github/workflows/doc-sync.yml`（工作流定义）、`openspec/changelog/`（变更日志目录，按版本/日期组织）、`openspec/specs/`（架构规格文档，可能被自动更新） |
| **核心导出** | 文档同步 PR（含 LLM 生成的文档更新） |
| **实现接口** | 无（GitHub Actions 工作流） |
| **共享类型** | `openspec/changelog/` 目录结构（`YYYY-MM-DD-title.md` 格式）、`openspec/specs/` 文档格式 |
| **事件生产** | `push`（文档同步 PR 的合并事件） |
| **事件消费** | CI Pipeline 的 `push`/`merge` 事件（代码变更触发） |
| **数据库** | 无 |
| **前端对应** | 无（通过 GitHub PR 界面交互） |

**⚠️ 开发注意**：
- 文档同步 PR 需人工审核后合并，不自动合并
- 仅更新与代码变更直接相关的文档段落，避免过度修改
- 修改 `openspec/specs/` 文档结构时，需同步更新 Doc Sync Engine 的文档路径映射
- 修改 `openspec/changelog/` 目录结构时，需同步更新 Changelog 生成模板
- LLM 生成文档更新时，保留原文风格和格式

---

### 1.30 E2E Testing (`web/e2e/` + `.github/workflows/e2e-nightly.yml`)

**职责**：Playwright E2E 测试框架，覆盖关键用户路径，确保全栈功能正确性。

| 维度 | 内容 |
|------|------|
| **上游依赖** | Web 前端（测试目标，`web/` 目录下的 Vue 3 应用）、后端服务（API 依赖，需运行 `cmd/admin`） |
| **下游影响** | `web/e2e/`（测试用例目录，含 Playwright spec 文件 + fixtures + helpers）、`.github/workflows/e2e-nightly.yml`（定时工作流定义） |
| **核心导出** | E2E 测试报告（HTML/JSON 格式）、测试失败告警 |
| **实现接口** | 无（测试基础设施） |
| **共享类型** | Playwright 配置（`web/playwright.config.ts`）、测试 fixtures（`web/e2e/fixtures/`） |
| **事件生产** | `workflow_run`（E2E 测试运行结果） |
| **事件消费** | 无 |
| **数据库** | 无（测试数据使用独立测试数据库，运行后清理） |
| **前端对应** | 无（测试基础设施，测试目标即为前端本身） |

**⚠️ 开发注意**：
- E2E 测试不阻塞 PR 合并（仅 nightly 报告），但关键路径失败触发告警
- 新增页面/功能时，需同步添加 E2E 测试覆盖
- 修改前端路由时，需同步更新 E2E 测试的页面导航
- 修改 API 接口时，需同步更新 E2E 测试的 API 调用
- Playwright 测试需独立测试环境（独立数据库 + 端口），避免影响开发环境
- 测试数据通过 fixtures 管理，运行后自动清理

---

### 1.31 Iteration Dashboard (`.github/workflows/iteration-dashboard.yml`)

**职责**：迭代指标采集与周报自动生成，量化自迭代引擎运行效果。

| 维度 | 内容 |
|------|------|
| **上游依赖** | Auto Fix Engine（修复统计数据：成功率/响应时间/修复模式分布）、Auto Release Pipeline（发布统计数据：频率/耗时/版本号） |
| **下游影响** | `.github/workflows/iteration-dashboard.yml`（工作流定义） |
| **核心导出** | 迭代周报（Markdown 格式，发布到 GitHub Issue / 飞书文档） |
| **实现接口** | 无（GitHub Actions 工作流） |
| **共享类型** | 迭代报告模板（`iteration-report-template.md`） |
| **事件生产** | `issues`（创建迭代报告 GitHub Issue） |
| **事件消费** | GitHub Actions API（查询 Auto Fix / Release 运行记录） |
| **数据库** | 无（指标数据从 GitHub Actions 运行记录中采集） |
| **前端对应** | 无（通过 GitHub Issue / 飞书文档查看） |

**⚠️ 开发注意**：
- 指标数据从 GitHub Actions 运行记录中采集，不侵入业务代码
- 修改 Auto Fix Engine 或 Auto Release Pipeline 的 workflow 名称时，需同步更新 Iteration Dashboard 的数据采集逻辑
- 迭代报告每周一自动生成，通过 `schedule: cron` 触发
- 指标采集依赖 GitHub Actions API，需确保 API Token 有足够权限

---

## 二、前端模块开发上下文卡片

### 2.1 Chat 域（最复杂的前端域）

**涉及文件**：`features/chat/api.ts`、`stores/chat/`（4 个 Store）、`components/chat/`（35+ 组件）、`realtime/`

| 维度 | 内容 |
|------|------|
| **Store 拆分** | `useChatSessionStore`（会话列表）、`useChatMessageStore`（消息）、`useChatRuntimeStore`（运行时控制）、`useChatConversationStore`（WS 投影） |
| **跨 Store 通信** | `sessionSync` 事件总线（与 SessionStore 同步）、AppStore → ChatSessionStore（Agent 切换重置） |
| **WS 事件消费** | text_delta、tool_call、tool_result、runner_completion、context_usage、error、run_status、intent_pass、member_message_*、team_run_* |
| **后端对应** | ChatService（HTTP API + WS /v1/ws） |
| **共享类型** | `Message`、`ChatOption`、`RunStatus`（定义在 features/chat/api.ts） |

**⚠️ 开发注意**：
- 消息分组必须使用堆栈模型（`groupMessagesByTurn` 按 `role=user` 边界），禁止使用 `turn_index`
- in-flight 消息（pending-user-*/ws-stream-*/member-*）排序在持久化消息之后
- 修改 `Envelope` 类型时，需同步 `realtime/envelope.ts`

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

**涉及文件**：`ws-transport.ts`、`globalWsHub.ts`、`useEnvelopeStream.ts`、`dispatcher.ts`、`envelope.ts`

| 维度 | 内容 |
|------|------|
| **核心导出** | `createEnvelopeStream()`、`useEnvelopeStream()`、`EnvelopeDispatcher`、`Envelope` 类型、46 种 `EnvelopeType` |
| **消费方** | Chat（会话流）、Monitor（日志流）、Teams（运行流）、Graph（执行流）、Orchestration（编排流） |
| **后端对应** | WSServer (`internal/server/ws.go`，使用本地 `WSTurnInput`/`WSTurnOptions`/`WSTurnExecutor` 类型，通过 Wire `wsTurnExecutorAdapter` 桥接 `biz.TurnExecutorGateway`) + EventBus (`internal/event/`) |

**⚠️ 开发注意**：
- 新增 `EnvelopeType` 时，必须同时更新：后端 `internal/event/envelope.go` + 前端 `realtime/envelope.ts`
- `globalWsHub` 使用引用计数，`acquireGlobalWsConsumer`/`releaseGlobalWsConsumer` 必须配对调用

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
| `Envelope` | `event/envelope.go` | 所有事件生产者/消费者 + 前端 `realtime/envelope.ts` |
| `EnvelopeType` | `event/envelope.go` | 所有事件生产者/消费者 + 前端 `realtime/envelope.ts` |
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

### 3.5 修改自迭代引擎模块时

| 修改的模块 | 影响的模块 | 需要同步更新 |
|-----------|-----------|-------------|
| CI Pipeline Job 名称/结构 | Auto Fix Engine、Auto Release Pipeline、Doc Sync Engine、E2E Testing | 各下游 workflow 的触发条件和 Job 引用 |
| Lint System 规则 | Auto Fix Engine | `.auto-fix/patterns.json` 中的修复策略、auto-fix 的修复命令 |
| `.goreleaser.yml` 配置 | Auto Release Pipeline、Iteration Dashboard | 发布产物路径、Changelog 生成逻辑、指标采集字段 |
| `.auto-fix/patterns.json` 格式 | Auto Fix Engine | 模式匹配逻辑、知识库解析代码 |
| `openspec/specs/` 文档结构 | Doc Sync Engine | 文档路径映射、影响分析逻辑 |
| `web/e2e/` 测试结构 | E2E Testing | `playwright.config.ts`、fixtures、helpers |
| Auto Fix Engine workflow 名称 | Iteration Dashboard | 指标采集的 workflow 名称引用 |
| Auto Release Pipeline workflow 名称 | Iteration Dashboard | 指标采集的 workflow 名称引用 |

---

## 四、前后端对齐速查

| 后端 Service | Proto 包 | 前端 Service 工厂 | 前端 Store | 前端页面 |
|-------------|---------|-----------------|-----------|---------|
| ChatService | chat/v1 | createChatService | useChatSessionStore + useChatMessageStore + useChatRuntimeStore + useChatConversationStore | ChatPage |
| AgentService | agent/v1 | createAgentService | useAppStore + useAgentsPageStore + useAgentDetailStore | AgentsPage + AgentSettingsPage |
| TeamService | team/v1 | createTeamService | useTeamsStore + useTeamsPageStore | TeamsPage + TeamOrchestratePage |
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
| MonitorService | monitor/v1 | createMonitorService | useMonitorStore | MonitorPage |
| A2AService | a2a/v1 | createA2AService | useA2AStore | A2APage |
| LlmProviderModelService | llm_provider_model/v1 | createLlmProviderModelService | usePlatformStore | ResourceManagerPage |
| UsageService | usage/v1 | createUsageService | useUsageStore | UsageEventsPage |
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

### 场景 7：新增 CI Pipeline Job

| 步骤 | 模块 | 文件 |
|------|------|------|
| 1. 添加 Job 定义 | CI Pipeline | `.github/workflows/ci.yml`（或新建 workflow） |
| 2. 更新 Job 依赖 | CI Pipeline | `needs:` 字段，确保 Job 执行顺序 |
| 3. 同步 Auto Fix 触发 | Auto Fix Engine | `.github/workflows/auto-fix.yml` 的触发条件 |
| 4. 同步 Iteration Dashboard | Iteration Dashboard | `.github/workflows/iteration-dashboard.yml` 的指标采集 |
| 5. 验证 | CI | 推送测试分支，确认新 Job 运行 |

**关联模块**：CI Pipeline → Auto Fix Engine → Iteration Dashboard

### 场景 8：新增 E2E 测试用例

| 步骤 | 模块 | 文件 |
|------|------|------|
| 1. 编写 Playwright spec | E2E Testing | `web/e2e/<feature>.spec.ts` |
| 2. 添加测试 fixtures（如需） | E2E Testing | `web/e2e/fixtures/` |
| 3. 更新 playwright.config（如需） | E2E Testing | `web/playwright.config.ts` |
| 4. 本地验证 | E2E Testing | `cd web && pnpm exec playwright test` |
| 5. Nightly CI 验证 | CI | 确认 `.github/workflows/e2e-nightly.yml` 包含新测试 |

**关联模块**：E2E Testing → Web 前端 → 后端服务

### 场景 9：新增失败模式到 Auto Fix 知识库

| 步骤 | 模块 | 文件 |
|------|------|------|
| 1. 识别失败模式 | Auto Fix Engine | 分析 CI 失败日志，提取模式特征 |
| 2. 编写修复策略 | Auto Fix Engine | `.auto-fix/patterns.json` 添加条目（pattern + solution + confidence） |
| 3. 验证模式匹配 | Auto Fix Engine | 模拟 CI 失败日志，确认模式匹配正确 |
| 4. 验证修复生成 | Auto Fix Engine | 确认 LLM 能基于策略生成正确修复代码 |

**关联模块**：Auto Fix Engine → CI Pipeline → Lint System

---

## agent-crud (from aranea-pack-import-export)

### Requirement: Agent 按 agent_key 幂等 upsert
AgentUsecase SHALL 支持通过 agent_key 进行幂等创建/更新操作，供 Pack 导入引擎使用。

#### Scenario: agent_key 不存在时创建
- **WHEN** Pack 导入引擎调用 AgentUsecase 的 upsert 方法，agent_key 在目标系统不存在
- **THEN** 系统 SHALL 创建新 Agent，使用 Pack 中定义的 agent_key

#### Scenario: agent_key 已存在时更新
- **WHEN** Pack 导入引擎调用 AgentUsecase 的 upsert 方法，agent_key 已存在且冲突策略为 overwrite
- **THEN** 系统 SHALL 更新已有 Agent 的可修改字段，保留原 ID 和 created_at

#### Scenario: agent_key 已存在时跳过
- **WHEN** Pack 导入引擎调用 AgentUsecase 的 upsert 方法，agent_key 已存在且冲突策略为 skip
- **THEN** 系统 SHALL 跳过该 Agent，返回已有 Agent 的 ID

### Requirement: Agent 创建时支持 Prompt 文件批量写入
AgentUsecase SHALL 支持在创建 Agent 时批量写入 Prompt 文件。

#### Scenario: 创建 Agent 同时写入文件
- **WHEN** Pack 导入引擎创建 Agent 并提供 files 列表
- **THEN** 系统 SHALL 在同一个事务中创建 Agent 记录和所有 Prompt 文件记录

### Requirement: Agent 创建时支持 RuntimeSettings 写入
AgentUsecase SHALL 支持在创建 Agent 时写入可移植的 RuntimeSettings。

#### Scenario: 创建 Agent 同时写入 RuntimeSettings
- **WHEN** Pack 导入引擎创建 Agent 并提供 runtime 配置
- **THEN** 系统 SHALL 在创建 Agent 后写入 RuntimeSettings，实例绑定字段使用默认值

---

## team-crud (from aranea-pack-import-export)

### Requirement: Team 创建时支持 agent_key 成员引用
TeamUsecase SHALL 支持在创建/更新 Team 时，成员通过 agent_key 引用而非仅通过 agent_id。

#### Scenario: 成员 agent_key 解析
- **WHEN** Pack 导入引擎创建 Team，members 中包含 agent_key 字段
- **THEN** 系统 SHALL 将 agent_key 解析为 agent_id，填充到 OrchestrationMember.AgentID

#### Scenario: agent_key 解析失败
- **WHEN** 成员引用的 agent_key 在目标系统不存在
- **THEN** 系统 SHALL 返回校验错误，列出未找到的 agent_key

### Requirement: Team 创建时支持 Graph 关联
TeamUsecase SHALL 支持在创建 Team 时关联 GraphDefinition。

#### Scenario: linked_graph_id 设置
- **WHEN** Pack 导入引擎创建 Team 并提供 graph_id 引用
- **THEN** 系统 SHALL 将解析后的 graph_id 写入 Team 的 definition_json 的 linked_graph_id 字段

#### Scenario: 内嵌 Graph 定义写入
- **WHEN** Pack 导入引擎创建 Team 并提供内嵌 Graph 定义
- **THEN** 系统 SHALL 将 Graph 定义（节点中 agent_key 已转换为 agent_id）写入 Team 的 definition_json 的 graph 字段
