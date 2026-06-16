# trpc-agent-go 框架架构 vs Aranea-Agents 项目实现对比分析

> 生成日期：2026-06-16
> 分析范围：trpc-agent-go 框架全部技术文档（42 篇）、全部示例代码（27 个）、全部模块源码（17 个核心模块）、Aranea-Agents 项目实现代码

---

## 一、总体评估

### 合规度评分

| 维度 | 合规度 | 说明 |
|------|--------|------|
| 核心架构遵循 | ★★★★☆ | Runner→Agent→Model→Tool→Session 主链路完全遵循框架设计 |
| 接口实现完整性 | ★★★★☆ | 框架核心接口均有实现，但部分接口有自定义扩展 |
| 扩展点利用 | ★★★★★ | 充分利用了框架的 Option/Callback/Plugin/Extension 机制 |
| 框架功能覆盖 | ★★★☆☆ | 框架 17 个模块中 4 个未使用（Evaluation/PromptIter/Knowledge/Artifact COS/S3），多个模块仅部分使用 |
| 自建 vs 框架比例 | ★★★☆☆ | 大量核心功能为自建（EventBus、Memory L0-L4、Session 压缩、Team 编排），框架仅作为运行时内核 |

### 核心结论

Aranea-Agents 项目**以 trpc-agent-go 为运行时内核**，而非完整采用框架架构。项目遵循了框架的 Runner→Agent→Model→Tool→Session 主链路，但在以下方面进行了大规模自建：

1. **事件系统**：框架提供 `<-chan *event.Event` 通道，项目自建了完整的双总线 EventBus + WAL + 可靠性分级
2. **记忆系统**：框架提供 `memory.Service` 接口 + 8 种后端，项目完全自建了 L0-L4 四层记忆体系
3. **会话管理**：框架提供 Session Service + Summary，项目自建了完整的压缩管道
4. **团队编排**：框架提供 Team 模块，项目自建了更复杂的 Team Runner + 多模式编排
5. **可观测性**：框架提供 Plugin + Callback，项目自建了 Callback Chain + 熔断器 + Activity Projector

---

## 二、逐模块对比详情

### 2.1 Runner 模块

| 对比项 | 框架设计 | 项目实现 | 合规性 |
|--------|---------|---------|--------|
| 核心接口 | `Runner.Run()` → `<-chan *event.Event` | 完全遵循，`RunTRPCUserTurn()` 封装 `r.Run()` | ✅ 完全合规 |
| ManagedRunner | `Cancel(requestID)` + `RunStatus(requestID)` | 完全使用，`CancelTRPCRun()` / `TRPCRunStatus()` | ✅ 完全合规 |
| SteerableRunner | `EnqueueUserMessage(requestID, msg)` | 完全使用，`EnqueueTRPCUserMessage()` | ✅ 完全合规 |
| AgentFactory | `NewRunnerWithAgentFactory()` | 完全使用，`bizAgentFactoryForKey()` 实现动态 Agent 构建 | ✅ 完全合规 |
| SessionService 注入 | `WithSessionService()` | 完全使用，SQLite 后端 | ✅ 完全合规 |
| MemoryService 注入 | `WithMemoryService()` | 完全使用，自建 SQLite/pgvector 实现 | ✅ 接口合规 |
| ArtifactService 注入 | `WithArtifactService()` | 完全使用，自建 ServiceAdapter | ✅ 接口合规 |
| Plugin 注入 | `WithPlugins()` | 完全使用，通过 PluginManager 管理 | ✅ 完全合规 |
| RalphLoop | `WithRalphLoop()` | 完全使用，从 AgentRuntimeSettings 解析 | ✅ 完全合规 |
| Runner 生命周期 | 框架无内置管理 | 自建 `RunnerManager` + `RunRegistry` | ⚠️ 框架缺失，项目自建 |
| Runner 缓存 | 框架无内置 | 自建 `RunnerInstanceRegistry`（Team 长期 Runner） | ⚠️ 框架缺失，项目自建 |

**偏差说明**：
- 框架的 Runner 是无状态的，每次 `Run()` 独立执行。项目需要 Runner 生命周期管理（跨 Turn 保持 Team Runner），因此自建了 `RunnerManager`。
- 框架的 `AgentFactory` 仅支持按 name 查找，项目扩展为按 `agent_key` 从数据库动态构建。

---

### 2.2 Agent 模块

| 对比项 | 框架设计 | 项目实现 | 合规性 |
|--------|---------|---------|--------|
| LLMAgent 创建 | `llmagent.New(name, ...Option)` | 完全遵循，`BuildTRPCLLMAgent()` | ✅ 完全合规 |
| Model 注入 | `WithModel()` | 完全使用，通过 `TRPCModelForProviderModel()` 构建 | ✅ 完全合规 |
| Instruction | `WithInstruction()` | 完全使用，从 Agent 数据库配置读取 | ✅ 完全合规 |
| ToolSets | `WithToolSets()` | 完全使用，`buildToolsetsForAgent()` 组装 | ✅ 完全合规 |
| Tools | `WithTools()` | 完全使用，与 ToolSets 配合 | ✅ 完全合规 |
| Skills | `WithSkills()` | 完全使用，FS/DB 双 Repository | ✅ 完全合规 |
| SkillFilter | `WithSkillFilter()` | 完全使用，`AgentVisibilityFilter` | ✅ 完全合规 |
| CodeExecutor | `WithCodeExecutor()` | 完全使用 | ✅ 完全合规 |
| Planner | `WithPlanner()` | 完全使用 | ✅ 完全合规 |
| ModelSelector | `WithModelSelector()` | 完全使用，多选择器链式组合 | ✅ 完全合规 |
| Callbacks | `WithAgentCallbacks()` / `WithModelCallbacks()` / `WithToolCallbacks()` | 完全使用，通过 Callback Chain 适配 | ✅ 完全合规 |
| EnableContextCompaction | `WithEnableContextCompaction()` | 完全使用 | ✅ 完全合规 |
| AddSessionSummary | `WithAddSessionSummary()` | 完全使用 | ✅ 完全合规 |
| PreloadMemory | `WithPreloadMemory()` | 完全使用 | ✅ 完全合规 |
| EnableParallelTools | `WithEnableParallelTools()` | 完全使用 | ✅ 完全合规 |
| ToolFilter | `WithToolFilter()` | 完全使用，基于 DeferredManager | ✅ 完全合规 |
| ToolCallRetryPolicy | `WithToolCallRetryPolicy()` | 完全使用 | ✅ 完全合规 |
| OutputSchema | `WithOutputSchema()` | 完全使用 | ✅ 完全合规 |
| SurfacePatch | `WithSurfacePatch()` | 完全使用，Graph 节点级配置差异 | ✅ 完全合规 |
| Agent 缓存 | 框架无内置 | 自建 `BuildCache`（LRU + singleflight + dirty-mark） | ⚠️ 框架缺失，项目自建 |
| LazyAgent | `NewLazyAgent()` | 未使用，项目使用 AgentFactory 替代 | ℹ️ 替代方案 |

**偏差说明**：
- 框架的 `LazyAgent` 通过 `AgentFactory` 延迟构造，项目使用 `AgentFactory` 注册到 Runner 中，功能等价但集成方式不同。
- Agent Build Cache 是项目的重要优化：框架每次 `Run()` 都需要重新构建 Agent，项目通过缓存避免重复构建。缓存键排除了 Provider/Model（通过 RunOption 运行时覆盖），这是一个精妙的设计。

---

### 2.3 Model 模块

| 对比项 | 框架设计 | 项目实现 | 合规性 |
|--------|---------|---------|--------|
| Provider 注册 | `provider.Register(name, constructor)` | 完全使用，额外注册 huggingface/bedrock | ✅ 完全合规 |
| Provider.Model | `provider.Model(providerName, modelName, opts...)` | 完全使用，`TRPCModelForProviderModel()` | ✅ 完全合规 |
| OpenAI 配置 | `WithAPIKey` / `WithBaseURL` / `WithModel` 等 | 完全使用，含 Cache/Reasoning/ContextWindow 扩展 | ✅ 完全合规 |
| Anthropic 配置 | `WithAPIKey` / `WithModel` 等 | 完全使用，含 Cache 扩展 | ✅ 完全合规 |
| Gemini 配置 | `WithClientConfig` 等 | 完全使用 | ✅ 完全合规 |
| Ollama 配置 | `WithKeepAlive` 等 | 完全使用 | ✅ 完全合规 |
| Failover | `failover.New()` | 完全使用，`wrapHA()` 中 | ✅ 完全合规 |
| Hedge | `hedge.New()` | 完全使用，`wrapHA()` 中 | ✅ 完全合规 |
| ModelSelector | `agent.ModelSelector` 接口 | 完全使用，4 种选择器 + 链式组合 | ✅ 完全合规 |
| TokenTailoring | `provider.WithEnableTokenTailoring()` | 完全使用 | ✅ 完全合规 |
| GenerationConfig | `WithMaxTokens` / `WithTemperature` 等 | 完全使用 | ✅ 完全合规 |
| StructuredOutput | `WithStructuredOutputJSON()` | 完全使用 | ✅ 完全合规 |
| 指标包装 | 框架无内置 | 自建 `WrapModelWithMetrics()` | ⚠️ 框架缺失，项目自建 |
| 传输层包装 | 框架无内置 | 自建 RateLimit → Retry → CircuitBreaker 逐层包装 | ⚠️ 框架缺失，项目自建 |

**偏差说明**：
- 框架的 Model 层是纯粹的 LLM 抽象，不包含传输层治理（限流/重试/熔断）。项目在 `http.Transport` 层自建了完整的治理链，这是生产级必需的。
- `WrapModelWithMetrics()` 是项目自建的指标中间件，框架的 Plugin/Callback 体系可以部分替代，但指标采集需要更底层的拦截。

---

### 2.4 Tool 模块

| 对比项 | 框架设计 | 项目实现 | 合规性 |
|--------|---------|---------|--------|
| FunctionTool | `function.NewFunctionTool()` | 完全使用 | ✅ 完全合规 |
| ToolSet | `tool.ToolSet` 接口 | 完全使用（Filesystem/ShellExec/WebResearch/MCP 等） | ✅ 完全合规 |
| MCPToolSet | `mcp.NewMCPToolSet()` | 完全使用，支持 stdio/SSE/StreamableHTTP | ✅ 完全合规 |
| MCPBroker | `mcpbroker.New()` | 完全使用 | ✅ 完全合规 |
| AgentTool | `agent.NewAgentTool()` | 完全使用（CallAgent/SubAgent） | ✅ 完全合规 |
| SkillTool | `skill.NewExecTool()` / `NewLoadTool()` 等 | 完全使用 | ✅ 完全合规 |
| PermissionPolicy | `tool.PermissionPolicy` 接口 | 完全使用，基于 DeferredManager 的确认门控 | ✅ 完全合规 |
| ToolCallbacks | `tool.Callbacks` | 完全使用，通过 Callback Chain 适配 | ✅ 完全合规 |
| RetryPolicy | `tool.RetryPolicy` | 完全使用 | ✅ 完全合规 |
| ToolFilter | `tool.FilterFunc` | 完全使用 | ✅ 完全合规 |
| StreamableTool | `tool.StreamableTool` 接口 | 部分使用 | ⚠️ 部分使用 |
| DeferredTool | `tool.DeferredTool` 接口 | 未使用 | ❌ 未使用 |
| ToolPipe Extension | `toolpipe.New()` | 未使用 | ❌ 未使用 |
| 工具级熔断器 | 框架无内置 | 自建 `CircuitBreaker` | ⚠️ 框架缺失，项目自建 |
| 工具确认门控 | 框架无内置 | 自建 `DeferredManager` + `ToolConfirmGate` | ⚠️ 框架缺失，项目自建 |

**偏差说明**：
- 框架的 `PermissionPolicy` 是运行级策略，项目的 `DeferredManager` 是更细粒度的工具确认门控（按工具类型/名称配置可见性和确认要求）。
- ToolPipe Extension（工具结果管道过滤）是框架提供的优化机制，项目未使用，可能存在 Token 浪费。
- 工具级熔断器是项目自建的重要功能，框架的 `RetryPolicy` 只处理重试，不处理熔断。

---

### 2.5 Session 模块

| 对比项 | 框架设计 | 项目实现 | 合规性 |
|--------|---------|---------|--------|
| Service 接口 | `session.Service` | 完全使用，SQLite 后端 | ✅ 完全合规 |
| SQLite 后端 | `sqlite.NewService()` | 完全使用，含 TablePrefix/SoftDelete 配置 | ✅ 完全合规 |
| InMemory 降级 | `inmemory.NewSessionService()` | 完全使用，SQLite 不可用时降级 | ✅ 完全合规 |
| Session State | `StateMap = map[string][]byte` | 完全使用，自定义 key 约定 | ✅ 完全合规 |
| Session Summary | `session.Summary` + `Summarizer` | 完全使用，`EnqueueFrameworkSummary()` | ✅ 完全合规 |
| AppendEventHook | `session.AppendEventHook` | 部分使用 | ⚠️ 部分使用 |
| GetSessionHook | `session.GetSessionHook` | 部分使用 | ⚠️ 部分使用 |
| Ingestor | `session.Ingestor` | 完全使用，注入 Runner | ✅ 完全合规 |
| SearchableService | `session.SearchableService` | 未使用 | ❌ 未使用 |
| WindowService | `session.WindowService` | 未使用 | ❌ 未使用 |
| Session 压缩 | 框架仅提供 Summary | 自建完整压缩管道（micro_compact/memory_compact/compress_policy） | ⚠️ 框架不足，项目自建 |
| State Key 约定 | 框架无约定 | 自建 `aranea:runner_snapshot`/`aranea:compressed_summary`/`aranea:state:*` 等 | ℹ️ 项目扩展 |

**偏差说明**：
- 框架的 Session Summary 是 LLM 驱动的自动摘要，项目在此基础上自建了更精细的压缩管道：micro_compact（轻量压缩）、memory_compact（记忆提取后压缩）、compress_policy（策略选择）。
- `SearchableService` 和 `WindowService` 是框架提供的搜索/窗口功能，项目未使用，可能是因为自建了 EventBus + 持久化方案。
- 项目的 State Key 约定是必要的业务扩展，框架的 `StateMap` 是通用的键值存储，不定义具体 key。

---

### 2.6 Event 模块

| 对比项 | 框架设计 | 项目实现 | 合规性 |
|--------|---------|---------|--------|
| Event 结构体 | `event.Event`（嵌入 `*model.Response`） | 完全使用，消费 Runner 返回的事件流 | ✅ 完全合规 |
| FilterKey | `event.FilterKey` 前缀匹配 | 完全使用 | ✅ 完全合规 |
| StateDelta | `event.StateDelta` | 完全使用 | ✅ 完全合规 |
| IsRunnerCompletion | `ev.IsRunnerCompletion()` | 完全使用 | ✅ 完全合规 |
| 事件流消费 | `<-chan *event.Event` | 完全使用，`turnStreamConsumer.consume()` | ✅ 完全合规 |
| 事件投影 | 框架无内置 | 自建 `EventProjector`/`ActivityProjector` | ⚠️ 框架缺失，项目自建 |
| EventBus | 框架无内置 | 自建双总线（Session + Monitor）+ 投递策略 + 优先级 | ⚠️ 框架缺失，项目自建 |
| EventWAL | 框架无内置 | 自建 WBPF（Write-Before-Publish-Fanout）保护 | ⚠️ 框架缺失，项目自建 |
| 可靠性分级 | 框架无内置 | 自建 AS-EVT-01 分级（Critical/Important/Informational） | ⚠️ 框架缺失，项目自建 |

**偏差说明**：
- 框架的事件系统是"裸通道"模式——`Runner.Run()` 返回 `<-chan *event.Event`，消费者自行处理。项目需要将事件分发到多个消费者（WebSocket/持久化/监控/日志），因此自建了 EventBus。
- EventWAL 是 Critical 事件的先写后发保护，确保进程崩溃时不丢失关键事件（如 ToolResult/Error/RunnerCompletion）。这是框架未提供但生产环境必需的功能。
- 可靠性分级（AS-EVT-01）是项目的架构标准，框架无此概念。

---

### 2.7 Memory 模块

| 对比项 | 框架设计 | 项目实现 | 合规性 |
|--------|---------|---------|--------|
| Service 接口 | `memory.Service` | 完全实现，`sqliteMemoryService` | ✅ 接口合规 |
| AddMemory | `AddMemory(ctx, UserKey, content, topics)` | 完全实现 | ✅ 完全合规 |
| SearchMemories | `SearchMemories(ctx, UserKey, query)` | 完全实现，pgvector 优先 + SQLite 降级 | ✅ 完全合规 |
| ReadMemories | `ReadMemories(ctx, UserKey, limit)` | 完全实现 | ✅ 完全合规 |
| UpdateMemory/DeleteMemory/ClearMemories | 框架接口方法 | 完全实现 | ✅ 完全合规 |
| Memory Tools | `memtool.NewAddTool()` 等 6 个工具 | 完全使用 | ✅ 完全合规 |
| EnqueueAutoMemoryJob | `EnqueueAutoMemoryJob()` | 完全使用 | ✅ 完全合规 |
| Agentic 模式 | Agent 自主决定何时读写 | 完全使用 | ✅ 完全合规 |
| Auto 模式 | 框架自动提取记忆 | 未使用 | ❌ 未使用 |
| 框架后端 | 8 种（SQLite/Redis/PostgreSQL/MySQL 等） | 完全自建 SQLite/pgvector 实现 | ⚠️ 自建实现 |
| 记忆分层 | 框架无分层概念 | 自建 L0-L4 四层记忆体系 | ⚠️ 框架缺失，项目自建 |
| PII 扫描 | 框架无内置 | 自建 `applyPIIScan()` | ⚠️ 框架缺失，项目自建 |
| MemorySet 聚合 | 框架无概念 | 自建 `MemorySet`（Service + L0-L4 管理端口） | ⚠️ 框架缺失，项目自建 |

**偏差说明**：
- 框架的 Memory 是扁平的键值存储（`UserKey` → `[]Entry`），项目自建了四层记忆体系：
  - **L0（Episode）**：原始对话片段
  - **L1（Fact）**：提取的事实知识
  - **L2（Graph）**：实体关系图谱
  - **L3/L4（Composite）**：复合记忆
- 框架的 8 种后端是通用实现，项目的 `sqliteMemoryService` 是专门为 SQLite + pgvector 优化的实现，支持向量搜索和 PII 扫描。
- Auto 模式（框架自动提取记忆）未使用，项目使用 Agentic 模式 + 自建的压缩管道替代。

---

### 2.8 Graph 模块

| 对比项 | 框架设计 | 项目实现 | 合规性 |
|--------|---------|---------|--------|
| StateGraph | `graph.NewStateGraph()` | 完全使用，`BuildStateGraphWithRegistryAndLogger()` | ✅ 完全合规 |
| AddNode / AddLLMNode / AddToolsNode | 框架节点 API | 完全使用 | ✅ 完全合规 |
| AddEdge / AddConditionalEdge | 框架边 API | 完全使用 | ✅ 完全合规 |
| Reducer | `DefaultReducer`/`AppendReducer`/`MergeReducer` 等 | 完全使用 | ✅ 完全合规 |
| CheckpointSaver | `graph.CheckpointSaver` 接口 | 完全实现，`SQLiteCheckpointSaver` | ✅ 完全合规 |
| GraphAgent | `graphagent.New()` | 完全使用 | ✅ 完全合规 |
| Interrupt/Resume | `graph.Interrupt()` / `ResumeValue[T]()` | 完全使用 | ✅ 完全合规 |
| Command.GoTo | `graph.Command` | 完全使用 | ✅ 完全合规 |
| TimeTravel | `graphAgent.TimeTravel()` | 完全使用 | ✅ 完全合规 |
| ExecutionEngine | `WithExecutionEngine(BSP/DAG)` | 完全使用 | ✅ 完全合规 |
| NodeCallbacks | `WithNodeCallbacks()` | 完全使用 | ✅ 完全合规 |
| RetryPolicy | `graph.RetryPolicy` | 完全使用 | ✅ 完全合规 |
| EventBridge | 框架无内置 | 自建 `graphtrpc.EventBridge` 桥接到 EventBus | ⚠️ 框架缺失，项目自建 |
| GraphBuilderFactory | 框架无内置 | 自建工厂模式，统一 Graph 构建/运行/恢复 | ℹ️ 项目扩展 |
| GraphRuntime 适配 | 框架无内置 | 自建 `trpcGraphRuntime` 实现 biz 三接口 | ℹ️ 项目扩展 |

**偏差说明**：
- Graph 模块是项目与框架对齐度最高的模块之一，几乎完全使用框架原生 API。
- `EventBridge` 是必要的适配层——框架的 Graph 事件通过 `<-chan *event.Event` 返回，项目需要桥接到自建的 EventBus。
- `GraphBuilderFactory` 和 `trpcGraphRuntime` 是项目对框架 Graph 能力的 biz 层封装，符合分层规范。

---

### 2.9 Team 模块

| 对比项 | 框架设计 | 项目实现 | 合规性 |
|--------|---------|---------|--------|
| Coordinator 模式 | `team.New(coordinator, members)` | 未直接使用 | ❌ 未直接使用 |
| Swarm 模式 | `team.NewSwarm(name, entryName, members)` | 未直接使用 | ❌ 未直接使用 |
| MemberToolConfig | `team.WithMemberToolConfig()` | 未直接使用 | ❌ 未直接使用 |
| SwarmConfig | `team.WithSwarmConfig()` | 未直接使用 | ❌ 未直接使用 |
| CrossRequestTransfer | `team.WithCrossRequestTransfer()` | 未直接使用 | ❌ 未直接使用 |
| Team 编排 | 框架 `team.Team`（实现 `agent.Agent`） | 自建 `TeamRunner` + 多模式编排 | ⚠️ 完全自建 |
| Team + Graph 集成 | 框架无内置 | 自建 `compileTeamRuntime()` 根据 Team 模式编译为 Graph | ⚠️ 框架缺失，项目自建 |

**偏差说明**：
- 这是项目与框架偏差最大的模块。框架的 `team.Team` 是一个简单的 Agent 封装（Coordinator 或 Swarm 模式），项目的 Team 系统远比框架复杂：
  - 6 种模式（sequential/parallel/coordinator/critic_loop/adaptive/swarm）
  - 完整的 Team Definition 解析
  - 成员 Agent 动态构建
  - Team + Graph 集成（通过 `ARANEA_TEAM_GRAPH_RUNTIME` 开关）
  - Team 级别的事件流管理和持久化
- 项目可能是因为框架的 Team 功能过于简单，无法满足多模式编排需求，因此完全自建。

---

### 2.10 Plugin 模块

| 对比项 | 框架设计 | 项目实现 | 合规性 |
|--------|---------|---------|--------|
| Plugin 接口 | `plugin.Plugin`（Name + Register） | 完全使用 | ✅ 完全合规 |
| Manager | `plugin.Manager` | 完全使用 | ✅ 完全合规 |
| Registry | `plugin.Registry`（7 个 Hook 注册方法） | 完全使用 | ✅ 完全合规 |
| 内置 Plugin | Logging / GlobalInstruction 等 | 完全使用 | ✅ 完全合规 |
| Plugin 合并 | 框架无内置 | 自建 `deps.PluginManager.MergeChain()` | ℹ️ 项目扩展 |

**偏差说明**：
- Plugin 模块对齐度很高，项目完全遵循框架设计。
- `MergeChain()` 是项目在框架 Plugin 之上的扩展，将 Plugin 层的 Callback 合并到产品的 Callback Chain 中。

---

### 2.11 Callback 模块

| 对比项 | 框架设计 | 项目实现 | 合规性 |
|--------|---------|---------|--------|
| ModelCallbacks | `BeforeModel` / `AfterModel` | 完全使用，通过 Chain 适配 | ✅ 完全合规 |
| ToolCallbacks | `BeforeTool` / `AfterTool` | 完全使用，通过 Chain 适配 | ✅ 完全合规 |
| AgentCallbacks | `BeforeAgent` / `AfterAgent` | 完全使用，通过 Chain 适配 | ✅ 完全合规 |
| ContinueOnError | `WithContinueOnError()` | 完全使用 | ✅ 完全合规 |
| ContinueOnResponse | `WithContinueOnResponse()` | 完全使用 | ✅ 完全合规 |
| StopError | `agent.StopError` | 完全使用 | ✅ 完全合规 |
| InvocationState | 回调间共享状态 | 完全使用 | ✅ 完全合规 |
| Callback Chain | 框架无内置 | 自建有序 Chain（短路/熔断器注册） | ⚠️ 框架缺失，项目自建 |
| AdaptAgentCallbacks | 框架无内置 | 自建适配器将 Chain 转为框架 Callback 接口 | ℹ️ 项目扩展 |

**偏差说明**：
- 框架的 Callback 是扁平的 Before/After 钩子列表，项目自建了 Chain 模式支持有序执行、短路、熔断器注册。
- `AdaptAgentCallbacks()`/`AdaptModelCallbacks()`/`AdaptToolCallbacks()` 是必要的适配层，将项目的 Chain 模式转换为框架的 Callback 接口。

---

### 2.12 Skill 模块

| 对比项 | 框架设计 | 项目实现 | 合规性 |
|--------|---------|---------|--------|
| Repository 接口 | `skill.Repository`（Summaries/Get/Path） | 完全实现，FS + DB 双 Repository | ✅ 完全合规 |
| FSRepository | `skill.NewFSRepository(root)` | 完全使用，`FSRepositoryAdapter` | ✅ 完全合规 |
| VisibilityFilter | `skill.VisibilityFilter` | 完全实现，`AgentVisibilityFilter` | ✅ 完全合规 |
| SkillLoadMode | `WithSkillLoadMode()` | 完全使用 | ✅ 完全合规 |
| SkillToolProfile | `WithSkillToolProfile()` | 完全使用 | ✅ 完全合规 |
| SkillsDirectoryHints | `WithSkillsDirectoryHints()` | 完全使用 | ✅ 完全合规 |
| DBRepository | 框架无内置 | 自建 `DBRepositoryAdapter`（从 biz 层加载 + TTL 缓存） | ⚠️ 框架缺失，项目自建 |
| Skill Body 惰性加载 | 框架无内置 | 自建惰性加载（避免一次性加载所有 Skill Body） | ⚠️ 框架缺失，项目自建 |

**偏差说明**：
- 框架的 `FSRepository` 基于文件系统，项目的 `DBRepositoryAdapter` 从数据库加载 Skill 定义，支持 TTL 缓存和惰性加载 Body。
- 这是因为项目的 Skill 定义存储在数据库中（用户可动态创建/编辑），而非文件系统。

---

### 2.13 Knowledge 模块

| 对比项 | 框架设计 | 项目实现 | 合规性 |
|--------|---------|---------|--------|
| Knowledge 接口 | `knowledge.Knowledge`（Search） | 未使用 | ❌ 未使用 |
| BuiltinKnowledge | 完整 RAG 管线 | 未使用 | ❌ 未使用 |
| VectorStore | 6 种后端 | 仅使用 pgvector（在 Memory 中） | ⚠️ 间接使用 |
| Embedder | 4 种嵌入器 | 未直接使用 | ❌ 未使用 |
| Reranker | 3 种重排器 | 未使用 | ❌ 未使用 |
| QueryEnhancer | LLM/Passthrough | 未使用 | ❌ 未使用 |
| Source | 5 种数据源 | 未使用 | ❌ 未使用 |
| KnowledgeSearch Tool | 框架内置 | 自建 `KnowledgeSearchTool` | ⚠️ 自建替代 |

**偏差说明**：
- 框架的 Knowledge 模块是完整的 RAG 管线，项目未使用，而是自建了 `KnowledgeSearchTool`。
- 项目可能是因为 Knowledge 模块的 Source/Embedder/Reranker 管线过于重量级，而业务场景只需要简单的知识搜索。
- pgvector 在 Memory 模块中间接使用（向量搜索记忆），但未通过 Knowledge 模块使用。

---

### 2.14 Artifact 模块

| 对比项 | 框架设计 | 项目实现 | 合规性 |
|--------|---------|---------|--------|
| Service 接口 | `artifact.Service`（Save/Load/List/Delete） | 完全实现，`ServiceAdapter` | ✅ 接口合规 |
| InMemory 后端 | `inmemory.NewService()` | 使用 | ✅ 完全合规 |
| COS 后端 | `cos.NewService()` | 未使用 | ❌ 未使用 |
| S3 后端 | `s3.NewService()` | 未使用 | ❌ 未使用 |
| SessionInfo | `artifact.SessionInfo` | 完全使用 | ✅ 完全合规 |

**偏差说明**：
- Artifact 模块对齐度较高，项目完全遵循框架接口。
- COS/S3 后端未使用，项目可能使用 InMemory 后端 + 自建持久化方案。

---

### 2.15 Evaluation 模块

| 对比项 | 框架设计 | 项目实现 | 合规性 |
|--------|---------|---------|--------|
| AgentEvaluator | `evaluation.New()` | 未使用 | ❌ 未使用 |
| EvalSet/EvalMetric/Evaluator | 完整评估框架 | 未使用 | ❌ 未使用 |
| 内置评估器 | tool_trajectory/final_response/llm_judge 等 | 未使用 | ❌ 未使用 |

**偏差说明**：
- Evaluation 模块完全未使用。项目可能有自己的评估方案，或者尚未实现评估功能。

---

### 2.16 Prompt 模块

| 对比项 | 框架设计 | 项目实现 | 合规性 |
|--------|---------|---------|--------|
| Text 模板 | `prompt.Text`（占位符 + Resolver） | 未使用 | ❌ 未使用 |
| Langfuse Source | `prompt.LangfuseSource` | 未使用 | ❌ 未使用 |

**偏差说明**：
- Prompt 模块未使用，项目可能直接使用 Go 的字符串拼接或模板库。

---

### 2.17 Server 模块

| 对比项 | 框架设计 | 项目实现 | 合规性 |
|--------|---------|---------|--------|
| A2A Server | `a2a.New()` | 未使用 | ❌ 未使用 |
| AG-UI Server | `agui.New()` | 未使用 | ❌ 未使用 |
| OpenAI Server | `openai.New()` | 未使用 | ❌ 未使用 |
| Gateway | `gateway.Config` | 未使用 | ❌ 未使用 |

**偏差说明**：
- Server 模块完全未使用，项目使用 Kratos v2 作为传输层（HTTP/gRPC/WebSocket），自建了完整的 API 层。
- 这是架构设计决策：Kratos 提供了更成熟的传输层（中间件/鉴权/配置），框架的 Server 模块是轻量级入口，不适合生产级 API 服务。

---

## 三、框架未使用功能清单

| 功能 | 框架模块 | 未使用原因 | 建议 |
|------|---------|-----------|------|
| Evaluation 评估框架 | evaluation | 项目可能未实现评估 | 考虑集成，支持 Agent 质量评估 |
| Knowledge RAG 管线 | knowledge | 项目自建了 KnowledgeSearchTool | 评估是否可迁移到框架 Knowledge |
| Prompt 模板系统 | prompt | 项目直接拼接字符串 | 考虑使用，支持模板化和远程管理 |
| A2A 协议 | server/a2a | 项目使用 Kratos 传输层 | 考虑集成，支持 Agent 间互操作 |
| AG-UI 协议 | server/agui | 项目使用 Kratos + 自建 WebSocket | 考虑集成，支持 CopilotKit 等前端 |
| OpenAI 兼容 API | server/openai | 项目使用 Kratos 传输层 | 考虑集成，支持 OpenAI 客户端直连 |
| Gateway | gateway | 项目使用 Kratos 传输层 | 不需要，Kratos 已覆盖 |
| ToolPipe Extension | toolpipe | 项目未使用工具结果过滤 | **强烈建议使用**，可减少 Token 消耗 |
| DeferredTool | tool | 项目自建了 DeferredManager | 评估是否可迁移到框架 DeferredTool |
| StreamableTool | tool | 部分使用 | 评估哪些工具可改为流式 |
| SearchableSession | session | 项目自建了搜索方案 | 评估是否可迁移到框架 SearchableService |
| WindowSession | session | 项目未使用 | 评估是否可用于上下文窗口管理 |
| Auto Memory 模式 | memory | 项目仅使用 Agentic 模式 | 考虑启用，减少 prompt 占用 |
| COS/S3 Artifact | artifact | 项目仅使用 InMemory | 生产环境建议使用持久化后端 |
| PromptIter 优化 | promptiter | 项目未使用 | 考虑集成，支持自动化 Prompt 优化 |
| CodeExecutor container/e2b | codeexecutor | 项目可能仅使用 local | 生产环境建议使用容器隔离 |

---

## 四、项目自建功能与框架能力对比

### 4.1 EventBus vs 框架事件流

| 能力 | 框架 `<-chan *event.Event` | 项目 EventBus |
|------|---------------------------|---------------|
| 多消费者 | 不支持（单通道） | 支持（Session + Monitor 双总线） |
| 投递策略 | 无 | DropOldest/DropNewest/BlockUpTo |
| 优先级 | 无 | ChannelPriorityCritical |
| 持久化 | 无 | EventWAL（WBPF） |
| 可靠性分级 | 无 | Critical/Important/Informational |
| 熔断器 | 无 | EventBusSink 内置熔断器 |

**结论**：框架的事件流是"裸通道"，适合单消费者场景。项目的 EventBus 是必要的生产级扩展。

### 4.2 Memory L0-L4 vs 框架 Memory

| 能力 | 框架 Memory | 项目 L0-L4 |
|------|------------|-----------|
| 存储模型 | 扁平键值（UserKey → []Entry） | 四层分层（Episode/Fact/Graph/Composite） |
| 搜索 | 关键词/向量搜索 | 向量搜索 + 关键词降级 |
| 自动提取 | Auto 模式（Extractor） | 压缩管道（micro/memory_compact） |
| PII 保护 | 无 | applyPIIScan() |
| 工具 | 6 个内置工具 | 6 个内置工具 + 自建工具 |

**结论**：项目的 L0-L4 体系远超框架的扁平 Memory，是必要的业务扩展。

### 4.3 Team Runner vs 框架 Team

| 能力 | 框架 Team | 项目 TeamRunner |
|------|----------|----------------|
| 模式 | Coordinator + Swarm | sequential/parallel/coordinator/critic_loop/adaptive/swarm |
| 成员管理 | 静态成员列表 | 动态构建 + AgentFactory |
| Graph 集成 | 无 | compileTeamRuntime() |
| 事件管理 | 继承 Agent 事件流 | 独立事件流 + 持久化 |
| 状态管理 | 继承 Session | 独立 Session 管理 |

**结论**：框架的 Team 过于简单，项目自建是合理的。但应考虑将自建模式贡献回框架。

### 4.4 Callback Chain vs 框架 Callbacks

| 能力 | 框架 Callbacks | 项目 Callback Chain |
|------|---------------|-------------------|
| 执行模式 | 扁平列表 | 有序链（支持短路） |
| 熔断器 | 无 | 支持（注册熔断器 Callback） |
| 适配层 | 无 | AdaptAgentCallbacks() 等 |
| Plugin 合并 | 无 | MergeChain() |

**结论**：Chain 模式是框架 Callback 的增强，适配层确保了兼容性。

---

## 五、架构对齐度矩阵

| 框架模块 | 接口实现 | Option 使用 | Callback 使用 | 自建扩展 | 总体对齐 |
|---------|---------|------------|-------------|---------|---------|
| Runner | ✅ 完全 | ✅ 完全 | N/A | RunnerManager/RunRegistry | ★★★★☆ |
| Agent | ✅ 完全 | ✅ 完全 | ✅ 完全 | BuildCache/Factory | ★★★★☆ |
| Model | ✅ 完全 | ✅ 完全 | ✅ 完全 | Metrics/Transport 包装 | ★★★★☆ |
| Tool | ✅ 完全 | ✅ 完全 | ✅ 完全 | CircuitBreaker/ConfirmGate | ★★★★☆ |
| Session | ✅ 完全 | ✅ 完全 | ⚠️ 部分 | 压缩管道/State Key | ★★★☆☆ |
| Event | ✅ 完全 | N/A | N/A | EventBus/WAL/分级/投影 | ★★☆☆☆ |
| Memory | ✅ 接口 | ✅ 完全 | N/A | L0-L4/PII/SQLite 实现 | ★★★☆☆ |
| Graph | ✅ 完全 | ✅ 完全 | ✅ 完全 | EventBridge/BuilderFactory | ★★★★★ |
| Team | ❌ 未用 | ❌ 未用 | ❌ 未用 | 完全自建 | ★☆☆☆☆ |
| Skill | ✅ 完全 | ✅ 完全 | N/A | DBRepository/惰性加载 | ★★★★☆ |
| Plugin | ✅ 完全 | ✅ 完全 | N/A | MergeChain | ★★★★★ |
| Callback | ✅ 完全 | ✅ 完全 | ✅ 完全 | Chain/适配器 | ★★★★☆ |
| Knowledge | ❌ 未用 | ❌ 未用 | ❌ 未用 | 自建 KnowledgeSearchTool | ★☆☆☆☆ |
| Artifact | ✅ 接口 | ✅ 完全 | N/A | ServiceAdapter | ★★★★☆ |
| Evaluation | ❌ 未用 | ❌ 未用 | ❌ 未用 | 无 | ☆☆☆☆☆ |
| Prompt | ❌ 未用 | ❌ 未用 | ❌ 未用 | 无 | ☆☆☆☆☆ |
| Server | ❌ 未用 | ❌ 未用 | ❌ 未用 | Kratos 替代 | ☆☆☆☆☆ |

---

## 六、改进建议

### 6.1 高优先级（建议立即评估）

| # | 建议 | 原因 | 预期收益 |
|---|------|------|---------|
| 1 | **启用 ToolPipe Extension** | 项目未使用工具结果过滤，长工具输出浪费 Token | Token 消耗降低 50-90%（框架 benchmark 数据） |
| 2 | **评估 Team 模块对齐** | 项目完全自建 Team，框架已提供 Coordinator/Swarm | 减少维护成本，但需评估功能差距 |
| 3 | **评估 Knowledge 模块集成** | 项目自建 KnowledgeSearchTool，框架有完整 RAG 管线 | 统一 RAG 架构，减少自建代码 |
| 4 | **评估 SearchableSession** | 项目自建搜索方案，框架提供 SearchableService | 减少自建代码 |

### 6.2 中优先级（建议下个迭代评估）

| # | 建议 | 原因 | 预期收益 |
|---|------|------|---------|
| 5 | **集成 Evaluation 模块** | 项目无评估框架，框架提供完整评估能力 | 支持 Agent 质量评估和回归测试 |
| 6 | **启用 Auto Memory 模式** | 项目仅使用 Agentic 模式 | 减少 prompt 占用，自动提取记忆 |
| 7 | **使用 Prompt 模板系统** | 项目直接拼接字符串 | 支持模板化、远程管理、Langfuse 集成 |
| 8 | **评估 Artifact 持久化后端** | 项目仅使用 InMemory | 生产环境数据持久化 |

### 6.3 低优先级（建议长期规划）

| # | 建议 | 原因 | 预期收益 |
|---|------|------|---------|
| 9 | **评估 A2A 协议集成** | 项目未使用 Agent 间互操作协议 | 支持 Agent 生态互操作 |
| 10 | **评估 AG-UI 协议集成** | 项目使用自建 WebSocket | 支持 CopilotKit 等前端框架 |
| 11 | **评估 PromptIter 优化** | 项目未使用自动化 Prompt 优化 | 支持 Prompt 自动调优 |
| 12 | **考虑 CodeExecutor 容器化** | 项目可能仅使用 local 执行器 | 生产环境安全隔离 |

---

## 七、框架缺失功能（建议贡献回框架）

| # | 功能 | 项目实现 | 建议贡献方式 |
|---|------|---------|-------------|
| 1 | Runner 生命周期管理 | RunnerManager + RunRegistry | 贡献为 Runner 扩展包 |
| 2 | Agent Build Cache | BuildCache（LRU + singleflight） | 贡献为 Agent 扩展包 |
| 3 | 事件总线 | EventBus（双总线 + 投递策略 + 优先级） | 贡献为 event 扩展包 |
| 4 | 事件 WAL | EventWAL（WBPF 保护） | 贡献为 event 扩展包 |
| 5 | 工具确认门控 | DeferredManager + ToolConfirmGate | 贡献为 tool 扩展包 |
| 6 | 工具级熔断器 | CircuitBreaker | 贡献为 tool 扩展包 |
| 7 | Model 指标中间件 | WrapModelWithMetrics() | 贡献为 model 扩展包 |
| 8 | 传输层治理 | RateLimit → Retry → CircuitBreaker | 贡献为 model 扩展包 |
| 9 | Callback Chain | 有序链 + 短路 + 熔断器 | 贡献为 callback 扩展包 |
| 10 | DB Skill Repository | DBRepositoryAdapter + TTL 缓存 | 贡献为 skill 扩展包 |

---

## 八、总结

Aranea-Agents 项目**正确使用了 trpc-agent-go 框架的核心运行时**（Runner→Agent→Model→Tool→Session 主链路），但在框架能力不足的领域进行了大规模自建。这种"框架内核 + 自建外壳"的架构是合理的——框架定位为运行时内核，项目定位为生产级平台。

**关键风险**：
1. **Team 模块完全自建**——框架已提供 Coordinator/Swarm 模式，项目未使用，存在维护负担
2. **Knowledge 模块未使用**——框架有完整 RAG 管线，项目自建了简化版，可能错失框架优化
3. **ToolPipe 未使用**——框架的 benchmark 数据显示可降低 50-90% Token 消耗

**关键优势**：
1. **Graph 模块高度对齐**——项目完全使用框架 Graph API，仅添加必要的适配层
2. **Plugin/Callback 充分利用**——项目通过 Chain 模式增强了框架的 Callback 体系
3. **Model 层治理完善**——项目在框架 Model 之上添加了传输层治理，是生产级必需的
