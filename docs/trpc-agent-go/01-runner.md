# 运行器（Runner）— 框架对齐分析

> 模块路径：`pkg/trpc-agent-go/runner/`
> 项目实现路径：`internal/agent/`、`internal/runtime/`
> 当前对齐度：★★★★☆

---

## 一、框架能力全景

### 1.1 核心接口

| 接口 | 方法 | 说明 |
|------|------|------|
| `Runner` | `Run(ctx, userID, sessionID, message, ...RunOption) (<-chan *event.Event, error)` | 执行一次 Agent 运行，返回事件流 |
| `Runner` | `Close() error` | 关闭 Runner，释放自有资源（幂等安全） |
| `ManagedRunner` | `Cancel(requestID) bool` | 按 requestID 取消运行中调用，返回是否找到匹配 |
| `ManagedRunner` | `RunStatus(requestID) (RunStatus, bool)` | 查询运行状态快照，返回 (状态, 是否存在) |
| `SteerableRunner` | `EnqueueUserMessage(requestID, message) error` | 向活跃请求注入用户消息（await 模式） |
| `Verifier` | `Verify(ctx, *agent.Invocation, *event.Event) (VerifyResult, error)` | Ralph Loop 自定义验证器 |
| `RalphLoopCommandRunner` | `Run(ctx, RalphLoopCommandSpec) (RalphLoopCommandResult, error)` | Ralph Loop 命令执行器（可替换） |

**接口继承关系**：`Runner` → `ManagedRunner`（+Cancel/RunStatus）→ `SteerableRunner`（+EnqueueUserMessage）

**关键类型**：

| 类型 | 说明 |
|------|------|
| `AgentFactory` | `func(ctx, agent.RunOptions) (agent.Agent, error)` — 按需构建 Agent 的工厂函数 |
| `RunStatus` | 运行状态快照：RequestID / InvocationID / AgentName / SessionKey / StartedAt / LastEventAt / EventCount |
| `RalphLoopConfig` | Ralph Loop 配置：MaxIterations / CompletionPromise / VerifyCommand / Verifiers 等 |
| `VerifyResult` | 验证结果：Passed / Feedback |
| `RalphLoopCommandSpec` | 命令规格：Command / WorkDir / Env / Timeout |
| `RalphLoopCommandResult` | 命令结果：Stdout / Stderr / ExitCode / Duration / TimedOut |

### 1.2 关键类型

| 类型 | 说明 |
|------|------|
| `runner` struct | 核心实现，持有 appName / agents / agentFactories / sessionService / memoryService / ingestor / artifactService / pluginManager / ralphLoop / runs map |
| `Options` struct | Runner 配置容器，所有 WithXxx Option 写入此结构 |
| `runHandle` struct | 运行中调用的内部句柄：cancel / queue / status |
| `interruptedAssistantAccumulator` | 取消流式运行时累积已发出的助手文本，用于持久化 |

### 1.3 扩展点

| 扩展点 | 机制 | 适用场景 |
|--------|------|---------|
| Agent 注册 | `WithAgent(name, ag)` | 静态注册预构建 Agent |
| Agent 工厂 | `WithAgentFactory(name, factory)` | 按需延迟创建 Agent |
| Session 服务 | 实现 `session.Service` 接口 | 替换会话存储后端 |
| Memory 服务 | 实现 `memory.Service` 接口 | 替换长期记忆后端 |
| Session Ingestor | 实现 `session.Ingestor` 接口 | 自定义会话摄入（如 mem0 桥接） |
| Artifact 服务 | 实现 `artifact.Service` 接口 | 替换制品存储后端 |
| Plugin 系统 | 实现 `plugin.Plugin` 接口 | 注入 BeforeAgent/AfterAgent/BeforeModel/AfterModel/BeforeTool/AfterTool/OnEvent Hook |
| Ralph Loop 验证器 | 实现 `Verifier` 接口 | 自定义验证逻辑 |
| Ralph Loop 命令执行器 | 实现 `RalphLoopCommandRunner` 接口 | 替换 shell 命令执行器（默认 hostRalphLoopRunner） |
| 用户消息重写器 | `agent.WithUserMessageRewriter(rewriter)` | 持久化前重写用户消息 |
| 工具过滤 | `agent.WithToolFilter(filter)` | 运行时动态过滤可用工具 |
| 模型覆盖 | `agent.WithModel(m)` / `agent.WithModelName(name)` | 单次 Run 级别覆盖模型 |
| StreamMode 过滤 | `agent.WithStreamMode(modes...)` | 按模式过滤事件流 |

### 1.4 配置选项

#### Runner 级 Option（`runner` 包）

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithSessionService(service)` | 设置 Session 服务 | `nil` → 自动创建 `inmemory.NewSessionService()` |
| `WithMemoryService(service)` | 设置 Memory 服务 | `nil`（不启用记忆） |
| `WithSessionIngestor(ingestor)` | 设置会话摄入器 | `nil`（不启用外部摄入） |
| `WithArtifactService(service)` | 设置 Artifact 服务 | `nil`（不启用制品存储） |
| `WithAgent(name, ag)` | 注册命名 Agent | 空 map |
| `WithAgentFactory(name, factory)` | 注册命名 Agent 工厂 | 空 map |
| `WithPlugins(plugins...)` | 注册插件 | 空 slice |
| `WithAwaitUserReplyRouting(enabled)` | 启用 await-user-reply 路由 | `false` |
| `WithPersistInterruptedAssistant(enabled)` | 取消时持久化已发出的助手文本 | `false` |
| `WithRalphLoop(cfg)` | 启用 Ralph Loop 模式 | `nil`（不启用） |

#### Run 级 Option（`agent` 包，`agent.RunOption`）

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithRequestID(requestID)` | 设置请求 ID | 自动生成 UUID |
| `WithStream(stream)` | 流式输出开关 | `nil` |
| `WithModel(m)` | 单次运行覆盖模型 | `nil` |
| `WithModelName(name)` | 按名称查找模型 | 空 |
| `WithRuntimeState(state)` | 运行时状态变量 | `nil` |
| `WithAgent(a)` | 单次运行覆盖 Agent | `nil` |
| `WithAgentByName(name)` | 按名称解析 Agent | 空 |
| `WithMessages(messages)` | 提供完整对话历史 | `nil` |
| `WithInjectedContextMessages(messages)` | 注入不持久化的上下文消息 | `nil` |
| `WithUserMessageRewriter(rewriter)` | 用户消息重写器 | `nil` |
| `WithResume(enabled)` | 从已有会话上下文恢复 | `false` |
| `WithKnowledgeFilter(filter)` | 知识库元数据过滤 | `nil` |
| `WithToolFilter(filter)` | 工具过滤函数 | `nil` |
| `WithAdditionalTools(tools)` | 附加工具 | `nil` |
| `WithExternalTools(tools)` | 外部工具（调用方执行） | `nil` |
| `WithMaxRunDuration(d)` | 最大运行时长 | 0（无限制） |
| `WithDetachedCancel(enabled)` | 忽略父 context 取消 | `false` |
| `WithEventChannelBufferSize(size)` | 事件通道缓冲区大小 | 0 |
| `WithStreamMode(modes...)` | 事件流模式过滤 | 空（不过滤） |
| `WithStructuredOutputJSONSchema(...)` | 结构化输出 JSON Schema | `nil` |
| `WithInstruction(instruction)` | 覆盖 Agent 指令 | 空 |
| `WithGlobalInstruction(instruction)` | 覆盖全局指令 | 空 |
| `WithAppName(name)` | 覆盖 Runner 默认 app name | 空（用 Runner 默认） |

### 1.5 框架内置实现

| 实现 | 路径 | 说明 |
|------|------|------|
| `runner` struct | `runner/runner.go` | 核心实现，实现 `SteerableRunner`（含 Runner + ManagedRunner + SteerableRunner） |
| `ralphLoopAgent` | `runner/ralph_loop.go` | Ralph Loop 包装器，实现 `agent.Agent` 接口 |
| `hostRalphLoopRunner` | `runner/ralph_loop.go` | 默认命令执行器，通过 `exec.CommandContext` 执行 shell |
| `plugin.Logging` | `plugin/logging.go` | 内置日志插件 |
| `plugin.GlobalInstruction` | `plugin/global_instruction.go` | 内置全局指令插件 |
| `EnqueueUserMessage()` | `runner/runner.go` | 安全类型断言辅助函数 |
| `RunWithMessages()` | `runner/runner.go` | 便捷入口（传入消息列表而非单条消息） |

**构造函数**：
- `NewRunner(appName, ag, ...Option) Runner` — 静态 Agent 模式
- `NewRunnerWithAgentFactory(appName, defaultAgentName, factory, ...Option) Runner` — 工厂模式

---

## 二、项目实现现状

### 2.1 框架接口实现情况

| 框架接口/功能 | 项目实现 | 合规性 | 说明 |
|--------------|---------|--------|------|
| `Runner.Run()` → `<-chan *event.Event` | `RunTRPCUserTurn()` 封装 `r.Run()` | ✅ 完全合规 | 接口调用方式一致 |
| `ManagedRunner.Cancel(requestID)` | `CancelTRPCRun()` → 类型断言后调用 `mr.Cancel()` | ✅ 完全合规 | 薄封装透传 |
| `ManagedRunner.RunStatus(requestID)` | `TRPCRunStatus()` → 类型断言后调用 `mr.RunStatus()` | ✅ 完全合规 | 薄封装透传 |
| `SteerableRunner.EnqueueUserMessage()` | `EnqueueTRPCUserMessage()` → 透传框架函数 | ✅ 完全合规 | 薄封装透传 |
| `AgentFactory` | `bizAgentFactoryForKey()` 实现动态 Agent 构建 | ✅ 完全合规 | 扩展了按 name 查找为按 agent_key 从数据库动态构建 |
| `WithSessionService()` | 完全使用，SQLite 后端 | ✅ 完全合规 | — |
| `WithMemoryService()` | 完全使用，自建 SQLite/pgvector 实现 | ✅ 接口合规 | 后端为自建实现 |
| `WithArtifactService()` | 完全使用，自建 ServiceAdapter | ✅ 接口合规 | 后端为自建实现 |
| `WithPlugins()` | 完全使用，通过 PluginManager 管理 | ✅ 完全合规 | — |
| `WithRalphLoop()` | 完全使用，从 AgentRuntimeSettings 解析 | ✅ 完全合规 | — |
| `WithSessionIngestor()` | 完全使用，`NewBizSessionIngestor()` 桥接 Memory | ✅ 完全合规 | — |
| `WithAwaitUserReplyRouting()` | 完全使用 | ✅ 完全合规 | — |
| `WithAgent()` | 完全使用，注册预构建 Agent 实例 | ✅ 完全合规 | — |
| `WithAgentFactory()` | 完全使用，注册按需构建工厂 | ✅ 完全合规 | — |

### 2.2 自建功能清单

| 自建功能 | 实现位置 | 替代框架功能 | 自建原因 |
|---------|---------|-------------|---------|
| RunnerManager | `internal/runtime/runner_manager.go` | 框架无对应 | 框架 Runner 无生命周期管理，项目需要集中化 Runner 组装和长生命周期注册 |
| RunRegistry | `internal/runtime/run_registry.go` | 框架无对应 | 框架无 per-session 运行状态跟踪，项目需要管理 activeRuns（sessionID → runner + cancel + runID） |
| RunnerInstanceRegistry | `internal/runtime/runner_registry.go` | 框架无对应 | 框架无 Runner 缓存机制，项目需管理 Team 长期 Runner 实例的缓存和复用 |
| RunGateway | `internal/runtime/gateway.go` | 框架无对应 | 统一的 per-session 运行控制面，被 Chat/Team/Cron/Channel/WebSocket 共享 |
| RunnerRollback | `internal/runtime/runner_rollback.go` + `internal/session/trpc/rollback.go` | 框架无对应 | 框架无事件回滚机制，项目需在 Turn 失败时软删除边界之后的事件 |
| turnStreamConsumer | `internal/agent/stream_consumer.go` | 框架无对应 | 框架只提供 `<-chan *event.Event`，不提供消费逻辑；项目需要 TTFT 跟踪、双投影器、stuck tool 检测 |
| EventProjector | `internal/agent/event_projector.go` | 框架无对应 | 框架事件到业务 Envelope 的映射是项目特有需求 |
| ActivityProjector | `internal/agent/activity_projector.go` | 框架无对应 | AF-1 新架构投影器，将 trpc 事件转为 biz.Activity 语义单元 |
| FrameworkRunStatusFromRunner | `internal/runtime/run_status.go` | 框架 `ManagedRunner.RunStatus()` | 薄适配层，将框架 RunStatus 转为项目内部结构体 |
| CancelTRPCRun | `internal/agent/trpc_runtime.go` | 框架 `ManagedRunner.Cancel()` | 完全透传封装，仅做类型断言 |
| TRPCRunStatus | `internal/agent/trpc_runtime.go` | 框架 `ManagedRunner.RunStatus()` | 完全透传封装，仅做类型断言 |
| EnqueueTRPCUserMessage | `internal/agent/trpc_runtime.go` | 框架 `runner.EnqueueUserMessage()` | 完全透传封装 |

### 2.3 未使用的框架功能

| 框架功能 | 未使用原因 | 是否需要启用 |
|---------|-----------|-------------|
| `WithPersistInterruptedAssistant()` | 项目自建了中断助手持久化逻辑（在 stream_consumer 中处理） | 评估中 — 需确认框架实现是否满足项目需求 |
| `RunWithMessages()` 便捷入口 | 项目使用 `Run()` + `model.NewUserMessage()` | 否 — 项目需要额外的 RunOption 注入 |
| `WithDetachedCancel()` | 项目自建了 context 取消保护（drain Critical 事件） | 评估中 — 需确认框架实现是否覆盖项目的 drain 逻辑 |
| `WithMaxRunDuration()` | 项目通过 context timeout 控制 | 否 — 项目方案等效 |
| `WithStreamMode()` | 项目自建了事件过滤逻辑（EventProjector/ActivityProjector） | 评估中 — 需确认框架过滤是否可替代自建过滤 |
| `WithEventChannelBufferSize()` | 项目使用默认值 0 | 否 — 当前无性能瓶颈 |
| `WithStructuredOutputJSONSchema()` | 项目未使用结构化输出 | 否 — 当前无业务需求 |
| `WithUserMessageRewriter()` | 项目未使用消息重写 | 否 — 当前无业务需求 |
| `WithResume()` | 项目使用 await 模式而非 resume 模式 | 否 — 业务模式不同 |
| `WithInjectedContextMessages()` | 项目通过 `MergeRuntimeState` 注入运行时变量 | 评估中 — 需确认两者适用场景差异 |

---

## 三、对比分析

### 3.1 框架优势（项目应采纳的）

| # | 框架优势 | 项目现状 | 对齐收益 |
|---|---------|---------|---------|
| 1 | `WithPersistInterruptedAssistant()` 框架内置中断助手持久化 | 项目在 stream_consumer 中自建了中断助手累积和持久化逻辑 | 代码减少约 30 行；与框架事件循环一致性行为保证 |
| 2 | `WithDetachedCancel()` 框架内置 context 取消隔离 | 项目自建了 context 取消保护（drain Critical 事件直到 RunnerCompletion） | 代码简化；需验证框架实现是否覆盖 drain 逻辑 |
| 3 | `WithStreamMode()` 框架内置事件流模式过滤 | 项目自建了 EventProjector/ActivityProjector 进行事件分类和过滤 | 可能减少部分过滤逻辑，但投影器有业务特有需求 |
| 4 | 框架 `runner.EnqueueUserMessage()` 包级辅助函数 | 项目 `EnqueueTRPCUserMessage()` 完全透传封装 | 代码减少 3 行（删除冗余封装） |
| 5 | 框架 `ManagedRunner.Cancel()` / `RunStatus()` 直接调用 | 项目 `CancelTRPCRun()` / `TRPCRunStatus()` 完全透传封装 | 代码减少约 10 行（删除冗余封装） |

### 3.2 项目优势（框架缺失的）

| # | 项目优势 | 框架现状 | 建议处理 |
|---|---------|---------|---------|
| 1 | RunnerManager — 集中化 Runner 组装和生命周期管理 | 框架无此功能 | 贡献回框架作为 Runner 扩展包 |
| 2 | RunRegistry — per-session 运行状态跟踪 | 框架无此功能 | 贡献回框架作为 Runner 扩展包 |
| 3 | RunnerInstanceRegistry — 长生命周期 Runner 缓存 | 框架无此功能 | 贡献回框架作为 Runner 扩展包 |
| 4 | RunGateway — 统一运行控制面 | 框架无此功能 | 保持自建（项目特有需求） |
| 5 | RunnerRollback — 事件回滚边界 | 框架无此功能 | 贡献回框架作为 Runner 扩展包 |
| 6 | turnStreamConsumer — 完整事件消费管线 | 框架只提供 channel，不提供消费逻辑 | 保持自建（项目特有需求，含 TTFT/Prometheus/双投影器） |
| 7 | EventProjector / ActivityProjector — 事件到业务模型投影 | 框架无此功能 | 保持自建（项目特有需求） |
| 8 | 三层取消机制（context CancelFunc → ManagedRunner.Cancel() → runner.Close() fallback） | 框架仅提供 ManagedRunner.Cancel() | 评估是否可简化为框架原生机制 |

### 3.3 差异根因分析

| 差异点 | 根因 | 影响范围 |
|--------|------|---------|
| Runner 视为 per-Turn 一次性执行器 | 架构决策 — 项目采用 BUILD → EXECUTE → PERSIST 三阶段 Turn 模型，每 Turn 创建新 Runner | `internal/agent/`、`internal/runtime/`、`internal/service/` |
| 自建 RunnerManager / RunRegistry / RunnerInstanceRegistry | 功能缺失 — 框架 Runner 无生命周期管理，项目需跨 Turn 保持 Team Runner | `internal/runtime/` |
| 自建 RunnerRollback | 功能缺失 — 框架无事件回滚机制 | `internal/runtime/`、`internal/session/trpc/` |
| 自建 turnStreamConsumer / EventProjector / ActivityProjector | 功能缺失 — 框架只提供原始事件流，不提供消费/投影逻辑 | `internal/agent/` |
| CancelTRPCRun / TRPCRunStatus / EnqueueTRPCUserMessage 透传封装 | 认知缺失 — 项目早期为统一接口创建封装，实际可直接使用框架 API | `internal/agent/trpc_runtime.go` |
| AgentFactory 扩展为按 agent_key 从数据库动态构建 | 架构决策 — 框架 AgentFactory 仅支持按 name 查找，项目需从数据库动态构建 | `internal/agent/factory.go` |
| sessionID = requestID | 架构决策 — 使框架的 request_id 与业务 session 对齐 | `internal/service/chat_orchestrator_turn_phases.go` |
| 运行时模型覆盖（`WithModel()` RunOption） | 架构决策 — 实现 Agent 缓存与模型解耦 | `internal/service/chat_orchestrator_turn_phases.go` |

---

## 四、对齐方案

### 4.1 对齐项清单

| # | 对齐项 | 类型 | 优先级 | 影响范围 | 预期收益 |
|---|--------|------|--------|---------|---------|
| 1 | 删除 CancelTRPCRun / TRPCRunStatus / EnqueueTRPCUserMessage 透传封装 | 替换自建实现 | P3 | `internal/agent/trpc_runtime.go`、调用方 | 代码减少约 15 行；减少间接调用层级 |
| 2 | 评估 `WithPersistInterruptedAssistant` 替代自建中断助手持久化 | 启用框架功能 | P3 | `internal/agent/stream_consumer.go` | 代码减少约 30 行；框架一致性保证 |
| 3 | 评估 `WithDetachedCancel` 替代自建 context 取消保护 | 启用框架功能 | P3 | `internal/agent/stream_consumer.go` | 代码简化；需验证 drain 逻辑 |
| 4 | 评估 `WithStreamMode` 替代部分自建事件过滤 | 启用框架功能 | P3 | `internal/agent/event_projector.go` | 可能减少过滤逻辑 |
| 5 | RunnerManager / RunRegistry / RunnerInstanceRegistry 贡献回框架 | 贡献回框架 | P3 | `internal/runtime/` | 长期维护成本降低；框架原生支持 |
| 6 | RunnerRollback 贡献回框架 | 贡献回框架 | P3 | `internal/runtime/`、`internal/session/trpc/` | 长期维护成本降低 |

### 4.2 对齐项详情

#### 对齐项 #1：删除透传封装函数

**类型**：替换自建实现

**现状**：
- 项目在 `internal/agent/trpc_runtime.go` 中定义了 3 个完全透传的封装函数：
  - `CancelTRPCRun(r Runner, requestID string) bool` → 仅做类型断言后调用 `mr.Cancel(requestID)`
  - `TRPCRunStatus(r Runner, requestID string) (RunStatus, bool)` → 仅做类型断言后调用 `mr.RunStatus(requestID)`
  - `EnqueueTRPCUserMessage(r Runner, requestID string, message model.Message) error` → 直接透传 `runner.EnqueueUserMessage()`
- 调用方仅 `RunRegistry` 使用这些函数

**对齐方案**：
1. 在调用方（`RunRegistry`）直接使用框架 API：
   - `runner.EnqueueUserMessage(r, requestID, msg)` 替代 `chatagent.EnqueueTRPCUserMessage(r, requestID, msg)`
   - 类型断言 `r.(trpcrunner.ManagedRunner)` 后直接调用 `Cancel()` / `RunStatus()`
2. 删除 `trpc_runtime.go` 中的 3 个封装函数
3. 更新 `RunRegistry` 中的调用点

**代码变更范围**：
- 修改：`internal/runtime/run_registry.go`（3 处调用点）
- 删除：`internal/agent/trpc_runtime.go` 中 3 个函数（约 15 行）

**兼容性风险**：
- 低 — 仅改变调用路径，行为不变

**回退方案**：
- 恢复封装函数和调用点

**验证方法**：
- `go build ./internal/agent/... ./internal/runtime/...`
- `go test ./internal/runtime/... -count=1`

**预期收益**：
- 代码减少：约 15 行
- 依赖简化：`internal/runtime` 不再依赖 `internal/agent` 的封装函数
- 维护成本：减少间接调用层级，代码更直观

---

#### 对齐项 #2：评估 WithPersistInterruptedAssistant 替代自建中断助手持久化

**类型**：启用框架功能

**现状**：
- 项目在 `turnStreamConsumer` 中自建了中断助手累积逻辑：context 取消时继续 drain 事件，累积已发出的助手文本
- 框架提供 `WithPersistInterruptedAssistant(enabled bool)` Option，在 Runner 事件循环中自动处理中断助手持久化
- 需要验证：框架的实现是否与项目的 drain 逻辑兼容（项目在取消后仍 drain Critical 事件直到 RunnerCompletion）

**对齐方案**：
1. 阅读框架 `WithPersistInterruptedAssistant` 的实现细节（`runner/runner.go` 中的 `shouldPersistInterruptedAssistant` 和 `interruptedAssistantAccumulator`）
2. 对比项目的 drain 逻辑（`stream_consumer.go` 中的 context 取消保护）
3. 如果框架实现满足需求：
   - 在 `NewTRPCRunner` 中添加 `runner.WithPersistInterruptedAssistant(true)` Option
   - 移除 stream_consumer 中的自建中断助手累积逻辑
4. 如果框架实现不满足需求（如不支持 drain Critical 事件）：
   - 保持自建实现，记录差异原因

**代码变更范围**：
- 修改：`internal/agent/trpc_runtime.go`（添加 Option）
- 可能修改：`internal/agent/stream_consumer.go`（移除自建逻辑，约 30 行）
- 可能修改：`internal/runtime/runner_manager.go`（传递配置）

**兼容性风险**：
- 中 — 中断助手持久化是关键路径，需确保行为一致

**回退方案**：
- 移除 Option，恢复自建逻辑

**验证方法**：
- 编写集成测试：发送消息后立即取消 context，验证助手文本是否正确持久化
- `go test ./internal/agent/... -run TestInterruptedAssistant -count=1`

**预期收益**：
- 代码减少：约 30 行
- 维护成本：框架升级时自动获得 bug 修复和优化
- 功能增强：框架实现与事件循环一致性行为保证

---

#### 对齐项 #3：评估 WithDetachedCancel 替代自建 context 取消保护

**类型**：启用框架功能

**现状**：
- 项目在 `turnStreamConsumer` 中自建了 context 取消保护：即使 turnCtx 被取消，仍继续 drain Critical 事件（ToolResult/RunnerCompletion/StateDelta），直到看到 RunnerCompletion 才退出
- 框架提供 `WithDetachedCancel(enabled bool)` RunOption，使运行忽略父 context 取消
- 差异：框架的 `WithDetachedCancel` 是完全忽略取消，项目的 drain 逻辑是选择性忽略（只 drain Critical 事件）

**对齐方案**：
1. 分析框架 `WithDetachedCancel` 的实现：是否在 context 取消后仍发出所有事件直到 RunnerCompletion
2. 如果框架实现与项目需求不完全匹配：
   - 保持自建 drain 逻辑，不启用 `WithDetachedCancel`
   - 记录差异原因：项目需要选择性 drain（仅 Critical 事件），而非完全忽略取消
3. 如果框架实现可满足需求：
   - 在 RunOption 中添加 `agent.WithDetachedCancel(true)`
   - 简化 stream_consumer 中的 drain 逻辑

**代码变更范围**：
- 可能修改：`internal/agent/stream_consumer.go`
- 可能修改：`internal/service/chat_orchestrator_turn_phases.go`（RunOption 构建）

**兼容性风险**：
- 高 — context 取消行为变更可能影响资源释放和超时控制

**回退方案**：
- 移除 RunOption，恢复自建 drain 逻辑

**验证方法**：
- 编写集成测试：取消 context 后验证事件流行为
- 压测：验证资源释放是否正常

**预期收益**：
- 代码简化：可能减少约 20 行 drain 逻辑
- 维护成本：框架保证行为一致性

---

#### 对齐项 #4：评估 WithStreamMode 替代部分自建事件过滤

**类型**：启用框架功能

**现状**：
- 项目自建了 EventProjector 和 ActivityProjector 进行事件分类和过滤
- 框架提供 `WithStreamMode(modes ...StreamMode)` RunOption，可按模式过滤事件流
- 项目的投影器不仅是过滤，还包含事件到业务模型的转换（Envelope / Activity），这是框架不提供的

**对齐方案**：
1. 分析框架 `StreamMode` 的过滤粒度：是否可减少到达投影器的事件量
2. 如果框架过滤可减少投影器处理的事件量：
   - 在 RunOption 中添加 `agent.WithStreamMode(...)` 过滤不需要的事件
   - 投影器仍保留，但处理的事件量减少
3. 如果框架过滤粒度不够细：
   - 保持现状，记录差异原因

**代码变更范围**：
- 可能修改：`internal/service/chat_orchestrator_turn_phases.go`（RunOption 构建）
- 可能修改：`internal/agent/event_projector.go`（减少处理逻辑）

**兼容性风险**：
- 低 — 仅影响事件流过滤，不影响核心逻辑

**回退方案**：
- 移除 StreamMode Option

**验证方法**：
- `go test ./internal/agent/... -count=1`
- 手动测试：验证 WebSocket 推送的事件是否完整

**预期收益**：
- 性能影响：减少投影器处理的事件量，降低 CPU 和内存使用
- 维护成本：可能减少投影器中的过滤逻辑

---

#### 对齐项 #5：RunnerManager / RunRegistry / RunnerInstanceRegistry 贡献回框架

**类型**：贡献回框架

**现状**：
- 项目自建了三个 Runner 生命周期管理组件：
  - `RunnerManager`（`internal/runtime/runner_manager.go`）：集中化 Runner 组装，将 PersistenceSet → TRPCRunnerDeps → Option 组装 → NewTRPCRunner 的流程封装
  - `RunRegistry`（`internal/runtime/run_registry.go`）：per-session 运行状态跟踪，管理 activeRuns（sessionID → runner + cancel + runID）
  - `RunnerInstanceRegistry`（`internal/runtime/runner_registry.go`）：长生命周期 Runner 缓存，支持 Replace/Unregister
- 框架的 Runner 是无状态的，每次 `Run()` 独立执行，不提供生命周期管理

**对齐方案**：
1. 提取通用逻辑，去除项目特有依赖（如 `PersistenceSet`、`TRPCRunnerDeps`）
2. 设计框架级 API：
   - `runner.NewManager(opts ...ManagerOption) *Manager` — Runner 生命周期管理器
   - `Manager.Run(ctx, spec RunSpec) (*RunHandle, error)` — 创建并注册 Runner
   - `Manager.Cancel(sessionID) bool` — 取消运行
   - `Manager.CloseRunner(sessionID) error` — 关闭并注销 Runner
3. 向 trpc-agent-go 提交 RFC / PR
4. 框架合并后，项目切换到框架实现

**代码变更范围**：
- 新增：框架侧 `runner/manager.go`、`runner/run_registry.go`、`runner/instance_registry.go`
- 修改：项目侧 `internal/runtime/` 切换到框架实现
- 删除：项目侧 `runner_manager.go`、`run_registry.go`、`runner_registry.go`（约 300 行）

**兼容性风险**：
- 中 — 需要框架团队评审和接受 API 设计

**回退方案**：
- 框架未合并前保持自建实现

**验证方法**：
- 框架侧：单元测试覆盖 Manager/Registry 生命周期
- 项目侧：集成测试验证 Runner 创建/取消/关闭行为不变

**预期收益**：
- 代码减少：约 300 行（框架合并后）
- 维护成本：框架升级时自动获得优化和 bug 修复
- 功能增强：其他框架用户也可使用 Runner 生命周期管理

---

#### 对齐项 #6：RunnerRollback 贡献回框架

**类型**：贡献回框架

**现状**：
- 项目自建了 RunnerRollback 机制：在 Turn 执行前标记事件边界，失败时软删除边界之后的事件
- 操作 `trpc_session_events` 表，通过 `deleted_at` 字段实现逻辑删除
- 框架无事件回滚机制

**对齐方案**：
1. 评估通用性：RunnerRollback 依赖项目的 `trpc_session_events` 表结构，需抽象为框架级接口
2. 设计框架级 API：
   - `runner.WithRollbackStore(store RollbackStore)` Option
   - `RollbackStore` 接口：`MarkBoundary(ctx, sessionID, eventID)` / `RollbackToBoundary(ctx, sessionID)`
3. 向 trpc-agent-go 提交 RFC / PR
4. 框架合并后，项目实现 `RollbackStore` 接口适配现有逻辑

**代码变更范围**：
- 新增：框架侧 `runner/rollback.go`
- 修改：项目侧 `internal/runtime/runner_rollback.go` 实现 `RollbackStore` 接口
- 可能删除：项目侧部分自建逻辑（约 50 行）

**兼容性风险**：
- 低 — 新增接口，不破坏现有行为

**回退方案**：
- 框架未合并前保持自建实现

**验证方法**：
- 集成测试：Turn 失败后验证事件回滚行为不变

**预期收益**：
- 代码减少：约 50 行（框架合并后）
- 维护成本：框架保证 Rollback 接口稳定性
- 功能增强：其他框架用户也可使用事件回滚

---

## 五、实施路线

### 5.1 阶段规划

| 阶段 | 对齐项 | 前置依赖 | 预计工作量 |
|------|--------|---------|-----------|
| Phase 1 | #1（删除透传封装） | 无 | 小 — 3 个函数删除 + 调用点更新 |
| Phase 2 | #2（WithPersistInterruptedAssistant 评估） | Phase 1 | 中 — 需对比框架实现与自建逻辑 |
| Phase 3 | #3（WithDetachedCancel 评估） | Phase 2 | 中 — 需验证 drain 行为兼容性 |
| Phase 4 | #4（WithStreamMode 评估） | 无 | 小 — 分析过滤粒度即可 |
| Phase 5 | #5（RunnerManager 等贡献回框架） | Phase 1 | 大 — 需框架团队协作 |
| Phase 6 | #6（RunnerRollback 贡献回框架） | Phase 5 | 中 — 依赖 #5 的 API 设计模式 |

### 5.2 风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| 框架 WithPersistInterruptedAssistant 行为与项目 drain 逻辑不兼容 | 中 | 中 | 先详细阅读框架实现，编写对比测试用例 |
| 框架 WithDetachedCancel 不支持选择性 drain | 高 | 中 | 保持自建 drain 逻辑，记录差异原因 |
| 框架团队不接受 RunnerManager 贡献 | 中 | 低 | 保持自建实现，不影响项目运行 |
| 删除透传封装后调用方代码可读性下降 | 低 | 低 | 添加注释说明类型断言原因 |

---

## 六、附录

### A. 框架示例代码参考（必填）

| 示例 | 路径 | 关键 API | 初始化模式 | 与项目实现差异 |
|------|------|---------|-----------|--------------|
| Runner Quickstart | `examples/runner/main.go` | `runner.NewRunner(appName, agent, WithSessionService)` | Model → Tools → LLMAgent → Runner 四层构建链 | 项目增加 RunnerManager/RunRegistry 封装层；项目动态组装 Option 而非静态硬编码 |
| Runner Quickstart | `examples/runner/tools.go` | `function.NewFunctionTool(fn, WithName, WithDescription)` | struct 方法 + jsonschema tag 自动推导 Schema | 项目工具注册通过 DB 配置动态加载，而非代码静态定义 |
| Runner Quickstart | `examples/runner/main.go` | `runner.Run()` → `for evt := range eventChan` + `evt.IsFinalResponse()` | range channel + IsFinalResponse 判断退出 | 项目使用 turnStreamConsumer 消费管线（TTFT/双投影器/stuck tool 检测/drain 保护），远复杂于示例 |
| Runner Quickstart | `examples/runner/main.go` | `runner.NewRunner` + `runner.WithSessionService(sessioninmemory.NewSessionService())` | 单一 WithSessionService Option | 项目使用 9 个 Runner Option（Session/Memory/Artifact/Plugin/Ingestor/Await/RalphLoop/Agent/AgentFactory） |
| Runner Quickstart | `examples/runner/main.go` | `agent.WithRequestID(uuid.New().String())` | 每轮生成唯一 requestID | 项目使用 sessionID 作为 requestID（`agent.WithRequestID(sessionID)`），使框架 request_id 与业务 session 对齐 |

**对齐目标**：对齐项 #1（删除透传封装）后，项目调用框架 API 的方式与示例代码一致（直接使用框架函数/类型断言）。

### B. 框架文档参考

| 文档 | 路径 |
|------|------|
| Runner 包文档 | `pkg/trpc-agent-go/runner/` (GoDoc) |
| Session 服务文档 | `pkg/trpc-agent-go/session/` (GoDoc) |
| Plugin 系统文档 | `pkg/trpc-agent-go/plugin/` (GoDoc) |
| Ralph Loop 文档 | `pkg/trpc-agent-go/runner/ralph_loop.go` (GoDoc) |
