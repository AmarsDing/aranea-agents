# 模块运行时关系全景

> 本文档描述 trpc-agent-go 框架各模块在运行时的交互关系，以及 Aranea-Agents 项目中的实际集成方式。
> 与 `00-guide.md`（逐模块对齐分析）不同，本文聚焦**跨模块的运行时依赖、初始化顺序、数据流向和生命周期编排**。

---

## 一、框架运行时核心架构

### 1.1 总体交互模式

框架所有示例遵循同一初始化和调用模式：

```
Model → Agent → Runner → Event Channel → 消费者
```

**初始化顺序**（从底向上构建）：

```
1. Model        ← openai.New(modelName, openai.WithVariant(...))
2. Tool         ← function.NewFunctionTool(fn, function.WithName(...))
3. Agent        ← llmagent.New(name, llmagent.WithModel(model), llmagent.WithTools(tools))
4. Session      ← sessioninmemory.NewSessionService() 或 sqlite.NewService(db)
5. Memory       ← memory.NewService(memory.WithStore(store), memory.WithEmbedder(emb))
6. Plugin       ← plugin.NewLogging(), plugin.NewGlobalInstruction(inst)
7. Runner       ← runner.NewRunner(appName, agent, runner.WithSessionService(ss), ...)
8. 调用         ← runner.Run(ctx, userID, sessionID, message) → <-chan *event.Event
```

### 1.2 依赖方向图

```
                    ┌──────────────────────────────────────────┐
                    │              Runner                      │
                    │  ┌─────────┐ ┌──────────┐ ┌──────────┐  │
                    │  │Session  │ │ Memory   │ │ Plugin   │  │
                    │  │Service  │ │ Service  │ │ Manager  │  │
                    │  └─────────┘ └──────────┘ └────┬─────┘  │
                    │       │           │              │        │
                    │       ▼           ▼              ▼        │
                    │  ┌──────────────────────────────────┐    │
                    │  │           Agent                   │    │
                    │  │  ┌───────┐ ┌──────┐ ┌─────────┐  │    │
                    │  │  │ Model │ │ Tool │ │ Skill   │  │    │
                    │  │  └───────┘ └──────┘ └─────────┘  │    │
                    │  │  ┌──────────┐ ┌─────────┐       │    │
                    │  │  │ Planner  │ │Callback │       │    │
                    │  │  └──────────┘ └─────────┘       │    │
                    │  └──────────────────────────────────┘    │
                    └──────────────────────────────────────────┘
                                       │
                                       ▼
                              Event Channel
```

**核心规则**：
- Runner 是唯一入口，持有 Agent + Session + Memory + Plugin
- Agent 持有 Model + Tool + Skill + Planner + Callback
- Memory 通过 Runner 注入，Memory 的 Tool 通过 Agent 注入
- Plugin 在 Runner 层注册，内部桥接到 Agent/Model/Tool 的 Callback

---

## 二、核心模块运行时关系

### 2.1 Runner ↔ Agent：编排与执行

**框架模式**（`examples/runner/main.go`）：

```go
// Runner 持有 Agent，调用 Agent.Run() 获取事件流
runner := runner.NewRunner(appName, agent,
    runner.WithSessionService(sessionService),
    runner.WithMemoryService(memoryService),
    runner.WithPlugins(plugin1, plugin2),
)
eventChan, err := runner.Run(ctx, userID, sessionID, message)
```

**Runner 的编排职责**：

| 阶段 | Runner 行为 | 涉及模块 |
|------|------------|---------|
| 1. Session 加载 | 从 SessionService 获取/创建 Session，加载历史消息 | Session |
| 2. Invocation 构建 | 将用户消息 + Session 上下文封装为 `agent.Invocation` | Session → Agent |
| 3. Plugin 前置钩子 | 调用 `BeforeAgent` 钩子 | Plugin |
| 4. Agent 执行 | 调用 `agent.Run(ctx, invocation)` | Agent |
| 5. Event 转发 | 从 Agent event channel 读取，经 Plugin `OnEvent` 处理后转发 | Plugin, Event |
| 6. Session 持久化 | 将事件追加到 Session | Session |
| 7. Steer 支持 | 通过 `EnqueueUserMessage()` 运行中插入新消息 | Agent |

**项目实现**（`internal/agent/trpc_runtime.go`）：

```go
// RunnerManager 集中管理 Runner 创建
func (m *RunnerManager) NewTurnRunner(root trpcagent.Agent, spec TurnRunnerSpec) (trpcrunner.ManagedRunner, error) {
    // 从 PersistenceSet 提取 Session/Memory/Artifact 服务
    deps := TRPCRunnerDeps{
        SessionService: spec.Persist.Session,
        MemoryService:  spec.Persist.Memory,
        ArtifactService: spec.Persist.Artifact,
        Plugins:        spec.Plugins,
        // ...
    }
    return chatagent.NewTRPCRunner(root, deps, opts...)
}
```

**差异**：项目通过 `RunnerManager` 工厂模式管理 Runner 生命周期，支持 Per-Turn 动态构建和 `RunnerInstanceRegistry` 长生命周期管理。

---

### 2.2 Agent ↔ Model：推理循环

**框架模式**（`examples/react/main.go`）：

```go
model := openai.New(modelName, openai.WithVariant(variant))
agent := llmagent.New(name, llmagent.WithModel(model))
```

**Agent 内部 ReAct 循环**：

```
用户消息 → [Session 历史] → Model.Generate() → 检查 ToolCalls
    ↓ 有 ToolCalls                              ↓ 无 ToolCalls
执行 Tool → Tool 结果追加 → 回到 Model.Generate()  → 返回最终响应
```

**Callback 拦截点**：

```
BeforeModel → Model.Generate() → AfterModel
BeforeTool  → Tool.Execute()   → AfterTool
```

**项目实现**（`internal/agent/trpc_build.go`）：

```go
func BuildTRPCLLMAgent(deps TRPCBuilderDeps, spec AgentBuildSpec) (trpcagent.Agent, error) {
    // 1. 解析 Provider/Model（支持系统默认模型回退）
    model := provider.TRPCModelForProviderModel(deps.ModelRoute.RT, provider, modelName)
    // 2. 构建 System Prompt（含文件注入、组织上下文）
    // 3. 配置 Planner（react/a2ui/builtin）
    // 4. 配置 Skill、ToolSets、Callback Chain
    // 5. 最终创建框架 Agent
    return trpcllmagent.New(agentKey, opts...)
}
```

**差异**：项目增加了 Provider 路由层（支持多 LLM 供应商）、System Prompt 模板引擎、Callback Chain 熔断器。

---

### 2.3 Agent ↔ Tool：工具调用

**框架模式**（`examples/runner/tools.go`）：

```go
calculatorTool := function.NewFunctionTool(
    c.calculate,
    function.WithName("calculator"),
    function.WithDescription("Perform calculations"),
)
agent := llmagent.New(name, llmagent.WithTools([]tool.Tool{calculatorTool}))
```

**Tool 调用生命周期**：

```
Model 返回 ToolCalls → Agent 解析 → BeforeTool 钩子
    → Tool.Execute() → AfterTool 钩子 → Tool 结果追加到上下文 → 继续 Model 循环
```

**项目实现**（`internal/agent/trpc_build.go` → `buildToolsetsForAgent()`）：

| 工具来源 | 说明 |
|---------|------|
| MCP 工具 | 通过 `biz.AgentMCPTooling` 获取 Agent 配置的 MCP Server 工具 |
| 内置工具 | Kanban、WebResearch、SubAgent、CodeExecutor 等 |
| Skill 工具 | 通过 `trpcskill.Repository` 加载（`skill_load` + `skill_run`） |
| Memory 工具 | `memoryService.Tools()` 提供的记忆存取工具 |
| Knowledge 工具 | `knowledgetool.NewKnowledgeSearchTool()` 提供的知识搜索工具 |
| Deferred Tool | 延迟加载工具的可见性管理 |

**差异**：项目工具来源远多于框架示例，增加了 MCP 协议工具、Deferred Tool Manager、Kanban Bridge 等业务工具。

---

### 2.4 Runner ↔ Session：对话持久化

**框架模式**（`examples/session/util.go`）：

```go
// 内存后端
sessionService := sessioninmemory.NewSessionService()
// SQLite 后端
sessionService, _ := trpcsqlite.NewService(db,
    trpcsqlite.WithTablePrefix("trpc_"),
    trpcsqlite.WithEnableAsyncPersist(false),
    trpcsqlite.WithSoftDelete(true),
)
runner := runner.NewRunner(appName, agent, runner.WithSessionService(sessionService))
```

**Session 在 Runner 中的角色**：

```
Runner.Run() → SessionService.GetOrCreate(userID, sessionID)
           → 加载历史消息到 Invocation
           → Agent 执行
           → SessionService.AppendEvent() 持久化事件
```

**项目实现**（`internal/session/trpc/sqlite.go`）：

```go
func NewSQLiteSessionService(db *sql.DB) (trpcsession.Service, error) {
    svc, _ := trpcsqlite.NewService(db,
        trpcsqlite.WithTablePrefix("trpc_"),
        trpcsqlite.WithEnableAsyncPersist(false),
        trpcsqlite.WithSoftDelete(true),
    )
    return svc, err
}
```

**项目扩展**（`internal/session/runtime.go`）：
- `SyncRunnerSnapshot()`：同步 Runner 快照到框架 Session State
- `EnqueueFrameworkSummary()`：触发框架异步摘要
- `Compressor`：对话压缩（超出 token 限制时自动摘要历史消息）

**差异**：项目增加了 Session 压缩、Runner 快照同步、异步摘要等框架未提供的功能。

---

### 2.5 Runner ↔ Memory：长期记忆

**框架模式**（`examples/memory/util.go`）：

```go
// Memory 在 Runner 层注入
memoryService := memory.NewService(memory.WithStore(store), memory.WithEmbedder(emb))
runner := runner.NewRunner(appName, agent,
    runner.WithSessionService(sessionService),
    runner.WithMemoryService(memoryService),
)
// Memory 的 Tool 注入 Agent
agent := llmagent.New(name,
    llmagent.WithModel(model),
    llmagent.WithTools(memoryService.Tools()),  // Agent 通过 Tool 存取记忆
)
```

**关键区别**：
- **Session**：管理对话历史（消息序列），由 Runner 自动管理
- **Memory**：管理长期记忆（跨 Session 的知识），Agent 通过 Tool 主动调用

**项目实现**（`internal/memory/trpc/sqlite_adapter.go`）：

项目实现了 L0-L4 五层记忆体系：

| 层级 | 名称 | 实现 | 框架对应 |
|------|------|------|---------|
| L0 | Session State | trpc-agent-go 内置 | ✅ 框架原生 |
| L1 | Session Admin | `SessionAdminStoreAdapter` | ❌ 项目自建 |
| L2 | Episode 记忆 | `L2Recall` | ❌ 项目自建 |
| L3 | Fact 记忆 | `L3FactReader/Writer` + pgvector | ⚠️ 部分使用框架 |
| L4 | 知识图谱 | 实体-关系 | ❌ 项目自建 |

**差异**：项目记忆体系远超框架能力，框架仅提供 L0 级别，项目自建了 L1-L4。

---

### 2.6 Agent ↔ Skill/Knowledge：技能与知识

**框架 Skill 模式**（`examples/skill/main.go`）：

```go
skillRepo, _ := skill.NewFSRepository(skillsRoot)    // 从文件系统加载
codeExec := localexec.New(localexec.WithWorkDir(...)) // 代码执行器
agent := llmagent.New("gaia-agent",
    llmagent.WithSkills(skillRepo),                    // 注入 Skill 仓库
    llmagent.WithSkillToolProfile(llmagent.SkillToolProfileFull),
    llmagent.WithCodeExecutor(codeExec),
)
// Agent 自动获得 skill_load + skill_run 两个工具
```

**框架 Knowledge 模式**（`examples/knowledge/basic/main.go`）：

```go
// 1. 创建数据源 + 向量存储 + 嵌入器
kb := knowledge.New(knowledge.WithVectorStore(vs), knowledge.WithEmbedder(emb), ...)
kb.Load(ctx)
// 2. 创建搜索工具
searchTool := knowledgetool.NewKnowledgeSearchTool(kb, knowledgetool.WithMaxResults(3))
// 3. 注入 Agent
agent := llmagent.New("assistant", llmagent.WithTools([]tool.Tool{searchTool}))
```

**项目实现**：

- **Skill**（`internal/agent/trpc_build.go`）：通过 `service.NewSkillDBRepository(uc, 2*time.Minute, lg)` 桥接到 `biz.SkillUsecase`，支持数据库存储而非文件系统
- **Knowledge**（`internal/knowledge/`）：多层检索架构

```
FederatedRetriever → AdaptiveRouter → HybridRetriever → Retriever (Embedding + Search + Rerank)
```

**差异**：
- Skill：项目使用 DB 存储替代文件系统，增加了 Skill 进化机制
- Knowledge：项目检索链路远超框架的简单 `knowledge_search` 工具，增加了自适应路由、混合检索、联邦检索

---

### 2.7 Team：多 Agent 协调

**框架模式**（`examples/team/swarm/main.go` + `examples/team/coord/main.go`）：

**Swarm 模式**：
```go
team, _ := team.NewSwarm(teamName, entryAgentName, members, teamOpts...)
// Agent 间通过 transfer_to_agent 工具转移控制权
```

**Coordinator 模式**：
```go
team, _ := team.New(coordinator, members, team.WithMemberToolConfig(memberCfg))
// Coordinator 通过 consult_agent 工具调用成员 Agent
```

**关键**：Team 实现了 `agent.Agent` 接口，可直接传给 Runner：
```go
runner := runner.NewRunner(appName, teamInstance, runner.WithSessionService(sessionService))
```

**项目实现**（`internal/runtime/`）：

```go
team.RunnerConfig{
    PluginRT:      pluginRT,
    PluginManager: pluginMgr,
    Knowledge:     &team.KnowledgeFacade{...},
    Runs:          runs,
    AgentHelper:   &chatagent.TeamAgentHelperAdapter{},
    GraphLoader:   graphadapter.NewLinkedGraphBuildConfigLoader(graphs),
    TeamGraphTasks: team.NewTaskUsecaseGraphTaskCreator(tasks),
}
```

**差异**：项目增加了 KnowledgeFacade（知识库集成）、GraphLoader（图编排集成）、TeamGraphTasks（任务管理集成）。

---

### 2.8 Plugin/Callback：生命周期钩子

**框架 Plugin 模式**（`examples/plugin/main.go`）：

```go
runner := runner.NewRunner(appName, agent,
    runner.WithPlugins(
        plugin.NewLogging(),
        plugin.NewGlobalInstruction(instruction),
        newDemoPlugin(debug),
    ),
)
```

**Plugin 的 6 个钩子点**：

| 钩子 | 触发时机 | 能力 |
|------|---------|------|
| `BeforeAgent` | Agent 执行前 | 修改 Invocation |
| `AfterAgent` | Agent 执行后 | 修改结果 |
| `BeforeModel` | Model 调用前 | 拦截请求、返回自定义响应（短路） |
| `AfterModel` | Model 调用后 | 修改响应 |
| `BeforeTool` | Tool 执行前 | 修改参数、返回自定义结果（短路） |
| `AfterTool` | Tool 执行后 | 修改结果 |
| `OnEvent` | 每个 Event 经过时 | 修改 Event（添加 Tag、修改内容） |

**框架 Callback 模式**（`examples/callbacks/main.go`）：

```go
agent := llmagent.New(name,
    llmagent.WithAgentCallbacks(agentCallbacks),  // Agent 级别
    llmagent.WithModelCallbacks(modelCallbacks),   // Model 级别
    llmagent.WithToolCallbacks(toolCallbacks),     // Tool 级别
)
```

**Callback vs Plugin**：
- **Callback**：直接注入 Agent，作用域限于单个 Agent
- **Plugin**：注入 Runner，作用于所有通过 Runner 运行的 Agent

**项目实现**（`internal/plugin/trpc/adapter.go`）：

| 插件 Key | 实现类 | 对应钩子 |
|---------|--------|---------|
| `audit_log` | AuditLogPlugin | OnEvent |
| `skill_usage_tracker` | SkillUsageTrackerPlugin | AfterTool |
| `retry_and_reflect` | RetryAndReflectPlugin | AfterAgent |
| `sensitive_data_mask` | SensitiveDataMaskPlugin | AfterModel |
| `confirmation_guard` | ConfirmationGuardPlugin | BeforeTool |
| `cost_guard` | CostGuardPlugin | BeforeModel/AfterModel |
| `model_router` | ModelRouterPlugin | BeforeModel |
| `permission_guard` | PermissionGuardPlugin | BeforeTool |
| `output_policy` | OutputPolicyPlugin | AfterAgent |

**项目 Callback Chain**（`internal/agent/callbacks/callbacks.go`）：
- Agent/Model/Tool 三级 Callback Chain
- 熔断器（CircuitBreakerRegistry）保护 Tool 调用

**差异**：项目插件体系远超框架示例，增加了成本守护、权限守卫、输出策略、熔断器等业务插件。

---

### 2.9 Graph：图编排

**框架模式**（`examples/graph/basic/main.go`）：

```go
// 1. 构建 StateGraph
stateGraph := graph.NewStateGraph(schema).
    AddNode("preprocess", preprocessFn).
    AddLLMNode("analyze", model, instruction, tools).
    AddToolsNode("tools", tools).
    AddConditionalEdges("route", conditionFn, edgeMap).
    SetEntryPoint("preprocess").
    SetFinishPoint("format_output")

// 2. 编译为 Graph
workflowGraph, _ := stateGraph.Compile()

// 3. 创建 GraphAgent（替代 LLMAgent）
graphAgent, _ := graphagent.New("doc-processor", workflowGraph,
    graphagent.WithInitialState(graph.State{}),
)

// 4. 传给 Runner（GraphAgent 实现了 Agent 接口）
runner := runner.NewRunner(appName, graphAgent, runner.WithSessionService(sessionService))
```

**Graph 的 Event 流**：
- `event.StateDelta` 携带节点执行元数据
- 节点执行阶段：`ExecutionPhaseStart` / `ExecutionPhaseComplete` / `ExecutionPhaseError`

**项目实现**（`internal/graph/trpc/`）：
- `Builder`：将 `biz.GraphBuildConfig` 编译为 `trpcgraph.Graph`
- `Registry`：管理节点函数注册
- `CheckpointSaver`：基于 SQLite 的检查点持久化
- `EventBridge`：将 trpc-agent-go Graph 事件转换为 Aranea Envelope

**差异**：项目增加了 Checkpoint 持久化和 EventBridge 事件桥接。

---

## 三、Event 流：贯穿所有模块的通信载体

### 3.1 框架 Event 流

```
Agent.Run() → <-chan *event.Event → Runner → Plugin.OnEvent() → 外部消费者
```

**Event 消费模式**（所有示例通用）：

```go
for evt := range eventChan {
    if evt.Error != nil { /* 处理错误 */ }
    if evt.IsToolCallResponse() { /* 显示工具调用 */ }
    if evt.IsToolResultResponse() { /* 显示工具结果 */ }
    // 流式用 Delta.Content，非流式用 Message.Content
    if evt.IsFinalResponse() { break }
}
```

### 3.2 项目 Event 流

```
1. trpc-agent-go Runner.Run() → <-chan *trpcevent.Event
2. WrapFrameworkEvents() → tee 到 TraceEmitter + OTel Observer
3. ChatOrchestrator 消费事件流 → 转换为 event.Envelope
4. event.Infra.Publish() → 路由到 SessionBus / MonitorBus
5. Bus.Publish() → 分发给所有 Subscriber
6. WSServer.eventPump() → 从 Subscriber channel 读取
7. 优先级队列（High/Normal/Low）→ writePump → WebSocket JSON 帧
```

**双 Bus 架构**（`internal/event/infra.go`）：

| Bus | 用途 | 事件类型 |
|-----|------|---------|
| SessionBus | 聊天/团队交互 | text_delta, error, runner_completion, run_status |
| MonitorBus | 监控/流日志 | flow_log, log, alert_notify, mcp_health_alert |

**事件可靠性分级**（AS-EVT-01）：

| 级别 | 事件类型 | 可靠性保证 |
|------|---------|-----------|
| Critical | ToolResult, Error, RunnerCompletion, Checkpoint | WBPF（先写 WAL 再发布）+ 重试 |
| Important | StateDelta, TokenUsage, RunStatus, SessionStatusChanged | BlockUpTo + 异步持久化 |
| Informational | TextDelta, FlowLog, Log, MemberDelta | 尽力而为 |

---

## 四、完整运行时交互流程

### 4.1 框架标准流程

```
用户输入
  │
  ▼
Runner.Run(ctx, userID, sessionID, message)
  │
  ├─ 1. 从 SessionService 加载历史
  ├─ 2. 构建 Invocation
  ├─ 3. Plugin.BeforeAgent()
  │
  ▼
Agent.Run(ctx, invocation)  ← LLMAgent / GraphAgent / Team
  │
  ├─ LLMAgent ReAct 循环:
  │   ├─ Model.Callbacks.BeforeModel()
  │   ├─ Model.Generate() ──→ LLM API
  │   ├─ Model.Callbacks.AfterModel()
  │   ├─ 检查 ToolCalls?
  │   │   ├─ Tool.Callbacks.BeforeTool()
  │   │   ├─ Tool.Execute()
  │   │   ├─ Tool.Callbacks.AfterTool()
  │   │   └─ 追加 Tool 结果 → 继续循环
  │   └─ 无 ToolCalls → 返回最终响应
  │
  ├─ GraphAgent: 按 Graph 拓扑执行节点
  │
  ├─ Team (Swarm/Coordinator): Agent 间协调
  │
  ▼
Event Channel
  │
  ├─ Plugin.OnEvent() 修改/标记 Event
  ├─ SessionService.AppendEvent() 持久化
  │
  ▼
外部消费者读取 Event Channel
```

### 4.2 项目实际流程

```
用户 WebSocket 消息
  │
  ▼
WSServer.handleUpstream() → ChatOrchestrator.HandleChatTurn()
  │
  ├─ ADMIT 阶段：准入检查（配额、并发、Session 状态）
  ├─ BUILD 阶段：构建 Agent 依赖 + Agent 实例 + Runner
  │   │
  │   ├─ RunnerManager.NewTurnRunner(root, spec)
  │   │   ├─ 从 PersistenceSet 提取 Session/Memory/Artifact
  │   │   ├─ 构建 TRPCRunnerDeps
  │   │   └─ chatagent.NewTRPCRunner(root, deps, opts...)
  │   │
  │   └─ BuildTRPCAgent(deps, spec)  [带缓存]
  │       ├─ 解析 Provider/Model
  │       ├─ 构建 System Prompt
  │       ├─ 配置 Planner + Skill + ToolSets + Callback Chain
  │       └─ trpcllmagent.New(agentKey, opts...)
  │
  ├─ EXECUTE 阶段：持久化用户消息 → Runner.Run() → 消费事件流
  │   │
  │   ├─ Runner.Run() → <-chan *trpcevent.Event
  │   ├─ WrapFrameworkEvents() → TraceEmitter + OTel
  │   ├─ 转换为 event.Envelope
  │   └─ event.Infra.Publish() → SessionBus / MonitorBus
  │
  ├─ POST-TURN：记录 Usage、触发 Memory 提取、Skill 进化
  │
  ▼
WSServer.eventPump()
  ├─ 从 Bus Subscriber channel 读取
  ├─ 优先级队列（High/Normal/Low）
  └─ writePump → WebSocket JSON 帧 → 前端
```

---

## 五、模块间依赖关系

### 5.1 初始化依赖图

```
                    ┌──────────┐
                    │  Model   │ ← 无依赖（最底层）
                    └────┬─────┘
                         │
            ┌────────────┼────────────┐
            ▼            ▼            ▼
      ┌──────────┐ ┌──────────┐ ┌──────────┐
      │  Tool    │ │  Skill   │ │ Planner  │ ← 依赖 Model
      └────┬─────┘ └────┬─────┘ └────┬─────┘
           │            │            │
           └────────────┼────────────┘
                        ▼
                   ┌──────────┐
                   │  Agent   │ ← 依赖 Model + Tool + Skill + Planner
                   └────┬─────┘
                        │
           ┌────────────┼────────────┐
           ▼            ▼            ▼
     ┌──────────┐ ┌──────────┐ ┌──────────┐
     │ Session  │ │  Memory  │ │  Plugin  │ ← 依赖 Agent
     └────┬─────┘ └────┬─────┘ └────┬─────┘
          │            │            │
          └────────────┼────────────┘
                       ▼
                  ┌──────────┐
                  │  Runner  │ ← 依赖 Agent + Session + Memory + Plugin
                  └────┬─────┘
                       │
                       ▼
                  ┌──────────┐
                  │  Event   │ ← Runner 产出 Event Channel
                  └──────────┘
```

### 5.2 对齐依赖顺序（来自 00-guide.md §6.3）

```
Event 对齐 → Session 对齐 → Memory 对齐
                          ↓
Team 对齐 → Agent 对齐 → Runner 对齐
              ↓
         Tool 对齐 → Callback 对齐
              ↓
         Skill 对齐 → Knowledge 对齐
```

**原因**：
- Event 是 Session/Memory 交互的基础（EventBus 与 Session/Memory 交互）
- Team 使用 Agent 接口，Agent 对齐是 Team 对齐的前提
- Knowledge Search 作为 Skill Tool，Knowledge 依赖 Skill

---

## 六、框架示例与项目实现的映射

### 6.1 初始化模式对比

| 模式 | 框架示例 | 项目实现 |
|------|---------|---------|
| Model 创建 | `openai.New(name, opts...)` | `provider.TRPCModelForProviderModel(rt, provider, model)` |
| Agent 创建 | `llmagent.New(name, opts...)` | `BuildTRPCLLMAgent(deps, spec)` + 缓存 |
| Runner 创建 | `runner.NewRunner(app, agent, opts...)` | `RunnerManager.NewTurnRunner(root, spec)` |
| Session 创建 | `sqlite.NewService(db, opts...)` | `NewSQLiteSessionService(db)` |
| Memory 创建 | `memory.NewService(opts...)` | `MemorySet{TRPC, Admin, L2, L3, Composite}` |
| Plugin 创建 | `plugin.NewLogging()` | `providePluginRuntime()` + 9 个业务插件 |
| Tool 注册 | `function.NewFunctionTool(fn, opts...)` | `buildToolsetsForAgent()` 6 类工具源 |

### 6.2 项目独有的适配层

| 适配器 | 路径 | 桥接方向 |
|--------|------|---------|
| RuntimeLogAdapter | `internal/adapter/runtime_log.go` | trpc-agent-go 日志 → loggateway Pipeline |
| SessionAdminStoreAdapter | `internal/session/` | Ent Session → trpc Session |
| SQLiteMemoryService | `internal/memory/trpc/sqlite_adapter.go` | L3 Fact → trpc Memory Service |
| ArtifactServiceAdapter | `internal/artifact/trpc/` | biz.ArtifactUsecase → trpc Artifact Service |
| PluginAdapter | `internal/plugin/trpc/adapter.go` | biz.Plugin → trpc Plugin |
| SkillDBRepository | `internal/agent/` | biz.SkillUsecase → trpc Skill Repository |
| GraphEventBridge | `internal/graph/trpc/event_bridge.go` | trpc Graph Event → Aranea Envelope |
| EventInfra | `internal/event/infra.go` | trpc Event → 双 Bus → WebSocket |

### 6.3 项目独有的扩展

| 扩展 | 位置 | 框架缺失 |
|------|------|---------|
| L1-L4 记忆体系 | `internal/memory/` | 框架仅 L0 |
| Session 压缩 | `internal/session/compressor.go` | 框架无压缩 |
| 自适应知识检索 | `internal/knowledge/` | 框架仅简单搜索 |
| Callback Chain 熔断器 | `internal/agent/callbacks/` | 框架无熔断 |
| 9 个业务插件 | `internal/plugin/trpc/` | 框架仅 Logging + GlobalInstruction |
| Deferred Tool Manager | `internal/agent/` | 框架无延迟加载 |
| 双 Bus 事件隔离 | `internal/event/` | 框架仅单 channel |
| WBPF 事件持久化 | `internal/event/` | 框架无 WAL |
| 三优先级 WS 推送 | `internal/server/ws_priority.go` | 框架无优先级 |

---

## 七、关键文件索引

### 框架示例

| 示例 | 路径 | 展示的模块关系 |
|------|------|--------------|
| Runner | `examples/runner/` | Runner → Agent → Model → Tool → Event |
| React | `examples/react/` | Agent + Planner (ReAct) → Model → Tool |
| Steer | `examples/steer/` | Runner.EnqueueUserMessage (运行中插入消息) |
| Team/Swarm | `examples/team/swarm/` | Team → Agent 间 transfer_to_agent |
| Team/Coord | `examples/team/coord/` | Team → Coordinator → consult_agent |
| Skill | `examples/skill/` | Agent + Skill Repository + CodeExecutor |
| Plugin | `examples/plugin/` | Runner + Plugin (6 钩子点) |
| Callbacks | `examples/callbacks/` | Agent + Model/Tool Callbacks |
| Session | `examples/session/` | Runner + Session Service |
| Memory | `examples/memory/` | Runner + Memory Service + Memory Tools |
| Graph | `examples/graph/basic/` | GraphAgent → StateGraph → LLM/Tool Node |
| Knowledge | `examples/knowledge/basic/` | Knowledge → VectorStore → SearchTool → Agent |
| ManagedRunner | `examples/managedrunner/` | Runner + Cancel/Status/Timeout |

### 项目核心文件

| 领域 | 文件路径 |
|------|---------|
| Wire 入口 | `cmd/admin/wire.go` |
| App 生命周期 | `cmd/admin/app.go` |
| Agent 构建 | `internal/agent/trpc_build.go` |
| Runner 创建 | `internal/agent/trpc_runtime.go` |
| Runner 管理 | `internal/runtime/runner_manager.go` |
| 运行时 Deps | `internal/runtime/deps.go` |
| Event 基础设施 | `internal/event/infra.go` |
| Session 运行时 | `internal/session/runtime.go` |
| Memory 适配器 | `internal/memory/trpc/sqlite_adapter.go` |
| Plugin 运行时 | `internal/plugin/trpc/runtime.go` |
| Graph 构建器 | `internal/graph/trpc/builder.go` |
| Chat 编排 | `internal/service/chat_orchestrator.go` |
| WS 服务器 | `internal/server/ws.go` |
| 日志适配器 | `internal/adapter/runtime_log.go` |
