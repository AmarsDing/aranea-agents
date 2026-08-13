# 智能体（Agent）— 框架对齐分析

> 模块路径：`pkg/trpc-agent-go/agent/`
> 项目实现路径：`internal/agent/`、`internal/biz/agent_*.go`、`internal/service/agent*.go`
> 当前对齐度：★★★★☆

---

## 一、框架能力全景

### 1.1 核心接口

| 接口 | 方法 | 说明 |
|------|------|------|
| `Agent` | `Run(ctx, *Invocation) (<-chan *event.Event, error)` | 执行代理逻辑，返回事件流通道 |
| `Agent` | `Tools() []tool.Tool` | 返回可用工具列表 |
| `Agent` | `Info() Info` | 返回代理元信息（名称/描述/输入输出 Schema） |
| `Agent` | `SubAgents() []Agent` | 返回子代理列表 |
| `Agent` | `FindSubAgent(name string) Agent` | 按名称查找子代理 |
| `SubAgentSetter` | `SetSubAgents(subAgents []Agent)` | 运行时动态更新子代理列表 |
| `CodeExecutor` | `CodeExecutor() codeexecutor.CodeExecutor` | 暴露代码执行器 |
| `PluginManager` | `AgentCallbacks() *Callbacks` | Runner 级全局 Agent 回调 |
| `PluginManager` | `ModelCallbacks() *model.Callbacks` | Runner 级全局 Model 回调 |
| `PluginManager` | `ToolCallbacks() *tool.Callbacks` | Runner 级全局 Tool 回调 |
| `PluginManager` | `OnEvent(ctx, *Invocation, *Event) (*Event, error)` | 事件拦截和变换 |
| `PluginManager` | `Close(ctx) error` | 资源清理 |
| `TransferController` | `OnTransfer(ctx, fromAgent, toAgent) (time.Duration, error)` | 控制 transfer_to_agent 的限流和策略 |
| `extension.Extension` | `Name() string` | 扩展名称 |
| `extension.Extension` | `Register(r *Registry)` | 注册回调和工具到 Agent 级扩展 |

### 1.2 关键类型

| 类型 | 说明 |
|------|------|
| `Info` | Agent 元信息（Name/Description/InputSchema/OutputSchema） |
| `Invocation` | 运行时上下文（约 30 字段），承载单次调用的全部上下文，支持 Clone/View 实现子代理隔离 |
| `RunOptions` | 运行时选项（约 50 字段），每次运行可不同，实现静态配置与动态覆盖的分离 |
| `Callbacks` | Agent 生命周期回调链（BeforeAgent/AfterAgent），支持短路和替换响应 |
| `CallbackContext` | 回调上下文，提供 Artifact 操作 |
| `ToolContext` | 工具调用上下文 |
| `InvocationContext` | Context 中存取 Invocation 的工具函数 |
| `StreamMode` | 流模式枚举（messages/updates/checkpoints/tasks/debug/custom） |
| `StopError` | 安全限制终止错误（MaxLLMCalls/MaxToolIterations 超出） |
| `TransferInfo` | 代理转移信息 |

### 1.3 扩展点

| 扩展点 | 机制 | 适用场景 |
|--------|------|---------|
| 实现 `Agent` 接口 | 自定义 Agent 实现 | 新增 Agent 类型（如项目中的编排路由） |
| `extension.Extension` 接口 | Agent 级扩展（工具+回调） | Agent 级横切关注点（如 TODO 强制执行、工具管道） |
| `PluginManager` 接口 | Runner 级插件 | 全局回调+事件拦截（横切关注点） |
| `TransferController` 接口 | transfer 限流策略 | 控制 transfer_to_agent 的超时和拒绝 |
| `Callbacks` 链 | BeforeAgent/AfterAgent 回调 | Agent 生命周期拦截（可短路、可替换响应） |
| `LazyAgent` | 延迟构造 | 先暴露 Info 使 transfer 可发现，实际 Run 时才创建 |
| `structure.Exporter` | 静态结构导出 | 可视化和运行时管理 |

### 1.4 配置选项

#### 构造时 Option（`llmagent.Option`，约 70+ 个）

| 分类 | Option | 说明 | 默认值 |
|------|--------|------|--------|
| 模型 | `WithModel(m)` | 注入模型实例 | — |
| 模型 | `WithModels(models)` | 多模型映射 | — |
| 模型 | `WithModelSelector(selector)` | 动态模型选择器 | — |
| 模型 | `WithGenerationConfig(cfg)` | 生成配置 | — |
| 指令 | `WithInstruction(text)` | 系统指令 | — |
| 指令 | `WithGlobalInstruction(text)` | 全局指令 | — |
| 指令 | `WithModelInstructions(map)` | 按模型指令 | — |
| 指令 | `WithModelGlobalInstructions(map)` | 按模型全局指令 | — |
| 工具 | `WithTools(tools)` | 工具列表 | — |
| 工具 | `WithToolSets(sets)` | 工具集 | — |
| 工具 | `WithActivatableToolSets(sets)` | 可激活工具集 | — |
| 工具 | `WithRefreshToolSetsOnRun(bool)` | 每次运行刷新工具集 | false |
| 工具 | `WithToolFilter(filter)` | 工具可见性过滤 | — |
| 工具 | `WithToolCallRetryPolicy(policy)` | 工具重试策略 | — |
| 工具 | `WithEnableParallelTools(bool)` | 并行工具执行 | false |
| 子代理 | `WithSubAgents(agents)` | 子代理列表 | — |
| 子代理 | `WithEndInvocationAfterTransfer(bool)` | transfer 后结束调用 | — |
| 子代理 | `WithDefaultTransferMessage(msg)` | 默认 transfer 消息 | — |
| 知识 | `WithKnowledge(knowledge)` | 知识库 | — |
| 知识 | `WithKnowledgeFilter(filter)` | 知识检索过滤 | — |
| 知识 | `WithKnowledgeConditionedFilter(filter)` | 条件知识过滤 | — |
| 知识 | `WithEnableKnowledgeAgenticFilter(bool)` | 智能知识过滤 | — |
| 技能 | `WithSkills(repo)` | 技能仓库 | — |
| 技能 | `WithSkillFilter(filter)` | 技能可见性过滤 | — |
| 技能 | `WithSkillToolProfile(profile)` | 技能工具配置 | — |
| 技能 | `WithSkillLoadMode(mode)` | 技能加载模式 | — |
| 技能 | `WithMaxLoadedSkills(n)` | 最大加载技能数 | — |
| 技能 | `WithSkillsDirectoryHints(bool)` | 技能目录提示 | — |
| 技能 | `WithSkillsLoadedContentInToolResults(bool)` | 技能内容在工具结果中 | — |
| 技能 | `WithSkillRunAllowedCommands(cmds)` | 技能运行允许命令 | — |
| 技能 | `WithSkillRunDeniedCommands(cmds)` | 技能运行拒绝命令 | — |
| 代码执行 | `WithCodeExecutor(exec)` | 代码执行器 | — |
| 代码执行 | `WithWorkspaceExecSurfaceEnabled(bool)` | workspace exec surface | — |
| 代码执行 | `WithWorkspaceExecAllowedCommands(cmds)` | 允许命令 | — |
| 代码执行 | `WithWorkspaceExecDeniedCommands(cmds)` | 拒绝命令 | — |
| 代码执行 | `WithWorkspaceBootstrap(bootstrap)` | 工作区引导 | — |
| 回调 | `WithAgentCallbacks(callbacks)` | Agent 回调 | — |
| 回调 | `WithModelCallbacks(callbacks)` | Model 回调 | — |
| 回调 | `WithToolCallbacks(callbacks)` | Tool 回调 | — |
| 回调 | `WithExtensions(exts...)` | Agent 级扩展 | — |
| 输出 | `WithOutputKey(key)` | 输出键 | — |
| 输出 | `WithOutputSchema(schema)` | 输出 Schema | — |
| 输出 | `WithInputSchema(schema)` | 输入 Schema | — |
| 输出 | `WithStructuredOutputJSONSchema(...)` | JSON Schema 结构化输出 | — |
| 输出 | `WithStructuredOutputJSON(...)` | 从 Go 类型推断结构化输出 | — |
| 上下文 | `WithAddContextPrefix(bool)` | 添加上下文前缀 | — |
| 上下文 | `WithAddSessionSummary(bool)` | 添加会话摘要 | — |
| 上下文 | `WithSessionSummaryInjectionMode(mode)` | 摘要注入模式 | — |
| 上下文 | `WithMaxHistoryRuns(n)` | 最大历史运行数 | — |
| 上下文 | `WithEnableContextCompaction(bool)` | 启用上下文压缩 | — |
| 上下文 | `WithContextCompactionThresholdRatio(ratio)` | 压缩阈值比例 | — |
| 上下文 | `WithContextCompactionKeepRecentRequests(n)` | 压缩保留最近请求数 | — |
| 上下文 | `WithContextCompactionOversizedToolResultMaxTokens(n)` | 超大工具结果截断 token 数 | — |
| 记忆 | `WithPreloadMemory(n)` | 预加载记忆条数 | — |
| 记忆 | `WithPreloadSessionRecall(bool)` | 预加载会话召回 | — |
| 规划 | `WithPlanner(planner)` | 规划器 | — |
| 时间 | `WithAddCurrentTime(bool)` | 添加当前时间 | — |
| 时间 | `WithTimezone(tz)` | 时区 | — |
| 时间 | `WithTimeFormat(fmt)` | 时间格式 | — |
| 限制 | `WithMaxLLMCalls(n)` | LLM 调用上限 | — |
| 限制 | `WithMaxToolIterations(n)` | 工具迭代上限 | — |
| 限制 | `WithChannelBufferSize(n)` | 事件通道缓冲 | — |
| 消息 | `WithMessageFilterMode(mode)` | 消息过滤模式 | — |
| 消息 | `WithMessageTimelineFilterMode(mode)` | 时间线过滤模式 | — |
| 消息 | `WithMessageBranchFilterMode(mode)` | 分支过滤模式 | — |
| 消息 | `WithReasoningContentMode(mode)` | 推理内容模式 | — |

#### 运行时 RunOption（`agent.RunOption`，约 40+ 个）

| 分类 | Option | 说明 | 默认值 |
|------|--------|------|--------|
| 应用 | `WithAppName(name)` | 覆盖 Runner 默认应用名 | — |
| 运行时 | `WithRuntimeState(state)` | 设置运行时状态 | — |
| 运行时 | `MergeRuntimeState(state)` | 合并运行时状态 | — |
| 代理 | `WithAgent(a)` | 覆盖默认 Agent | — |
| 代理 | `WithAgentByName(name)` | 按名称解析 Agent | — |
| 模型 | `WithModel(m)` | 覆盖模型 | — |
| 模型 | `WithModelName(name)` | 按名称选择模型 | — |
| 模型 | `WithModelSelector(selector)` | 动态模型选择器 | — |
| 模型 | `WithModelContextWindow(tokens)` | 上下文窗口 | — |
| 模型 | `WithModelRequestExtraFields(fields)` | 模型请求额外字段 | — |
| 模型 | `WithModelRequestHeaders(headers)` | 模型请求 HTTP 头 | — |
| 代码执行 | `WithCodeExecutor(exec)` | 覆盖代码执行器 | — |
| 流式 | `WithStream(stream)` | 覆盖流式设置 | — |
| 指令 | `WithInstruction(instruction)` | 覆盖指令 | — |
| 指令 | `WithGlobalInstruction(instruction)` | 覆盖全局指令 | — |
| 输出 | `WithStructuredOutputJSONSchema(...)` | JSON Schema 结构化输出 | — |
| 输出 | `WithStructuredOutputJSON(...)` | 从 Go 类型推断结构化输出 | — |
| 工具 | `WithToolFilter(filter)` | 工具可见性过滤 | — |
| 工具 | `WithAdditionalTools(tools)` | 运行时附加工具 | — |
| 工具 | `WithExternalTools(tools)` | 外部执行工具 | — |
| 工具 | `WithToolExecutionFilter(filter)` | 工具执行过滤 | — |
| 工具 | `WithToolPermissionPolicy(policy)` | 工具权限策略 | — |
| 知识 | `WithKnowledgeFilter(filter)` | 知识检索过滤 | — |
| 消息 | `WithMessages(messages)` | 提供对话历史 | — |
| 消息 | `WithInjectedContextMessages(messages)` | 注入上下文消息 | — |
| 消息 | `WithUserMessageRewriter(rewriter)` | 用户消息重写 | — |
| 恢复 | `WithResume(enabled)` | 恢复模式 | — |
| 恢复 | `WithPersistInterruptedAssistant(enabled)` | 中断时持久化 | — |
| 流模式 | `WithStreamMode(modes...)` | 流模式选择 | — |
| 追踪 | `WithDisableTracing(bool)` | 禁用追踪 | — |
| 超时 | `WithMaxRunDuration(d)` | 最大运行时长 | — |
| 取消 | `WithDetachedCancel(enabled)` | 分离取消信号 | — |
| 请求 | `WithRequestID(id)` | 请求 ID | — |
| 事件 | `WithEventFilterKey(key)` | 事件过滤键 | — |
| 图 | `WithGraphEmitFinalModelResponses(bool)` | 图节点最终响应 | — |
| 图 | `WithGraphTerminalMessagesOnly(bool)` | 仅终端节点消息 | — |
| 自定义 | `WithCustomAgentConfigs(configs)` | 自定义 Agent 配置 | — |

### 1.5 框架内置实现

| 实现 | 路径 | 说明 |
|------|------|------|
| `LLMAgent` | `agent/llmagent/` | 核心 LLM Agent，基于 LLM Flow 的 ReAct 风格代理 |
| `GraphAgent` | `agent/graphagent/` | 图编排 Agent，基于 `graph.Graph` 执行 DAG |
| `ChainAgent` | `agent/chainagent/` | 顺序链式 Agent，子代理串行执行 |
| `CycleAgent` | `agent/cycleagent/` | 循环 Agent，子代理循环执行直到升级条件或最大迭代 |
| `ParallelAgent` | `agent/parallelagent/` | 并行 Agent，子代理并发执行 |
| `A2AAgent` | `agent/a2aagent/` | A2A 协议 Agent，对接外部 A2A 服务 |
| `ClaudeCode Agent` | `agent/claudecode/` | Claude Code CLI 集成 |
| `Codex Agent` | `agent/codex/` | OpenAI Codex CLI 集成 |
| `Dify Agent` | `agent/dify/` | Dify 平台集成 |
| `N8N Agent` | `agent/n8n/` | N8N 工作流集成 |
| `LazyAgent` | `agent/lazy_agent.go` | 延迟构造代理 |
| `TODOEnforcer` | `agent/extension/todoenforcer/` | 内置扩展：TODO 强制执行器 |
| `ToolPipe` | `agent/extension/toolpipe/` | 内置扩展：工具管道 |

---

## 二、项目实现现状

### 2.1 框架接口实现情况

| 框架接口/功能 | 项目实现 | 合规性 | 说明 |
|--------------|---------|--------|------|
| `llmagent.New(name, opts...)` | ✅ 完全实现 | ✅ | 通过 `BuildTRPCLLMAgent()` 封装，所有 Option 合规使用 |
| `WithModel()` | ✅ 完全实现 | ✅ | 通过 `TRPCModelForProviderModel()` 构建模型实例 |
| `WithInstruction()` | ✅ 完全实现 | ✅ | 从数据库配置 + 提示文件 + 岗位职责 + 行业上下文构建 |
| `WithToolSets()` + `WithTools()` | ✅ 完全实现 | ✅ | `buildToolsetsForAgent()` 组装（含 MCP/Broker/Knowledge/Custom/Kanban/Deferred） |
| `WithSkills()` + `WithSkillFilter()` | ✅ 完全实现 | ✅ | FS/DB 双 Repository + `AgentVisibilityFilter` |
| `WithCodeExecutor()` | ✅ 完全实现 | ✅ | 多后端工厂（local/docker/e2b/container） |
| `WithPlanner()` | ✅ 完全实现 | ✅ | `agentplanner.Select()` 按 DialogMode/PlannerKind 选择 |
| `WithModelSelector()` | ✅ 完全实现 | ✅ | 多选择器链式组合（Plugin/CostGuard/CostAware/QualityAware/LatencyAware） |
| `WithAgentCallbacks()` | ✅ 完全实现 | ✅ | 通过 Callback Chain 适配 |
| `WithModelCallbacks()` | ✅ 完全实现 | ✅ | 通过 Callback Chain 适配 |
| `WithToolCallbacks()` | ✅ 完全实现 | ✅ | 通过 Callback Chain 适配 |
| `WithEnableContextCompaction()` | ✅ 完全实现 | ✅ | 含阈值/保留/截断配置 |
| `WithAddSessionSummary()` | ✅ 完全实现 | ✅ | — |
| `WithPreloadMemory()` | ✅ 完全实现 | ✅ | — |
| `WithEnableParallelTools()` | ✅ 完全实现 | ✅ | — |
| `WithToolFilter()` | ✅ 完全实现 | ✅ | 基于 DeferredManager |
| `WithToolCallRetryPolicy()` | ✅ 完全实现 | ✅ | — |
| `WithOutputSchema()` | ✅ 完全实现 | ✅ | — |
| `WithModelInstructions()` | ✅ 完全实现 | ✅ | 从 `ModelInstructionsJSON` 解析 |
| `WithSkillToolProfile()` | ✅ 完全实现 | ✅ | — |
| `WithSkillLoadMode()` | ✅ 完全实现 | ✅ | — |
| `WithSkillsDirectoryHints()` | ✅ 完全实现 | ✅ | — |
| `WithSkillsLoadedContentInToolResults()` | ✅ 完全实现 | ✅ | — |
| `WithContextCompactionThresholdRatio()` | ✅ 完全实现 | ✅ | — |
| `WithContextCompactionKeepRecentRequests()` | ✅ 完全实现 | ✅ | — |
| `WithContextCompactionOversizedToolResultMaxTokens()` | ✅ 完全实现 | ✅ | — |
| `WithChannelBufferSize()` | ✅ 完全实现 | ✅ | 固定 256 |
| `WithGenerationConfig()` | ✅ 完全实现 | ✅ | 固定 Stream: true |
| `WithDescription()` | ✅ 完全实现 | ✅ | — |
| `chainagent.New()` | ✅ 完全实现 | ✅ | `BuildChainAgent()` |
| `cycleagent.New()` | ✅ 完全实现 | ✅ | `BuildCycleAgent()`（含 MaxIterations/EscalationFunc） |
| `parallelagent.New()` | ✅ 完全实现 | ✅ | `BuildParallelAgent()` |
| `a2aagent.New()` | ✅ 完全实现 | ✅ | `BuildTRPCA2AAgent()` |
| `claudecode.New()` | ✅ 完全实现 | ✅ | `BuildClaudeCodeAgent()` |
| `LazyAgent` | ❌ 未使用 | ⚠️ | 项目使用 `AgentFactory` 替代，功能等价但集成方式不同 |
| `Extension` 机制 | ❌ 未使用 | ⚠️ | 项目使用 Callback Chain 替代，未使用框架的 Agent 级扩展 |
| `TransferController` | ❌ 未使用 | — | 项目未实现 transfer 限流策略 |
| `structure.Exporter` | ❌ 未使用 | — | 项目未使用静态结构导出 |
| `WithEndInvocationAfterTransfer()` | ❌ 未使用 | — | — |
| `WithDefaultTransferMessage()` | ❌ 未使用 | — | — |
| `WithActivatableToolSets()` | ❌ 未使用 | — | 项目使用自建 DeferredManager 替代 |
| `WithRefreshToolSetsOnRun()` | ❌ 未使用 | — | 项目显式禁用（注释说明原因：避免每次 LLM 调用刷新 MCP） |
| `WithSubAgents()` | ✅ 部分使用 | ✅ | 仅在编排 Agent 中使用，LLM Agent 不使用 |
| `WithSurfacePatch()` | ❌ 未使用 | — | 项目未使用 Graph 节点级配置差异 |

### 2.2 自建功能清单

| 自建功能 | 实现位置 | 替代框架功能 | 自建原因 |
|---------|---------|-------------|---------|
| BuildCache（LRU + singleflight + dirty-mark） | `internal/agent/cache.go` | 框架无缓存机制 | 框架每次 `Run()` 都需重新构建 Agent，项目通过缓存避免重复构建（2-15s 冷启动） |
| AgentFactory 替代 LazyAgent | `internal/agent/factory.go` | `agent.LazyAgent` | 项目需按 `agent_key` 从数据库动态构建，框架 LazyAgent 仅支持按 name 查找 |
| Callback Chain | `internal/agent/callback_chain.go` + `internal/agent/callbacks/` | `extension.Extension` | 产品层回调链（Metrics/Cue/Memory/Confirmation/CircuitBreaker 等），比 Extension 更灵活 |
| DeferredManager（工具延迟可见） | `internal/agent/tool_assembly.go` | `WithActivatableToolSets()` | 项目实现了按需激活的工具可见性管理，与框架 ActivatableToolSets 机制不同 |
| Agent 目录管理 | `internal/biz/agent_usecase.go` | 框架无此功能 | 框架无 Agent 持久化目录，项目自建完整 CRUD + 状态机 |
| Agent 匹配 | `internal/agent/agent_matcher.go` | 框架无此功能 | 框架无 Agent 匹配，项目实现 Jaccard + TF-IDF 混合匹配 |
| Agent 分配器 | `internal/agent/agent_allocator_impl.go` | 框架无此功能 | 三层匹配（精确/语义/LLM 冷启动）+ 性能数据 + 降级策略 |
| Agent 状态机 | `internal/biz/agent_state_machine.go` | 框架无此功能 | 框架无 Agent 生命周期管理，项目自建三态状态机 |
| 运行时设置域视图 | `internal/biz/agent_settings.go` | 框架无此功能 | 140 字段扁平结构 + 8 个域视图访问器 |
| Tool 确认门 | `internal/agent/tool_confirm_gate.go` | 框架无此功能 | 敏感工具执行前的人工确认 |
| Tool 命令安全 | `internal/agent/tool_command_safety.go` | 框架无此功能 | 危险命令拦截 |
| Tool 熔断器 | `internal/agent/tool_circuit_breaker.go` | 框架无此功能 | 工具级熔断保护 |
| Tool 结果门 | `internal/agent/tool_result_gate_hook.go` | 框架无此功能 | 工具结果过滤/截断 |
| Runtime Cue 注入 | `internal/agent/runtime_cue_inject.go` | 框架无此功能 | 动态运行时提示注入 |
| Skill 引导注入 | `internal/agent/skill_guidance_inject.go` | 框架无此功能 | 技能使用引导 |
| Memory 注入 | `internal/agent/memory_inject.go` + `working_memory_inject.go` | 框架无此功能 | 记忆注入到上下文 |
| Knowledge 注入 | `internal/agent/knowledge_inject.go` | 框架无此功能 | 知识库检索注入 |
| A2UI Pipeline | `internal/agent/a2ui/` | 框架无此功能 | Agent-to-UI 结构化输出管道 |
| Model Selector | `internal/agent/model_selector.go` | 框架有 `ModelSelector` 接口 | 项目实现了 cost-aware/quality-aware/latency-aware 选择器，合规使用框架接口 |
| 代码执行器工厂 | `internal/agent/codeexecutor/` | 框架有 `CodeExecutor` 接口 | 多后端工厂（local/docker/e2b/container），合规使用框架接口 |
| L0 快照持久化 | `internal/agent/l0_snapshot_persist.go` | 框架无此功能 | 上下文压缩快照持久化 |
| Ralph Loop | `internal/agent/ralph_loop.go` | 框架无此功能 | 承诺-验证循环 |
| Intent Pass | `internal/agent/intent/` | 框架无此功能 | 预 Turn 意图分类 |
| Activity 投射 | `internal/agent/activity_projector.go` | 框架无此功能 | 运行时活动投射/发布/持久化 |
| Prompt 预览 | `internal/agent/prompt_preview.go` | 框架无此功能 | 系统提示预览报告 |
| Tool Invocation 记录 | `internal/agent/tool_invocation_recorder.go` | 框架无此功能 | 工具调用审计 |
| Tool 结果缓存 | `internal/agent/tool_result_cache.go` | 框架无此功能 | 工具结果缓存 |

### 2.3 未使用的框架功能

| 框架功能 | 未使用原因 | 是否需要启用 |
|---------|-----------|-------------|
| `LazyAgent` | 项目使用 `AgentFactory` 替代，功能等价但集成方式不同（按 agent_key 动态构建 vs 按 name 查找） | 评估中 — 若框架 LazyAgent 增强支持按 key 查找可考虑迁移 |
| `extension.Extension` | 项目使用 Callback Chain 替代，更灵活且已有完整产品层实现 | 否 — Callback Chain 功能更丰富，迁移收益不大 |
| `TransferController` | 项目未实现 transfer 限流策略，当前无业务需求 | 否 — 当前无需求 |
| `structure.Exporter` | 项目未使用静态结构导出 | 评估中 — 可用于可视化和管理界面 |
| `WithEndInvocationAfterTransfer()` | 项目未使用 transfer 后结束调用行为 | 否 — 当前无需求 |
| `WithDefaultTransferMessage()` | 项目未使用默认 transfer 消息 | 否 — 当前无需求 |
| `WithActivatableToolSets()` | 项目使用自建 DeferredManager 替代 | 评估中 — 功能重叠但机制不同 |
| `WithRefreshToolSetsOnRun()` | 项目显式禁用，避免每次 LLM 调用刷新 MCP（0.2-5s 开销） | 否 — 已有更好的缓存策略 |
| `WithSurfacePatch()` | 项目未使用 Graph 节点级配置差异 | 否 — 当前无需求 |
| `WithKnowledge()` / `WithKnowledgeFilter()` | 项目通过 Callback Chain 注入知识而非框架原生知识检索 | 评估中 — 可简化知识注入逻辑 |
| `WithPreloadSessionRecall()` | 项目未使用会话召回预加载 | 评估中 — 可增强记忆能力 |
| `WithAddCurrentTime()` / `WithTimezone()` | 项目未使用框架时间注入 | 评估中 — 可替代自建时间注入 |
| `WithMaxLLMCalls()` / `WithMaxToolIterations()` | 项目未使用框架安全限制 | 评估中 — 可替代自建限制逻辑 |
| `WithMessageFilterMode()` / `WithMessageTimelineFilterMode()` | 项目未使用框架消息过滤 | 评估中 — 可简化消息处理 |
| `WithReasoningContentMode()` | 项目未使用推理内容模式 | 否 — 当前无需求 |

---

## 三、对比分析

### 3.1 框架优势（项目应采纳的）

| # | 框架优势 | 项目现状 | 对齐收益 |
|---|---------|---------|---------|
| 1 | `WithKnowledge()` + `WithKnowledgeFilter()` 原生知识检索 | 项目通过 Callback Chain BeforeModel 注入知识 | 简化知识注入逻辑，减少约 50 行自建代码，知识检索与框架 Skill/Tool 生命周期一致 |
| 2 | `WithPreloadSessionRecall()` 会话召回预加载 | 项目未使用会话召回 | 增强记忆能力，自动从历史会话中召回相关上下文 |
| 3 | `WithAddCurrentTime()` + `WithTimezone()` 时间注入 | 项目未使用框架时间注入 | 减少自建时间注入代码，框架自动处理时区和格式 |
| 4 | `WithMaxLLMCalls()` + `WithMaxToolIterations()` 安全限制 | 项目未使用框架安全限制 | 利用框架内置计数器和 StopError，减少自建限制逻辑 |
| 5 | `WithMessageFilterMode()` 消息过滤 | 项目自建消息过滤逻辑 | 利用框架内置消息过滤，减少自建代码 |
| 6 | `structure.Exporter` 静态结构导出 | 项目未使用 | 可用于前端可视化和管理界面，零成本获得 Agent 结构快照 |
| 7 | `TransferController` transfer 限流 | 项目无 transfer 限流 | 防止 transfer_to_agent 滥用，增强系统稳定性 |
| 8 | `extension.Extension` Agent 级扩展 | 项目使用 Callback Chain | 框架扩展机制更规范（工具+回调统一注册），但迁移成本较高 |

### 3.2 项目优势（框架缺失的）

| # | 项目优势 | 框架现状 | 建议处理 |
|---|---------|---------|---------|
| 1 | BuildCache（LRU + singleflight + dirty-mark） | 框架无缓存机制 | 贡献回框架 — 设计精妙（缓存键排除 Provider/Model），其他使用者也能受益 |
| 2 | AgentFactory 按 agent_key 动态构建 | 框架 LazyAgent 仅支持按 name 查找 | 贡献回框架 — 增强 LazyAgent 支持自定义查找键 |
| 3 | Callback Chain 产品层回调链 | 框架 Extension 机制较简单 | 保持自建 — 功能更丰富，迁移收益不大 |
| 4 | DeferredManager 工具延迟可见 | 框架 ActivatableToolSets 机制不同 | 评估中 — 两者功能重叠但机制不同，需进一步对比 |
| 5 | Agent 目录管理（CRUD + 状态机） | 框架无此功能 | 保持自建 — 属于业务层，框架不应包含 |
| 6 | Agent 匹配/分配器 | 框架无此功能 | 保持自建 — 属于业务层 |
| 7 | Tool 确认门/命令安全/熔断器/结果门 | 框架无此功能 | 贡献回框架（Tool 熔断器）或保持自建 |
| 8 | A2UI Pipeline | 框架无此功能 | 贡献回框架 — 通用能力，其他使用者可能需要 |
| 9 | Runtime Cue / Skill 引导 / Memory 注入 | 框架无此功能 | 保持自建 — 产品层增值，通过 Callback Chain 集成 |
| 10 | Ralph Loop（承诺-验证循环） | 框架无此功能 | 贡献回框架 — 通用编排模式 |
| 11 | Intent Pass（预 Turn 意图分类） | 框架无此功能 | 保持自建 — 产品层增值 |

### 3.3 差异根因分析

| 差异点 | 根因 | 影响范围 |
|--------|------|---------|
| LazyAgent 未使用 | 功能差异 — 框架 LazyAgent 仅支持按 name 查找，项目需按 agent_key 从数据库动态构建 | `internal/agent/factory.go` |
| Extension 未使用 | 架构决策 — 项目先于框架 Extension 机制开发了 Callback Chain，功能更丰富且已稳定 | `internal/agent/callback_chain.go` + `internal/agent/callbacks/` |
| TransferController 未使用 | 认知缺失 — 项目未意识到框架提供 transfer 限流能力 | — |
| structure.Exporter 未使用 | 认知缺失 — 项目未意识到框架提供静态结构导出能力 | — |
| WithKnowledge 未使用 | 架构决策 — 项目通过 Callback Chain 注入知识，与框架知识检索生命周期不一致 | `internal/agent/knowledge_inject.go` |
| WithActivatableToolSets 未使用 | 功能差异 — 项目 DeferredManager 按需激活机制与框架 ActivatableToolSets 激活机制不同 | `internal/agent/tool_assembly.go` |
| WithRefreshToolSetsOnRun 显式禁用 | 架构决策 — 项目发现每次 LLM 调用刷新 MCP 导致 0.2-5s 开销，改用缓存+失效策略 | `internal/agent/trpc_build.go` |
| 安全限制（MaxLLMCalls/MaxToolIterations）未使用 | 认知缺失 — 项目未启用框架内置安全限制 | — |
| 时间注入未使用 | 认知缺失 — 项目未启用框架时间注入 | — |

---

## 四、对齐方案

### 4.1 对齐项清单

| # | 对齐项 | 类型 | 优先级 | 影响范围 | 预期收益 |
|---|--------|------|--------|---------|---------|
| 1 | 启用框架知识检索（WithKnowledge + WithKnowledgeFilter） | 启用框架功能 | P3 | `internal/agent/knowledge_inject.go` | 代码减少约 50 行，知识检索与框架生命周期一致 |
| 2 | 启用框架安全限制（WithMaxLLMCalls + WithMaxToolIterations） | 启用框架功能 | P3 | `internal/agent/trpc_build.go` | 利用框架内置计数器，减少自建限制逻辑 |
| 3 | 启用框架时间注入（WithAddCurrentTime + WithTimezone） | 启用框架功能 | P3 | `internal/agent/trpc_build.go` | 减少自建时间注入代码 |
| 4 | 评估 ActivatableToolSets vs DeferredManager | 评估中 | P3 | `internal/agent/tool_assembly.go` | 可能减少自建代码 |
| 5 | 贡献 BuildCache 回框架 | 贡献回框架 | P3 | `internal/agent/cache.go` | 其他使用者受益，框架升级时减少适配 |
| 6 | 贡献 AgentFactory 增强（按 key 查找）回框架 | 贡献回框架 | P3 | `internal/agent/factory.go` | 增强 LazyAgent 能力，未来可迁移 |
| 7 | 评估 structure.Exporter 用于可视化 | 评估中 | P4 | 前端可视化 | 零成本获得 Agent 结构快照 |
| 8 | 评估 TransferController 用于 transfer 限流 | 评估中 | P4 | `internal/agent/trpc_runtime.go` | 增强系统稳定性 |

### 4.2 对齐项详情

#### 对齐项 #1：启用框架知识检索

**类型**：启用框架功能

**现状**：
- 项目当前实现：通过 Callback Chain BeforeModel 钩子（`newKnowledgeCueBeforeHook`）在每次 LLM 调用前注入知识
- 框架提供能力：`WithKnowledge()` + `WithKnowledgeFilter()` + `WithKnowledgeConditionedFilter()` + `WithEnableKnowledgeAgenticFilter()` 原生知识检索

**对齐方案**：
1. 评估框架知识检索的生命周期与项目知识注入的时机是否一致
2. 若一致，将 `knowledge_inject.go` 中的逻辑迁移为 `WithKnowledge()` + `WithKnowledgeFilter()` Option
3. 保留 Callback Chain 钩子作为 fallback，直到框架知识检索验证通过

**代码变更范围**：
- 新增：无
- 修改：`internal/agent/trpc_build.go`（添加 WithKnowledge Option）
- 修改：`internal/agent/knowledge_inject.go`（标记 deprecated）
- 删除：待验证通过后删除 `knowledge_inject.go` 中的 Callback Chain 钩子

**兼容性风险**：
- 框架知识检索的注入时机可能与项目 Callback Chain 注入时机不同
- 框架知识检索的结果格式可能与项目前端期望不同

**回退方案**：
- 保留 Callback Chain 钩子，仅添加框架 Option 作为补充

**验证方法**：
- 对比框架知识检索和 Callback Chain 注入的知识内容一致性
- 验证前端知识展示不受影响

**预期收益**：
- 代码减少：约 50 行
- 性能影响：中性（框架知识检索可能更高效）
- 维护成本：减少框架升级时的适配工作

---

#### 对齐项 #2：启用框架安全限制

**类型**：启用框架功能

**现状**：
- 项目当前实现：未使用框架安全限制，依赖外部控制
- 框架提供能力：`WithMaxLLMCalls(n)` + `WithMaxToolIterations(n)` 内置计数器和 StopError

**对齐方案**：
1. 在 `buildTRPCRuntimeOptions()` 中根据 Agent 设置添加 `WithMaxLLMCalls()` 和 `WithMaxToolIterations()`
2. 从 `AgentRuntimeSettings` 中读取配置值

**代码变更范围**：
- 新增：无
- 修改：`internal/agent/trpc_build.go`（在 `buildTRPCRuntimeOptions` 中添加 Option）
- 删除：无

**兼容性风险**：
- 低 — 框架安全限制是纯增量功能，不影响现有逻辑

**回退方案**：
- 不添加 Option 即可回退

**验证方法**：
- 设置低阈值验证 StopError 触发
- 验证错误信息正确传递到前端

**预期收益**：
- 代码减少：约 10 行（减少自建限制逻辑）
- 性能影响：中性
- 维护成本：减少

---

#### 对齐项 #3：启用框架时间注入

**类型**：启用框架功能

**现状**：
- 项目当前实现：未使用框架时间注入
- 框架提供能力：`WithAddCurrentTime(true)` + `WithTimezone(tz)` + `WithTimeFormat(fmt)`

**对齐方案**：
1. 在 `buildTRPCRuntimeOptions()` 中添加 `WithAddCurrentTime(true)`
2. 根据系统设置注入时区

**代码变更范围**：
- 新增：无
- 修改：`internal/agent/trpc_build.go`（添加 Option）
- 删除：无

**兼容性风险**：
- 低 — 框架时间注入是纯增量功能

**回退方案**：
- 不添加 Option 即可回退

**验证方法**：
- 验证系统提示中包含当前时间信息
- 验证时区正确

**预期收益**：
- 代码减少：约 10 行
- 功能增强：Agent 自动感知当前时间

---

#### 对齐项 #4：评估 ActivatableToolSets vs DeferredManager

**类型**：评估中

**现状**：
- 项目当前实现：`DeferredManager` 按需激活工具（工具在首次被引用时才变为可见）
- 框架提供能力：`WithActivatableToolSets()` 提供可激活工具集机制

**对齐方案**：
1. 深入对比 DeferredManager 和 ActivatableToolSets 的激活机制
2. 若框架机制可覆盖项目需求，迁移到框架实现
3. 若框架机制不足，保持 DeferredManager 并贡献增强建议

**代码变更范围**：
- 待评估

**兼容性风险**：
- 中 — 工具可见性逻辑影响 Agent 行为

**回退方案**：
- 保持 DeferredManager

**验证方法**：
- 对比两种机制的工具激活时机和行为
- 验证工具列表展示正确

**预期收益**：
- 代码减少：约 100 行（若迁移成功）
- 维护成本：减少

---

#### 对齐项 #5：贡献 BuildCache 回框架

**类型**：贡献回框架

**现状**：
- 项目当前实现：`BuildCache`（LRU + singleflight + dirty-mark），缓存键排除 Provider/Model
- 框架提供能力：无缓存机制

**对齐方案**：
1. 将 `BuildCache` 提取为独立包 `agent/buildcache/`
2. 提交 PR 到 trpc-agent-go 框架
3. 项目改为依赖框架 BuildCache

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/agent/buildcache/`（框架侧）
- 修改：`internal/agent/cache.go`（改为 import 框架包）
- 删除：`internal/agent/cache.go`（迁移后）

**兼容性风险**：
- 低 — BuildCache 是独立组件，无外部依赖

**回退方案**：
- 保持项目自建 BuildCache

**验证方法**：
- 框架 BuildCache 单元测试通过
- 项目集成测试验证缓存行为不变

**预期收益**：
- 代码减少：约 150 行
- 维护成本：减少框架升级时的适配
- 功能增强：其他使用者也能受益

---

#### 对齐项 #6：贡献 AgentFactory 增强回框架

**类型**：贡献回框架

**现状**：
- 项目当前实现：`BizAgentFactoryOptions()` 为 Runner 注册 per-agent-key 工厂，支持按 `agent_key` 从数据库动态构建
- 框架提供能力：`LazyAgent` 仅支持按 name 查找

**对齐方案**：
1. 增强 `LazyAgent` 支持自定义查找键（泛化 factory 函数签名）
2. 提交 PR 到 trpc-agent-go 框架
3. 项目改为使用增强后的 LazyAgent

**代码变更范围**：
- 新增：无
- 修改：`pkg/trpc-agent-go/agent/lazy_agent.go`（框架侧，增强 factory 签名）
- 修改：`internal/agent/factory.go`（改为使用增强后的 LazyAgent）
- 删除：`internal/agent/factory.go`（迁移后）

**兼容性风险**：
- 中 — LazyAgent 是框架核心组件，修改需谨慎

**回退方案**：
- 保持项目自建 AgentFactory

**验证方法**：
- 框架 LazyAgent 单元测试通过
- 项目集成测试验证 transfer/swarm 解析正常

**预期收益**：
- 代码减少：约 40 行
- 维护成本：减少
- 功能增强：LazyAgent 更通用

---

## 五、实施路线

### 5.1 阶段规划

| 阶段 | 对齐项 | 前置依赖 | 预计工作量 |
|------|--------|---------|-----------|
| Phase 1 | #2（安全限制）、#3（时间注入） | 无 | 小 — 仅添加 Option |
| Phase 2 | #1（知识检索） | Phase 1 | 中 — 需验证注入时机一致性 |
| Phase 3 | #4（ActivatableToolSets 评估） | Phase 2 | 中 — 需深入对比机制 |
| Phase 4 | #5（BuildCache 贡献）、#6（AgentFactory 贡献） | 框架 PR 流程 | 大 — 需框架团队协作 |
| Phase 5 | #7（structure.Exporter 评估）、#8（TransferController 评估） | Phase 4 | 小 — 评估为主 |

### 5.2 风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| 框架知识检索注入时机与项目不一致 | 中 | 中 | 保留 Callback Chain 钩子作为 fallback |
| 框架安全限制 StopError 格式与前端期望不一致 | 低 | 低 | 前端适配 StopError 格式 |
| ActivatableToolSets 机制不足以替代 DeferredManager | 中 | 中 | 保持 DeferredManager，贡献增强建议 |
| BuildCache/AgentFactory PR 被框架团队拒绝 | 低 | 低 | 保持项目自建实现 |
| 框架 LazyAgent 增强破坏现有行为 | 低 | 中 | 充分单元测试 + 集成测试 |

---

## 六、附录

### A. 框架示例代码参考（必填）

| 示例 | 路径 | 关键 API | 初始化模式 | 与项目实现差异 |
|------|------|---------|-----------|--------------|
| React 示例 | `examples/react/main.go` | `llmagent.New()` + `WithModel()` + `WithInstruction()` + `WithTools()` + `WithPlanner()` + `WithGenerationConfig()` + `runner.NewRunner()` + `runner.Run()` | 直接构造：model → agent → runner → run | 项目通过 `BuildTRPCLLMAgent()` 封装，增加了缓存/回调链/工具集/技能等复杂组装逻辑 |
| Steer 示例 | `examples/steer/main.go` | `llmagent.New()` + `WithModel()` + `WithInstruction()` + `WithGenerationConfig()` + `WithTools()` + `sessioninmemory.NewSessionService()` + `runner.WithSessionService()` + `runner.EnqueueUserMessage()` + `agent.WithRequestID()` | 显式 SessionService + 非流式 + 运行中转向 | 项目使用 SQLite Session（非内存），流式模式，转向通过 `EnqueueTRPCUserMessage()` 封装 |

**对齐验证要点**：
- 项目 `BuildTRPCLLMAgent()` 的 Option 使用方式与 React 示例一致（`llmagent.New(name, opts...)`）
- 项目 Runner 创建方式与示例一致（`runner.NewRunner(appName, agent, opts...)`）
- 项目 Run 执行方式与示例一致（`r.Run(ctx, userID, sessionID, message, opts...)`）
- 差异点均为产品层增值（缓存/回调链/工具集/技能），非框架用法偏差

### B. 框架文档参考

| 文档 | 路径 |
|------|------|
| Agent 概述 | `docs/mkdocs/zh/agent.md` |
| LLMAgent 详细文档 | `docs/mkdocs/zh/agent-llm.md` |
| Extension 扩展 | `docs/mkdocs/zh/agent-extension.md` |
| LazyAgent | `docs/mkdocs/zh/agent-lazy.md` |
| 编排 Agent | `docs/mkdocs/zh/agent-orchestration.md` |
