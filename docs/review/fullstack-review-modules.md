# Aranea-Agents 项目模块汇总与关联

> 审查日期：2026-05-29 | 审查工具：aranea-review SKILL

---

## 一、项目概览

Aranea-Agents 是基于 trpc-agent-go 的多智能体编排平台。以 Kratos v2 为传输壳层、trpc-agent-go 为运行时内核。

**技术栈**：Go + Kratos v2（HTTP/gRPC/WebSocket）| trpc-agent-go（Agent 运行时）| Vue 3 + Quasar + Pinia + TypeScript | SQLite（Ent ORM）| Wire（编译期 DI）

**双框架分工**：
- Kratos v2：传输层（HTTP/gRPC/WebSocket）、配置、鉴权、中间件、Wire DI
- trpc-agent-go：Agent 编排（Runner/Agent/Session/Memory/Tool/Event/Skill/Graph/Team）

---

## 二、后端模块清单

### 2.1 核心分层模块

| 模块 | 路径 | 职责 | 核心类型 |
|------|------|------|----------|
| **Server** | `internal/server` | 传输注册 + 中间件 | `ServiceRegistry`, `WSServer`, `RunCanceller`, `ChatSender` |
| **Service** | `internal/service` | proto↔biz 映射 + Runner 编排 | `ChatService`, `ChatOrchestrator`, `AgentService`, `SessionService`, `TeamService`, `ChannelService`, `GraphService`, `MemoryService`, `KnowledgeService`, `SkillService`, `PluginService`, `HookService`, `MonitorService`, `CronService`, `A2AService`, `EvaluationService` 等 35+ Service |
| **Biz** | `internal/biz` | 领域模型 + Usecase + Repo 接口 | `AgentUsecase`, `ChatUsecase`, `SessionUsecase`, `TeamUsecase`, `ChannelUsecase`, `GraphUsecase`, `MemoryUsecase`, `TaskUsecase`, `WebhookUsecase`, `EvolutionUsecase`, `MCPServerUsecase` 等 79 个 NewXxx |
| **Data** | `internal/data` | Repo 实现（Ent ORM + SQLite） | `agentRepo`, `sessionRepo`, `teamRepo`, `toolRepo`, `knowledgeRepo`, `skillRepo`, `memoryRepo`, `graphRepo`, `channelRepo`, `cronRepo`, `monitorRepo`, `mcpServerRepo`, `usageRepo` 等 64 个 NewXxx |

### 2.2 Agent 运行时模块

| 模块 | 路径 | 职责 | 核心类型 |
|------|------|------|----------|
| **Agent** | `internal/agent` | Agent 构建（BuildLLMAgent、Memory、Plugins） | `TRPCBuilderDeps`, `TRPCRunnerDeps`, `BuildCache`, `OpenAICompatLLMCaller`, `DynamicLLMCaller`, `EventProjector`, `BizSessionIngestor` |
| **Agent/Callbacks** | `internal/agent/callbacks` | Agent 回调链 | `Chain` |
| **Agent/Intent** | `internal/agent/intent` | 意图识别 | `Pass` |
| **Agent/Planner** | `internal/agent/planner` | A2UI 规划器 | -- |
| **Agent/CodeExecutor** | `internal/agent/codeexecutor` | 代码执行器 | `Factory` |
| **Team** | `internal/team` | Team 工作流 | `builder`, `definition`, `graph_loader`, `runner`, `summary`, `trpc_build` |
| **Tools** | `internal/tools` | 工具注册中心 + Assemble 装配 | `ToolRegistration`, `AssemblyConfig`, `AssembledToolsets`, `ResultCache` |
| **Provider** | `internal/provider` | LLM 模型驱动 | `RoundTrip`, `ModelCatalogInput`, `CatalogConfig` |
| **Memory** | `internal/memory` | 记忆适配 | `trpc.*` |
| **Session** | `internal/session` | 会话存储适配 | `trpc.Service` |
| **Graph** | `internal/graph` | 图编排 | `GraphAgent`, `Registry`, `EventBridge`, `SQLiteCheckpointSaver`, `ValidationResult` |
| **Runtime** | `internal/runtime` | Runner 生命周期管理 | `RunnerManager`, `RunRegistry`, `LaneScheduler`, `PendingMessageQueue`, `Catalog` |
| **Channel** | `internal/channel` | 渠道集成（飞书/钉钉/企微/Slack/Discord/Telegram 等） | `port.InboundHandler`, `runtime.Manager`, `Runner` |
| **CronRunner** | `internal/cronrunner` | 定时任务调度与执行 | `Runner`, `Deps`, `CronChatRunner` |
| **A2A** | `internal/a2a` | Agent-to-Agent 协议 | `AgentTurnRunner`, `InvokerFunc`, `PublicBaseURLStore`, `EndpointRegistry` |
| **Plugin** | `internal/plugin` | 插件系统 | `Manager`, `Runtime`, `CostGuardPlugin`, `ModelRouterPlugin`, `SensitiveDataMaskPlugin`, `PermissionGuardPlugin`, `AuditLogPlugin` 等 |
| **Evaluation** | `internal/evaluation` | 评估系统 | `Runner`, `FrameworkBridge`, `AfterTurnTrigger` |
| **Knowledge** | `internal/knowledge` | 知识库 | `Chunker`, `Embedder`, `Retriever`, `HybridRetriever`, `AdaptiveRouter`, `FederatedRetriever`, `QueryRewriter` |
| **Compress** | `internal/compress` | LLM 驱动的会话压缩 | `LLMService`, `Compressor` |
| **ModelCatalog** | `internal/modelcatalog` | 模型目录同步/迁移/定价 | `Store`, `Runner`, `Syncer`, `Applier` |
| **Skill** | `internal/skill` | 技能存储/导入 | `importer.Engine`, `storage.*` |

### 2.3 横切关注点模块

| 模块 | 路径 | 职责 | 核心类型 |
|------|------|------|----------|
| **Event** | `internal/event` | 事件总线 + FlowLog + Trace | `Bus`, `Infra`, `Buffer`, `TraceEmitter`, `TurnObserver`, `FlowLogEntry` |
| **Metrics** | `internal/metrics` | 指标回调 | `Callback` |
| **Telemetry** | `internal/telemetry` | 遥测 | `sampler` |
| **MCP** | `internal/mcp` | MCP 默认配置/健康检查/探针 | `Defaults`, `health.Runner`, `probe.Eval` |
| **Artifact** | `internal/artifact` | 制品签名 | `ArtifactService` |
| **Conf** | `internal/conf` | 配置（proto 生成） | `Server`, `Data`, `Features` |
| **CLI** | `internal/cli` | CLI 客户端 | `Client`, `REPL` |
| **LLMContext** | `internal/llmcontext` | LLM 上下文窗口估算 | `Window`, `Metrics` |
| **LLMInspect** | `internal/llminspect` | LLM 模型探查 | `Inspect` |
| **OrgImport** | `internal/orgimport` | 组织导入 | `Loader`, `Applier` |
| **PkgInstall** | `internal/pkginstall` | 包安装 | `Installer`, `Loader` |
| **ChatActivity** | `internal/chatactivity` | 聊天活动取消 | `Cancel` |
| **Workspace** | `internal/workspace` | 工作空间 | -- |

### 2.4 公共包

| 包 | 路径 | 职责 |
|----|------|------|
| `pkg/auth` | 鉴权中间件 | `Auth`, `HealthAuth`, `NewContext` |
| `pkg/safego` | 安全 goroutine | `Go(ctx, name, fn)` |
| `pkg/apierror` | API 错误封装 | `Error` struct |
| `pkg/ctxuser` | 上下文用户提取 | `Get`/`Set` 函数 |
| `pkg/outboundguard` | 出站请求守卫 | `NewClient` |
| `pkg/outboundwebhook` | 出站 Webhook 签名 | `Sign` 函数 |
| `pkg/jsonutil` | JSON 工具 | 工具函数 |
| `pkg/strutil` | 字符串工具 | 工具函数 |
| `pkg/validate` | 校验工具 | `Validate` 函数 |
| `pkg/trpcscope` | 应用作用域 | `App` struct |
| `pkg/webhookurl` | Webhook URL 校验 | `NewOutboundHTTPClient`, `Validate` |
| `pkg/trpc-agent-go` | **Agent 框架真相源** | Runner/Agent/Session/Memory/Tool/Event/Skill/Graph/Team/Model 等 40+ 子包 |

### 2.5 CronRunner Jobs 清单

| Job | 职责 |
|-----|------|
| `AutoMemoryWorker` | 自动记忆提取 |
| `MemoryDeadLetterReplayer` | 记忆死信重放 |
| `MemoryFactIndexReconciler` | 记忆事实索引对账 |
| `MemoryL4DecayWorker` | L4 记忆衰减 |
| `MemoryL3DecayWorker` | L3 记忆衰减 |
| `MemoryL2DecayWorker` | L2 记忆衰减 |
| `MemoryDataMigrationWorker` | 记忆数据迁移 |
| `MemoryEpisodeBackfillWorker` | 记忆片段回填 |
| `MonitorTraceBackfillWorker` | 监控追踪回填 |
| `MonitorAlertCooldownCleanup` | 告警冷却清理 |
| `FlowLogCleanup` | 流日志清理 |
| `ToolAuditCleanup` | 工具审计清理 |
| `EventStoreCleanup` | 事件存储清理 |
| `ChannelDeliveryWorker` | 渠道投递 |
| `ChannelHealthScanner` | 渠道健康扫描 |
| `ProviderHealthScanner` | Provider 健康扫描 |
| `EvolutionScanner` | 进化扫描 |

---

## 三、前端模块清单

### 3.1 服务层

| 模块 | 路径 | 职责 |
|------|------|------|
| **services** | `web/src/services/` | 26 个 `create*Service()` 工厂函数 + `kratosApi`（axios 实例）+ 401/429/5xx 拦截 + 超时策略 |

### 3.2 状态管理

| Store | 路径 | State 要点 | Actions 要点 |
|-------|------|-----------|-------------|
| `useAppStore` | `stores/app.ts` | agents[], selectedAgent | loadAgents, addAgent, removeAgent |
| `useAuthStore` | `stores/auth.ts` | user, sessionChecked | ensureSession, login, logout |
| `useAdminStore` | `stores/admin/` | currentAdmin, loading | loginByUsername, loginByEmail, logout |
| `useAgentsPageStore` | `stores/agents/` | keyword, filters, agents[], total | loadAgentList, validateCreateModel, copyAgent |
| `useAgentDetailStore` | `stores/agents/detail.ts` | loading, saving, previewLoading | fetchById, patch, fetchPromptPreview |
| `useAgentsCatalogStore` | `stores/agents/catalog.ts` | -- | fetchAgents |
| `useAvatarCatalogStore` | `stores/avatar/` | agentsCatalog[], pickerSystem[], pickerMine[] | ensureAgentsCatalog, uploadAvatarFromFile |
| `useChannelsStore` | `stores/channels/` | channels[], catalog[], loading | loadChannels, addChannel, testConnection |
| `useChatSessionStore` | `stores/chat/sessionStore.ts` | sessions[], selectedSession | loadAgentSessions, addAgentSession, removeSessionLocal |
| `useChatMessageStore` | `stores/chat/messageStore.ts` | messagesBySession{}, sessionRevisionBySession{} | getMessages, setMessages, loadMessages |
| `useChatRuntimeStore` | `stores/chat/runtimeStore.ts` | wsConnectedBySession{} | setWsConnected, stop, enqueue, submitFeedback |
| `useChatConversationStore` | `stores/chat/conversationStore.ts` | currentTarget, sessionsById{}, turnsById{} | setCurrentTarget, upsertSession, applyProjection |
| `useCronStore` | `stores/cron/` | tasks[], runs[], agents[], teams[] | loadTasks, addTask, triggerTask |
| `useEcosystemStore` | `stores/ecosystem/` | products[], loading | load, install, publish |
| `useEvaluationStore` | `stores/evaluation/` | datasets[], runs[], agentOptions[] | loadDatasets, startRun, loadRunResults |
| `useEventStore` | `stores/event/` | loading, error | fetchSessionEvents |
| `useGraphStore` | `stores/graph/` | graphs[], activeGraph, templates[] | loadGraphs, runGraph, fetchCheckpoints, timeTravelExecution |
| `useHooksStore` | `stores/hooks/` | hooks[], loading | loadHooks, addHook, saveHook |
| `useHeartbeatStore` | `stores/heartbeat/` | isAlive, lastPongAt | onPong, onDisconnect |
| `useKnowledgeStore` | `stores/knowledge/` | collections[], documentsByCollection{} | loadCollections, ingest, search |
| `useMcpStore` | `stores/mcp/` | servers[], loading | loadServers, addServer, test |
| `useMemoryStore` | `stores/memory/` | snapshots[], facts[], entities[], cascadeProposals[] | loadSnapshots, loadFacts, approveCascade |
| `useMonitorStore` | `stores/monitor/` | auditLogs[], events[], runnerMetrics, alertRules[] | loadAuditLogs, startRuntimeEventsStream, loadAlertRules |
| `useOrchestrationStore` | `stores/orchestration/` | -- | compileTeam, fetchRunObservatory |
| `usePlatformStore` | `stores/platform/` | providerModels[], categoryTree[] | loadProviderModels, loadCategoryTree, checkModel |
| `usePluginsStore` | `stores/plugins/` | plugins[], total, loading | loadPlugins, toggle, setConfig |
| `useSessionStore` | `stores/session/` | sessions[], activeSession, turns, timeline, messages | loadSessions, fetchTurns, fetchMessages, exportSession |
| `useSkillsStore` | `stores/skills/` | skills[], total, loading | loadSkills, toggle, publish |
| `useSystemSettingsStore` | `stores/system-settings/` | settings, loading | loadSettings, saveSettings |
| `useTeamsStore` | `stores/teams/` | teams[], activeTeam, loading | loadTeams, fetchTeam, addTeam |
| `useTeamsPageStore` | `stores/teams/page.ts` | agents[] | loadAgents, loadTeams, testTeam |
| `useToolsStore` | `stores/tools/` | tools[], activeTool, total, summary | loadTools, fetchTool, addTool, fetchEffectiveTools |
| `useToolDetailStore` | `stores/tools/toolDetail.ts` | open, tool, activeTab, overrides[], testResult | openDetail, runToolTest, saveConfig |
| `useToolEditorStore` | `stores/tools/toolEditor.ts` | open, editingId, saving, form, dirty | openCreate, openEdit, save |
| `useUsageStore` | `stores/usage/` | overview, trends[], events[] | loadOverview, loadTrends, exportEventsCsv |
| `useA2AStore` | `stores/a2a/` | agentCards[], auditLog[], remoteAgents[] | discover, invoke, registerRemote |
| `useArtifactStore` | `stores/artifact/` | artifacts[], total, loading | loadArtifacts, upload, signDownload |
| `useInboundNotificationStore` | `stores/inboundNotifications.ts` | items[], unreadCount | upsert, markRead, markAllRead |

### 3.3 域逻辑层

| 域 | api.ts | composable 数量 | types.ts |
|----|--------|-----------------|----------|
| a2a | ✓ | 1 | ✓ |
| admin | ✓ | 0 | -- |
| agents | ✓ | 20+ | ✓ |
| artifact | ✓ | 3 | ✓ |
| avatar | ✓ | 3 | ✓ |
| callback | -- | 0 | -- |
| channels | ✓ | 4 | ✓ |
| chat | ✓ | 17 | ✓ |
| cron | ✓ | 2 | ✓ |
| ecosystem | ✓ | 1 | -- |
| evaluation | ✓ | 1 | ✓ |
| event | ✓ | 0 | -- |
| graph | ✓ | 13 | ✓ |
| heartbeat | ✓ | 1 | -- |
| hooks | ✓ | 0 | ✓ |
| knowledge | ✓ | 2 | ✓ |
| mcp | ✓ | 3 | ✓ |
| memory | ✓ | 1 | ✓ |
| model-catalog | ✓ | 6 | -- |
| monitor | ✓ | 8 | ✓ |
| orchestration | ✓ | 1 | ✓ |
| platform | ✓ | 4 | ✓ |
| plugins | ✓ | 2 | ✓ |
| session | ✓ | 8 | ✓ |
| skills | ✓ | 2 | ✓ |
| system-settings | ✓ | 1 | -- |
| teams | ✓ | 3 | ✓ |
| tools | ✓ | 4 | ✓ |
| usage | ✓ | 4 | ✓ |

### 3.4 展示组件层

| 域 | 组件数 | 关键组件 |
|----|--------|----------|
| a2a | 5 | A2ADiscoverPanel, A2AInvokePanel, A2ARemoteAgentPanel |
| agents | 24 | AgentCard, AgentCreateDialog, AgentAdvancedDialog, AgentToolsSection |
| avatar | 3 | AgentAvatarPicker, AgentAvatarQ, ResolvedAvatarImg |
| channels | 9 | ChannelsTable, ChannelEditorDialog, ChannelCatalogFilters |
| chat | 30+ | ChatWorkspaceShell, ChatComposer, ChatMessagePanel, TurnBlock, BranchTree |
| cron | 4 | CronTaskFormDialog, CronTaskFormFields |
| evaluation | 5 | EvaluationCreateDialog, EvaluationRunDialog, EvaluationResultsDialog |
| graph | 16 | GraphEditorCanvas, GraphFlowNode, GraphPropertyPanel, GraphNodePalette |
| hooks | 2 | HooksTable, CallbackEditor |
| knowledge | 6 | KnowledgeCollectionList, KnowledgeCreateDialog, KnowledgeDocumentsPanel |
| layout | 7 | AppPageHero, AppRegistryTable, AppRegistryPagination |
| mcp | 1 | McpServersTable |
| monitor | 17 | AuditTable, FlowLogStream, FlowTracePanel, RealtimeEvents |
| orchestration | 5 | OrchestrationKanban, OrchestrationActivityTimeline |
| platform | 4 | ProviderModelsTable, ProviderHAConfig, ProviderTrendDialog |
| plugins | 5 | PluginsTable, PluginConfigDialog, PluginSchemaForm |
| sessions | 17 | SessionsTableSection, SessionTimelinePanel, SessionMessagesPanel |
| skills | 8 | SkillTable, SkillEditorDialog, SkillFilterBar |
| teams | 10 | TeamCard, TeamEditorDialog, TeamMemberKanban |
| tools | 22 | ToolsTable, ToolEditorDialog, ToolDetailDrawer, ToolSchemaForm |
| usage | 10 | UsageMetricCards, UsageTrendChart, UsageBreakdownCharts |

### 3.5 页面层

共 40 个页面，覆盖：Login, Agents, AgentSettings, Chat, Sessions, Teams, Graphs, Tools, Skills, Channels, Hooks, Plugins, MCP, Cron, Knowledge, Memory, Evaluation, Artifacts, A2A, ResourceManager, SystemSettings, Overview, UsageEvents, Monitor, Ecosystem, ThemePreview 等。

### 3.6 CSS 主题结构

```
style.sass → css/style.sass → app-global.sass → app-theme.sass → theme/*.sass (15 个 partial)
```

| Partial | 职责 |
|---------|------|
| `_cream-constants.sass` | 奶油色系常量（日间主题基础色） |
| `_css-vars-light.sass` | 日间 CSS 变量定义 |
| `_css-vars-dark.sass` | 夜间 CSS 变量定义 |
| `_page-patterns.sass` | 通用页面布局模式 |
| `_entity-pages.sass` | 实体页面样式 |
| `_glass-dialog.sass` | 玻璃态对话框样式 |
| `_form-layout.sass` | 表单布局 |
| `_settings-panels.sass` | 设置面板样式 |
| `_registry-page.sass` | 注册表页面样式 |
| `_usage-charts.sass` | 用量图表样式 |
| `_sidebar.sass` | 侧边栏样式 |
| `_tech-night.sass` | 暗夜主题特殊样式 |
| `_chat-message-panel.sass` | 聊天消息面板样式 |
| `_graph-pages.sass` | 图编辑器页面样式 |
| `_team-orchestrate.sass` | Team 编排页面样式 |

---

## 四、模块间依赖关系

### 4.1 后端依赖方向图

```
                        ┌─────────────┐
                        │  api/proto  │  (唯一对外契约)
                        └──────┬──────┘
                               │
                        ┌──────▼──────┐
                        │   server    │  传输注册 + 中间件
                        └──────┬──────┘
                               │ import
                        ┌──────▼──────┐
                        │   service   │  proto↔biz 映射 + Runner 编排
                        └──┬───┬───┬──┘
                           │   │   │
              ┌────────────┘   │   └──────────────┐
              │                │                  │
       ┌──────▼──────┐  ┌─────▼─────┐  ┌────────▼────────┐
       │     biz     │  │   agent   │  │    runtime      │
       │ (领域模型)   │  │ (Agent构建)│  │ (Runner管理)    │
       └──┬───┬───┬──┘  └──┬──┬──┬──┘  └──┬──┬──┬──┬────┘
          │   │   │        │  │  │         │  │  │  │
    ┌─────▼─┐ │   │   ┌────▼┐ │  │   ┌────▼┐ │  │  │
    │ event │ │   │   │tools│ │  │   │graph│ │  │  │
    └───────┘ │   │   └──┬──┘ │  │   └──┬──┘ │  │  │
              │   │      │    │  │      │    │  │  │
       ┌──────▼───▼──────▼────▼──▼──────▼────▼──▼──▼──┐
       │               pkg/trpc-agent-go               │
       │         (Agent 框架真相源)                      │
       └───────────────────────────────────────────────┘
              │
       ┌──────▼──────┐
       │     data    │  Repo 实现 (Ent ORM + SQLite)
       └─────────────┘
```

### 4.2 后端依赖矩阵

| 模块 | 依赖的 internal 模块 | 依赖的 pkg 模块 |
|------|---------------------|----------------|
| **server** | `biz`, `conf`, `event`, `service`, `a2a/trpc` | `auth`, `trpc-agent-go` |
| **service** | `biz`, `agent`, `event`, `knowledge`, `graph/adapter`, `tools/kanban`, `plugin/trpc`, `session`, `runtime`, `team`, `tools/trpc`, `a2a`, `compress`, `provider`, `skill/importer`, `metrics`, `chatactivity`, `telemetry/turntrace`, `mcp/config` | `trpc-agent-go/*` |
| **biz** | `event`, `event/contract`, `modelcatalog`, `biz/knowledge`, `biz/skill`, `biz/tool`, `biz/session`, `biz/monitor`, `biz/artifact`, `biz/shared` | **无框架依赖** |
| **data** | `biz`, `event`, `tools`, `llmcontext` | `trpc-agent-go/session`, `trpc-agent-go/memory` |
| **agent** | `biz`, `event`, `provider`, `knowledge`, `tools`, `plugin/trpc`, `agent/callbacks`, `agent/codeexecutor`, `mcp/config`, `skill/storage` | `trpc-agent-go/*` |
| **tools** | `biz`, `event`, `knowledge`, `a2a`, `mcp`, `mcp/config`, `metrics` | `trpc-agent-go/tool` |
| **event** | `event/contract`, `metrics` | 无 |
| **graph** | `biz`, `event`, `agent`, `provider`, `tools` | `trpc-agent-go/graph`, `trpc-agent-go/agent` |
| **channel** | `biz`, `event`, `channel/port`, `channel/runtime`, `metrics` | 无 |
| **cronrunner** | `biz`, `data`, `event`, `memory/trpc`, `metrics`, `service` | `trpc-agent-go/session` |
| **runtime** | `biz`, `event`, `session`, `provider`, `agent`, `graph/trpc`, `session/trpc` | `trpc-agent-go/*` |
| **plugin** | `biz`, `event`, `metrics` | `trpc-agent-go/agent`, `trpc-agent-go/tool`, `trpc-agent-go/plugin` |
| **provider** | 无 internal 依赖 | `trpc-agent-go/model` |
| **knowledge** | `biz` | `trpc-agent-go/model` |
| **evaluation** | `biz`, `provider` | `trpc-agent-go/evaluation`, `trpc-agent-go/runner` |
| **a2a** | `biz` | `trpc-agent-go/agent`, `trpc-agent-go/server/a2a` |

### 4.3 前端数据流

```
services/index.ts (createXxxService)
  → features/<域>/api.ts (HTTP 门面 + 类型归一化)
    → stores/<域>/index.ts (状态 + action 调 api)
      → features/<域>/useXxxPage.ts (composable 组合 Store)
        → pages/XxxPage.vue (布局 + 传参)
          → components/<域>/*.vue (纯展示：props in / emits out)
```

**实时数据流**（Chat / Monitor / Team / Graph）：

```
realtime/ws-transport.ts → realtime/envelope.ts → features/chat/dispatcher.ts
    → features/chat/streamHandlers.ts → stores/chat/messageStore.ts
    → features/chat/composables/useChatStreamManager.ts → components/chat/*.vue
```

**跨 Store 同步**：

```
stores/sessionSync.ts (事件总线)
    ← emitSessionMutation() — 由 useSessionStore / useChatSessionStore 发布
    → onSessionMutation() — 由 useChatSessionStore 订阅
```

---

## 五、核心业务流程

### 5.1 聊天流程

```
用户消息 → ChatService → ChatOrchestrator → TurnPipeline
  → Runner 装配（agent.BuildTRPCLLMAgent + tools.Assemble）
    → trpc-agent-go Runner 执行
      → LLM 调用（provider.TRPCModelForProviderModel）
      → 工具调用（tools.Assemble 装配的工具集）
      → 事件流（event.Bus → WebSocket → 前端）
  → 事件持久化（event_persist_handler → asyncEnvelopeWorker）
  → 记忆提取（TurnMemoryWorker → AutoMemoryQueue）
```

### 5.2 Team 编排流程

```
Team 定义 → TeamService → TeamUsecase
  → Team Builder 构建成员 Agent
    → 每个 Agent 通过 agent.BuildTRPCLLMAgent 构建
    → 工具通过同一 tools.Assemble 装配
  → trpc-agent-go Team Runtime 执行
  → 事件流同 Chat
```

### 5.3 Graph 工作流

```
Graph 定义 → GraphService → GraphUsecase
  → GraphBuilderFactory 创建图运行时
  → GraphAgent 封装节点执行
  → SQLiteCheckpointSaver 持久化检查点
  → 支持暂停/恢复/时间旅行
```

### 5.4 渠道集成流程

```
第三方消息 → ChannelIngress → ChannelUsecase
  → 渠道适配器解析（lark/dingtalk/wecom/slack/discord/telegram 等）
  → 转换为统一 InboundEvent
  → 通过 TurnGateway 执行 Turn
  → 出站通过渠道适配器回复
```

### 5.5 知识库流程

```
文档上传 → KnowledgeService → KnowledgeUsecase
  → OCR（可选）→ Chunker 分块 → Embedder 嵌入 → 向量存储
  → 查询时：QueryRewriter 改写 → HybridRetriever 混合检索 → Reranker 重排
  → 自适应路由：AdaptiveRouter 根据查询类型选择检索策略
```
