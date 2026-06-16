# 服务端（Server）— 框架对齐分析

> 模块路径：`pkg/trpc-agent-go/server/`
> 项目实现路径：`internal/server/`、`internal/a2a/trpc/`、`internal/service/openai_compat.go`
> 当前对齐度：☆☆☆☆☆

---

## 一、框架能力全景

### 1.1 核心接口

框架 `server/` 包提供 5 个独立子包，每个实现一种协议/用途的 HTTP 服务，共享统一模式：`New(opts ...Option)` 工厂 + `Handler() http.Handler` 暴露路由。

| 接口/工厂 | 方法 | 说明 |
|-----------|------|------|
| `agui.New(runner, opts...)` | `→ (*Server, error)` | 创建 AG-UI 协议服务器（SSE 流式） |
| `agui.Server` | `Handler() http.Handler` | 返回 HTTP 路由 |
| `agui.Server` | `Path() string` | 返回对话端点路径 |
| `agui.Server` | `BasePath() string` | 返回基础路径 |
| `a2a.New(opts...)` | `→ (*a2a.A2AServer, error)` | 创建 A2A 协议服务器 |
| `openai.New(opts...)` | `→ (*Server, error)` | 创建 OpenAI 兼容 API 服务器 |
| `openai.Server` | `Handler() http.Handler` | 返回 HTTP 路由 |
| `openai.Server` | `Close() error` | 关闭服务器 |
| `evaluation.New(opts...)` | `→ (*Server, error)` | 创建在线评估 API 服务器 |
| `evaluation.Server` | `Handler() http.Handler` | 返回 HTTP 路由 |
| `promptiter.New(opts...)` | `→ (*Server, error)` | 创建 PromptIter 优化 API 服务器 |
| `promptiter.Server` | `Handler() http.Handler` | 返回 HTTP 路由 |

### 1.2 关键类型

| 类型 | 包路径 | 说明 |
|------|--------|------|
| `agui.Server` | `server/agui/` | AG-UI 协议服务器，基于 Runner 暴露 SSE 流式端点 |
| `aguirunner.Runner` | `server/agui/runner/` | AG-UI 请求适配器，连接 AG-UI 请求与 trpc-agent-go Runner |
| `translator.Translator` | `server/agui/translator/` | 内部事件 → AG-UI 事件翻译器 |
| `translator.Factory` | `server/agui/translator/` | Translator 工厂函数类型 |
| `a2ui.Translator` | `server/agui/translator/a2ui/` | A2UI Translator，将文本消息解析为 A2UI JSONL |
| `a2a.A2AServer` | `server/a2a/` | A2A 协议服务器（实际类型来自 trpc-a2a-go） |
| `openai.Server` | `server/openai/` | OpenAI 兼容 API 服务器 |
| `evaluation.Server` | `server/evaluation/` | 在线评估 API 服务器 |
| `promptiter.Server` | `server/promptiter/` | PromptIter 优化 API 服务器 |

### 1.3 扩展点

| 扩展点 | 机制 | 适用场景 |
|--------|------|---------|
| `aguirunner.TranslatorFactory` | 替换事件翻译器工厂 | 自定义 AG-UI 事件翻译逻辑（如 A2UI） |
| `aguirunner.UserIDResolver` | 从请求解析用户 ID | 自定义用户身份解析 |
| `aguirunner.AppNameResolver` | 从请求解析应用名 | 动态应用名 |
| `aguirunner.StateResolver` | 从请求解析运行时状态 | 注入自定义运行时状态 |
| `aguirunner.RunOptionResolver` | 从请求解析 agent.RunOption | 自定义运行选项 |
| `aguirunner.RunAgentInputHook` | 请求预处理钩子 | 请求校验/转换/增强 |
| `aguirunner.StartSpan` | OpenTelemetry span 创建 | 自定义链路追踪 |
| `aguirunner.TranslateCallbacks` | 翻译前/后回调 | 事件监控/审计 |
| `agui.WithServiceFactory` | 替换传输层实现 | 自定义 SSE 传输（如 WebSocket） |
| `a2a.ProcessorBuilder` | 自定义消息处理器构建 | 自定义 A2A 消息处理逻辑 |
| `a2a.ProcessMessageHook` | 消息处理钩子（装饰器） | A2A 消息处理中间件 |
| `a2a.TaskManagerBuilder` | 自定义 TaskManager 构建 | 自定义 A2A 任务管理 |
| `a2a.A2AMessageToAgentMessage` | A2A → Agent 消息转换 | 自定义协议转换 |
| `a2a.EventToA2AMessage` | Agent → A2A 事件转换 | 自定义事件转换（一元 + 流式） |
| `a2a.ResponseRewriter` | 出站响应重写 | 响应过滤/脱敏 |
| `a2a.ErrorHandler` | 自定义错误处理 | A2A 错误响应定制 |
| `evaluation.RouteRegistrar` | 自定义路由注册 | 评估 API 路由扩展 |

### 1.4 配置选项

#### AG-UI Server 选项（30+ 个）

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithBasePath(path)` | 基础路径 | `/` |
| `WithPath(path)` | 对话端点路径 | `/` |
| `WithCancelPath(path)` | 取消端点路径 | `/cancel` |
| `WithCancelEnabled()` | 启用取消端点 | false |
| `WithCancelOnContextDoneEnabled()` | 请求 context 取消时终止 run | false |
| `WithDistributedCancelEnabled()` | 多实例分布式取消 | false |
| `WithDistributedCancelPollInterval(d)` | 分布式取消轮询间隔 | — |
| `WithServiceFactory(f)` | 替换传输层实现 | `sse.New` |
| `WithTimeout(d)` | run 最大执行时间 | 1h |
| `WithFlushInterval(d)` | 事件刷盘间隔 | — |
| `WithPostRunFinalizationTimeout(d)` | run 结束后收尾超时 | 5s |
| `WithHeartbeatInterval(d)` | SSE 心跳间隔 | — |
| `WithGraphNodeLifecycleActivityEnabled()` | graph 节点生命周期事件 | false |
| `WithGraphNodeInterruptActivityEnabled()` | graph 中断事件 | false |
| `WithReasoningContentEnabled()` | 推理内容事件 | false |
| `WithEventSourceMetadataEnabled()` | 原始事件元数据 | false |
| `WithToolResultInputTranslationEnabled()` | 工具结果输入翻译 | false |
| `WithToolCallDeltaStreamingEnabled()` | 流式工具调用参数 | false |
| `WithStreamingToolResultActivityEnabled()` | 流式工具结果活动事件 | false |
| `WithMessagesSnapshotEnabled()` | 消息快照 | false |
| `WithMessagesSnapshotPath(path)` | 快照端点路径 | `/history` |
| `WithMessagesSnapshotFollowEnabled()` | 快照后尾随实时事件 | false |
| `WithMessagesSnapshotFollowMaxDuration(d)` | 尾随最大时长 | — |
| `WithMessagesSnapshotRunLifecycleEventsEnabled()` | 快照含 RUN_* 事件 | false |
| `WithAppName(n)` | 应用名 | — |
| `WithAppNameResolver(r)` | 动态应用名解析 | — |
| `WithSessionService(s)` | 会话服务 | — |
| `WithAGUIRunnerOptions(...)` | 透传 Runner 层选项 | — |

#### A2A Server 选项（20+ 个）

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithAgent(agent, enableStreaming)` | 设置 Agent（互斥 WithRunner） | — |
| `WithRunner(r)` | 设置 Runner（互斥 WithAgent） | — |
| `WithAgentCard(card)` | 自定义 AgentCard | — |
| `WithHost(host)` | 服务地址 | — |
| `WithSessionService(s)` | 会话服务 | `inmemory.NewSessionService()` |
| `WithUserIDHeader(header)` | 用户 ID HTTP 头 | `X-User-ID` |
| `WithProcessorBuilder(builder)` | 自定义消息处理器构建 | — |
| `WithProcessMessageHook(hook)` | 消息处理钩子 | — |
| `WithTaskManagerBuilder(builder)` | 自定义 TaskManager 构建 | — |
| `WithRunOptions(...)` | 附加 agent.RunOption | — |
| `WithA2AToAgentConverter(converter)` | A2A→Agent 消息转换 | — |
| `WithEventToA2AConverter(converter)` | Agent→A2A 事件转换 | — |
| `WithGraphEventObjectAllowlist(...)` | graph 事件白名单 | — |
| `WithResponseRewriter(rewriter)` | 出站响应重写 | — |
| `WithADKCompatibility(enabled)` | ADK 兼容模式 | true |
| `WithStreamingEventType(eventType)` | 流式事件类型 | `TaskArtifactUpdate` |
| `WithEventToA2APartMapper(mapper)` | 事件→Part 映射器 | — |
| `WithDebugLogging(debug)` | 调试日志 | false |
| `WithErrorHandler(handler)` | 自定义错误处理 | — |
| `WithStructuredTaskErrors(enable)` | 结构化错误传播 | — |
| `WithExtraA2AOptions(...)` | 透传底层 A2A Server 选项 | — |

#### OpenAI Server 选项

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithBasePath(path)` | 基础路径 | `/v1` |
| `WithPath(path)` | 端点路径 | `/chat/completions` |
| `WithSessionService(svc)` | 会话服务 | `inmemory` |
| `WithAgent(ag)` | 设置 Agent | — |
| `WithRunner(r)` | 设置 Runner | — |
| `WithModelName(name)` | 响应模型名 | `gpt-3.5-turbo` |
| `WithAppName(name)` | 应用名 | `openai-server` |

#### Evaluation Server 选项

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithAppName(name)` | 应用名 | 必需 |
| `WithBasePath(path)` | 基础路径 | `/evaluation` |
| `WithSetsPath/WithMetricsPath/WithRunsPath/WithResultsPath` | 子路径 | `/sets` 等 |
| `WithTimeout(d)` | 超时 | 无限制 |
| `WithAgentEvaluator(e)` | Agent 评估器 | 必需 |
| `WithEvalSetManager(m)` | 评估集管理器 | 可选 |
| `WithMetricManager(m)` | 指标管理器 | 可选 |
| `WithEvalResultManager(m)` | 结果管理器 | 可选 |
| `WithRouteRegistrar(r)` | 自定义路由注册 | 可选 |

#### PromptIter Server 选项

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithAppName(name)` | 应用名 | 必需 |
| `WithBasePath(path)` | 基础路径 | `/promptiter/v1/apps` |
| `WithStructurePath/WithRunsPath/WithAsyncRunsPath` | 子路径 | `/structure` 等 |
| `WithTimeout(d)` | 超时 | 无限制 |
| `WithEngine(e)` | PromptIter 引擎 | 必需 |
| `WithManager(m)` | 异步 run 管理器 | 可选 |

### 1.5 框架内置实现

| 实现 | 路径 | 说明 |
|------|------|------|
| AG-UI SSE Service | `server/agui/service/sse/` | 默认 SSE 传输实现 |
| AG-UI Translator | `server/agui/translator/` | 默认事件翻译器（内部事件 → AG-UI 事件） |
| A2UI Translator | `server/agui/translator/a2ui/` | A2UI JSONL 翻译器（包装默认 Translator） |
| AG-UI Runner | `server/agui/runner/` | AG-UI 请求适配器（含分布式取消、消息快照） |
| A2A AgentCard 构建 | `server/a2a/agent_card.go` | 自动构建符合协议规范的 AgentCard |
| A2A 事件隧道 | `server/a2a/tunnel.go` | 批量事件处理（produce → buffer → batch consume） |
| A2A 状态增量编解码 | `server/a2a/state_delta.go` | StateDelta 元数据编码/解码 |
| OpenAI 格式转换器 | `server/openai/converter/` | OpenAI Chat Completion ↔ Agent 格式转换 |
| Evaluation Langfuse 集成 | `server/evaluation/langfuse/` | Langfuse 评估后端 |
| Gateway | `server/gateway/` | 常驻助手 HTTP 入口（webhook/IM 桥接） |

---

## 二、项目实现现状

### 2.1 框架接口实现情况

| 框架接口/功能 | 项目实现 | 合规性 | 说明 |
|--------------|---------|--------|------|
| `a2a.New()` | ✅ 已使用 | ✅ | 通过 `a2atrpc.BuildA2AEndpointServer()` 桥接，使用 `WithRunner` + `WithAgentCard` |
| `agui.New()` | ❌ 未使用 | ❌ | 项目使用自建 WebSocket 协议替代 AG-UI SSE |
| `openai.New()` | ✅ 已使用 | ✅ | 通过 `OpenAICompatService` 桥接，使用 `WithRunner` + `WithBasePath` + `WithModelName` |
| `evaluation.New()` | ❌ 未使用 | ❌ | 项目自建评估 API（通过 Kratos Proto Service） |
| `promptiter.New()` | ❌ 未使用 | ❌ | 项目无 PromptIter 功能 |
| Gateway | ❌ 未使用 | ❌ | 项目使用 Kratos HTTP 替代 |

**关键发现**：对比分析报告标注 Server 模块对齐度 ☆☆☆☆☆（"框架功能完全未使用"），但实际上 **A2A 和 OpenAI 两个子模块已被集成**。实际对齐度应修正为 ★★☆☆☆。

### 2.2 自建功能清单

| 自建功能 | 实现位置 | 替代框架功能 | 自建原因 |
|---------|---------|-------------|---------|
| Kratos HTTP Server | `internal/server/http.go` | 框架各 Server 的独立 HTTP 监听 | 架构决策：Kratos 提供中间件/鉴权/配置等生产级能力 |
| Kratos gRPC Server | `internal/server/grpc.go` | 无（框架无 gRPC 支持） | Kratos 提供 gRPC 传输，框架无此能力 |
| 自建 WebSocket Server | `internal/server/ws*.go`（7 个文件） | `agui.Server`（SSE 流式） | 历史决策：项目先于 AG-UI 协议存在，WebSocket 满足实时通信需求 |
| WS 三优先级队列 | `internal/server/ws_priority.go` | 无（框架无优先级机制） | 性能优化：区分 alert/normal/low 优先级 |
| WS 事件订阅/重放 | `internal/server/ws_event.go` | `agui.MessagesSnapshot` | 功能差异：项目基于 EventBus 订阅，框架基于 Session 快照 |
| WS 连接管理 | `internal/server/ws_conn.go`、`ws_conn_manager.go` | 无（框架无连接管理） | 多租户需求：Session 模式/Global Monitor 模式/Probe 模式 |
| ServiceRegistry | `internal/server/service_registry.go` | 无 | 30+ 个 Proto 服务的统一注册 |
| A2A EndpointRegistry | `internal/a2a/trpc/registry.go` | 无（框架 A2A 是单 Agent 模式） | 多租户需求：按 agentID 动态路由到不同 A2A Handler |
| A2A Public Base URL 管理 | `internal/service/a2a_public_base.go` | 无 | 运维需求：三级优先级 URL 推导 + 热更新 |
| A2A 健康检查 | `internal/a2a/health/` | 无 | 运维需求：Gateway 健康探测 |
| 评估 API（Kratos Proto） | `internal/service/evaluation.go` | `evaluation.Server` | 架构决策：评估 API 走 Kratos 统一传输层 |
| 自定义中间件链 | `internal/server/` + `middleware/` | 无（框架 Server 无中间件） | 生产需求：CORS/鉴权/就绪门控/工作空间/错误翻译/校验 |

### 2.3 未使用的框架功能

| 框架功能 | 未使用原因 | 是否需要启用 |
|---------|-----------|-------------|
| `agui.Server` | 项目使用自建 WebSocket 替代 SSE | 评估中（见对齐方案） |
| `agui.MessagesSnapshot` | 项目使用自建事件重放 | 评估中 |
| `agui.DistributedCancel` | 项目使用自建取消机制 | 否（WS 模式下不需要） |
| `agui.Translator` | 项目使用自建事件翻译 | 评估中 |
| `agui.A2UI Translator` | 项目未实现 A2UI 协议 | 否（当前无需求） |
| `a2a.ProcessMessageHook` | 项目未使用 A2A 消息处理钩子 | 评估中 |
| `a2a.ResponseRewriter` | 项目未使用 A2A 响应重写 | 评估中 |
| `a2a.ErrorHandler` | 项目未自定义 A2A 错误处理 | 评估中 |
| `a2a.GraphEventObjectAllowlist` | 项目未过滤 graph 事件 | 评估中 |
| `evaluation.Server` | 项目使用 Kratos Proto Service | 否（已有完整实现） |
| `promptiter.Server` | 项目无 PromptIter 功能 | 否（无需求） |
| Gateway | 项目使用 Kratos HTTP 替代 | 否（Kratos 已覆盖） |

---

## 三、对比分析

### 3.1 框架优势（项目应采纳的）

| # | 框架优势 | 项目现状 | 对齐收益 |
|---|---------|---------|---------|
| 1 | AG-UI 协议标准实现（SSE + 事件流） | 自建 WebSocket 协议，与 CopilotKit 等前端框架不兼容 | 获得 CopilotKit 等生态兼容，减少前端适配成本 |
| 2 | AG-UI 事件翻译器（30+ 种事件类型） | 自建事件翻译（仅覆盖项目所需子集） | 减少事件翻译维护，自动获得新事件类型支持 |
| 3 | AG-UI 消息快照（Session 持久化 + 历史回放） | 自建事件重放（基于 EventBus Buffer） | 标准化历史回放，支持断线重连后完整恢复 |
| 4 | AG-UI 分布式取消（多实例场景） | 自建取消机制（单实例） | 支持水平扩展时的 run 取消 |
| 5 | A2A 扩展点（ProcessMessageHook/ResponseRewriter/Error Handler） | 仅使用 WithRunner + WithAgentCard 基础模式 | 获得 A2A 消息审计/过滤/错误定制能力 |
| 6 | A2A Graph 事件白名单 | 未过滤 graph 事件 | 精细控制 A2A 暴露的 graph 事件类型 |
| 7 | OpenAI Server 完整选项（SessionService/AppName） | 仅使用 WithRunner + WithBasePath + WithModelName | 获得 OpenAI 会话持久化能力 |

### 3.2 项目优势（框架缺失的）

| # | 项目优势 | 框架现状 | 建议处理 |
|---|---------|---------|---------|
| 1 | Kratos 统一传输层（HTTP/gRPC/WebSocket + 中间件 + 鉴权 + 配置） | 框架 Server 是轻量级独立 HTTP 入口，无中间件/鉴权 | 保持自建，框架定位不同 |
| 2 | 自建 WebSocket 实时通信（三优先级 + 背压 + 连接管理） | 框架仅提供 SSE，无 WebSocket 支持 | 贡献回框架（WebSocket ServiceFactory）或保持自建 |
| 3 | 多租户 A2A 路由（EndpointRegistry 按 agentID 动态路由） | 框架 A2A 是单 Agent 模式 | 贡献回框架（多租户路由器）或保持自建 |
| 4 | gRPC 传输 | 框架无 gRPC 支持 | 保持自建（Kratos 能力） |
| 5 | 生产级中间件链（CORS/鉴权/就绪门控/工作空间/错误翻译/校验） | 框架 Server 无中间件概念 | 保持自建 |
| 6 | A2A Public Base URL 三级推导 + 热更新 | 框架仅支持 `WithHost` 静态配置 | 贡献回框架或保持自建 |
| 7 | A2A 健康检查（Gateway 探测） | 框架无 A2A 健康检查 | 保持自建 |

### 3.3 差异根因分析

| 差异点 | 根因 | 影响范围 |
|--------|------|---------|
| AG-UI 未使用 | **历史遗留**：项目 WebSocket 实现先于 AG-UI 协议设计，现有前端已深度绑定 WS 协议 | 前端实时通信层、事件翻译层 |
| Kratos 替代框架 Server | **架构决策**：Kratos 提供生产级传输能力（中间件/鉴权/配置/gRPC），框架 Server 定位为轻量级协议入口 | 全部传输层 |
| A2A 仅用基础模式 | **认知缺失**：项目未充分使用框架 A2A 的扩展点（Hook/Rewriter/ErrorHandler） | A2A 消息处理 |
| OpenAI 仅用基础模式 | **认知缺失**：项目未使用 SessionService/AppName 等选项 | OpenAI 会话管理 |
| 评估 API 自建 | **架构决策**：评估 API 走 Kratos 统一传输层，与项目其他 API 保持一致 | 评估模块 |
| WebSocket vs SSE | **功能差异**：项目需要双向通信（用户消息上行），框架 AG-UI 仅提供 SSE 下行 | 实时通信架构 |

---

## 四、对齐方案

### 4.1 对齐项清单

| # | 对齐项 | 类型 | 优先级 | 影响范围 | 预期收益 |
|---|--------|------|--------|---------|---------|
| 1 | 启用 AG-UI 协议端点（与 WS 并行） | 启用框架功能 | P2 | `internal/server/`、前端 | 获得 CopilotKit 生态兼容 |
| 2 | 启用 A2A 扩展点 | 启用框架功能 | P2 | `internal/a2a/trpc/` | A2A 消息审计/过滤/错误定制 |
| 3 | 启用 OpenAI Server 完整选项 | 启用框架功能 | P3 | `internal/service/openai_compat.go` | OpenAI 会话持久化 |
| 4 | 贡献 WebSocket ServiceFactory | 贡献回框架 | P3 | `pkg/trpc-agent-go/server/agui/service/` | 框架获得 WebSocket 传输能力 |
| 5 | 贡献多租户 A2A 路由器 | 贡献回框架 | P3 | `pkg/trpc-agent-go/server/a2a/` | 框架获得多 Agent A2A 路由能力 |

### 4.2 对齐项详情

#### 对齐项 #1：启用 AG-UI 协议端点（与 WS 并行）

**类型**：启用框架功能

**现状**：
- 项目当前：自建 WebSocket 协议实现实时通信（7 个文件，约 1500 行），前端深度绑定 WS 协议
- 框架提供：`agui.Server` 提供 AG-UI 协议标准实现（SSE 流式 + 事件翻译 + 消息快照 + 分布式取消），支持 CopilotKit 等前端框架

**对齐方案**：
1. 在 Kratos HTTP Server 中注册 AG-UI 端点（与 WS 端点并行，不替代）
2. 复用现有 `chatagent.BuildTRPCAgentCached()` + `NewTurnRunner()` 构建 Runner
3. 调用 `agui.New(runner, opts...)` 创建 AG-UI Server，挂载 `srv.Handler()` 到 Kratos 路由
4. 逐步启用 AG-UI 高级功能：消息快照、分布式取消、graph 事件
5. 前端新增 AG-UI SSE 客户端（与现有 WS 客户端并行）

**代码变更范围**：
- 新增：`internal/server/agui.go`（AG-UI 端点注册，约 80 行）
- 修改：`internal/server/http.go`（注册 AG-UI 路由）
- 修改：`internal/conf/conf.proto`（新增 AG-UI 配置项）
- 修改：`cmd/admin/wire.go`（注入 AG-UI 依赖）

**兼容性风险**：
- 低：AG-UI 与 WS 并行运行，不影响现有 WS 通道
- 需注意：AG-UI SSE 是单向推送，用户消息需走 HTTP API（与 WS 双向通信模式不同）

**回退方案**：
- AG-UI 端点可通过配置开关关闭，不影响 WS 通道

**验证方法**：
- 使用 CopilotKit 前端连接 AG-UI 端点，验证对话流式响应
- 使用 AG-UI SDK 测试消息快照、取消功能
- 对比 AG-UI 和 WS 通道的事件完整性

**预期收益**：
- 代码减少：约 0 行（新增而非替换）
- 功能增强：获得 CopilotKit 生态兼容、AG-UI 标准协议支持
- 维护成本：长期可考虑用 AG-UI 替代 WS，减少自建代码维护

---

#### 对齐项 #2：启用 A2A 扩展点

**类型**：启用框架功能

**现状**：
- 项目当前：仅使用 `a2a.New(WithRunner, WithAgentCard)` 基础模式
- 框架提供：`ProcessMessageHook`（消息处理钩子）、`ResponseRewriter`（响应重写）、`ErrorHandler`（错误处理）、`GraphEventObjectAllowlist`（graph 事件白名单）等扩展点

**对齐方案**：
1. 启用 `WithProcessMessageHook`：添加 A2A 消息审计日志（记录入站/出站消息）
2. 启用 `WithGraphEventObjectAllowlist`：过滤 A2A 暴露的 graph 事件类型，仅暴露白名单内的节点
3. 启用 `WithErrorHandler`：统一 A2A 错误响应格式，与项目 API 错误规范对齐
4. 评估 `WithResponseRewriter`：用于 A2A 响应脱敏（如隐藏内部 Agent 信息）

**代码变更范围**：
- 修改：`internal/a2a/trpc/server.go`（添加扩展点 Option）
- 新增：`internal/a2a/trpc/hooks.go`（A2A Hook 实现，约 60 行）

**兼容性风险**：
- 低：扩展点是装饰器模式，不影响现有 A2A 功能

**回退方案**：
- 移除对应 Option 即可回退

**验证方法**：
- A2A 消息审计：发送 A2A 消息后检查审计日志
- Graph 事件过滤：验证非白名单事件被过滤
- 错误处理：触发 A2A 错误，验证响应格式

**预期收益**：
- 代码减少：约 0 行
- 功能增强：A2A 消息审计、graph 事件精细控制、统一错误格式
- 维护成本：减少自建 A2A 监控代码

---

#### 对齐项 #3：启用 OpenAI Server 完整选项

**类型**：启用框架功能

**现状**：
- 项目当前：仅使用 `WithRunner` + `WithBasePath` + `WithModelName`
- 框架提供：`WithSessionService`（会话持久化）、`WithAppName`（应用名）、`WithAgent`（直接注入 Agent）

**对齐方案**：
1. 启用 `WithSessionService`：注入项目的 Session Service 实现，使 OpenAI 兼容 API 支持会话持久化
2. 启用 `WithAppName`：设置应用名为项目 Agent 名称

**代码变更范围**：
- 修改：`internal/service/openai_compat.go`（添加 SessionService 和 AppName Option）

**兼容性风险**：
- 低：仅新增 Option，不改变现有行为

**回退方案**：
- 移除 Option 即可回退

**验证方法**：
- OpenAI 客户端连续对话，验证会话上下文保持

**预期收益**：
- 代码减少：约 0 行
- 功能增强：OpenAI 兼容 API 会话持久化
- 维护成本：减少自建会话管理代码

---

#### 对齐项 #4：贡献 WebSocket ServiceFactory

**类型**：贡献回框架

**现状**：
- 项目当前：自建 WebSocket 服务器（三优先级队列 + 背压 + 连接管理），约 1500 行
- 框架提供：`agui.WithServiceFactory` 扩展点，默认 `sse.New`，可替换为 WebSocket 实现

**对齐方案**：
1. 将项目 WebSocket 传输层抽象为 `agui.ServiceFactory` 实现
2. 提取核心传输逻辑（优先级队列、背压、连接管理）为通用 WebSocket Service
3. 保留项目特有的业务逻辑（Session 模式、Global Monitor 模式、Probe 模式）在项目层
4. 贡献通用 WebSocket Service 到框架 `server/agui/service/ws/`

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/server/agui/service/ws/`（通用 WebSocket Service，约 400 行）
- 修改：`internal/server/ws*.go`（使用框架 WebSocket Service 替代部分自建代码）

**兼容性风险**：
- 中：需要将 WS 事件格式适配为 AG-UI 事件格式
- 需注意：项目 WS 协议与 AG-UI 协议的事件模型不完全一致

**回退方案**：
- 保留自建 WS 实现，框架 WebSocket Service 为可选替代

**验证方法**：
- 现有 WS 客户端连接框架 WebSocket Service，验证功能等价
- 性能对比：优先级队列延迟、背压效果

**预期收益**：
- 代码减少：约 600 行（通用传输逻辑移至框架）
- 依赖简化：减少 `internal/server/` 中 WS 传输代码
- 维护成本：框架统一维护 WebSocket 传输，项目仅维护业务逻辑

---

#### 对齐项 #5：贡献多租户 A2A 路由器

**类型**：贡献回框架

**现状**：
- 项目当前：`EndpointRegistry` 实现按 agentID 动态路由到不同 A2A Handler，支持缓存和热更新
- 框架提供：`a2a.New()` 创建单 Agent A2A Server，无多租户路由

**对齐方案**：
1. 将 `EndpointRegistry` 的核心路由逻辑（按 agentID 路由 + 缓存 + 热更新）抽象为框架级多租户路由器
2. 贡献到框架 `server/a2a/registry.go`
3. 项目保留业务适配层（`A2AEndpointBuilder`、`PublicBaseURLStore`）

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/server/a2a/registry.go`（多租户路由器，约 200 行）
- 修改：`internal/a2a/trpc/registry.go`（使用框架路由器替代自建路由逻辑）

**兼容性风险**：
- 低：路由器是独立组件，不影响现有 A2A 功能

**回退方案**：
- 保留自建 EndpointRegistry

**验证方法**：
- 多 Agent A2A 路由测试：不同 agentID 请求路由到正确 Handler
- 缓存失效测试：Agent 配置变更后 Handler 重建

**预期收益**：
- 代码减少：约 100 行（路由逻辑移至框架）
- 维护成本：框架统一维护多租户路由

---

## 五、实施路线

### 5.1 阶段规划

| 阶段 | 对齐项 | 前置依赖 | 预计工作量 |
|------|--------|---------|-----------|
| Phase 1 | #2（A2A 扩展点）、#3（OpenAI 完整选项） | 无 | 小 |
| Phase 2 | #1（AG-UI 端点） | Phase 1（验证框架 Server 集成模式） | 中 |
| Phase 3 | #4（WebSocket ServiceFactory）、#5（多租户 A2A 路由器） | Phase 2（AG-UI 集成经验） | 大 |

### 5.2 风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| AG-UI SSE 与 WS 协议事件模型不一致 | 高 | 中 | 对齐项 #1 采用并行模式，不替代 WS；长期评估统一 |
| WebSocket ServiceFactory 抽象难度大 | 中 | 中 | 先实现最小可用版本，逐步完善优先级/背压功能 |
| 多租户 A2A 路由器贡献回框架周期长 | 中 | 低 | 先在项目内使用，框架贡献作为后续任务 |
| AG-UI 端点增加运维复杂度 | 低 | 低 | 配置开关控制，默认关闭 |

---

## 六、附录

### A. 框架示例代码参考（必填）

| 示例 | 路径 | 关键 API | 初始化模式 | 与项目实现差异 |
|------|------|---------|-----------|--------------|
| AG-UI 默认服务端 | `examples/agui/server/default/` | `agui.New(runner, agui.WithPath)` | Agent → Runner → AGUI Server → `http.ListenAndServe` | 项目使用 Kratos HTTP 挂载 `srv.Handler()`，而非独立监听；项目未使用 AG-UI |
| A2UI 默认服务端 | `examples/a2ui/server/default/` | `agui.New` + `a2uitranslator.NewFactory` + `aguirunner.WithTranslatorFactory` | Agent(A2UI Planner) → Runner(SessionService) → AGUI Server(A2UI Translator) → `http.ListenAndServe` | 项目未使用 A2UI 协议；若启用 AG-UI 可参考此模式集成 A2UI |
| A2UI SBTI 示例 | `examples/a2ui/server/sbti/` | 双 Agent 节点图 + A2UI 渲染 | Graph(A2UI Agent) → Runner → AGUI Server | 项目 Graph 未集成 A2UI 渲染 |

**示例代码关键差异总结**：
1. **独立监听 vs Kratos 挂载**：示例直接 `http.ListenAndServe`，项目将 `srv.Handler()` 挂载到 Kratos HTTP Server 路由
2. **单 Agent vs 多租户**：示例单 Agent 单 Server，项目 A2A 需要多 Agent 动态路由
3. **SSE vs WebSocket**：示例使用 SSE，项目使用自建 WebSocket
4. **SessionService**：A2UI 示例使用 `inmemory.NewSessionService()`，项目使用自建 Session 实现

### B. 框架文档参考

| 文档 | 路径 |
|------|------|
| AG-UI 使用指南 | `docs/mkdocs/zh/agui/index.md` |
| A2UI 使用文档 | `docs/mkdocs/zh/a2ui.md` |
| Gateway 服务 | `docs/mkdocs/zh/gateway.md` |
