# 工具（Tool）— 框架对齐分析

> 模块路径：`pkg/trpc-agent-go/tool/`
> 项目实现路径：`internal/tools/`、`internal/biz/tool/`、`internal/agent/tool_*.go`
> 当前对齐度：★★★★☆

---

## 一、框架能力全景

### 1.1 核心接口

| 接口 | 方法 | 说明 |
|------|------|------|
| `tool.Tool` | `Declaration() *Declaration` | 基础接口：仅声明元数据 |
| `tool.CallableTool` | `Call(ctx, jsonArgs) (any, error)` + `Tool` | 可调用工具：同步执行 |
| `tool.StreamableTool` | `StreamableCall(ctx, jsonArgs) (*StreamReader, error)` + `Tool` | 可流式工具：流式执行 |
| `tool.ToolSet` | `Tools(ctx) []Tool` / `Close() error` / `Name() string` | 工具集：统一管理工具集合生命周期 |
| `tool.MetadataProvider` | `ToolMetadata() ToolMetadata` | 元数据发布：ReadOnly/Destructive/ConcurrencySafe/OpenWorld 等 |
| `tool.ConcurrencyAware` | `IsConcurrencySafe() bool` | 并发安全声明 |
| `tool.DeferredTool` | `ShouldDefer(ctx) bool` | 延迟加载声明：宿主侧按需暴露 |
| `tool.PermissionChecker` | `CheckPermission(ctx, *PermissionRequest) (PermissionDecision, error)` | 工具自检权限 |
| `tool.PermissionPolicy` | `CheckToolPermission(ctx, *PermissionRequest) (PermissionDecision, error)` | 全局权限策略 |
| `extension.Extension` | `Name() string` / `Register(r *extension.Registry)` | 扩展点：注册 BeforeModel/BeforeTool/AfterTool 回调 |

### 1.2 关键类型

| 类型 | 说明 |
|------|------|
| `tool.Declaration` | 工具声明：Name/Description/InputSchema/OutputSchema |
| `tool.Schema` | JSON Schema 定义：Type/Required/Properties/Items/Enum/Ref/Defs |
| `tool.ToolMetadata` | 工具元数据：ReadOnly/Destructive/ConcurrencySafe/SearchOrRead/OpenWorld/MaxResultSize |
| `tool.PermissionRequest` | 权限请求：Tool/ToolName/ToolCallID/Declaration/Arguments/Metadata |
| `tool.PermissionDecision` | 权限决策：Action(allow/deny/ask) + Reason |
| `tool.RetryPolicy` | 重试策略：MaxAttempts/InitialInterval/BackoffFactor/MaxInterval/Jitter/RetryOn |
| `tool.RetryInfo` | 重试信息：ToolName/Attempt/MaxAttempts/Result/Error |
| `tool.Callbacks` | 回调链：BeforeTool/AfterTool/ToolResultMessages |
| `tool.FilterFunc` | 工具过滤函数：`func(ctx, Tool) bool` |
| `tool.Stream` / `StreamReader` / `StreamWriter` | 流式系统：Reader/Writer 双向通道 |
| `tool.StreamChunk` | 流式块：Content + Metadata |
| `tool.InnerTextMode` | 内部文本模式：""(default)/"include"/"exclude" |
| `tool.FinalResultChunk` / `FinalResultStateChunk` | 最终结果块：Result + StateDelta |

### 1.3 扩展点

| 扩展点 | 机制 | 适用场景 |
|--------|------|---------|
| 实现 `CallableTool` | 接口实现 | 自定义同步工具 |
| 实现 `StreamableTool` | 接口实现 | 自定义流式工具 |
| 实现 `ToolSet` | 接口实现 | 自定义工具集（如 MCP ToolSet） |
| 实现 `MetadataProvider` | 接口实现 | 声明工具元数据（影响权限/过滤决策） |
| 实现 `DeferredTool` | 接口实现 | 声明延迟加载（宿主侧按需暴露声明） |
| 实现 `PermissionChecker` | 接口实现 | 工具自检权限（allow/deny/ask） |
| 实现 `PermissionPolicy` | 接口实现 | 全局权限策略 |
| 实现 `extension.Extension` | Extension 机制 | 注册 BeforeModel/BeforeTool/AfterTool 回调 |
| `FilterFunc` + `FilterTools` / `FilterToolSet` | 函数式过滤 | 按名称包含/排除工具 |
| `Callbacks` 钩子 | BeforeTool/AfterTool/ToolResultMessages | 修改参数、替换结果、跳过执行、跳过总结 |
| `RetryPolicy` | 指数退避 + 自定义重试条件 | 工具调用失败重试 |

### 1.4 配置选项

#### FunctionTool Option

| Option | 说明 | 默认值 |
|--------|------|--------|
| `function.WithName(name)` | 工具名（须符合 `^[a-zA-Z0-9_-]+$`） | 函数名 |
| `function.WithDescription(desc)` | 工具描述 | "" |
| `function.WithLongRunning(bool)` | 标记长时运行 | false |
| `function.WithSkipSummarization(bool)` | 跳过外层总结 | false |
| `function.WithInputSchema(*Schema)` | 自定义输入 Schema | 自动推导 |
| `function.WithOutputSchema(*Schema)` | 自定义输出 Schema | nil |

#### MCPToolSet Option

| Option | 说明 | 默认值 |
|--------|------|--------|
| `mcp.WithToolFilterFunc(FilterFunc)` | 工具过滤 | nil |
| `mcp.WithMCPOptions(...mcp.ClientOption)` | MCP 客户端选项 | nil |
| `mcp.WithSessionReconnect(maxAttempts)` | 自动会话重连（1-10 次） | 0（不重连） |
| `mcp.WithSessionReconnectConfig(SessionReconnectConfig)` | 完整重连配置 | nil |
| `mcp.WithName(name)` | ToolSet 名称 | 服务器名 |

#### AgentTool Option

| Option | 说明 | 默认值 |
|--------|------|--------|
| `agent.WithSkipSummarization(bool)` | 跳过外层总结 | false |
| `agent.WithStreamInner(bool)` | 转发内部流式事件 | false |
| `agent.WithInnerTextMode(InnerTextMode)` | 控制内部文本转发 | "" |
| `agent.WithHistoryScope(HistoryScope)` | 历史继承：Isolated/ParentBranch | Isolated |
| `agent.WithResponseMode(ResponseMode)` | Default/FinalOnly | Default |
| `agent.WithDescription(string)` | 覆盖描述 | Agent 描述 |
| `agent.WithName(string)` | 仅 DynamicTool 有效 | "dynamic_agent" |

#### DynamicTool 专属 Option

| Option | 说明 | 默认值 |
|--------|------|--------|
| `agent.WithTemplateAgent(agent.Agent)` | 模板 Agent | nil |
| `agent.WithCapabilityProvider(CapabilitySurfaceProvider)` | 能力面解析 | nil |
| `agent.WithCapabilityTools([]tool.Tool)` | 固定最大工具面 | nil |
| `agent.WithCapabilitySkills(skill.Repository)` | 固定最大 Skill 面 | nil |
| `agent.WithExposeToolSelection(bool)` | 暴露 tools 字段 | true |
| `agent.WithExposeSkillSelection(bool)` | 暴露 skills 字段 | false |
| `agent.WithExposeInstruction(bool)` | 暴露 instruction 字段 | true |

#### ToolPipe Extension Option

| Option | 说明 | 默认值 |
|--------|------|--------|
| `toolpipe.WithToolNames(names...)` | 工具名白名单 | 空（必须指定） |
| `toolpipe.WithToolScope(fn)` | 动态工具选择器函数 | nil |
| `toolpipe.WithAllowedOps(ops...)` | 允许的过滤操作集合 | {grep, head, tail} |
| `toolpipe.WithMaxOutputBytes(n)` | 输出窗口大小/截断阈值 | 64KB |
| `toolpipe.WithMaxInputBytes(n)` | 过滤前最大输入大小 | 2MB |
| `toolpipe.WithFilterField(name)` | 注入到 InputSchema 的字段名 | "result_filter" |
| `toolpipe.WithPrompt(text)` | 自定义引导提示 | 内置默认提示 |

#### MCPBroker Option

| Option | 说明 | 默认值 |
|--------|------|--------|
| `mcpbroker.WithServers(map[string]ConnectionConfig)` | 命名服务器配置 | nil |
| `mcpbroker.WithAllowAdHocHTTP(bool)` | 允许 ad-hoc HTTP URL | false |
| `mcpbroker.WithAdHocHTTPTimeout(Duration)` | ad-hoc HTTP 操作超时 | 30s |
| `mcpbroker.WithAdHocSensitiveHeaderDenylist([]string)` | 敏感头拒绝列表 | 内置列表 |
| `mcpbroker.WithHTTPHeaderInjector(HTTPHeaderInjector)` | 每次运行 HTTP 头注入 | nil |
| `mcpbroker.WithClientOptionsProvider(ClientOptionsProvider)` | 自定义 MCP 客户端选项 | nil |
| `mcpbroker.WithErrorInterceptor(ErrorInterceptor)` | HTTP 错误拦截/转换 | nil |

### 1.5 框架内置实现

| 实现 | 路径 | 说明 |
|------|------|------|
| FunctionTool | `tool/function/` | 泛型函数包装：`NewFunctionTool[I,O](fn, opts...)` |
| StreamableFunctionTool | `tool/function/` | 流式函数包装：`NewStreamableFunctionTool[I,O](fn, opts...)` |
| MCPToolSet | `tool/mcp/` | MCP 远程工具集：stdio/SSE/Streamable HTTP 三种传输 |
| MCPBroker | `tool/mcpbroker/` | 运行时 MCP 发现/调用代理，含 SSRF 防护 |
| AgentTool | `tool/agent/` | 将 Agent 包装为 Tool（含 DynamicTool） |
| TransferTool | `tool/transfer/` | 对话控制权移交 |
| SkillToolSet | `tool/skill/` | Skill 工具集：load/list_docs/select_docs/run/exec |
| OpenAPIToolSet | `tool/openapi/` | REST API 包装 |
| HostExecToolSet | `tool/hostexec/` | 宿主机命令执行 |
| WorkspaceExecToolSet | `tool/workspaceexec/` | 沙箱工作区命令执行 |
| FileToolSet | `tool/file/` | 文件读写搜索 |
| ClaudeCodeToolSet | `tool/claudecode/` | Claude Code 兼容工具集 |
| CodeExecTool | `tool/codeexec/` | 代码执行 |
| AwaitReplyTool | `tool/awaitreply/` | 等待用户回复 |
| TaskRunTool | `tool/taskrun/` | 后台任务运行 |
| TodoTool | `tool/todo/` | 任务管理 |
| DuckDuckGoTool | `tool/duckduckgo/` | DuckDuckGo 搜索 |
| GoogleSearchTool | `tool/google/search/` | Google 搜索 |
| ArXivSearchTool | `tool/arxivsearch/` | ArXiv 论文搜索 |
| WikipediaTool | `tool/wikipedia/` | Wikipedia 搜索 |
| EmailTool | `tool/email/` | 邮件发送 |
| WebFetchTool | `tool/webfetch/` | URL 内容抓取（3 后端） |
| ToolPipe Extension | `agent/extension/toolpipe/` | 工具结果过滤/窗口化（grep/head/tail/jq） |

---

## 二、项目实现现状

### 2.1 框架接口实现情况

| 框架接口/功能 | 项目实现 | 合规性 | 说明 |
|--------------|---------|--------|------|
| `function.NewFunctionTool()` | ✅ 完全使用 | ✅ | 自建工具均通过此 API 创建 |
| `tool.ToolSet` 接口 | ✅ 完全使用 | ✅ | File/ShellExec/WebResearch/MCP 等均实现 ToolSet |
| `mcp.NewMCPToolSet()` | ✅ 完全使用 | ✅ | 支持 stdio/SSE/StreamableHTTP |
| `mcpbroker.New()` | ✅ 完全使用 | ✅ | MCP Broker 运行时发现/调用 |
| `agent.NewTool()` / `NewDynamicTool()` | ✅ 完全使用 | ✅ | CallAgent/SubAgent |
| `skill.NewExecTool()` / `NewLoadTool()` 等 | ✅ 完全使用 | ✅ | Skill 工具集 |
| `tool.PermissionPolicy` | ✅ 完全使用 | ✅ | 基于 DeferredManager 的确认门控 |
| `tool.Callbacks` | ✅ 完全使用 | ✅ | 通过 Callback Chain 适配 |
| `tool.RetryPolicy` | ✅ 完全使用 | ✅ | 工具重试策略 |
| `tool.FilterFunc` | ✅ 完全使用 | ✅ | 工具过滤 |
| `StreamableTool` | ⚠️ 部分使用 | ⚠️ | 仅部分工具实现流式接口 |
| `DeferredTool` | ❌ 未使用 | ❌ | 项目自建了 DeferredManager |
| `ToolPipe Extension` | ❌ 未使用 | ❌ | 项目自建了 ToolResultGate |

### 2.2 自建功能清单

| 自建功能 | 实现位置 | 替代框架功能 | 自建原因 |
|---------|---------|-------------|---------|
| 全局工具注册表 | `internal/tools/toolset.go`（686 行） | 无 | 框架无注册表/元数据管理，需统一管理工具的分类、风险等级、默认启用状态、确认要求 |
| 10 阶段组装流程 | `internal/tools/toolset_assemble.go`（443 行） | 无 | 框架无分阶段组装/配置依赖解析，需按依赖顺序组装不同工具 |
| 延迟加载 + 工具发现 | `internal/tools/deferred/`（342 行） | `DeferredTool` 接口 | 框架 DeferredTool 仅声明意图，无实例化/发现/激活机制；项目实现了完整的 DeferredCallableTool + DeferredToolManager + ToolSearchTool |
| Agent 级配置解析/合并 | `internal/agent/tool_assembly.go`（473 行） | 无 | 框架无业务配置概念，需将 biz.Agent Settings 转换为工具配置 |
| 确认门控（Before Hook） | `internal/agent/tool_confirmation.go`（155 行） | 无 | 框架无"暂停执行等待用户确认"机制 |
| 多层确认策略 + Fail-closed | `internal/agent/tool_confirm_gate.go`（210 行） | 无 | 框架无声明式确认策略，需同时支持 DB 级 + 插件级策略 |
| 确认策略纯函数 | `internal/biz/tool/requires_confirmation.go`（44 行） | 无 | 框架无确认策略，需 Fail-closed 安全模型 |
| 熔断器核心 | `internal/biz/tool/circuit_breaker.go`（248 行） | 无 | 框架无熔断器，连续失败的工具会持续消耗资源 |
| 熔断器注册表 + 预设 | `internal/biz/tool/circuit_breaker_config.go`（237 行） | 无 | 框架无分类级预设/运行时配置覆盖/状态持久化 |
| 熔断器 Hook | `internal/agent/tool_circuit_breaker.go`（166 行） | 无 | 框架无故障保护机制 |
| 调用记录 + 审计 | `internal/agent/tool_invocation_recorder.go`（311 行） | 无 | 框架无调用持久化/审计 |
| 大结果门控 | `internal/agent/tool_result_gate_hook.go`（116 行） | `ToolPipe Extension` | 框架 ToolPipe 处理输出过滤，但无大结果持久化/摘要替换/降级截断 |
| 结果缓存 | `internal/agent/tool_result_cache.go`（88 行） | 无 | 框架无结果缓存 |
| 确认声明注解 | `internal/tools/trpc/confirmation.go`（117 行） | 无 | 框架无声明级确认标注，需在 Description 中注入提示文本 |
| 配置映射/剪枝/合并 | `internal/tools/trpc/`（191 行） | 无 | 框架无运行时配置覆盖 |
| trpc 桥接层 | `internal/tools/trpc/toolsets.go`（250 行） | 无 | 框架无业务适配层 |
| 工具名消歧 | `internal/tools/disambiguation.go` | 无 | 框架无工具名冲突解决机制 |
| 运行时别名映射 | `internal/tools/runtime_alias.go` | 无 | 框架无工具名别名机制 |
| MCP 生产安全控制 | `internal/tools/mcp_production.go` | 无 | 框架无生产环境 MCP 安全控制 |
| 工具预览脱敏 | `internal/biz/tool/tool_preview.go` | 无 | 框架无敏感信息脱敏 |
| 工具调用写入脱敏 | `internal/biz/tool/tool_invocation_sanitize.go` | 无 | 框架无调用参数脱敏 |
| 工具配置校验 | `internal/biz/tool/tool_config_validate.go` | 无 | 框架无 JSON Schema 校验/MCP SSRF 防护 |
| 工具测试 | `internal/biz/tool/tool_test_invoke.go` | 无 | 框架无工具测试入口 |
| Agent 级工具覆盖 | `internal/biz/tool_agent_override_runtime.go` | 无 | 框架无 per-agent 工具策略覆盖 |
| 有效工具矩阵 | `internal/biz/agent_effective_tools.go` | 无 | 框架无 profile/allow/deny 策略计算 |
| EventBus 工具调用消费 | `internal/biz/event_bus_tool_call_consumer.go` | 无 | 框架无事件驱动的工具调用 |
| 工具结果 Blob/Replacement | `internal/biz/tool_result_ports.go` + `internal/data/tool_result_repo.go` | 无 | 框架无大结果持久化 |

### 2.3 未使用的框架功能

| 框架功能 | 未使用原因 | 是否需要启用 |
|---------|-----------|-------------|
| `DeferredTool` 接口 | 项目自建了更完整的 DeferredManager（含实例化/发现/激活），框架接口仅声明意图 | 评估中——可考虑让 DeferredCallableTool 同时实现 DeferredTool 接口 |
| `ToolPipe Extension` | 项目自建了 ToolResultGate（大结果持久化），但未实现输出过滤/窗口化 | **强烈建议启用**——可减少 50-90% Token 消耗 |
| `StreamableTool` 接口 | 仅部分工具实现流式接口，多数工具仍为同步 | 评估中——需逐工具评估流式化收益 |

---

## 三、对比分析

### 3.1 框架优势（项目应采纳的）

| # | 框架优势 | 项目现状 | 对齐收益 |
|---|---------|---------|---------|
| 1 | ToolPipe Extension：工具结果过滤/窗口化（grep/head/tail/jq），自动截断大输出 | 项目仅通过 ToolResultGate 做大结果持久化，无输出过滤能力 | Token 消耗降低 50-90%（框架 benchmark 数据），减少上下文溢出风险 |
| 2 | DeferredTool 接口：标准化的延迟加载声明 | 项目自建 DeferredManager 但未实现框架 DeferredTool 接口 | 与框架生态兼容，框架 Runner 可识别延迟工具 |
| 3 | ToolPipe 的框架工具自动跳过：5 类框架工具（AgentTool/LongRunning/StateDelta 等）不增强 | 项目无此机制，ToolResultGate 对所有工具统一处理 | 避免对框架语义工具（transfer/await_reply/todo 等）的误过滤 |
| 4 | ToolPipe 的安全机制：Shell 语法解析拒绝危险操作、Pipeline 长度限制、执行超时 | 项目无输出过滤安全机制 | 防止 LLM 构造恶意过滤表达式 |

### 3.2 项目优势（框架缺失的）

| # | 项目优势 | 框架现状 | 建议处理 |
|---|---------|---------|---------|
| 1 | 工具级熔断器：三态状态机 + 分类预设 + 瞬态错误识别 + 状态持久化 | 框架无熔断器，RetryPolicy 仅处理重试 | 贡献为 tool 扩展包 |
| 2 | 工具确认门控：多层策略（DB + 插件）+ Fail-closed + 超时 + 审计 | 框架 PermissionPolicy 仅支持 allow/deny/ask 决策，无"暂停等待确认"机制 | 贡献为 tool 扩展包 |
| 3 | 全局工具注册表：统一元数据管理（分类/风险/确认/别名） | 框架无注册表，工具创建后无统一管理 | 评估贡献——可能过于业务特定 |
| 4 | 延迟加载 + 工具发现：DeferredCallableTool + DeferredToolManager + ToolSearchTool | 框架 DeferredTool 仅声明意图，无实例化/发现/激活机制 | 贡献为 tool 扩展包 |
| 5 | 调用记录 + 审计：输入/输出/耗时/状态持久化 + 安全审计 | 框架无调用持久化/审计 | 评估贡献——可能过于业务特定 |
| 6 | 大结果持久化 + 摘要替换：超阈值自动持久化到 Blob，LLM 上下文用预览替代 | 框架 ToolPipe 仅做过滤/截断，无持久化 + 替换 | 评估贡献——与 ToolPipe 互补 |
| 7 | 结果缓存：per-tool 缓存策略 + TTL | 框架无结果缓存 | 贡献为 tool 扩展包 |
| 8 | 工具名消歧 + 别名映射 | 框架无工具名冲突解决/别名机制 | 评估贡献 |
| 9 | MCP 生产安全控制 | 框架无生产环境 MCP 安全控制 | 评估贡献 |
| 10 | 工具预览脱敏 + 调用写入脱敏 | 框架无敏感信息脱敏 | 评估贡献 |

### 3.3 差异根因分析

| 差异点 | 根因 | 影响范围 |
|--------|------|---------|
| ToolPipe 未使用 | 认知缺失——项目在 ToolPipe 发布前已自建 ToolResultGate，未评估 ToolPipe 的过滤能力 | 所有工具的输出均未过滤，Token 浪费严重 |
| DeferredTool 未实现 | 功能缺失——框架 DeferredTool 仅声明意图，项目需要完整的实例化/发现/激活机制 | DeferredManager 与框架 DeferredTool 接口不兼容 |
| 熔断器自建 | 功能缺失——框架无熔断器，项目需要故障保护 | `circuit_breaker.go` + `circuit_breaker_config.go` + `tool_circuit_breaker.go` 共 651 行自建代码 |
| 确认门控自建 | 功能缺失——框架 PermissionPolicy 仅支持 allow/deny/ask，无"暂停等待确认"机制 | `tool_confirmation.go` + `tool_confirm_gate.go` + `requires_confirmation.go` + `confirmation.go` 共 526 行自建代码 |
| 全局注册表自建 | 架构决策——框架定位为原子工具库，不提供业务级注册表 | `toolset.go` 686 行自建代码，但与框架定位不冲突 |
| 调用记录自建 | 功能缺失——框架不持久化调用记录 | `tool_invocation_recorder.go` 311 行自建代码 |
| 大结果门控自建 | 功能缺失——框架 ToolPipe 仅做过滤/截断，无持久化 + 替换 | `tool_result_gate_hook.go` 116 行自建代码，与 ToolPipe 互补而非替代 |
| 结果缓存自建 | 功能缺失——框架无缓存机制 | `tool_result_cache.go` 88 行自建代码 |

---

## 四、对齐方案

### 4.1 对齐项清单

| # | 对齐项 | 类型 | 优先级 | 影响范围 | 预期收益 |
|---|--------|------|--------|---------|---------|
| 1 | 启用 ToolPipe Extension | 启用框架功能 | P1 | 所有工具的输出处理 | Token 消耗降低 50-90% |
| 2 | DeferredCallableTool 实现 DeferredTool 接口 | 新增适配层 | P2 | 延迟加载工具 | 与框架生态兼容 |
| 3 | 熔断器贡献为 tool 扩展包 | 贡献回框架 | P2 | 熔断器模块 | 代码减少约 400 行（框架内化后） |
| 4 | 确认门控贡献为 tool 扩展包 | 贡献回框架 | P2 | 确认门控模块 | 代码减少约 300 行（框架内化后） |
| 5 | 评估 StreamableTool 逐工具启用 | 启用框架功能 | P3 | 部分工具 | 流式输出改善用户体验 |
| 6 | ToolResultGate 与 ToolPipe 协同 | 新增适配层 | P2 | 大结果处理 | 互补：ToolPipe 过滤 + Gate 持久化 |

### 4.2 对齐项详情

#### 对齐项 #1：启用 ToolPipe Extension

**类型**：启用框架功能

**现状**：
- 项目当前实现：通过 ToolResultGate（BeforeModel Hook）对超过 50000 字符的工具结果做持久化 + 预览替换，但无输出过滤能力
- 框架提供能力：ToolPipe Extension 通过 BeforeModel/BeforeTool/AfterTool 三回调实现工具结果过滤/窗口化（grep/head/tail/jq），自动截断大输出

**对齐方案**：
1. 在 `internal/agent/tool_assembly.go` 的 `buildToolsetsForAgent()` 中创建 ToolPipe Extension
2. 配置白名单工具（duckduckgo_search、web_fetch、read_file 等大输出工具）
3. 启用全部 4 种操作（grep/head/tail/jq）
4. 保留 ToolResultGate 作为补充——ToolPipe 处理过滤，Gate 处理持久化
5. ToolPipe 的 `WithMaxOutputBytes` 设置为 64KB（默认值），与 ToolResultGate 的 50000 字符阈值协调

**代码变更范围**：
- 新增：`internal/agent/tool_toolpipe.go`（ToolPipe 构建逻辑，约 50 行）
- 修改：`internal/agent/tool_assembly.go`（集成 ToolPipe 到 Agent 构建）
- 修改：`internal/agent/agent.go` 或 Runner 构建逻辑（注册 Extension）

**兼容性风险**：
- ToolPipe 会修改工具的 InputSchema（添加 `result_filter` 字段），可能影响前端工具参数展示
- ToolPipe 的输出格式从纯文本变为 `ToolResult` 结构体，可能影响下游消费者
- 需要确保 ToolPipe 的 BeforeTool 回调与项目现有的 BeforeTool Hook 链不冲突

**回退方案**：
- ToolPipe 通过 Agent Settings 开关控制，可随时关闭
- 关闭后行为与当前完全一致

**验证方法**：
1. 单元测试：验证 ToolPipe 与 ToolResultGate 的协同行为
2. 集成测试：对比启用/禁用 ToolPipe 的 Token 消耗
3. 回归测试：确保现有工具调用流程不受影响

**预期收益**：
- 代码减少：约 0 行（新增约 50 行，但减少 Token 消耗）
- 性能影响：Token 消耗降低 50-90%（框架 benchmark 数据），LLM 调用成本显著降低
- 维护成本：利用框架维护的过滤引擎，减少自建截断逻辑

---

#### 对齐项 #2：DeferredCallableTool 实现 DeferredTool 接口

**类型**：新增适配层

**现状**：
- 项目当前实现：`DeferredCallableTool`（`internal/tools/deferred/deferred_tool.go`）实现 `trpctool.CallableTool` 接口，通过 factory 延迟实例化
- 框架提供能力：`DeferredTool` 接口（`ShouldDefer(ctx) bool`）声明延迟加载意图，`ShouldDefer(ctx, t)` 辅助函数检查工具是否应延迟

**对齐方案**：
1. 让 `DeferredCallableTool` 同时实现 `DeferredTool` 接口
2. `ShouldDefer()` 返回 `true`（DeferredCallableTool 始终应延迟加载）
3. 在 `DeferredToolManager` 的 `ToolFilter()` 中使用框架的 `tool.ShouldDefer()` 辅助函数作为补充判断

**代码变更范围**：
- 修改：`internal/tools/deferred/deferred_tool.go`（添加 `ShouldDefer()` 方法，约 5 行）

**兼容性风险**：
- 极低——仅添加接口实现，不改变现有行为

**回退方案**：
- 删除 `ShouldDefer()` 方法即可回退

**验证方法**：
- 单元测试：验证 `tool.ShouldDefer(ctx, deferredTool)` 返回 `true`

**预期收益**：
- 代码减少：约 0 行
- 维护成本：与框架 DeferredTool 生态兼容，框架 Runner 可识别延迟工具
- 功能增强：未来框架如果基于 DeferredTool 实现自动延迟逻辑，项目可自动受益

---

#### 对齐项 #3：熔断器贡献为 tool 扩展包

**类型**：贡献回框架

**现状**：
- 项目当前实现：`CircuitBreaker`（248 行）+ `CircuitBreakerRegistry`（237 行）+ `circuitBreakerBeforeHook`/`AfterHook`（166 行），共 651 行自建代码
- 框架提供能力：`RetryPolicy` 仅处理重试，无熔断器

**对齐方案**：
1. 将 `CircuitBreaker` 核心逻辑提取为 `pkg/trpc-agent-go/tool/circuitbreaker/` 扩展包
2. 实现 `extension.Extension` 接口，通过 BeforeTool/AfterTool 回调集成
3. 提供 `WithCategoryPresets()` / `WithToolOverride()` / `WithStateRepo()` 等 Option
4. 项目侧改为使用框架扩展包，自建代码仅保留业务特化的注册表配置

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/tool/circuitbreaker/`（框架扩展包，约 400 行）
- 修改：`internal/agent/tool_circuit_breaker.go`（改为使用框架扩展包）
- 修改：`internal/biz/tool/circuit_breaker.go`（核心逻辑移至框架）
- 修改：`internal/biz/tool/circuit_breaker_config.go`（注册表保留，核心逻辑移至框架）

**兼容性风险**：
- 中等——需要确保框架扩展包的 API 覆盖项目所有使用场景
- 瞬态错误识别逻辑需要一并迁移

**回退方案**：
- 保留自建代码，框架扩展包为可选替代

**验证方法**：
- 单元测试：迁移后熔断器行为与现有实现一致
- 集成测试：验证 BeforeTool/AfterTool 回调链顺序不变

**预期收益**：
- 代码减少：约 400 行（核心逻辑移至框架）
- 维护成本：框架维护熔断器核心逻辑，项目仅维护业务特化配置
- 功能增强：框架生态其他项目可复用熔断器

---

#### 对齐项 #4：确认门控贡献为 tool 扩展包

**类型**：贡献回框架

**现状**：
- 项目当前实现：`toolConfirmationBeforeHook`（155 行）+ `toolConfirmGate`（210 行）+ `requires_confirmation.go`（44 行）+ `confirmation.go`（117 行），共 526 行自建代码
- 框架提供能力：`PermissionPolicy` 仅支持 allow/deny/ask 决策，无"暂停等待确认"机制

**对齐方案**：
1. 将确认门控核心逻辑提取为 `pkg/trpc-agent-go/tool/confirmation/` 扩展包
2. 实现 `extension.Extension` 接口，通过 BeforeTool 回调集成
3. 提供 `WithConfirmPolicy()` / `WithTimeout()` / `WithReplyFunc()` 等 Option
4. 项目侧改为使用框架扩展包，自建代码仅保留业务特化的策略配置

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/tool/confirmation/`（框架扩展包，约 350 行）
- 修改：`internal/agent/tool_confirmation.go`（改为使用框架扩展包）
- 修改：`internal/agent/tool_confirm_gate.go`（策略逻辑保留，Hook 逻辑移至框架）

**兼容性风险**：
- 中等——需要确保框架扩展包支持项目的多层策略（DB + 插件）和 Fail-closed 安全模型
- 确认超时和审计记录需要一并迁移

**回退方案**：
- 保留自建代码，框架扩展包为可选替代

**验证方法**：
- 单元测试：迁移后确认门控行为与现有实现一致
- 安全测试：验证 Fail-closed 行为不变

**预期收益**：
- 代码减少：约 300 行（Hook 逻辑移至框架）
- 维护成本：框架维护确认门控核心逻辑，项目仅维护策略配置
- 功能增强：框架生态其他项目可复用确认门控

---

#### 对齐项 #5：评估 StreamableTool 逐工具启用

**类型**：启用框架功能

**现状**：
- 项目当前实现：多数工具仅实现 `CallableTool`，少数工具（如 MCP 工具）支持流式
- 框架提供能力：`StreamableTool` 接口 + `StreamableFunctionTool` 泛型包装

**对齐方案**：
1. 逐工具评估流式化收益——优先评估长时运行工具（web_fetch、duckduckgo_search、hostexec）
2. 对适合流式的工具，将 `NewFunctionTool` 改为 `NewStreamableFunctionTool`
3. 在 Agent 构建时启用 `WithEnableParallelTools`

**代码变更范围**：
- 修改：各工具的创建逻辑（逐工具迁移）
- 修改：`internal/agent/tool_assembly.go`（启用并行工具执行）

**兼容性风险**：
- 低——流式工具向下兼容同步调用

**回退方案**：
- 恢复为 `NewFunctionTool` 即可

**验证方法**：
- 集成测试：验证流式工具的输出与同步版本一致
- 性能测试：对比流式 vs 同步的用户体验

**预期收益**：
- 代码减少：约 0 行
- 性能影响：长时运行工具的输出可实时流式返回，改善用户体验
- 功能增强：启用并行工具执行，减少多工具调用的总延迟

---

#### 对齐项 #6：ToolResultGate 与 ToolPipe 协同

**类型**：新增适配层

**现状**：
- 项目当前实现：ToolResultGate（BeforeModel Hook）对超过 50000 字符的工具结果做持久化 + 预览替换
- 框架提供能力：ToolPipe（AfterTool Hook）对工具结果做过滤/窗口化，默认 64KB 输出上限

**对齐方案**：
1. 调整执行顺序：ToolPipe（AfterTool）先过滤 → ToolResultGate（BeforeModel）再持久化
2. ToolPipe 过滤后的结果通常在 64KB 以内，大幅减少触发 ToolResultGate 的频率
3. 仍需保留 ToolResultGate 作为兜底——某些工具结果即使过滤后仍可能超过阈值
4. 协调阈值：ToolPipe `maxOutput=64KB` < ToolResultGate `threshold=50000字符`，确保 ToolPipe 优先处理

**代码变更范围**：
- 修改：`internal/agent/tool_result_gate_hook.go`（适配 ToolPipe 后的结果格式）
- 修改：`internal/agent/tool_assembly.go`（确保 Extension 注册顺序正确）

**兼容性风险**：
- 低——ToolPipe 和 ToolResultGate 在不同阶段执行，不直接冲突

**回退方案**：
- 关闭 ToolPipe 后行为与当前一致

**验证方法**：
- 集成测试：验证 ToolPipe 过滤后 ToolResultGate 的触发频率降低
- 回归测试：确保超大结果仍能正确持久化

**预期收益**：
- 代码减少：约 0 行
- 性能影响：ToolResultGate 触发频率大幅降低，减少 Blob 写入
- 维护成本：两层防护互补，降低单点依赖风险

---

## 五、实施路线

### 5.1 阶段规划

| 阶段 | 对齐项 | 前置依赖 | 预计工作量 |
|------|--------|---------|-----------|
| Phase 1 | #1 启用 ToolPipe Extension | 无 | 中 |
| Phase 2 | #6 ToolResultGate 与 ToolPipe 协同 | Phase 1 | 小 |
| Phase 2 | #2 DeferredCallableTool 实现 DeferredTool | 无 | 小 |
| Phase 3 | #3 熔断器贡献为 tool 扩展包 | 框架侧审批 | 大 |
| Phase 3 | #4 确认门控贡献为 tool 扩展包 | 框架侧审批 | 大 |
| Phase 4 | #5 StreamableTool 逐工具启用 | 无 | 中 |

### 5.2 风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| ToolPipe 修改 InputSchema 影响前端展示 | 中 | 中 | 前端适配 `result_filter` 字段，或通过 `WithFilterField` 自定义字段名 |
| ToolPipe 输出格式变化影响下游消费者 | 中 | 中 | ToolPipe 的 `ToolResult` 结构体包含 `content` 字段，可兼容纯文本消费 |
| 熔断器/确认门控贡献回框架被拒 | 低 | 低 | 保留自建代码，框架扩展包为可选替代 |
| ToolPipe 与现有 Hook 链冲突 | 低 | 高 | ToolPipe 通过 Extension 机制注册，与自建 Hook 链独立；需验证回调执行顺序 |
| DeferredTool 接口未来变更 | 低 | 低 | 仅添加接口实现，不依赖接口行为 |

---

## 六、附录

### A. 框架示例代码参考（必填）

| 示例 | 路径 | 关键 API | 初始化模式 | 与项目实现差异 |
|------|------|---------|-----------|--------------|
| MCP Tool | `examples/mcptool/` | `function.NewFunctionTool`、`mcp.NewMCPToolSet`、`mcp.WithToolFilterFunc`、`mcp.WithSessionReconnect`、`mcp.WithMCPOptions` | 直接创建 Tool/ToolSet → `llmagent.WithTools`/`WithToolSets` 注册 | 项目通过 `tools.Assemble()` 统一组装，不直接创建；项目额外有 MCP 认证头注入、生产安全控制 |
| Email Tool | `examples/email/` | `email.NewToolSet()` | `email.NewToolSet()` → `llmagent.WithToolSets` | 项目未使用框架 email 工具 |
| Wiki Tool | `examples/wiki/` | `wikipedia.NewToolSet(WithLanguage, WithMaxResults, WithUserAgent)` | `wikipedia.NewToolSet(opts...)` → `llmagent.WithToolSets` | 项目未使用框架 wikipedia 工具 |
| Runner Tool | `examples/runner/tools.go` | `function.NewFunctionTool(fn, WithName, WithDescription)`、`llmagent.WithEnableParallelTools` | 直接创建 FunctionTool → `WithTools` 注册 | 项目使用 `jsonschema` tag 定义参数描述（与示例一致）；项目额外有确认门控/熔断器/缓存 Hook |
| ToolPipe Interactive | `examples/toolpipe/interactive/` | `toolpipe.New(WithToolNames, WithAllowedOps, WithMaxOutputBytes)`、`llmagent.WithExtensions` | 创建 ToolPipe → `WithExtensions` 注册到 Agent | **项目未使用 ToolPipe**，这是最重要的待对齐项 |
| ToolPipe Benchmark | `examples/toolpipe/benchmark/` | 同上 + A/B 对比测试 | 同上 | 项目无 Token 消耗基准测试 |

### B. 框架文档参考

| 文档 | 路径 |
|------|------|
| ToolPipe 设计文档（中文） | `pkg/trpc-agent-go/agent/extension/toolpipe/toolpipe_zh.md` |
| ToolPipe 设计文档（英文） | `pkg/trpc-agent-go/agent/extension/toolpipe/toolpipe.md` |
