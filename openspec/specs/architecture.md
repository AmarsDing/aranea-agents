# 架构规范

> 来源：`docs/architecture-blueprint.md` 精简版。聚焦模块结构与职责，去除叙述性内容。

---

## 一、项目定位

基于 **trpc-agent-go** 的多智能体编排平台。**Kratos v2** 为传输壳层，**trpc-agent-go** 为运行时内核。

| 框架 | 职责 | 禁止 |
|------|------|------|
| Kratos v2 | HTTP/gRPC/WebSocket 传输、配置、鉴权、中间件、Wire DI | 不承载 Agent 编排、不实现第二套事件循环 |
| trpc-agent-go | Agent 编排（Runner/Agent/Session/Memory/Tool/Event/Skill/Graph/Team） | 不直接写业务数据库、不处理 HTTP 路由 |

**框架真相源**：`pkg/trpc-agent-go` 是 Agent 框架唯一真相源。先查框架 API 后再实现。

---

## 二、后端模块结构

### 2.1 Kratos 标准四层

```
api/**/*.proto          ← 唯一对外契约
        ↓
internal/service        ← 传输桥点：proto ↔ biz 映射 + Runner 编排
        ↓
internal/biz            ← 领域模型 + Usecase + Repo 接口定义
        ↓
internal/data           ← Repo 实现（Ent ORM + SQLite）
```

**跨层只允许向内依赖。**

#### Server 层 (`internal/server/`)

| 文件 | 功能 |
|------|------|
| `http.go` | HTTP Server，注册 `RegisterXxxHTTPServer`，中间件链：recovery → tracing → logging → auth → cors |
| `grpc.go` | gRPC Server，注册 `RegisterXxxServiceServer` |
| `ws.go` | WebSocket Server，`/v1/ws`，消息路由到 `TurnExecutorGateway` |
| `server.go` | 统一 Server 工厂 |

**约束**：不得 new `runner.Runner` 或 `llmagent.New`。

#### Service 层 (`internal/service/`)

proto ↔ biz 类型映射 + Runner 编排。Wire 装配中心，唯一允许创建 Runner 的层。

| Service | Proto | Biz 依赖 | Runner 装配 |
|---------|-------|---------|------------|
| **ChatService** | chat/v1 | ChatOrchestrator | 实现 `NativeTurnGateway`/`TurnControlGateway`/`DurableResumeGateway`/`A2ARunnerFactory` |
| **AgentService** | agent/v1 | AgentUsecase | 无（纯 CRUD） |
| **TeamService** | team/v1 | TeamUsecase | 无 |
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

**ChatOrchestrator** 核心编排器，实现 `biz.TurnExecutor`：准入控制 → Agent 构建 → Runner 创建 → Turn 执行 → 事件流处理。

#### Biz 层 (`internal/biz/`)

领域模型 + Usecase + Repo 接口定义 + 跨模块端口接口。

**禁止**：import `pkg/trpc-agent-go` 和 `api/*/v1`。

**核心 Usecase**：

| Usecase | 职责 | 关键依赖 |
|---------|------|---------|
| AgentUsecase | Agent CRUD、Prompt 管理、Token 估算 | `AgentRepository`、`ToolCatalogReader`、`SystemSettingRepo` |
| ChatUsecase | Turn 生命周期、Run 状态、待处理队列 | `ChatRunGateway`、`ChatSessionLocker`、`ChatPendingQueue` |
| TeamUsecase | Team CRUD、定义验证（6 种模式） | `TeamRepository`、`AgentIDExistenceChecker` |
| GraphUsecase | Graph 定义/执行/缓存、GC | `GraphRepo`、`GraphRunRepo`、`GraphBuilderFactory` |
| SessionUsecase | 会话 CRUD、消息持久化、5 态状态机、删除保护 | `SessionRepo`、`SessionRunRepo`、`SessionStatusPublisher` |
| ChannelUsecase | 渠道 CRUD、入站路由、出站投递 | `ChannelRepo`、`ChannelPeerSessionRepo`、`CredentialCrypto` |
| CronUsecase | 定时任务 CRUD、调度 | `CronRepo`、`NativeTurnGateway` |
| MemoryUsecase | 记忆 CRUD、PII 检测 | `MemoryRepo`、`EmbeddingService` |
| MonitorUsecase | 告警规则、Flow Log | `MonitorRepo`、`UsageRepo` |

**跨模块端口接口**（定义在 biz，Wire 绑定在 service）：

| 端口接口 | 消费者 |
|---------|--------|
| `TurnExecutorGateway` | WSServer |
| `TurnRunControlGateway` | DurableWorker |
| `TurnControlGateway` | ChannelIngress |
| `NativeTurnGateway` | Channel/Cron |
| `DurableResumeGateway` | DurableWorker |
| `A2ARunnerFactory` | A2AService |
| `CronTriggerGateway` | ChannelIngress |
| `GraphExecutor` | Channel/Cron |
| `GraphBuildConfig` | Team |
| `GraphRuntime` | Graph |

#### Data 层 (`internal/data/`)

实现 biz 定义的 Repo 接口，封装 Ent ORM + 原生 SQL。

| 数据库 | 用途 | 访问方式 |
|--------|------|---------|
| **SQLite** | 主数据库 | `d.Ent()` + `d.rawDB` |
| **PostgreSQL** | 向量存储（Memory/Knowledge） | `d.Postgres()`（可选） |

**42 个 Repo 实现**，每个有编译期接口检查 `var _ biz.XxxRepo = (*xxxRepo)(nil)`。

### 2.2 Agent 运行时模块

| 模块 | 路径 | 职责 |
|------|------|------|
| Agent 构建 | `internal/agent/` | `BuildTRPCLLMAgent` → trpcagent.Agent；`NewTRPCRunner` → ManagedRunner；`RunTRPCUserTurn` → event chan |
| 工具装配 | `internal/tools/` | 18 内置工具 + AgentTool + MCP + Custom；`Assemble(ctx, AssemblyConfig) → AssembledToolsets` |
| LLM Provider | `internal/provider/` | 7 种 Provider（OpenAI/Anthropic/Gemini/Ollama/Hunyuan/HuggingFace/Bedrock）；HA: Failover/Hedge |
| 记忆系统 | `internal/memory/` | Agentic（工具调用）/ Auto（自动提取）；5 层：L0 快照 → L1 字段 → L2 事实 → L3 实体 → L4 级联 |
| 会话存储 | `internal/session/` | 适配 `trpcsession.Service`，`SQLiteSessionService` |
| 技能系统 | `internal/skill/` | 技能导入、执行、Watch 热重载 |

### 2.3 编排模块

| 模块 | 路径 | 模式/能力 |
|------|------|----------|
| Team | `internal/team/` | Sequential/Parallel/Coordinator/CriticLoop/Swarm/Adaptive；默认走 GraphAgent 编译路径 |
| Graph | `internal/graph/` | 可视化图定义、条件分支/并行/循环、HITL、检查点+时间旅行、模板、版本管理、变量引用、失败策略 |
| Channel | `internal/channel/` | 12+ 平台；Runner/OutboundText/InboundHandler 三层接口 |
| Cron | `internal/cronrunner/` | `cronrunner.Runner` 管理 Cron 调度，通过 `NativeTurnGateway` 执行 Turn |

### 2.4 横切模块

| 模块 | 路径 | 职责 |
|------|------|------|
| 事件系统 | `internal/event/` | 双总线：SessionBus（WS 推送）+ MonitorBus（Flow Log/告警）；投递策略：DropOldest/DropNewest/BlockUpTo |
| 运行时依赖 | `internal/runtime/` | `TurnDeps`：Catalog + Persist + Pipeline + RunnerMgr |
| 上下文压缩 | `internal/compress/` | L0 压缩：LLM 摘要替换历史，CAS + 事务保证原子性 |
| 知识库 | `internal/knowledge/` | 上传 → OCR → 分块 → Embedding → pgvector → 检索 |
| 评估 | `internal/evaluation/` | LLM Judge 评估框架 |
| A2A | `internal/a2a/` | Agent-to-Agent 通信协议 |
| 插件 | `internal/plugin/` | 生命周期管理 + 回调链 + 费用守卫 |
| 模型目录 | `internal/modelcatalog/` | LLM 模型目录同步 + 定价 |

---

## 三、会话状态机

### 5 种执行状态

| 状态 | 可转换到 |
|------|---------|
| `idle` | `running` |
| `running` | `completed` / `interrupted` / `awaiting_confirmation` |
| `completed` | `running` |
| `interrupted` | `running` |
| `awaiting_confirmation` | `running` / `interrupted` |

### 11 种状态原因

`user_cancelled` / `timeout` / `budget_escalated` / `error` / `context_overflow` / `server_shutdown` / `unexpected_shutdown` / `confirmation_timeout` / `tool_confirmation` / `agent_awaiting_reply` / `manual_override`

### 删除保护

`running` 和 `awaiting_confirmation` 状态禁止删除/归档。

### 生命周期守卫

- **OnStartup**：恢复孤儿 running 会话（标记 `interrupted` + `unexpected_shutdown`）
- **OnShutdown**：批量将 running 会话标记为 `interrupted` + `server_shutdown`

---

## 四、前端模块结构

### 分层

```
services/index.ts (25 个 createXxxService)
  → features/<域>/api.ts (HTTP 门面 + 类型归一化)
    → stores/<域>/index.ts (36 个 Store: 状态 + action)
      → features/<域>/useXxxPage.ts (composable 组合 Store)
        → pages/XxxPage.vue (布局 + composable 绑定)
          → components/<域>/*.vue (纯展示: props in / emits out)
```

### 核心域 Store

| Store | 关键 Action |
|-------|------------|
| useAppStore | loadAgents、addAgent、removeAgent |
| useChatSessionStore | loadAgentSessions、addAgentSession、removeSessionLocal |
| useChatMessageStore | loadMessages、setMessages、clearSessionMessages |
| useChatRuntimeStore | fetchRunStatus、stop、enqueue |
| useChatConversationStore | setCurrentTarget、upsertSession、applyProjection |
| useGraphStore | loadGraphs、runGraph、validateGraphDefinition、saveCheckpoint |
| useTeamsStore | loadTeams、addTeam、editTeam |
| useToolsStore | loadTools、fetchCatalog、saveOverride |
| useMonitorStore | loadAuditLogs、startRuntimeEventsStream |
| useSessionStore | loadSessions、searchPage、batchArchive、patchSessionStatus |

### 实时通信

```
WebSocket Server (/v1/ws)
  → ws-transport.ts → globalWsHub.ts
    → useEnvelopeStream.ts → dispatcher.ts
      → features/chat/composables/useChatStreamManager.ts
      → features/monitor/api.ts
      → features/teams/api.ts
      → features/graph/runtime/useGraphExecutionStream.ts
      → features/orchestration/useOrchestrationStream.ts
```

**47 种 Envelope 类型**。

### 跨 Store 通信

| 机制 | 生产者 | 消费者 |
|------|--------|--------|
| sessionSync 事件总线 | ChatSessionStore、SessionStore | ChatSessionStore |
| AppStore → ChatStore | AppStore | ChatSessionStore、ChatMessageStore |
| InboundNotificationStore | WS 事件 | InboundNotificationBell |

---

## 五、数据库 Schema

### 核心表（Ent Schema）

| 表 | 用途 |
|----|------|
| agents | Agent 定义 |
| sessions | 会话记录（5 态 status + status_reason + status_changed_at） |
| messages | 聊天消息 |
| session_turns | Turn 记录 |
| session_runs | Run 记录 |
| teams / team_runs | Team 定义/运行 |
| tools / hooks / plugins / crontasks / admins / system_settings | 辅助 |

### 扩展表（原生 SQL）

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

## 六、Wire 依赖注入

```
cmd/admin/wire.go
  ├── server.ProviderSet    — HTTP/gRPC/WS 注册
  ├── data.ProviderSet      — 42 个 Repo 实现
  ├── biz.ProviderSet       — ~35 个 Usecase
  ├── event.ProviderSet     — 事件基础设施
  ├── session.ProviderSet   — 会话运行时
  └── service.ProviderSet   — ~25 个 Service + Wire 接口绑定
```

**关键 Wire 绑定**（`service.go`）：

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

## 七、开发决策树

| 新增功能 | 改动路径 |
|---------|---------|
| HTTP/gRPC 接口 | `api/**/*.proto` → `make api` → `internal/service` → `internal/server` |
| 业务逻辑 | `internal/biz`（模型 + Repo 接口 + Usecase） |
| 数据库表 | `internal/data/ent/schema` → `go generate` → `internal/data` |
| LLM Agent 能力 | `internal/agent`（BuildLLMAgent 扩展） |
| 程序化 Agent | `internal/agent`（实现 `agent.Agent` 接口 + Runner 包装） |
| 工具 | `internal/tools`（Registry 注册 + `builtin_tools_seed.go` 种子） |
| Team 编排模式 | `internal/team`（BuildWorkflowRoot） |
| LLM 厂商 | `internal/provider`（实现 `model.LLM`） |
| 记忆能力 | `internal/memory`（适配器 → trpcmemory.Service） |
| 渠道平台 | `internal/channel/<platform>`（实现 Runner + OutboundText） |
| 定时同步任务 | `internal/agent` → `internal/cronrunner` → `cmd/admin/wire.go` |
| 横切关注点 | `internal/server` + `pkg/auth` |
| 前端页面 | `features/<域>/api.ts` → `stores/<域>/` → `pages/XxxPage.vue` → `components/<域>/` |
| 前端 Store | `stores/<域>/index.ts` → `stores/index.ts` 具名导出 |
