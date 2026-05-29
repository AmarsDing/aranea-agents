# Aranea-Agents 总体设计蓝图

> 本文档是项目的架构真相源，指导 AI 在各模块开发时理解全局上下文，避免信息孤岛。
> 编码规范详见 SKILLs（`aranea-coding-guide` / `aranea-frontend-guide`），本文聚焦**模块功能、关联运作与业务流程**。

---

## 一、项目定位

Aranea-Agents 是基于 **trpc-agent-go** 的多智能体编排平台。以 **Kratos v2** 为传输壳层、**trpc-agent-go** 为运行时内核，提供 Agent 创建/编排/执行/监控的全生命周期管理。

**双框架分工**：

| 框架 | 职责 | 禁止 |
|------|------|------|
| Kratos v2 | HTTP/gRPC/WebSocket 传输、配置、鉴权、中间件、Wire DI | 不承载 Agent 编排、不实现第二套事件循环 |
| trpc-agent-go | Agent 编排（Runner/Agent/Session/Memory/Tool/Event/Skill/Graph/Team） | 不直接写业务数据库、不处理 HTTP 路由 |

---

## 二、架构全景图

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          前端 (Vue 3 + Quasar)                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐ │
│  │ ChatPage │  │AgentsPage│  │GraphsPage│  │TeamsPage│  │ Monitor  │ │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘ │
│       │             │             │             │             │        │
│  ┌────┴─────────────┴─────────────┴─────────────┴─────────────┴────┐   │
│  │              Stores (Pinia) — 36 个域 Store                      │   │
│  └────┬─────────────┬─────────────┬─────────────┬──────────────┬───┘   │
│       │             │             │             │              │        │
│  ┌────┴─────┐  ┌────┴─────┐  ┌────┴─────┐  ┌───┴──────┐ ┌────┴─────┐ │
│  │features/ │  │features/ │  │features/ │  │features/ │ │ realtime │ │
│  │ chat/api │  │agents/api│  │graph/api │  │teams/api │ │  WS Hub  │ │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └───┬──────┘ └────┬─────┘ │
│       │             │             │             │              │        │
│  ┌────┴─────────────┴─────────────┴─────────────┴───┐    ┌────┴─────┐ │
│  │         services/index.ts (Kratos HTTP Client)    │    │  /v1/ws  │ │
│  └────────────────────┬──────────────────────────────┘    └────┬─────┘ │
└───────────────────────┼─────────────────────────────────────────┼──────┘
                        │ HTTP/gRPC                              │ WS
┌───────────────────────┼─────────────────────────────────────────┼──────┐
│                       ▼                                         ▼      │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │                     Server 层 (Kratos Transport)                  │  │
│  │  HTTP Server ── gRPC Server ── WebSocket Server                  │  │
│  └──────────────────────┬──────────────────────────────────────────┘  │
│                         │                                              │
│  ┌──────────────────────┴──────────────────────────────────────────┐  │
│  │                     Service 层 (传输桥点)                         │  │
│  │  ChatService ── AgentService ── TeamService ── GraphService ...  │  │
│  │  ┌──────────────────────────────────────────────────────────┐    │  │
│  │  │  Runner 装配入口 (唯一位置)                                │    │  │
│  │  │  ChatOrchestrator: BuildTRPCAgent → NewTRPCRunner → Run   │    │  │
│  │  └──────────────────────────────────────────────────────────┘    │  │
│  └──────┬────────────┬──────────────┬──────────────┬────────────────┘  │
│         │            │              │              │                    │
│  ┌──────┴─────┐ ┌────┴──────┐ ┌────┴──────┐ ┌────┴──────┐             │
│  │  Biz 层    │ │  Agent    │ │  Tools    │ │  Provider │             │
│  │ (领域核心) │ │  构建器   │ │  装配中心 │ │  LLM 驱动 │             │
│  └──────┬─────┘ └───────────┘ └───────────┘ └───────────┘             │
│         │                                                              │
│  ┌──────┴─────┐ ┌───────────┐ ┌───────────┐ ┌───────────┐            │
│  │  Data 层   │ │  Event    │ │  Memory   │ │  Session  │            │
│  │ (Ent ORM)  │ │  Bus      │ │  记忆服务 │ │  会话存储 │            │
│  └────────────┘ └───────────┘ └───────────┘ └───────────┘            │
│                                                                       │
│  ┌────────────┐ ┌───────────┐ ┌───────────┐ ┌───────────┐            │
│  │  Channel   │ │  Graph    │ │  Team     │ │  Cron     │            │
│  │  渠道集成  │ │  图编排   │ │  多Agent  │ │  定时任务 │            │
│  └────────────┘ └───────────┘ └───────────┘ └───────────┘            │
│                                                                       │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │              pkg/trpc-agent-go (Agent 框架真相源)                  │  │
│  │  Runner / Agent / Model / Tool / Session / Memory / Event        │  │
│  └──────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────────┘
```

---

## 三、后端模块详解

### 3.1 Kratos 标准四层

#### Server 层 (`internal/server/`)

**职责**：传输注册 + 中间件，不写业务逻辑。

| 文件 | 功能 |
|------|------|
| `http.go` | HTTP Server 创建，注册所有 `RegisterXxxHTTPServer`，中间件链：recovery → tracing → logging → auth → cors |
| `grpc.go` | gRPC Server 创建，注册所有 `RegisterXxxServiceServer` |
| `ws.go` | WebSocket Server，处理 `/v1/ws` 连接，消息路由到 `TurnExecutorGateway`，事件推送到前端 |
| `server.go` | 统一 Server 工厂 |

**关键约束**：Server 层不得 new `runner.Runner` 或 `llmagent.New`（红线 #1）。

#### Service 层 (`internal/service/`)

**职责**：proto ↔ biz 类型映射 + Runner 编排。这是 Wire 装配中心和唯一允许创建 Runner 的层。

| Service | 对应 Proto | Biz 依赖 | Runner 装配 |
|---------|-----------|---------|------------|
| **ChatService** | chat/v1 | ChatOrchestrator | 实现 `NativeTurnGateway`/`TurnControlGateway`/`DurableResumeGateway`/`A2ARunnerFactory` |
| **AgentService** | agent/v1 | AgentUsecase | 无（纯 CRUD） |
| **TeamService** | team/v1 | TeamUsecase | 无（纯 CRUD） |
| **GraphService** | graph/v1 | GraphUsecase + GraphBuilderFactory | 实现 `biz.GraphExecutor` |
| **SessionService** | session/v1 | SessionUsecase | 无 |
| **ChannelService** | channel/v1 | ChannelUsecase | 无 |
| **CronService** | cron/v1 | CronUsecase | 无 |
| **PluginService** | plugin/v1 | PluginUsecase | 无 |
| **SkillService** | skill/v1 | SkillUsecase | 无 |
| **MemoryService** | memory/v1 | MemoryAdminUsecase + L4CascadeUsecase | 无 |
| **ToolService** | tool/v1 | ToolUsecase | 无 |
| **KnowledgeService** | knowledge/v1 | KnowledgeUsecase | 无 |
| **A2AService** | a2a/v1 | A2AUsecase | 通过 `A2ARunnerFactory` 端口 |
| **其他** | admin/avatar/hook/usage/evaluation/ecosystem... | 对应 Usecase | 无 |

**ChatOrchestrator** 是核心编排器，实现 `biz.TurnExecutor`，负责：
1. 准入控制（会话锁、活跃 Run 检查、待处理队列）
2. Agent 构建（`BuildTRPCAgent`）
3. Runner 创建（`NewTRPCRunner`）
4. Turn 执行（`RunTRPCUserTurn`）
5. 事件流处理（投影为 Envelope → EventBus → WebSocket）

#### Biz 层 (`internal/biz/`)

**职责**：领域模型 + Usecase + Repo 接口定义 + 跨模块端口接口。**禁止** import `pkg/trpc-agent-go` 和 `api/*/v1`。

**核心 Usecase**：

| Usecase | 职责 | 关键依赖 |
|---------|------|---------|
| **AgentUsecase** | Agent CRUD、Prompt 文件管理、运行时设置、Token 估算 | `AgentRepository`、`ToolCatalogReader`、`SystemSettingRepo` |
| **ChatUsecase** | 聊天 Turn 生命周期、Run 状态、待处理队列、Await-Reply | `ChatRunGateway`、`ChatSessionLocker`、`ChatPendingQueue` |
| **TeamUsecase** | Team CRUD、定义验证（6 种模式）、Run 管理 | `TeamRepository`、`AgentIDExistenceChecker` |
| **GraphUsecase** | Graph 定义/执行/缓存、任务协调、GC | `GraphRepo`、`GraphRunRepo`、`GraphBuilderFactory` |
| **SessionUsecase** | 会话 CRUD、消息持久化、Turn 记录、压缩 | `SessionRepo`、`SessionRunRepo` |
| **ChannelUsecase** | 渠道 CRUD、入站路由、出站投递 | `ChannelRepo`、`NativeTurnGateway`、`GraphExecutor` |
| **CronUsecase** | 定时任务 CRUD、调度 | `CronRepo`、`NativeTurnGateway` |
| **MemoryUsecase** | 记忆 CRUD、PII 检测、策略执行 | `MemoryRepo`、`EmbeddingService` |
| **MonitorUsecase** | 告警规则、Flow Log、Trace 投影 | `MonitorRepo`、`UsageRepo` |
| **PluginUsecase** | 插件 CRUD、Schema 验证 | `PluginRepo` |
| **SkillUsecase** | 技能 CRUD、导入、热重载 | `SkillRepo` |
| **ToolUsecase** | 工具目录 CRUD、Agent 覆盖 | `ToolRepo` |
| **KnowledgeUsecase** | 知识库 CRUD、文档摄入 | `KnowledgeRepo` |
| **A2AUsecase** | A2A 端点 CRUD、远程调用 | `A2ARepo` |
| **UsageUsecase** | 用量记录、配额管理、定价 | `UsageRepo` |

**跨模块端口接口**（定义在 biz，Wire 绑定在 service）：

```
TurnExecutorGateway     ← WSServer 消费（只需执行 Turn）
TurnRunControlGateway   ← DurableWorker 消费（只需 Run 控制）
TurnControlGateway      ← ChannelIngress 消费（执行 + 控制 + 卡片操作）
NativeTurnGateway       ← Channel/Cron 消费（全部能力，Deprecated 过渡中）
DurableResumeGateway    ← DurableWorker 消费（恢复持久化 Run）
A2ARunnerFactory        ← A2AService 消费（构建 A2A Runner）
CronTriggerGateway      ← ChannelIngress 消费（触发定时任务）
GraphExecutor           ← Channel/Cron 消费（执行 Graph）
GraphBuildConfig        ← Team 消费（Graph 编译配置）
GraphRuntime            ← Graph 消费（运行时端口）
```

#### Data 层 (`internal/data/`)

**职责**：实现 biz 定义的 Repo 接口，封装 Ent ORM + 原生 SQL 操作。

**双数据库架构**：

| 数据库 | 用途 | 访问方式 |
|--------|------|---------|
| **SQLite** | 主数据库（Agent/Session/Tool/Channel/Hook/Plugin/Cron/Graph/Monitor/Usage...） | `d.Ent()` + `d.rawDB` |
| **PostgreSQL** | 向量存储（Memory Embedding / Knowledge Chunk） | `d.Postgres()`（可选，无则降级为 SQLite） |

**启动流程**：`initSQLite` → `ensureSchemaDDL`（30+ DDL 补丁）→ `initPostgres` → `runPendingDataMigrations` → `seedInitialData`

**42 个 Repo 实现**，每个都有编译期接口检查 `var _ biz.XxxRepo = (*xxxRepo)(nil)`。

### 3.2 Agent 运行时模块

#### Agent 构建 (`internal/agent/`)

**核心函数**：`BuildTRPCLLMAgent(ctx, bizAgent, TRPCBuilderDeps) → trpcagent.Agent`

**构建流程**：
1. 解析 LLM 模型 → `provider.TRPCModelForProviderModel`
2. 构建系统 Prompt → L1(角色) + L2(工具) + L3(记忆) + L4(知识) 四层叠加
3. 配置 `llmagent.Options`：model、instruction、planner、skills、tools、memory、callbacks
4. 应用运行时设置：上下文压缩、会话摘要、输出 Schema、模型选择器
5. 返回 `llmagent.New(agentKey, opts...)`

**TRPCBuilderDeps** 是稳定扩展 DTO，分 6 组：
- `TRPCCatalogDeps`：Agent/Tool/LLM/Skill/Settings 仓库
- `TRPCModelRouteDeps`：Provider/Model 路由
- `TRPCToolAssemblyDeps`：工具装配
- `TRPCMemoryKnowledgeDeps`：记忆 + 知识检索
- `TRPCPluginDeps`：插件回调
- `TRPCSkillDeps`：技能系统

#### Runner 创建 (`internal/agent/trpc_runtime.go`)

**核心函数**：`NewTRPCRunner(rootAgent, TRPCRunnerDeps) → ManagedRunner`

包装 `trpcrunner.NewRunner`，注入 Session/Memory/Artifact/Plugin 服务。

**Turn 执行**：`RunTRPCUserTurn(ctx, runner, userID, sessionID, content) → <-chan *trpcevent.Event`

#### 工具装配 (`internal/tools/`)

**18 个内置工具**：

| 工具 | 类别 | 风险 | 默认 |
|------|------|------|------|
| file | 文件系统 | low | ✅ |
| hostexec | 执行 | critical | ❌ |
| httpfetch / geminifetch | Web | medium | ❌ |
| duckduckgo / google_search / arxiv_search / wikipedia | 搜索 | low-medium | ❌ |
| email | 通信 | high | ❌ |
| todo | 效率 | low | ❌ |
| await_user_reply | 交互 | low | ❌ |
| claudecode / workspace_exec | 编码/执行 | critical | ❌ |
| openapi / agent / mcp / mcpbroker | 集成 | medium | ❌ |

**装配入口**：`Assemble(ctx, AssemblyConfig) → AssembledToolsets`

**装配顺序**：Registry 注册 → 配置覆盖 → OpenAPI → workspace_exec → AgentTool → MCP ToolSet → MCP Broker → CustomTools

#### LLM Provider (`internal/provider/`)

**核心函数**：`TRPCModelForProviderModel(provider, model, uc, opts) → trpcmodel.Model`

支持 7 种 Provider：OpenAI（默认）、Anthropic、Gemini、Ollama、Hunyuan、HuggingFace、Bedrock。

**HA 策略**：Failover（`trpcfailover.New`）或 Hedge（`trpchedge.New`）。

#### 记忆系统 (`internal/memory/`)

**两种模式**：

| 模式 | 行为 | 接入方式 |
|------|------|---------|
| Agentic | Agent 主动调用 memory_add/search 等工具 | `llmagent.WithMemoryService(service)` |
| Auto | 对话结束后 LLM 自动提取记忆 | `service.EnqueueAutoMemoryJob(ctx, session)` |

**5 层记忆架构**：

| 层级 | 内容 | 存储 |
|------|------|------|
| L0 | 会话快照 | SQLite Session |
| L1 | 任务/字段提取 | SQLite Memory |
| L2 | 事实检索 | SQLite + pgvector |
| L3 | 实体关系图 | SQLite + pgvector |
| L4 | 级联演化 | SQLite Memory |

#### 会话存储 (`internal/session/`)

适配 `trpcsession.Service`，提供会话快照读写。通过 `SQLiteSessionService` 实现。

#### 技能系统 (`internal/skill/`)

技能导入、执行、Watch 热重载。通过 `trpcskill.Filter` + `trpcskill.Tools` 适配框架。

### 3.3 编排模块

#### Team 多 Agent (`internal/team/`)

**6 种编排模式**：

| 模式 | 实现 | 适用场景 |
|------|------|---------|
| Sequential | chainagent | 顺序流水线 |
| Parallel | parallelagent | 并行执行 |
| Coordinator | 协调者 Agent 调度成员 | 中央决策 |
| Critic Loop | cycleagent | 生成+评审循环 |
| Swarm | trpcteam.NewSwarm | 自由协作、handoff |
| Adaptive | 动态选择 | 自适应 |

**当前默认路径**：Team 运行使用 **GraphAgent 编译路径**（M53 Phase 7）。原生路径为紧急回退。

#### Graph 图编排 (`internal/graph/`)

**核心能力**：
- 可视化图定义（节点 + 边 + 条件边 + 状态字段）
- 执行引擎（支持条件分支、并行、循环）
- 人工任务节点（HITL：Claim/Submit/Review）
- 检查点 + 时间旅行（前端 GraphCheckpointPanel 快照预览 + 回退确认）
- 模板系统（用户模板 + 系统模板）
- 版本管理 + 回滚（compactNodesForVersion 精简存储）
- 前端实时校验（useGraphLocalValidation：8 种规则，区分 error/warning）
- 变量引用（GraphVariablePicker：`{{nodeId.field}}` 格式）
- 失败策略（Skip/RetryThenBlock/FailFast + CircuitBreakerPolicy）

**端口接口**：
- `GraphExecutor`：Channel/Cron 消费的执行入口
- `GraphBuilderFactory`：构建运行时 Graph
- `GraphBuildConfig`：Team 消费的编译配置

#### Channel 渠道集成 (`internal/channel/`)

**支持 12+ 平台**：飞书/Lark、钉钉、Discord、Slack、Teams、微信、企业微信、Line、Mattermost、QQ、OneBot...

**三层接口**：
- `Runner`：长连接运行（WS/轮询 Bot）
- `OutboundText`：出站消息投递
- `InboundHandler`：入站消息处理（由 `ChannelIngress` 实现）

**入站流程**：Webhook → `ChannelIngress.ProcessInbound` → `NativeTurnGateway.ExecuteTurn` → 事件流 → 出站投递

#### Cron 定时任务 (`internal/cronrunner/`)

**核心**：`cronrunner.Runner` 管理 Cron 调度，触发时通过 `NativeTurnGateway` 执行 Turn。

### 3.4 横切模块

#### 事件系统 (`internal/event/`)

**双总线架构**：

| 总线 | 消费者 | 事件类型 |
|------|--------|---------|
| **SessionBus** | WebSocket 推送、会话投影 | text_delta、tool_call、runner_completion... |
| **MonitorBus** | Flow Log、Trace、告警 | flow_log、alert_notify... |

**投递策略**：`DropOldest`（默认）、`DropNewest`、`BlockUpTo`（可靠）。关键事件（tool_result、error、runner_completion）永不丢弃。

#### 运行时依赖 (`internal/runtime/`)

`TurnDeps` 是每个 Chat Turn 的统一依赖集：
- `Catalog`：Agent/Tool/LLM/Skill/Settings 仓库（只读）
- `Persist`：Session/Memory/MCP/Artifact 服务
- `Pipeline`：EventBus + Buffer
- `RunnerMgr`：Runner 生命周期管理

#### 上下文压缩 (`internal/compress/`)

L0 上下文压缩：长对话超过阈值时，用 LLM 生成摘要替换历史消息。通过 CAS + 事务保证原子性。

#### 知识库 (`internal/knowledge/`)

文档摄入管线：上传 → OCR（图片/PDF）→ 分块 → Embedding → pgvector 存储 → 检索。

#### 评估系统 (`internal/evaluation/`)

LLM Judge 评估框架：数据集管理 → 运行评估 → 评分（自动 + LLM Judge）→ 结果统计。

#### A2A 协议 (`internal/a2a/`)

Agent-to-Agent 通信协议：Agent Card 验证、远程调用、Graph 恢复、健康检查。

#### 插件系统 (`internal/plugin/`)

Plugin 生命周期管理：注册 → 配置 → 热加载 → 回调链（audit/modify/notify）→ 费用守卫。

#### 模型目录 (`internal/modelcatalog/`)

LLM 模型目录同步：从 Provider API 拉取模型列表 → 定价同步 → 搜索/筛选 → Logo 管理。

---

## 四、前端模块详解

### 4.1 分层架构

```
services/index.ts (25 个 createXxxService)
  → features/<域>/api.ts (HTTP 门面 + 类型归一化)
    → stores/<域>/index.ts (36 个 Store: 状态 + action)
      → features/<域>/useXxxPage.ts (composable 组合 Store)
        → pages/XxxPage.vue (布局 + composable 绑定)
          → components/<域>/*.vue (纯展示: props in / emits out)
```

### 4.2 核心域 Store

| Store | 状态 | 关键 Action | API 调用 |
|-------|------|------------|---------|
| **useAppStore** | agents[]、selectedAgent | loadAgents、addAgent、removeAgent | listAgents、createAgent、deleteAgent |
| **useChatSessionStore** | sessions[]、selectedSession | loadAgentSessions、addAgentSession、removeSessionLocal | listSessions、createSession、deleteSession |
| **useChatMessageStore** | messagesBySession{} | loadMessages、setMessages、clearSessionMessages | listSessionChatMessages |
| **useChatRuntimeStore** | wsConnectedBySession{} | fetchRunStatus、stop、enqueue、submitFeedback | getRunStatus、stopGeneration、enqueueMessage |
| **useChatConversationStore** | currentTarget、sessionsById{} | setCurrentTarget、upsertSession、applyProjection | 无（纯 WS 投影） |
| **useAgentsPageStore** | keyword、filters、agents[] | loadAgentList、toggleAgentFavorite、copyAgent | listAgentsPaged、duplicateAgent |
| **useGraphStore** | graphs[]、activeGraph、executionHistory、checkpoints[]、templates[] | loadGraphs、runGraph、validateGraphDefinition、saveCheckpoint、loadTemplates、rollbackGraph | 全部 Graph API |
| **useTeamsStore** | teams[]、activeTeam | loadTeams、addTeam、editTeam | listTeams、createTeam、updateTeam |
| **useToolsStore** | tools[]、activeTool | loadTools、fetchCatalog、saveOverride | 全部 Tool API |
| **useMonitorStore** | auditLogs[]、events[]、alertRules[] | loadAuditLogs、startRuntimeEventsStream | 全部 Monitor API |
| **useSessionStore** | sessions[]、activeSession | loadSessions、searchPage、batchArchive | 全部 Session API |

### 4.3 实时通信层

```
WebSocket Server (Go /v1/ws)
  │
  ▼
ws-transport.ts          ← 原始 WS 连接、重连、心跳
  │
  ├── globalWsHub.ts     ← 共享 session_id=* 连接（Monitor/Team/Orchestration 消费）
  │
  ▼
useEnvelopeStream.ts     ← Composable 工厂：创建传输或获取全局 Hub 消费者
  │
  ▼
dispatcher.ts            ← EnvelopeDispatcher: 按 type/channel/sessionId/teamId 发布订阅
  │
  ├── features/chat/composables/useChatStreamManager.ts
  ├── features/monitor/api.ts
  ├── features/teams/api.ts
  ├── features/graph/runtime/useGraphExecutionStream.ts
  └── features/orchestration/useOrchestrationStream.ts
```

**46 种 Envelope 类型**：text_delta、tool_call、tool_result、runner_completion、context_usage、graph_node_start/end、team_run_started/finished、alert.notify 等。

### 4.4 跨 Store 通信

| 机制 | 生产者 | 消费者 | 说明 |
|------|--------|--------|------|
| **sessionSync 事件总线** | ChatSessionStore、SessionStore | ChatSessionStore | 会话变更通知（remove/update/archive/refresh） |
| **AppStore → ChatStore** | AppStore | ChatSessionStore、ChatMessageStore | Agent 切换时重置会话和消息 |
| **InboundNotificationStore** | WS 事件 | InboundNotificationBell | 通知铃铛 |
| **MonitorStore → channels/api** | MonitorStore | — | 告警渠道选项加载 |

### 4.5 页面路由

| 路由 | 页面 | 核心功能 |
|------|------|---------|
| `/overview` | OverviewPage | 用量概览 |
| `/chat` | ChatPage | 聊天工作台（Agent 对话 + Team 对话） |
| `/sessions` | SessionsPage | 会话列表 + 搜索 |
| `/memory` | MemoryCenterPage | 记忆中心（5 层记忆浏览） |
| `/agents` | AgentsPage | Agent 列表 + 创建 |
| `/agents/:id/settings` | AgentSettingsPage | Agent 设置（Prompt/工具/模型/记忆） |
| `/team` | TeamsPage | Team 列表 + 编排 |
| `/graphs` | GraphsPage | Graph 列表 + 编辑器 |
| `/graphs/:id/edit` | GraphEditorPage | Graph 可视化编辑（Vue Flow 画布 + 属性面板 + 实时校验 + 撤销重做） |
| `/graphs/:id/run` | GraphRunPage | Graph 执行态（步骤时间线 + 任务看板 + HITL + 检查点回退） |
| `/graphs/executions` | GraphExecutionsPage | Graph 执行历史（服务端过滤 + 状态/时间范围筛选） |
| `/models` | ResourceManagerPage | LLM Provider/Model 管理 |
| `/channels` | ChannelsPage | 渠道管理 |
| `/tools` | ToolsPage | 工具目录 |
| `/monitor/logs` | MonitorPage | 监控日志 + 告警 |
| `/cron` | CronTasksPage | 定时任务 |
| `/hooks` | HooksPage | Webhook 管理 |
| `/knowledge` | KnowledgePage | 知识库 |
| `/plugins` | PluginsPage | 插件管理 |
| `/skills` | SkillsPage | 技能管理 |
| `/mcp-servers` | McpServersPage | MCP 服务器 |
| `/a2a` | A2APage | A2A 端点 |
| `/evaluation` | EvaluationPage | 评估 |
| `/usage/events` | UsageEventsPage | 用量事件 |
| `/settings` | SystemSettingsPage | 系统设置 |

---

## 五、核心业务流程

### 5.1 Chat Turn 完整流程

```
用户发送消息
  │
  ▼
HTTP POST /v1/chat/send 或 WS user_message
  │
  ▼
Server 层路由到 ChatService
  │
  ▼
ChatService → ChatOrchestrator.ExecuteTurn(TurnInput)
  │
  ├── 1. 准入控制：检查会话锁、活跃 Run、待处理队列
  │
  ├── 2. 目录查找：AgentRepository.GetAgentByAgentKey()
  │     → Data 层 Ent 查询 SQLite
  │
  ├── 3. Agent 构建：BuildTRPCAgent(agent, deps)
  │     ├── Provider 解析 LLM 模型
  │     ├── Tools.Assemble() 构建工具集
  │     ├── Skill 解析技能策略
  │     ├── Memory 注入记忆工具
  │     ├── Plugin 注册回调链
  │     └── Prompt 四层叠加（L1角色 + L2工具 + L3记忆 + L4知识）
  │
  ├── 4. Runner 创建：NewTRPCRunner(rootAgent, deps)
  │     → trpcrunner.NewRunner + Session + Memory + Plugins
  │
  ├── 5. Turn 执行：RunTRPCUserTurn(runner, userID, sessionID, content)
  │     → runner.Run() → <-chan *trpcevent.Event
  │
  ├── 6. 事件流处理循环：
  │     For each event:
  │       → 转换为 biz Envelope
  │       → Infra.Publish() 路由到 SessionBus + MonitorBus
  │       → SessionBus → WebSocket 推送到前端
  │       → MonitorBus → Flow Log + Trace 投影
  │       → 持久化消息到 SessionUsecase
  │
  └── 7. Turn 完成后处理：
        ├── SetRunStatus("completed")
        ├── 记忆提取（异步 EventBus → TurnMemoryWorker）
        ├── 会话压缩（超阈值时 LLM 摘要）
        ├── 用量记录
        └── Monitor Trace 记录
```

### 5.2 Channel 入站流程

```
飞书/Discord/Slack Webhook
  │
  ▼
ChannelService.ReceiveWebhook()
  │
  ▼
ChannelIngress.ProcessInbound(channel, InboundEvent)
  │
  ├── 1. 消息解析 + 去重（InboundReceiptRepo）
  ├── 2. 路由匹配（ChannelRoutingRules）
  ├── 3. Turn 执行：NativeTurnGateway.ExecuteTurn()
  │     → 复用 Chat Turn 完整流程
  ├── 4. 事件流 → 出站投递队列
  └── 5. ChannelDeliveryWorker 轮询 → OutboundText.SendText()
```

### 5.3 Team 编排流程

```
用户创建 Team（定义成员 + 编排模式）
  │
  ▼
TeamService.RunTeam()
  │
  ├── GraphAgent 编译路径（默认）：
  │     Team 定义 → Graph 编译 → GraphBuilderFactory.BuildAndRun()
  │     → Graph 执行引擎 → 节点间数据传递 → 事件流
  │
  └── 原生路径（紧急回退）：
        BuildTRPCTeam() → chainagent/parallelagent/cycleagent/swarm
        → 每个 Member Agent 独立构建 → 事件流
```

### 5.4 Graph 执行流程

```
用户创建 Graph（节点 + 边定义）
  │
  ▼
前端实时校验（useGraphLocalValidation）
  ├── 8 种规则：no_entry_point / duplicate_node / edge_source_missing / edge_target_missing / unreachable_node / loop_no_exit（无条件循环=error） / conditional_loop（条件循环=warning） / orphan_node
  └── 与后端校验结果合并去重（key=code:nodeId:field）
  │
  ▼
GraphService.ExecuteGraph()
  │
  ├── 1. GraphBuilderFactory.BuildAndRun(definition)
  │     → 编译为可执行 Graph
  │
  ├── 2. 执行引擎：
  │     ├── 条件节点：LLM 判断分支
  │     ├── Agent 节点：构建 Agent → Turn 执行
  │     ├── 并行节点：多分支并发
  │     ├── HITL 节点：暂停等待人工操作
  │     └── 子 Graph 节点：递归执行
  │
  ├── 3. 检查点：每个节点完成后保存快照
  │     → 前端 GraphCheckpointPanel 查看快照 + 确认回退
  │
  └── 4. 事件流 → WebSocket → 前端实时可视化
```

### 5.5 前端 Chat 实时流程

```
用户输入消息 → ChatComposer
  │
  ├── 1. 创建 pending-user-{uuid} 占位消息
  ├── 2. HTTP POST /v1/chat/send
  ├── 3. WS 连接建立（/v1/ws?session_id=xxx）
  │
  ▼
WS 事件流：
  ├── text_delta → 追加到 ws-stream-{sessionId} 消息
  ├── tool_call → 显示工具调用步骤
  ├── tool_result → 显示工具结果
  ├── runner_completion → 触发 loadMessages 获取持久化消息
  │
  ▼
消息合并：
  ├── mergeSessionMessages：服务端消息替换占位消息
  └── groupMessagesByTurn：按 role=user 边界堆栈分组
```

---

## 六、模块间关联矩阵

### 6.1 后端模块依赖关系

| 消费者 | 提供者 | 交互方式 | 端口接口 |
|--------|--------|---------|---------|
| Channel → Chat | Service 层 | 同步调用 | `NativeTurnGateway` / `TurnControlGateway` |
| Channel → Graph | Service 层 | 同步调用 | `GraphExecutor` |
| Cron → Chat | Service 层 | 同步调用 | `NativeTurnGateway` |
| A2A → Chat | Service 层 | 同步调用 | `A2ARunnerFactory` |
| DurableWorker → Chat | Service 层 | 同步调用 | `TurnRunControlGateway` + `DurableResumeGateway` |
| WSServer → Chat | Service 层 | 同步调用 | `TurnExecutorGateway` |
| Graph → Agent | 直接 import | 同步调用 | `BuildTRPCAgent` |
| Team → Agent | 直接 import | 同步调用 | `BuildTRPCAgent` |
| Team → Graph | 直接 import | 同步调用 | `GraphBuildConfig` |
| Chat → Agent | 直接 import | 同步调用 | `BuildTRPCAgent` |
| Chat → Tools | 直接 import | 同步调用 | `Assemble` |
| Chat → Provider | 直接 import | 同步调用 | `TRPCModelForProviderModel` |
| Chat → Memory | 直接 import | 同步调用 | `MemoryService.Tools()` |
| Chat → Event | 直接 import | 异步事件 | `Infra.Publish` |
| Channel → Event | 直接 import | 异步事件 | `Infra.Publish` |
| Monitor → Event | 异步消费 | 异步事件 | Bus Consumer |
| Memory → Event | 异步消费 | 异步事件 | Bus Consumer |

### 6.2 前端模块依赖关系

| 消费者 | 提供者 | 交互方式 |
|--------|--------|---------|
| Page → Store | Store action | 同步调用 |
| Page → Composable | composable 返回值 | 同步调用 |
| Store → features/*/api.ts | HTTP 请求 | 异步 |
| ChatStore → realtime | WS 事件 | 异步推送 |
| ChatSessionStore ↔ SessionStore | sessionSync 事件总线 | 异步事件 |
| AppStore → ChatSessionStore | 直接 import | 同步调用 |
| MonitorStore → channels/api | 直接 API 调用 | 异步 |

---

## 七、数据库 Schema 概览

### 7.1 核心表

| 表 | Ent Schema | 用途 |
|----|-----------|------|
| agents | ✅ | Agent 定义（key/name/prompt/settings） |
| sessions | ✅ | 会话记录 |
| messages | ✅ | 聊天消息 |
| session_turns | ✅ | Turn 记录 |
| session_runs | ✅ | Run 记录 |
| teams | ✅ | Team 定义 |
| team_runs | ✅ | Team Run 记录 |
| tools | ✅ | 工具目录 |
| hooks | ✅ | Webhook 定义 |
| plugins | ✅ | 插件定义 |
| crontasks | ✅ | 定时任务 |
| admins | ✅ | 管理员 |
| system_settings | ✅ | 系统设置 |

### 7.2 扩展表（原生 SQL）

| 表 | DDL 文件 | 用途 |
|----|---------|------|
| memory_facts | 10_memory_l2.sql | L2 事实记忆 |
| memory_entities | 10_memory_l3.sql | L3 实体关系 |
| memory_l4_graph | 10_memory_l4.sql | L4 级联图 |
| flow_log_events | 15_flow_log.sql | Flow Log |
| event_store | 18_event_store.sql | 事件存储 |
| plugin_runs | 13_plugin_run.sql | 插件运行记录 |
| message_fts | 16_message_fts.sql | 消息全文搜索 |
| memory_chain | 16_memory_chain.sql | 记忆链 |
| usage_events / usage_quotas | 08_usage.sql | 用量记录/配额 |

---

## 八、Wire 依赖注入

### 8.1 ProviderSet 组装

```
cmd/admin/wire.go
  ├── server.ProviderSet    — HTTP/gRPC/WS 注册
  ├── data.ProviderSet      — 42 个 Repo 实现
  ├── biz.ProviderSet       — ~35 个 Usecase
  ├── event.ProviderSet     — 事件基础设施
  ├── session.ProviderSet   — 会话运行时
  └── service.ProviderSet   — ~25 个 Service + Wire 接口绑定
```

### 8.2 关键 Wire 绑定（在 service.go）

```go
wire.Bind(new(biz.TurnExecutorGateway), new(*ChatService))
wire.Bind(new(biz.TurnRunControlGateway), new(*ChatService))
wire.Bind(new(biz.TurnGateway), new(*ChatService))
wire.Bind(new(biz.TurnControlGateway), new(*ChatService))
wire.Bind(new(biz.NativeTurnGateway), new(*ChatService))
wire.Bind(new(biz.DurableResumeGateway), new(*ChatService))
wire.Bind(new(biz.A2ARunnerFactory), new(*ChatService))
wire.Bind(new(biz.TurnExecutor), new(*ChatOrchestrator))
wire.Bind(new(biz.GraphExecutor), new(*GraphService))
```

---

## 九、开发决策树

### 新增功能时该改哪些模块

```
新增 HTTP/gRPC 接口
  → api/**/*.proto → make api → internal/service → internal/server

新增业务逻辑
  → internal/biz（模型 + Repo 接口 + Usecase）

新增数据库表
  → internal/data/ent/schema → go generate → internal/data

新增 LLM Agent 能力
  → internal/agent（BuildLLMAgent 扩展）

新增工具
  → internal/tools（Registry 注册 + builtin_tools_seed.go 种子）

新增 Team 编排模式
  → internal/team（BuildWorkflowRoot）

新增 LLM 厂商
  → internal/provider（实现 model.LLM）

新增记忆能力
  → internal/memory（适配器 → trpcmemory.Service）

新增渠道平台
  → internal/channel/<platform>（实现 Runner + OutboundText 接口）

新增前端页面
  → features/<域>/api.ts → stores/<域>/ → pages/XxxPage.vue → components/<域>/

新增前端 Store
  → stores/<域>/index.ts → stores/index.ts 具名导出
```

---

## 十、验证命令速查

| 改动类型 | 最小验证 |
|----------|---------|
| 仅 Service | `go test ./internal/service/... -run TestXxx -count=1` |
| 仅 Biz/Data | `go test ./internal/biz/... ./internal/data/... -count=1` |
| Proto 变更 | `make api && go build ./...` |
| Wire 注入 | `make wire && go build ./cmd/admin` |
| 前端 | `cd web && pnpm lint && pnpm test && pnpm build` |
| **提交前** | 后端：`make api && make wire && make build && make test && make lint`；前端：`cd web && pnpm lint && pnpm test && pnpm build` |
