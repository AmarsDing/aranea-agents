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
│  │              Stores (Pinia) — 43 个域 Store                      │   │
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
| `ws.go` | WebSocket Server，处理 `/v1/ws` 连接，使用本地 `WSTurnInput`/`WSTurnOptions`/`WSTurnExecutor` 类型（通过 Wire `wsTurnExecutorAdapter` 桥接 `biz.TurnExecutorGateway`），事件推送到前端 |
| `server.go` | 统一 Server 工厂 |

**关键约束**：Server 层不得 new `runner.Runner` 或 `llmagent.New`（红线 #1）；Server 层不得 import `internal/biz`（通过 Wire adapter 桥接端口接口）。

#### Service 层 (`internal/service/`)

**职责**：proto ↔ biz 类型映射 + Runner 编排。这是 Wire 装配中心和唯一允许创建 Runner 的层。

| Service | 对应 Proto | Biz 依赖 | Runner 装配 |
|---------|-----------|---------|------------|
| **ChatService** | chat/v1 | ChatOrchestrator | 实现 `NativeTurnGateway`/`TurnControlGateway`/`DurableResumeGateway`/`A2ARunnerFactory` |
| **AgentService** | agent/v1 | AgentUsecase | 无（纯 CRUD） |
| **TeamService** | team/v1 | TeamUsecase | 无（纯 CRUD） |
| **GraphService** | graph/v1 | GraphUsecase + GraphBuilderFactory | 实现 `biz.GraphExecutor` |
| **SessionService** | session/v1 | SessionUsecase | `SessionStatusGuard`（Kratos 生命周期钩子） |
| **ChannelService** | channel/v1 | ChannelUsecase | 无 |
| **CronService** | cron/v1 | CronUsecase | 无 |
| **PluginService** | plugin/v1 | PluginUsecase | 无 |
| **SkillService** | skill/v1 | SkillUsecase | 无 |
| **MemoryService** | memory/v1 | MemoryAdminUsecase + L4CascadeUsecase | 无 |
| **ToolService** | tool/v1 | ToolUsecase | 无 |
| **KnowledgeService** | knowledge/v1 | KnowledgeUsecase | 无 |
| **A2AService** | a2a/v1 | A2AUsecase | 通过 `A2ARunnerFactory` 端口 |
| **AvatarService** | avatar/v1 | AvatarUsecase | `ChannelIconRefresher` 函数钩子（biz 包 init 注册） |
| **AgentCategoryService** | agent_category/v1 | AgentCategoryUsecase | 无 |
| **AIRefineService** | ai_refine/v1 | — | 条件注册 |
| **GatewayService** | gateway/v1 | — | 无 |
| **IndustryService** | industry/v1 | IndustryUsecase | 无 |
| **其他** | admin/hook/usage/evaluation/ecosystem/event/model_catalog/system_setting/mcp_server/artifact/learning_loop/skill_evolution/taxonomy/plan/webhook | 对应 Usecase | 无 |
| **SpiritSynthesisService** | spirit/v1 | SpiritTeamUsecase | 通过 SpiritTeamUsecase |
| **OpenAICompatService** | — | — | OpenAI 兼容 API |
| **PersistentTurnService** | — | — | 持久化 Turn 管理 |
| **FlowLogService** | monitor/v1 | FlowLogUsecase | 无 |
| **CodeExecutorService** | monitor/v1 | — | 代码执行监控 |
| **TaxonomyService** | taxonomy/v1 | TaxonomyUsecase | 无 |
| **SkillEvolutionService** | skill_evolution/v1 | SkillEvolutionUsecase | 无 |
| **ArtifactService** | artifact/v1 | ArtifactUsecase | 无 |
| **EventService** | event/v1 | EventStoreUsecase | 无 |
| **LlmProviderModelService** | llm_provider_model/v1 | LlmProviderModelUsecase | 无 |
| **ModelCatalogService** | model_catalog/v1 | ModelRegistryUsecase | 无 |
| **MCPServerService** | mcp_server/v1 | MCPServerUsecase | 无 |
| **EcosystemService** | ecosystem/v1 | — | 无 |
| **LearningLoopService** | learning_loop/v1 | LearningLoopUsecase | 无 |
| **WebhookService** | — | WebhookUsecase | 无 |
| **PlanService** | — | PlanUsecase | 无 |

**ChatOrchestrator** 是核心编排器，实现 `biz.TurnExecutor`，负责：
1. 准入控制（会话锁、活跃 Run 检查、待处理队列）
2. Agent 构建（`BuildTRPCAgent`）
3. Runner 创建（`NewTRPCRunner`）
4. Turn 执行（`RunTRPCUserTurn`）
5. 事件流处理（投影为 Envelope → EventBus → WebSocket）

**依赖边界**：Service 层不得直接 import `internal/data`（红线 #13）。所有数据访问通过 biz 端口接口（如 `SeedVersionRepo`），由 data 层实现、Wire 绑定。

#### Biz 层 (`internal/biz/`)

**职责**：领域模型 + Usecase + Repo 接口定义 + 跨模块端口接口。**禁止** import `pkg/trpc-agent-go` 和 `api/*/v1`。

**核心 Usecase**：

| Usecase | 职责 | 关键依赖 |
|---------|------|---------|
| **AgentUsecase** | Agent CRUD、Prompt 文件管理、运行时设置、Token 估算 | `AgentRepository`、`ToolCatalogReader`、`SystemSettingRepo` |
| **ChatUsecase** | 聊天 Turn 生命周期、Run 状态、待处理队列、Await-Reply | `ChatRunGateway`、`ChatSessionLocker`、`ChatPendingQueue` |
| **TeamUsecase** | Team CRUD、定义验证（6 种模式）、Run 管理 | `TeamRepository`、`AgentIDExistenceChecker` |
| **GraphUsecase** | Graph 定义/执行/缓存、任务协调、GC | `GraphRepo`、`GraphRunRepo`、`GraphBuilderFactory` |
| **SessionUsecase** | 会话 CRUD、消息持久化、Turn 记录、压缩、**状态转换（5 态状态机）**、**删除保护** | `SessionRepo`、`SessionRunRepo`、`SessionStatusPublisher`（端口） |
| **ChannelUsecase** | 渠道 CRUD、入站路由、出站投递、Peer 绑定管理、入站去重、路由解析、Agent/Team 查询 | `ChannelRepo`、`ChannelPeerSessionRepo`、`ChannelInboundReceiptRepo`、`AgentRepository`、`TeamRepository`、`CredentialCrypto` |
| **CronUsecase** | 定时任务 CRUD、调度 | `CronRepo`、`NativeTurnGateway` |
| **MemoryUsecase** | 记忆 CRUD、PII 检测、策略执行 | `MemoryRepo`、`EmbeddingService` |
| **MonitorUsecase** | 告警规则、Flow Log、Trace 投影 | `MonitorRepo`、`UsageRepo` |
| **PluginUsecase** | 插件 CRUD、Schema 验证 | `PluginRepo` |
| **SkillUsecase** | 技能 CRUD、导入、热重载 | `SkillRepo` |
| **ToolUsecase** | 工具目录 CRUD、Agent 覆盖 | `ToolRepo` |
| **KnowledgeUsecase** | 知识库 CRUD、文档摄入 | `KnowledgeRepo` |
| **A2AUsecase** | A2A 端点 CRUD、远程调用 | `A2ARepo` |
| **UsageUsecase** | 用量记录、配额管理、定价 | `UsageRepo` |
| **SpiritTeamUsecase** | Spirit 模式：动态组装 Team 执行任务、综合结果 | `TaskDAG`、`SynthesisEngine`、`TeamStarterPort` |
| **TaskUsecase** | 任务管理：Graph 任务生命周期 | `TaskRepo`、`TaskLinkRepo` |
| **SessionRunUsecase** | 会话 Run 管理：Run 状态、检查点 | `SessionRunRepo`、`SessionRunCheckpointRepo` |
| **MemoryAdminUsecase** | 记忆管理后台：批量操作、PII 审计 | `MemoryAdminStore` 端口集 |
| **L4CascadeUsecase** | L4 级联演化：跨层记忆传播 | `CascadeProposalStore`/`CascadeGraphReader`/`CascadeFactMutator`/`CascadeSagaStore` |
| **L4GraphUsecase** | L4 图记忆 CRUD | `L4GraphRepo`、`L4GraphWriter` |
| **MemoryL2RecallUsecase** | L2 事实召回 | `MemoryFactReader` |
| **MemoryL3RecallUsecase** | L3 融合召回 | `MemoryL3Store` |
| **MemoryCompositeRecallUsecase** | 复合记忆召回（L2+L3 融合） | `SessionCompositeRecallStore` |
| **EvolutionUsecase** | Agent 演化：指标采集、建议生成/应用/拒绝 | `EvolutionMetricsRepo`、`EvolutionSuggestionRepo` |
| **TaxonomyUsecase** | 分类法管理 | `TaxonomyRepo` |
| **PositionUsecase** | 岗位 CRUD（含祖先链查询） | `PositionRepository` |
| **IndustryUsecase** | 行业 CRUD | `IndustryRepository` |
| **DepartmentUsecase** | 部门 CRUD | `DepartmentRepository` |
| **AgentCategoryUsecase** | Agent 分类 CRUD | `AgentCategoryRepo` |
| **PlanUsecase** | 计划管理：draft→approved→executing→completed/failed | `PlanRepository` |
| **AgentTemplateUsecase** | Agent 模板管理 | `AgentRepository` |
| **MCPServerUsecase** | MCP 服务器 CRUD + 连通性测试 + 健康元数据 | `MCPServerRepo` |
| **ChannelTurnJobUsecase** | 渠道 Turn Job 管理 | `ChannelTurnJobRepo` |
| **SystemSettingUsecase** | 系统设置（含 DefaultRefineLLM） | `SystemSettingRepo` |
| **WebhookUsecase** | Webhook 配置管理 | `WebhookRepository` |
| **EventStoreUsecase** | 事件存储查询 | `EventStoreRepo` |
| **ModelRegistryUsecase** | 模型注册表：策略/同步/应用/迁移/Logo | `modelregistry.Store` |
| **LlmProviderModelUsecase** | LLM Provider/Model 目录管理 | `LlmProviderModelRepo` |
| **DeliveryUsecase** | Webhook 投递管理 | `hook.DeliveryRepo` |
| **AdminUsecase** | 管理员 CRUD | `AdminRepo` |

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
SeedVersionRepo         ← IndustryAgentSeed 消费（查询种子版本号）
```

#### Data 层 (`internal/data/`)

**职责**：实现 biz 定义的 Repo 接口，封装 Ent ORM + 原生 SQL 操作。

**双数据库架构**：

| 数据库 | 用途 | 访问方式 |
|--------|------|---------|
| **SQLite** | 主数据库（Agent/Session/Tool/Channel/Hook/Plugin/Cron/Graph/Monitor/Usage...） | `d.Ent()` + `d.rawDB` |
| **PostgreSQL** | 向量存储（Memory Embedding / Knowledge Chunk） | `d.Postgres()`（可选，无则降级为 SQLite） |

**启动流程**：`initSQLite` → `ensureSchemaDDL`（30+ DDL 补丁）→ `initPostgres` → `runPendingDataMigrations` → `seedInitialData`

**~60 个 Repo/Adapter 实现（65 条编译期接口检查）**，每个都有编译期接口检查 `var _ biz.XxxRepo = (*xxxRepo)(nil)`。

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
- `TRPCMemoryKnowledgeDeps`：记忆 + 知识检索（含 `MemoryService trpcmemory.Service`，提供 `Service.Tools()` 统一注入路径）
- `TRPCPluginDeps`：插件回调
- `TRPCSkillDeps`：技能系统

#### Runner 创建 (`internal/agent/trpc_runtime.go`)

**核心函数**：`NewTRPCRunner(rootAgent, TRPCRunnerDeps) → ManagedRunner`

包装 `trpcrunner.NewRunner`，注入 Session/Memory/Artifact/Plugin 服务。

**Turn 执行**：`RunTRPCUserTurn(ctx, runner, userID, sessionID, content) → <-chan *trpcevent.Event`

#### 工具装配 (`internal/tools/`)

**28 个注册工具 + ~37 个运行时注入工具**：

| 工具 | 类别 | 风险 | 默认 |
|------|------|------|------|
| file | 文件系统 | low | ✅ |
| hostexec | 执行 | critical | ❌ |
| httpfetch / claudefetch / geminifetch | Web | medium | ❌ |
| duckduckgo / google_search / arxiv_search / wikipedia | 搜索 | low-medium | ❌ |
| email / message | 通信 | high | ❌ |
| todo | 效率 | low | ❌ |
| await_user_reply | 交互 | low | ❌ |
| claudecode / workspace_exec | 编码/执行 | critical | ✅/❌ |
| openapi / agent / mcp / mcpbroker / model_registry_sync | 集成 | medium | ❌ |
| subagents_spawn / list / get / cancel | 组合 | low-medium | ✅ |
| browser | 浏览器 | critical | ❌ |
| read_document / read_spreadsheet / read_tool_result | 媒体 | low-medium | ✅/❌ |

**装配入口**：`Assemble(ctx, AssemblyConfig) → AssembledToolsets`

**装配顺序**：Registry 注册 → 配置覆盖 → OpenAPI → workspace_exec → AgentTool → MCP ToolSet → MCP Broker → CustomTools

**MemoryTools 注入**：`AssemblyConfig.MemoryTools` 优先于 `memorytool.DefaultTools()`。Service 层从 `Persist.Memory.TRPC` 获取 `MemoryService`，调用 `Service.Tools()` 过滤（移除 `memory_clear`）后注入 `MemoryTools`。

#### LLM Provider (`internal/provider/`)

**核心函数**：`TRPCModelForProviderModel(provider, model, uc, opts) → trpcmodel.Model`

支持 7 种 Provider：OpenAI（默认）、Anthropic、Gemini、Ollama、Hunyuan、HuggingFace、Bedrock。

**HA 策略**：Failover（`trpcfailover.New`）或 Hedge（`trpchedge.New`）。

#### 记忆系统 (`internal/memory/` + `internal/data/sessionmemory/` + `internal/tools/working_memory/`)

**三种模式**：

| 模式 | 行为 | 接入方式 |
|------|------|---------|
| Agentic (框架) | Agent 主动调用 memory_add/search 等工具 | `Service.Tools()` → 过滤（移除 memory_clear）→ `AssemblyConfig.MemoryTools` + `llmagent.WithMemoryService(service)` |
| Agentic (L1) | Agent 主动调用 working_memory.read/write/list/patch/delete 工具 | `working_memory.ToolSet` → `BeforeToolHook` 注入 L1TaskWriter/L1FieldWriter/L1AdminReader |
| Auto | 对话结束后 LLM 自动提取记忆 | `service.EnqueueAutoMemoryJob(ctx, session)` |

**5 层记忆架构**：

| 层级 | 内容 | 存储 | 核心模块 | 进度 |
|------|------|------|----------|------|
| L0 | 会话快照（Prompt 组装观测 + 压缩摘要） | SQLite Session | `biz/l0_assembly_snapshot.go` + `agent/l0_snapshot_persist.go` + `data/sessionmemory/store_l0_snapshot.go` | 95% |
| L1 | 工作记忆（任务/字段读写 + 归档 + 自动归档 Worker） | SQLite Memory | `biz/memory_admin_store.go`(L1TaskWriter/L1FieldWriter/L1AdminReader) + `tools/working_memory/tools.go` + `agent/working_memory_inject.go` + `cronrunner/jobs/memory_l1_archive.go` | 75% |
| L2 | 会话事件（Episode + 融合召回 + Consolidation Worker） | SQLite + pgvector | `data/sessionmemory/store_episodes.go` + `biz/memory_l2_recall.go` + `cronrunner/jobs/memory_l2_consolidate.go` + `cronrunner/jobs/memory_l2_decay.go` | 85% |
| L3 | 语义知识（Fact CRUD + 冲突检测 + PII 审核 + 5维评分召回） | SQLite + pgvector | `data/sessionmemory/store_facts_ops.go` + `biz/memory_l3_fused_recall.go` + `biz/memory_admin_usecase.go`(DetectFactConflicts) + `data/sessionmemory/store_l3_recall.go` | 80% |
| L4 | 持久进化（知识图谱 + Cascade Saga + 衰减 + 中文 regex） | SQLite Memory | `biz/memory_l4_usecase.go` + `biz/memory_l4_cascade.go` + `data/sessionmemory/entity.go` + `cronrunner/jobs/memory_l4_decay.go` | 75% |

**关键接口拆分**（遵守 ≤5 方法规范）：

| 接口 | 方法数 | 职责 |
|------|--------|------|
| `L1TaskWriter` | 4 | L1 任务写操作（Start/End/Get/Archive） |
| `L1FieldWriter` | 4 | L1 字段写操作（Upsert/Delete/Get/Patch） |
| `L1AdminReader` | 4 | L1 管理读操作（ListTasks/ListFields/GetTask/GetField） |
| `L4EntityStore` | 5 | L4 实体/图操作 |
| `L4EvolutionStore` | 4 | L4 进化操作 |
| `L2ConsolidationStore` | 2 | Episode pending→consolidated 状态机 |
| `L3ConflictStore` | 2 | 冲突检测 + 计数 |
| `PIIReviewStore` | 3 | PII 审核（list/approve/reject） |

**Cron Workers**：

| Worker | 间隔 | 职责 |
|--------|------|------|
| `MemoryL1ArchiveWorker` | 5min | 自动归档空闲 L1 任务（60min 阈值） |
| `MemoryL2ConsolidateWorker` | 10min | pending→consolidated Episode 状态转换 |
| `MemoryL2DecayWorker` | 定期 | Episode importance 衰减 + retention purge |
| `MemoryL3DecayWorker` | 定期 | Fact importance 衰减 |
| `MemoryL4DecayWorker` | 定期 | Entity 指数衰减 + reinforcement |
| `MemoryFactIndexReconciler` | 6h | pgvector 索引 stale/fresh 状态同步 |

#### 会话存储 (`internal/session/`)

适配 `trpcsession.Service`，提供会话快照读写。通过 `SQLiteSessionService` 实现。

#### 会话状态监控 (`internal/biz/session/status*.go` + `internal/service/session_status_guard.go`)

**5 种执行状态**：

| 状态 | 含义 | 可转换到 |
|------|------|---------|
| `idle` | 空闲（默认） | `running` |
| `running` | 执行中 | `completed`/`interrupted`/`awaiting_confirmation` |
| `completed` | 正常完成 | `running` |
| `interrupted` | 非正常中断 | `running` |
| `awaiting_confirmation` | 等待确认（工具/Agent） | `running`/`interrupted` |

**11 种状态原因**：`user_cancelled`/`timeout`/`budget_escalated`/`error`/`context_overflow`/`server_shutdown`/`unexpected_shutdown`/`confirmation_timeout`/`tool_confirmation`/`agent_awaiting_reply`/`manual_override`

**删除保护**：`running` 和 `awaiting_confirmation` 状态禁止删除/归档。

**状态机**：`SessionStatusMachine` 校验合法转换，非法转换返回 `kerrors.BadRequest`。

**端口接口**：`SessionStatusPublisher`（biz 定义，service 实现）—— 发布 `session.status_changed` WS Envelope。

**生命周期守卫**：`SessionStatusGuard` 注册到 Kratos 生命周期钩子：
- `OnStartup`：恢复孤儿 running 会话（标记 `interrupted` + `unexpected_shutdown`）
- `OnShutdown`：批量将 running 会话标记为 `interrupted` + `server_shutdown`

**数据迁移**：`active → idle`，通过 `schema_migrations` 版本门控保证幂等。

#### 技能系统 (`internal/skill/`)

技能导入、执行、Watch 热重载。通过 `trpcskill.Filter` + `trpcskill.Tools` 适配框架。

### 3.2b 横切基础设施模块

#### Chat 活动卡片 (`internal/chatactivity/`)

用户停止生成时，将 tool_running 状态消息卡片标记为已取消。

#### LLM 上下文窗口 (`internal/llmcontext/`)

LLM 上下文窗口大小解析与 Token 估算。`ResolveWindow()` 按优先级解析（provider config → session → agent → 128K 默认）。阈值：warning=60%、critical=80%、exceeded=95%。

#### LLM Provider 探测 (`internal/llminspect/`)

探测远程 LLM Provider 元数据（模型存在性、上下文窗口、定价），支持 OpenRouter/OpenAI/Anthropic/Gemini/Ollama/混元/Bedrock。

#### MCP 服务器管理 (`internal/mcp/`)

6 个子包：config（配置解析）、health（健康检查 Runner）、alert（告警发布）、classify（工具调用分类）、metadata（元数据读写）、probe（探针策略）。

#### Prometheus 指标 (`internal/metrics/`)

30+ 业务指标（`aranea_` 前缀），覆盖 Chat/Agent/EventBus/Graph/Tool/Provider/Plugin/MCP/Channel/Skill 等。

#### 模型注册表 (`internal/modelregistry/`)

LLM 模型目录的文件系统存储、远程同步、定价应用、Provider 迁移。14 个文件。

#### 出站消息路由 (`internal/outbound/`)

统一的出站消息路由层：Router 注册 TextSender/MessageSender，按 channel ID 路由。含 `MessageTool`（message 内置工具）。

**DI 链路**：`provideOutboundRouter` (Wire) → RuntimeTooling → ChatOrchestratorDeps → TRPCBuilderDeps → ToolsetConfig → AssemblyConfig。

**Runtime State 注入**：ChatOrchestrator 在 Runner 构建时注入 `aranea.delivery.channel` + `aranea.delivery.target`，MessageTool 自动从 RuntimeState 解析投递目标。

**SessionResolver**：ChatService 初始化时注册，从 Session Metadata 解析 ChannelID/PeerID，为无显式 target 的出站消息提供回退解析。

**渠道注册辅助**：`RegisterFromInboundEvent()` 函数，供渠道适配器在 Inbound 事件时自动注册 OutboundText sender。

#### ClaudeCode 安全沙箱 (`internal/tools/claudecode_sandbox.go`)

ClaudeCode 工具的安全约束层：`ClaudeCodeSandboxConfig`（BaseDir/ReadOnly/CommandAllowList）+ `SandboxedToolSet` 包装 + `whitelistedBashTool` 命令白名单。当 `CommandAllowList` 非空时，bash 工具仅允许白名单前缀的命令执行。

#### SubAgent 通知闭环 (`internal/tools/subagent/service.go`)

SubAgentService 完成后通过 OutboundRouter.SendText() 通知父 Agent 渠道。并发限制 `defaultMaxConcurrentSubAgents=5`，嵌套派生防护。生命周期由 ChatService 管理（Start/Close）。

#### 组织架构导入 (`internal/orgimport/`)

CLI 驱动的组织架构自动导入（行业→部门→岗位→Agent+Team），纯 HTTP API 通信。

#### 包安装器 (`internal/pkginstall/`)

`aranea pkg install` 命令实现，6 步安装（MCP→Skill→Org→Agent→Team→Graph）。

#### 场景种子 (`internal/scenario/`)

YAML 驱动的行业 Agent/Team 种子数据加载。

#### 遥测 (`internal/telemetry/`)

OpenTelemetry Tracer/Meter Provider 初始化 + Langfuse 可观测性集成。

#### 工作空间 (`internal/workspace/`)

多租户 workspace ID 的 context 传播。`SystemWorkspaceID = "__system__"`。

#### 制品签名 (`internal/artifact/`)

制品下载 URL 的 HMAC-SHA256 签名与验证。

#### 调试录制器 (`internal/debug/`)

Turn 级别调试事件录制，JSONL 输出，safe/full 模式。

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

#### Spirit 动态编排 (`internal/biz/spirit_team_usecase.go` + `internal/biz/spirit_synthesis.go` + `internal/biz/spirit_task_dag.go`)

**核心能力**：
- 动态组装 Team 并行执行任务
- 任务有向无环图（TaskDAG）：依赖验证、环检测、拓扑排序
- 结果综合引擎（SynthesisEngine）：template/prompt/hybrid 三种策略
- 6 种 Spirit 相关 EnvelopeType：spirit_team_assembled/completed/failed/progress、spirit_teams_all_completed、spirit_synthesis_completed

**核心 Usecase**：`SpiritTeamUsecase`（AssembleTeams → ExecuteTaskDAG → SynthesizeResults）

**前端对应**：SpiritEntry/SynthesisResultCard/TeamAssemblyCard/TeamProgressCard/TaskExecutionPanel 组件

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

**FlowTracker（流程追踪）**：TraceEmitter 已从单一 struct 拆分为三层组件：
- **FlowTracker**（`internal/event/flow_tracker.go`）：流程追踪核心，持有 FlowContext + SpanCollector + UsageAggregator，提供 LogStart/LogDone/LogError 等方法
- **SpanCollector**（`internal/event/span_collector.go`）：Span 树管理，管理 LLM/Tool span 生命周期，生成 usage.metadata_json
- **UsageAggregator**（`internal/event/usage_aggregator.go`）：用量聚合，观察 trpc-agent-go 框架事件并聚合 usage 元数据
- **TraceEmitter**（`internal/event/trace_emitter.go`）：v2 embedding wrapper，嵌入 FlowTracker，添加 `ObserveFrameworkEvent` 桥接 trpc-agent-go 事件流

**SinkGroup（独立 goroutine 隔离的 Sink 包装器）**：
- 每个 Sink 由独立 `SinkGroup`（`pkg/logpipeline/sink_group.go`）包装，独立 goroutine + channel 缓冲 + DropPolicy
- 慢 Sink 不影响其他 Sink，Panic 自动恢复
- DropPolicy：`DropNewest`（默认，缓冲区满时丢弃新条目）、`DropBlock`（阻塞调用方）
- Pipeline 内部维护 `[]*SinkGroup`，`AddSink()` 自动包装为默认 SinkGroup

**RuntimeLogAdapter（运行时日志桥接）**：
- `RuntimeLogAdapter`（`internal/adapter/runtime_log.go`）实现 `agentlog.Logger` 接口
- 将 trpc-agent-go 运行时日志（独立 zap.Sugar）桥接到 loggateway Pipeline
- Fatal/Fatalf 特殊处理：直写 stderr + `os.Exit(1)`，不走异步 Pipeline
- 解决了架构偏差 A-2（运行时日志未接入 loggateway）和 A-3（双日志接口无桥接）

**配置驱动 Sink 注册**：
- `conf.proto` 定义 `SinkType`（FILE/STDOUT/EVENTBUS）和 `DropPolicy`（NEWEST/BLOCK）枚举
- `Logging.sinks` 字段（`repeated LoggingSink`）支持配置驱动的 Sink 列表
- `sink_factory.go` 工厂模式：`NewSinkFromConfig(cfg SinkConfig, deps SinkFactoryDeps) → (Sink, error)`
- `SinkConfig` 与 proto 解耦，cmd/admin/main.go 负责转换；EventBus Sink 的 Publisher 通过 `SinkFactoryDeps` 注入

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

#### 学习闭环 (`internal/biz/learning_loop.go`)

Observation → Pattern → Proposal → Validation → Registration 完整学习闭环。Agent 从对话经验中自动识别重复行为模式（tool_call/feedback/memory_hit/memory_miss），生成知识提议，经审批后注册为持久化知识。

**核心 Usecase**：`LearningLoopUsecase`（CollectObservations → DetectPatterns → GenerateProposals → ApproveProposal → RegisterKnowledge）

**数据模型**：`Observation`（原始观察）、`Pattern`（识别的模式，detected/confirmed/dismissed）、`KnowledgeProposal`（知识提议，draft/pending/approved/rejected/applied）

**API**：`LearningLoopService`（6 个 HTTP/gRPC 端点：ListProposals/ListPatterns/ListObservations/ApproveProposal/RejectProposal/RunLoop）

**前端**：AgentLearningLoopPanel（Agent 详情页"学习闭环"Tab），包含 LearningLoopOverview/LearningPatternList/LearningProposalList 组件。

#### 技能自创建 (`internal/biz/skill_evolution.go`)

检测 Agent 重复工具调用模式，自动提议创建新 Skill。完整闭环：检测 → 生成 SKILL.md → 审批 → 注册。

**核心 Usecase**：`SkillEvolutionUsecase`（DetectAndPropose → ApproveProposal → RejectProposal → RegisterApproved）

**端口接口**：`SkillAutoCreator`（LLM 生成 SKILL.md）、`SkillRegistrationPort`（注册 Skill 到仓库）、`SkillProposalReadWriter`（提议持久化）

**数据模型**：`SkillProposal`（pending/approved/rejected/registered/expired）

**触发方式**：Cron 定时任务（`internal/cronrunner/jobs/skill_evolution.go`）或 API 手动触发。

**前端**：待集成（后端 API 已就绪）。

#### 技能管家工具 (`internal/tools/skills_butler/`)

4 个技能管家核心工具，让 `__skills__` Agent 主动进化自身技能：

| 工具 | 功能 | 核心依赖 |
|------|------|---------|
| `evolve_skill` | 基于失败模式分析优化 Skill body，创建新版本 | SkillUsecase + LLM + SkillQueryReader |
| `optimize_skill` | 分析工具权重并生成调整建议 | EvolutionMetricsRepo + ToolInvocationReader + LLM |
| `recommend_skills` | 基于任务描述推荐 Skill 组合 | SkillUsecase.ScoreByEmbedding |
| `analyze_skill_usage` | 分析 Skill 调用频率、成功率、趋势 | SkillQueryReader + EvolutionMetricsRepo |

工具仅对 `__skills__` Agent 注入（`IsSkillsButlerAllowed` 控制），通过 `ChatOrchestrator.skillsButlerTools()` 装配。

#### Skill 渐进加载 (`internal/agent/skill_guidance_inject.go`)

将 Skill Prompt 注入策略从 Eager 全量注入改为 3 阶段渐进加载，通过 `skill_load_mode=progressive` 配置切换：

| 层级 | 内容 | 注入方式 | Token 成本 |
|------|------|---------|-----------|
| L0 | Skill manifest（name + description + `[routed]` 标记） | system prompt | ~3K / 40 skill |
| L1 | SKILL.md 正文 | `skill_load` 工具返回值 | ~2-8K / skill（按需） |
| L2 | 关联引用文件 | `skill_select_docs` 工具返回值 | ~1-5K / doc（按需） |

progressive 模式下：`newSkillGuidanceBeforeHook` 不注入 guidance（返回 nil），LLM 必须通过 `skill_load` 工具按需获取 Skill 正文；自动启用 `WithSkillsLoadedContentInToolResults(true)` 避免 loaded body 再次注入 system prompt。目标降低 Skill 相关 token 消耗 50-80%。

#### 错误处理规范

统一使用 `kerrors`，禁止 `fmt.Errorf` 返回业务错误：

```go
kerrors.BadRequest("AGENT", "id is required")
kerrors.NotFound("AGENT", "agent not found")
kerrors.InternalServer("AGENT", err.Error())
```

**错误吞没禁止**：所有无法返回给调用方的 error 必须通过 `loggateway.Logger` 记录（`lg.Warn`/`lg.Error`），禁止静默丢弃。禁止使用 `log.Printf`/`log/slog`。

### 3.5 自迭代引擎自动化模块

> 以下模块构成项目的自迭代引擎（Self-Iteration Engine），实现从 CI 失败检测到自动修复、从代码变更到文档同步、从手动测试到 E2E 自动化的完整闭环。

#### CI Pipeline (`.github/workflows/`)

**职责**：GitHub Actions CI/CD 流水线，代码推送/PR 时自动执行构建、测试、检查。

| 维度 | 内容 |
|------|------|
| **Job 数量** | 12 个 Job（从初始 6 个扩展） |
| **核心 Job** | `lint`（golangci-lint）、`test`（go test）、`build`（go build）、`api-check`（proto 生成一致性）、`wire-check`（Wire 生成一致性）、`frontend-lint`（ESLint + Prettier）、`frontend-test`（Vitest）、`frontend-build`（Vite build）、`e2e`（Playwright）、`auto-fix`（自动修复）、`release`（GoReleaser）、`doc-sync`（文档同步） |
| **触发条件** | push to main/develop、PR to main、tag push、定时调度 |
| **关键约束** | 所有 Job 必须通过才能合并 PR；`lint` 和 `test` 为硬门禁 |

#### Lint System

**职责**：代码质量门禁，覆盖后端 Go + 前端 TypeScript/Vue。

| 维度 | 内容 |
|------|------|
| **后端 Lint** | golangci-lint（20+ linter 集成）、`go vet`、`goimports` |
| **前端 Lint** | ESLint（Vue/TypeScript 规则）+ Prettier（代码格式化） |
| **提交门禁** | Husky + lint-staged + commitlint（commit message 规范：Conventional Commits） |
| **关键约束** | 提交前自动检查暂存文件；commit message 必须符合 `type(scope): description` 格式 |

#### Auto Fix Engine (`.github/workflows/auto-fix.yml` + `.auto-fix/`)

**职责**：CI 失败自动检测 → LLM 诊断 → 修复生成 → 验证 → PR 创建，形成失败自动修复闭环。

| 维度 | 内容 |
|------|------|
| **核心流程** | CI 失败事件 → 解析失败日志 → LLM 诊断根因 → 生成修复代码 → 本地验证 → 创建修复 PR |
| **失败模式知识库** | `.auto-fix/` 目录存储历史失败模式与修复策略，支持模式匹配和快速诊断 |
| **上游依赖** | CI Pipeline（失败事件触发）、Lint System（修复命令参考） |
| **下游影响** | `.auto-fix/` 目录（知识库）、`.github/workflows/auto-fix.yml`（工作流定义） |
| **关键约束** | 修复 PR 必须通过完整 CI 才能合并；单次修复超时 10 分钟；每日最多 5 次自动修复 |

#### Auto Release Pipeline (`.github/workflows/release.yml` + `.goreleaser.yml`)

**职责**：GoReleaser 驱动的自动化构建/发布/变更日志生成流水线。

| 维度 | 内容 |
|------|------|
| **核心流程** | Tag push → GoReleaser 构建（多平台二进制 + Docker 镜像）→ Staging 冒烟测试 → 生成 Changelog → GitHub Release 发布 |
| **构建产物** | Linux/macOS/Windows 二进制、Docker 镜像（multi-arch）、Changelog |
| **上游依赖** | CI Pipeline（CI 门禁通过后才允许发布） |
| **下游影响** | `.goreleaser.yml`（发布配置）、`.github/workflows/release.yml`（工作流定义）、`Dockerfile`（容器构建） |
| **关键约束** | 发布前必须通过完整 CI + Staging 冒烟测试；Changelog 从 Conventional Commits 自动生成 |

#### Doc Sync Engine (`.github/workflows/doc-sync.yml` + `openspec/changelog/`)

**职责**：代码变更 → 影响分析 → 文档自动更新 → PR 创建，保持文档与代码同步。

| 维度 | 内容 |
|------|------|
| **核心流程** | PR 合并事件 → 变更文件分析 → 确定受影响文档 → LLM 生成文档更新 → 创建文档同步 PR |
| **文档范围** | `openspec/specs/`（架构规格）、`openspec/changelog/`（变更日志）、`README.md`、API 文档 |
| **上游依赖** | CI Pipeline（合并事件触发） |
| **下游影响** | `.github/workflows/doc-sync.yml`（工作流定义）、`openspec/changelog/`（变更日志目录） |
| **关键约束** | 文档同步 PR 需人工审核后合并；仅更新与代码变更直接相关的文档段落 |

#### E2E Testing (`web/e2e/` + `.github/workflows/e2e-nightly.yml`)

**职责**：Playwright E2E 测试框架，覆盖关键用户路径，确保全栈功能正确性。

| 维度 | 内容 |
|------|------|
| **测试框架** | Playwright（支持 Chromium/Firefox/WebKit） |
| **关键路径覆盖** | 登录 → Agent 创建 → 对话 → Team 编排 → Graph 执行 → 渠道集成 |
| **运行策略** | Nightly CI（每日凌晨全量运行）+ PR 关键路径选择性运行 |
| **上游依赖** | Web 前端（测试目标）、后端服务（API 依赖） |
| **下游影响** | `web/e2e/`（测试用例目录）、`.github/workflows/e2e-nightly.yml`（定时工作流） |
| **关键约束** | E2E 测试不阻塞 PR 合并（仅 nightly 报告）；关键路径失败触发告警通知 |

#### Iteration Dashboard (`.github/workflows/iteration-dashboard.yml`)

**职责**：迭代指标采集与周报自动生成，量化自迭代引擎运行效果。

| 维度 | 内容 |
|------|------|
| **采集指标** | Auto Fix 成功率/响应时间、Release 频率/耗时、Doc Sync 覆盖率、E2E 通过率、CI 失败率趋势 |
| **报告输出** | 每周自动生成迭代报告（Markdown），发布到 GitHub Issue / 飞书文档 |
| **上游依赖** | Auto Fix Engine（修复统计数据）、Auto Release Pipeline（发布统计数据） |
| **下游影响** | `.github/workflows/iteration-dashboard.yml`（工作流定义） |
| **关键约束** | 指标数据从 GitHub Actions 运行记录中采集，不侵入业务代码 |

---

## 四、前端模块详解

### 4.1 分层架构

```
services/index.ts (31 个 createXxxService)
  → features/<域>/api.ts (HTTP 门面 + 类型归一化)
    → stores/<域>/index.ts (43 个 Store: 状态 + action)
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
| **useSessionStore** | sessions[]、activeSession | loadSessions、searchPage、batchArchive、**patchSessionStatus** | 全部 Session API |

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

**45 种 Envelope 类型（后端）+ 43 种（前端）**：text_delta、tool_call、tool_result、runner_completion、context_usage、graph_node_start/end、team_run_started/finished、alert.notify、**session.status_changed**、**spirit_team_***、**token_usage** 等。

### 4.4 跨 Store 通信

| 机制 | 生产者 | 消费者 | 说明 |
|------|--------|--------|------|
| **sessionSync 事件总线** | ChatSessionStore、SessionStore | ChatSessionStore | 会话变更通知（remove/update/archive/refresh/**status_changed**） |
| **AppStore → ChatStore** | AppStore | ChatSessionStore、ChatMessageStore | Agent 切换时重置会话和消息 |
| **InboundNotificationStore** | WS 事件 | InboundNotificationBell | 通知铃铛 |
| **MonitorStore → channels/api** | MonitorStore | — | 告警渠道选项加载 |

### 4.5 页面路由

| 路由 | 页面 | 核心功能 |
|------|------|---------|
| `/overview` | OverviewPage | 用量概览 |
| `/chat` | ChatPage | 聊天工作台（Agent 对话 + Team 对话） |
| `/sessions` | SessionsPage | 会话列表 + 搜索 |
| `/sessions/:sessionId` | SessionDetailPage | 会话详情 |
| `/memory` | MemoryCenterPage | 记忆中心（5 层记忆浏览） |
| `/agents` | AgentsPage | Agent 列表 + 创建 |
| `/agents/:id/settings` | AgentSettingsPage | Agent 设置（Prompt/工具/模型/记忆） |
| `/settings/agent-categories` | AgentCategoriesPage | Agent 分类管理 |
| `/team` | TeamsPage | Team 列表 + 编排 |
| `/teams/:teamId/orchestrate` | TeamOrchestratePage | Team 编排界面 |
| `/teams/:teamId/runs/:runId/observatory` | TeamRunObservatoryPage | Team 运行观测 |
| `/graphs` | GraphsPage | Graph 列表 + 编辑器 |
| `/graphs/new` | GraphEditorPage | Graph 新建 |
| `/graphs/:id` | GraphEditorPage | Graph 可视化编辑（Vue Flow 画布 + 属性面板 + 实时校验 + 撤销重做） |
| `/graphs/:id/run/:execId` | GraphRunPage | Graph 执行态（步骤时间线 + 任务看板 + HITL + 检查点回退） |
| `/graphs/:id/executions` | GraphExecutionsPage | Graph 执行历史（服务端过滤 + 状态/时间范围筛选） |
| `/models` | ResourceManagerPage | LLM Provider/Model 管理 |
| `/channels` | ChannelsPage | 渠道管理 |
| `/tools` | ToolsPage | 工具目录 |
| `/tools/audits` | ToolAuditsPage | 工具审计 |
| `/tools/runs` | ToolRunsPage | 工具运行记录 |
| `/monitor/logs` | MonitorPage | 监控日志 + 告警 |
| `/cron` | CronTasksPage | 定时任务 |
| `/hooks` | HooksPage | Webhook 管理 |
| `/hooks/deliveries` | HookDeliveriesPage | Webhook 投递记录 |
| `/webhooks` | WebhooksPage | Webhook 配置 |
| `/knowledge` | KnowledgePage | 知识库 |
| `/artifacts` | ArtifactsPage | 制品管理 |
| `/plugins` | PluginsPage | 插件管理 |
| `/plugins/runs` | PluginRunsPage | 插件运行记录 |
| `/skills` | SkillsPage | 技能管理 |
| `/skills/runs` | SkillRunsPage | 技能运行记录 |
| `/mcp-servers` | McpServersPage | MCP 服务器 |
| `/a2a` | A2APage | A2A 端点 |
| `/evaluation` | EvaluationPage | 评估 |
| `/usage/events` | UsageEventsPage | 用量事件 |
| `/shop` | EcosystemPage | 生态商店 |
| `/industries` | IndustryMarketPage | 行业市场 |
| `/industries/:key` | IndustryDetailPage | 行业详情 |
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
        ├── TransitionSessionStatus → completed / interrupted / awaiting_confirmation
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
  ├── 3. 构建 biz.TurnInput（prepareChannelChatRequest）
  ├── 4. Turn 执行：NativeTurnGateway.ExecuteTurn(TurnInput)
  │     → 复用 Chat Turn 完整流程
  ├── 5. 事件流 → 出站投递队列
  └── 6. ChannelDeliveryWorker 轮询 → OutboundText.SendText()
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
| WSServer → Chat | Wire adapter | 同步调用 | `TurnExecutorGateway`（通过 `wsTurnExecutorAdapter` 桥接） |
| Graph → Agent | 直接 import | 同步调用 | `BuildTRPCAgent` |
| Team → Agent | 直接 import | 同步调用 | `BuildTRPCAgent` |
| Team → Graph | 直接 import | 同步调用 | `GraphBuildConfig` |
| Chat → Agent | 直接 import | 同步调用 | `BuildTRPCAgent` |
| Chat → Tools | 直接 import | 同步调用 | `Assemble` |
| Chat → Provider | 直接 import | 同步调用 | `TRPCModelForProviderModel` |
| Chat → Memory | 直接 import | 同步调用 | `MemoryService.Tools()` → 过滤 → `AssemblyConfig.MemoryTools` |
| Chat → Event | 直接 import | 异步事件 | `Infra.Publish` |
| Channel → Event | 直接 import | 异步事件 | `Infra.Publish` |
| Monitor → Event | 异步消费 | 异步事件 | Bus Consumer |
| Memory → Event | 异步消费 | 异步事件 | Bus Consumer |
| 所有模块 → LogGateway | 直接 import | 同步调用 | `loggateway.Logger` |
| Auto Fix Engine → CI Pipeline | GitHub Actions 事件 | 异步触发 | `workflow_run` 事件 |
| Auto Fix Engine → Lint System | 修复命令参考 | 同步调用 | lint 规则 + fix 命令 |
| Auto Release Pipeline → CI Pipeline | GitHub Actions 门禁 | 异步触发 | `workflow_run` 事件 |
| Doc Sync Engine → CI Pipeline | GitHub Actions 事件 | 异步触发 | `push`/`merge` 事件 |
| E2E Testing → Web 前端 | Playwright 浏览器 | 同步调用 | HTTP + WS |
| E2E Testing → 后端服务 | Playwright API 请求 | 同步调用 | HTTP/gRPC |
| Iteration Dashboard → Auto Fix Engine | GitHub Actions API | 异步采集 | 运行记录查询 |
| Iteration Dashboard → Auto Release Pipeline | GitHub Releases API | 异步采集 | Release 数据查询 |

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
| sessions | ✅ | 会话记录（status: idle/running/completed/interrupted/awaiting_confirmation + status_reason + status_changed_at） |
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
| memory_facts | 10_memory_l2.sql | L3 语义知识（Fact CRUD + 冲突检测 + PII 审核） |
| memory_entities | 10_memory_l3.sql | L4 实体关系图 |
| memory_l4_graph | 10_memory_l4.sql | L4 级联图 |
| memory_episodes | 10_memory_l2.sql | L2 会话事件（Episode + pending/consolidated 状态） |
| memory_l1_tasks | memory_l1.sql | L1 工作记忆任务（active/completed/cancelled + archived_at） |
| memory_l1_fields | memory_l1.sql | L1 工作记忆字段（field_path + revision + visibility） |
| memory_l1_field_history | memory_l1.sql | L1 字段版本历史（归档旧值） |
| flow_log_events | 15_flow_log.sql | Flow Log |
| event_store | 18_event_store.sql | 事件存储 |
| plugin_runs | 13_plugin_run.sql | 插件运行记录 |
| message_fts | 16_message_fts.sql | 消息全文搜索 |
| memory_chain | 16_memory_chain.sql | 记忆链 |
| usage_events / usage_quotas | 08_usage.sql | 用量记录/配额 |
| learning_observations / learning_patterns / learning_proposals | learning_loop.sql | 学习闭环（观察/模式/提议） |
| skill_proposals | skill_evolution.sql | 技能自创建提议 |
| plans | plan.sql | 计划表（goal/steps/status） |
| mcp_servers | mcp_server.go | MCP 服务器定义 |
| channel_turn_jobs | channel_turn_job.go | 渠道 Turn Job 队列 |
| evolution_metrics | evolution_metrics_repo.go | Agent 演化指标 |
| evolution_suggestions | evolution_suggestion_repo.go | Agent 演化建议 |
| compiled_teams | compiled_team_repo.go | 编译后 Team 缓存 |
| orchestration_cache | orchestration_cache_repo.go | 编排缓存 |
| channel_runtime_leases | channel_runtime_lease.go | 渠道运行时租约 |
| channel_peer_sessions | channel_peer_session.go | 渠道对端会话映射 |
| channel_inbound_receipts | channel_inbound_receipt.go | 渠道入站去重收据 |
| team_graph_sessions | team_graph_session_repo.go | Team-Graph-Session 映射 |
| session_run_checkpoints | session_run_checkpoint_repo.go | Session Run 检查点 |
| mcp_user_credentials | mcp_user_credential.go | MCP 用户级凭据 |
| seed_versions | seed_version_repo.go | 种子数据版本号 |
| task_links | task_link.go | 任务关联链接 |

---

## 八、Wire 依赖注入

### 8.1 ProviderSet 组装

```
cmd/admin/wire.go
  ├── server.ProviderSet    — HTTP/gRPC/WS 注册
  ├── data.ProviderSet      — ~60 个 Repo/Adapter 实现
  ├── biz.ProviderSet       — 36 个 Usecase
  ├── event.ProviderSet     — 事件基础设施
  ├── session.ProviderSet   — 会话运行时
  └── service.ProviderSet   — 38 个 Service + 16 条 Wire 接口绑定
```

**Wire adapter 模式**：当 Server 层需要消费 biz 端口接口但不得 import `internal/biz` 时，在 `cmd/admin/wire.go` 中定义 adapter（如 `wsTurnExecutorAdapter` 将 `biz.TurnExecutorGateway` 转换为 `server.WSTurnExecutor`）。

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
wire.Bind(new(a2apkg.AgentTurnRunner), new(*ChatService))
wire.Bind(new(biz.SessionProjection), new(*SessionProjectionAdapter))
wire.Bind(new(biz.EmbeddingService), new(*MemoryEmbeddingAdapter))
wire.Bind(new(biz.SkillEmbedder), new(*knowledge.Embedder))
wire.Bind(new(biz.MemoryTextExtractor), new(*MemoryLLMExtractor))
wire.Bind(new(biz.TeamStarterPort), new(*TeamStarter))
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

新增 CI Job
  → .github/workflows/（添加 Job 到现有 workflow 或新建 workflow）

新增 E2E 测试
  → web/e2e/（Playwright spec 文件）+ .github/workflows/e2e-nightly.yml

新增自动修复模式
  → .auto-fix/（失败模式知识库）+ .github/workflows/auto-fix.yml

新增发布配置
  → .goreleaser.yml + Dockerfile + .github/workflows/release.yml
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

---

## memory-admin-interfaces (from data-architecture-overhaul)

### Requirement: SessionAdminStore deprecated interface migration
The `biz.SessionAdminStore` deprecated composite interface SHALL be replaced by its constituent sub-interfaces in all Wire bindings. Code that depends on `SessionAdminStore` SHALL be migrated to depend on the specific sub-interfaces it needs (e.g., `L0AdminStore`, `L1TaskWriter`, `L3FactReader`).

#### Scenario: Wire binding uses specific sub-interface
- **WHEN** a usecase needs L0 snapshot operations
- **THEN** it SHALL depend on `biz.L0AdminStore`, NOT `biz.SessionAdminStore`

#### Scenario: SessionAdminStore removed from Wire
- **WHEN** all consumers have been migrated to specific sub-interfaces
- **THEN** `biz.SessionAdminStore` SHALL be deleted

### Requirement: CascadeGraphStore split into sub-interfaces
The `biz.CascadeGraphStore` composite interface SHALL be split into `CascadeProposalRepo` and `CascadeSagaRepo`. Consumers SHALL depend on the specific sub-interface they need.

#### Scenario: Cascade proposal operations
- **WHEN** a usecase needs cascade proposal CRUD
- **THEN** it SHALL depend on `biz.CascadeProposalRepo` with methods: `InsertCascadeProposal`, `GetCascadeProposalRow`, `ListCascadeProposalRows`, `UpdateCascadeProposalStatus`

#### Scenario: Cascade saga operations
- **WHEN** a usecase needs cascade saga step management
- **THEN** it SHALL depend on `biz.CascadeSagaRepo` with methods: `InitCascadeSagaSteps`, `GetCascadeSagaSteps`, `UpdateSagaStepState`, `UpdateSagaStepResult`, `HasCascadeSaga`

#### Scenario: CascadeGraphStore removed
- **WHEN** all consumers have been migrated
- **THEN** `biz.CascadeGraphStore` SHALL be deleted

---

## memory-store-decomposition (from data-architecture-overhaul)

### Requirement: Store decomposition into independent Repos
The system SHALL decompose `sessionmemory.Store` (96 methods) into 6 independent Repo structs: `L0SnapshotRepo` (4 methods), `L1WorkingMemoryRepo` (8 methods), `L2EpisodeRepo` (12 methods), `L3FactRepo` (16 methods), `L4EntityRepo` (12 methods), `CascadeRepo` (14 methods). Each Repo SHALL hold `*Data` (not `*ent.Client`).

#### Scenario: Each Repo independently implements biz interfaces
- **WHEN** a biz layer usecase needs L3 fact operations
- **THEN** it SHALL depend on `biz.L3FactReader` / `biz.L3FactWriter` interfaces, implemented by `L3FactRepo`, without depending on other memory layer repos

#### Scenario: No Store.Client() backdoor
- **WHEN** any code needs to execute raw SQL against memory tables
- **THEN** it SHALL use `Data.ExecInTx` / `Data.ClientFromCtx` / `ReadWriteDB`, NOT `Store.Client()`

### Requirement: Wire adapter relocation
All data-layer adapters currently in `cmd/admin/wire_memory.go` SHALL be relocated to `internal/data/`. The `wireSessionAdminStoreAdapter` and `wireL3FactWriterAdapter` SHALL become `internal/data/memory_admin_adapter.go` and `internal/data/memory_l3_fact_writer_adapter.go`.

#### Scenario: Adapter in data layer
- **WHEN** Wire assembles the dependency graph
- **THEN** all data-layer adapters SHALL be in `internal/data/` package, not in `cmd/admin/`

### Requirement: Eliminate Store satisfying biz interfaces directly
`*sessionmemory.Store` SHALL NOT directly implement any biz interface. All biz interface satisfaction SHALL go through explicit adapter structs in `internal/data/`.

#### Scenario: SessionL2RecallStore via adapter
- **WHEN** `biz.MemoryL2RecallUsecase` needs `SessionL2RecallStore`
- **THEN** it SHALL receive an explicit `l2RecallAdapter` struct, NOT `*sessionmemory.Store`

### Requirement: Store method parameters use data-layer DTOs
Store method parameters that currently accept `biz.L0AssemblySnapshotInsert`, `biz.L1TaskInsert`, `biz.L1FieldInsert`, `biz.L1ArchiveEpisodeInsert`, `biz.ReinforcementSignal`, `biz.L4DecayConfig` SHALL be replaced with data-layer DTOs. Conversion SHALL happen in the adapter layer.

#### Scenario: L1 task insert with data DTO
- **WHEN** `L1WorkingMemoryRepo.StartL1Task` is called
- **THEN** it SHALL accept a `data.L1TaskInsert` DTO, and the adapter SHALL convert from `biz.L1TaskInsert` to `data.L1TaskInsert`

### Requirement: Shim migration phase
During migration, each new Repo SHALL delegate to the existing Store methods (shim pattern). This allows incremental migration without breaking existing functionality.

#### Scenario: L3FactRepo delegates to Store
- **WHEN** `L3FactRepo.UpsertFactRow` is called during shim phase
- **THEN** it SHALL delegate to `Store.UpsertFactRow` internally

#### Scenario: Store removal after full migration
- **WHEN** all Store methods have been migrated to independent Repos
- **THEN** the `sessionmemory.Store` struct SHALL be deleted

---

## session-repo-interfaces (from data-architecture-overhaul)

### Requirement: Session repo interfaces for split tables
The `biz.SessionRepo` composite interface SHALL be updated to include new sub-interfaces for `session_metrics` and `session_runtime` tables. The existing `SessionReader`, `SessionWriter`, `ContextUpdater` interfaces SHALL be modified to reflect the table split.

#### Scenario: SessionMetricsReader interface
- **WHEN** a usecase needs to read session metrics
- **THEN** it SHALL depend on `biz.SessionMetricsReader` interface with methods: `GetSessionMetrics`, `BatchGetSessionMetrics`

#### Scenario: SessionMetricsWriter interface
- **WHEN** the delta flush mechanism writes metrics
- **THEN** it SHALL depend on `biz.SessionMetricsWriter` interface with method: `ApplyMetricsDelta`

#### Scenario: SessionRuntimeReader interface
- **WHEN** a usecase needs to read session runtime state
- **THEN** it SHALL depend on `biz.SessionRuntimeReader` interface with methods: `GetSessionRuntime`, `GetSessionRevision`

#### Scenario: SessionRuntimeWriter interface
- **WHEN** runtime state changes during a turn
- **THEN** it SHALL depend on `biz.SessionRuntimeWriter` interface with methods: `PatchSessionState`, `UpdateRunnerSnapshot`, `BumpSessionRevision`

#### Scenario: SessionRepo composite includes new sub-interfaces
- **WHEN** `biz.SessionRepo` is used for Wire binding
- **THEN** it SHALL embed `SessionMetricsReader` + `SessionMetricsWriter` + `SessionRuntimeReader` + `SessionRuntimeWriter` in addition to existing sub-interfaces

---

## session-table-split (from data-architecture-overhaul)

### Requirement: Session table cold-hot split
The system SHALL split the `sessions` table into three tables: `sessions` (cold metadata), `session_metrics` (hot aggregates), and `session_runtime` (runtime state). Each table SHALL have `session_id` as primary key with `session_runtime.session_id` and `session_metrics.session_id` as foreign keys referencing `sessions.id`.

#### Scenario: New session creation writes to all three tables
- **WHEN** a new session is created
- **THEN** the system SHALL INSERT a row into `sessions`, INSERT a row into `session_metrics` with zeroed counters, and INSERT a row into `session_runtime` with initial state

#### Scenario: Session metrics are written asynchronously
- **WHEN** a chat turn completes and metrics delta is flushed
- **THEN** the system SHALL UPDATE `session_metrics` asynchronously via the existing `SessionMetricsDelta` mechanism, without blocking the synchronous write path

#### Scenario: Session runtime state is written synchronously
- **WHEN** runtime state changes (status, state_json, revision, runner_snapshot)
- **THEN** the system SHALL UPDATE `session_runtime` synchronously, merging multiple patches into minimal writes

### Requirement: Session list query with metrics JOIN
The system SHALL support `SearchSessions` queries that LEFT JOIN `session_metrics` to return complete session data in a single query.

#### Scenario: List sessions with metrics
- **WHEN** `SearchSessions` is called
- **THEN** the system SHALL return sessions with metrics fields populated from `session_metrics` table via LEFT JOIN

#### Scenario: Metrics cache hit
- **WHEN** `SearchSessions` is called and session metrics are in the LRU cache
- **THEN** the system SHALL return cached metrics without querying the `session_metrics` table

### Requirement: Session metrics cache
The system SHALL maintain an in-process LRU cache (capacity 500, TTL 30s) for `session_metrics` rows. Cache SHALL be invalidated when metrics are flushed.

#### Scenario: Cache miss triggers DB read
- **WHEN** a session's metrics are not in cache
- **THEN** the system SHALL query `session_metrics` from DB and populate the cache

#### Scenario: Metrics flush invalidates cache
- **WHEN** `ApplyMetricsDelta` writes to `session_metrics`
- **THEN** the system SHALL remove the affected session_id from cache

### Requirement: MetricsUpdated WebSocket event
The system SHALL publish an `EnvelopeTypeMetricsUpdated` event via EventBus when session metrics are flushed, so the frontend can update in real-time.

#### Scenario: Metrics updated event published
- **WHEN** `ApplyMetricsDelta` completes
- **THEN** the system SHALL publish `EnvelopeTypeMetricsUpdated` with session_id and updated metrics fields

### Requirement: Feature flag controlled migration
The session table split SHALL be controlled by a feature flag with three states: `legacy` (write to old sessions columns), `dual_write` (write to both old and new tables), `new_table` (write only to new tables).

#### Scenario: Legacy mode
- **WHEN** feature flag is `legacy`
- **THEN** the system SHALL write metrics/runtime fields to the `sessions` table as before

#### Scenario: Dual write mode
- **WHEN** feature flag is `dual_write`
- **THEN** the system SHALL write to both old `sessions` columns and new tables, reading from new tables

#### Scenario: New table mode
- **WHEN** feature flag is `new_table`
- **THEN** the system SHALL write only to `session_metrics` and `session_runtime`, ignoring old columns in `sessions`

---

## wild-table-ent-migration (from data-architecture-overhaul)

### Requirement: Batch 1 wild tables into Ent Schema
The system SHALL create Ent Schema definitions for the following 6 high-frequency tables: `session_runs`, `session_participants`, `session_run_checkpoints`, `channel_inbound_receipts`, `channel_turn_jobs`, `channel_runtime_lease`. These tables SHALL be managed by Ent's `Schema.Create` for new installations and DDL migration for existing installations.

#### Scenario: New installation creates tables via Ent
- **WHEN** a fresh database is initialized
- **THEN** Ent `Schema.Create` SHALL create these 6 tables with correct columns and indexes

#### Scenario: Existing installation migrates via DDL registry
- **WHEN** an existing database is upgraded
- **THEN** the DDL migration registry SHALL detect missing columns and add them via ALTER TABLE

### Requirement: Batch 2 memory tables into Ent Schema
The system SHALL create Ent Schema definitions for the following 6 memory tables: `memory_facts`, `memory_entities`, `memory_relations`, `memory_episodes`, `memory_l1_tasks`, `memory_l1_fields`. Complex queries (vector search, cascade, JSON aggregation) MAY remain as Raw SQL.

#### Scenario: Memory table schema defined in Ent
- **WHEN** a new column is added to `memory_facts`
- **THEN** the Ent Schema SHALL be the single source of truth for the column definition

#### Scenario: Complex queries remain Raw SQL
- **WHEN** a vector similarity search is needed
- **THEN** the Repo MAY use Raw SQL via `ReadWriteDB`, but the table structure SHALL be defined in Ent Schema

### Requirement: memory_chain.sql deduplication
The system SHALL remove table definitions from `memory_chain.sql` that overlap with Ent Schema definitions (23 tables). `memory_chain.sql` SHALL only contain the 34 Memory-specific tables not managed by Ent.

#### Scenario: Overlapping table removed from SQL file
- **WHEN** a table is defined in both Ent Schema and `memory_chain.sql`
- **THEN** the `memory_chain.sql` definition SHALL be removed, and Ent Schema SHALL be the single source of truth

### Requirement: DDL migration system SQL file support
The `ddl_migration_registry` SHALL support registering SQL file paths (embedded via `go:embed`) in addition to Go functions. This reduces inline SQL strings in Go code.

#### Scenario: Migration from SQL file
- **WHEN** a DDL migration is registered with a `SQL` field pointing to an embedded SQL file
- **THEN** the migration system SHALL read and execute the SQL file contents

### Requirement: Zero wild tables target
The long-term target SHALL be 0 wild tables (all 34 pure-wild tables managed by Ent Schema). Batch 3 (remaining ~28 tables after Batch 1 and 2) SHALL be migrated incrementally after Batch 1 and 2 are stable.

#### Scenario: Wild table count tracking
- **WHEN** a new table is added to the system
- **THEN** it MUST be defined in Ent Schema first, with no raw SQL CREATE TABLE allowed

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

## graph-template (from aranea-pack-import-export)

### Requirement: Graph 模板从 Pack 数据源加载
ListGraphTemplates API SHALL 同时返回内置模板（从 .arpack 加载）和用户模板（从 DB 加载）。

#### Scenario: 内置模板从 Pack 加载
- **WHEN** 调用 ListGraphTemplates API
- **THEN** 系统 SHALL 从 embed 的 builtin-templates.arpack 中读取内置 Graph 模板，与用户模板合并返回

#### Scenario: Pack 中无 Graph 模板
- **WHEN** builtin-templates.arpack 不包含 graphs/ 目录
- **THEN** 系统 SHALL 只返回用户模板，不报错

### Requirement: Graph 模板 YAML 与 Go 结构体互转
系统 SHALL 支持 Graph 模板在 YAML 格式和 GraphTemplate Go 结构体之间互转。

#### Scenario: YAML 转为 GraphTemplate
- **WHEN** 从 Pack 读取 graphs/pipeline.yaml
- **THEN** 系统 SHALL 将其反序列化为 `GraphTemplate` 结构体，与现有 `templates.go` 中的结构一致

#### Scenario: GraphTemplate 转为 YAML
- **WHEN** 导出 Graph 模板
- **THEN** 系统 SHALL 将 `GraphTemplate` 结构体序列化为 YAML 格式写入 Pack

---

## pack-seed-migration (from aranea-pack-import-export)

### Requirement: 内置数据转为 .arpack 格式
系统 SHALL 将现有 YAML 数据源转换为 .arpack 目录结构，通过 go:embed 嵌入二进制。

#### Scenario: 内置模板 Pack
- **WHEN** 应用编译时
- **THEN** `internal/scenario/packs/builtin-templates/` 目录 SHALL 包含 agent templates（fox/programmer/...）和 graph templates（pipeline/approval/...）的 .arpack 格式文件

#### Scenario: 行业 Pack
- **WHEN** 应用编译时
- **THEN** `internal/scenario/packs/finance/`、`internal/scenario/packs/selfmedia/`、`internal/scenario/packs/softwaredev/` 目录 SHALL 包含对应行业的 .arpack 格式文件

### Requirement: 启动时通过 Pack 引擎加载内置种子
系统 SHALL 在启动时使用统一的 Pack 导入引擎加载内置 .arpack 数据，替代 RawSQL 种子。

#### Scenario: P1 阶段加载基础数据
- **WHEN** 应用启动 P1 阶段
- **THEN** 系统 SHALL 加载 `builtin-templates.arpack`（taxonomy + agent templates + graph templates），使用 overwrite 冲突策略

#### Scenario: Lazy 阶段加载行业数据
- **WHEN** 应用启动 Lazy 阶段
- **THEN** 系统 SHALL 依次加载 `finance.arpack`、`selfmedia.arpack`、`softwaredev.arpack`，使用 overwrite 冲突策略

#### Scenario: 版本门控
- **WHEN** 内置 Pack 的版本号与 schema_migrations 表记录一致
- **THEN** 系统 SHALL 跳过该 Pack 的加载

### Requirement: 删除 RawSQL 种子代码
系统 SHALL 删除以下 RawSQL 种子文件，其功能由 Pack 引擎替代。

#### Scenario: 删除 RawSQL 文件
- **WHEN** Pack 种子迁移完成
- **THEN** 系统 SHALL 删除 `seed_industry_agents_rawsql.go`、`seed_builtin_taxonomy.go`（RawSQL 版）、`seed_agent_templates.go`（RawSQL 版）

### Requirement: 废弃 orgimport 包
系统 SHALL 废弃 `internal/orgimport/` 包，其功能由 Pack 导入引擎替代。

#### Scenario: orgimport 标记为废弃
- **WHEN** Pack 导入引擎可用后
- **THEN** `internal/orgimport/` 包 SHALL 标记为 deprecated，并在后续版本中删除

### Requirement: 删除 Go 硬编码 Graph 模板
系统 SHALL 将 `internal/graph/trpc/templates.go` 中的 6 个内置模板迁移到 .arpack 格式。

#### Scenario: 模板从 Go 代码迁移到 YAML
- **WHEN** builtin-templates.arpack 包含 pipeline.yaml、approval.yaml 等 Graph 模板
- **THEN** `templates.go` 中的 `builtinTemplates` 变量 SHALL 从 Pack 数据源加载，而非硬编码

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

---

## seed-version-gating (from modelregistry-refactor)

### 种子数据版本门控 + 分类体系统一

> 日期: 2026-05-30
> 状态: 已批准

#### 一、目标

1. **版本门控**: 配置文件(YAML)驱动的种子数据，仅在版本号变更时录入数据库，日常启动零开销
2. **分类统一**: 废弃 industries/departments/positions 三表，统一为 agent_category_nodes 单表
3. **Agent 模版 YAML 化**: 7 个硬编码模版移入 YAML 配置，录入数据库
4. **Team 行业归属**: Team 模型增加 category_industry_id 字段，显式存储

#### 二、版本门控机制

复用 `schema_migrations` 表，为每类种子分配版本号常量:

```go
const (
    SeedCategoriesV2     = 20260530
    SeedAgentTemplatesV1 = 20260531
    SeedIndustryAgentsV1 = 20260601
)
```

执行流程:
- `isMigrationApplied(version)` → 已录入则跳过
- 未录入 → 从 YAML 加载 → ON CONFLICT DO UPDATE 写入 → `recordMigrationApplied(version, name)`
- 配置变更时递增版本号常量

#### 三、分类体系统一

**删除**:
- `industries`/`departments`/`positions` 三张表及 Ent Schema
- `IndustryUsecase`/`DepartmentUsecase`/`PositionUsecase` 三个 Usecase
- `industryRepo`/`departmentRepo`/`positionRepo` 三个 Repo
- `SeedBuiltinIndustries` 种子函数

**保留**:
- `agent_category_nodes` 单表 (已有 level/parent_id 字段)
- `AgentCategoryUsecase` (扩展方法)
- `SeedBuiltinAgentCategories` (改为从 YAML 加载)

**新增**:
- `agent_category_nodes` 增加 `scenario_key` 字段
- `CategoryAncestors` 类型替代 `PositionAncestors`
- `AgentCategoryUsecase` 增加 `ListByLevel`/`GetAncestors`/`GetPositionPrompt`/`ListPositionVariants` 方法

**IndustryService 改造**:
- 6 个 RPC 保持 HTTP 路由不变(前端兼容)
- 内部改为查询 AgentCategoryUsecase

#### 四、Agent 模版 YAML 化

- 新建 `internal/scenario/agent_templates.yaml`
- 新建 `agent_templates` 数据库表
- `ListAgentTemplates()` 从数据库读取
- 前端删除 `descriptionTemplates` 本地 fallback

#### 五、Team 行业归属

- Team 模型增加 `category_industry_id` 字段
- 创建/编辑时从成员 Agent 推导或用户显式选择
- 前端 `groupTeamsByIndustry` 直接读取字段

#### 六、YAML 配置文件结构

```
internal/scenario/
├── categories.yaml          ← 行业/部门/岗位层级定义
├── agent_templates.yaml     ← Agent 预设模版
├── finance/agents.yaml      ← 金融行业 Agent/Team
├── selfmedia/agents.yaml    ← 自媒体行业 Agent/Team
└── softwaredev/agents.yaml  ← 软件开发行业 Agent/Team
```

#### 七、影响面

- 后端: ~29 个文件
- 前端: ~14 个文件
- Proto: industry.proto 保持 HTTP 路由兼容

#### 八、实施顺序

1. 版本门控种子机制 + categories.yaml
2. 分类体系统一(废弃三表，IndustryService 改造)
3. Agent 模版 YAML 化 + Team 行业归属字段
4. 前端适配
5. aranea-review 审查

---

## architecture (from team-graph-optimization)

### Requirement: GraphBuildConfig field count
The `GraphBuildConfig` struct SHALL contain 11 fields (down from 13). The `FailurePolicy *TeamFailurePolicy` and `ParallelBranchIDs []string` fields SHALL be removed.

#### Scenario: GraphBuildConfig has no Team domain concepts
- **WHEN** `GraphBuildConfig` is defined in `internal/biz/graph.go`
- **THEN** it SHALL NOT contain any field that references Team domain types (`TeamFailurePolicy`, `ParallelBranchIDs`)

#### Scenario: Graph runtime consumes only universal NodeDef fields
- **WHEN** Graph runtime processes a node failure
- **THEN** it SHALL read `NodeDef.FailureAction`, `NodeDef.FallbackAgent`, `NodeDef.RetryMaxAttempts` — NOT `GraphBuildConfig.FailurePolicy`

### Requirement: NodeDef field count
The `NodeDef` struct SHALL contain 20 fields (down from 28). The 8 Task metadata fields SHALL be moved to `NodeTaskMeta`.

#### Scenario: NodeDef contains only graph topology and universal failure fields
- **WHEN** `NodeDef` is defined in `internal/biz/graph.go`
- **THEN** it SHALL NOT contain: `RequiredRole`, `AssignmentMode`, `AssignmentStrategy`, `ReviewerAgent`, `ReviewRules`, `TimeoutSeconds`, `HeartbeatIntervalSeconds`, `EnableLeaseExtension`
