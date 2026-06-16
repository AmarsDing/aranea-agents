# 扩展 Agent 与工具模块（TaskRun / Todo / ToolPipe / Codex / Dify / N8N）— 框架对齐分析

> 模块路径：`pkg/trpc-agent-go/agent/taskrun/`、`pkg/trpc-agent-go/tool/todo/`、`pkg/trpc-agent-go/agent/extension/toolpipe/`、`pkg/trpc-agent-go/agent/codex/`、`pkg/trpc-agent-go/agent/dify/`、`pkg/trpc-agent-go/agent/n8n/`
> 项目实现路径：`internal/biz/task.go`、`internal/tools/toolset.go`、`internal/agent/tool_todo_args_guard.go` 等
> 当前对齐度：★★☆☆☆（6 模块中仅 Todo 已集成，其余 5 个未使用）

---

## 一、框架能力全景

### 1.1 核心接口

| 模块 | 接口 | 方法 | 说明 |
|------|------|------|------|
| TaskRun | `taskrun.Controller` | `Spawn(ctx, SpawnRequest) (Run, error)` | 创建后台任务运行 |
| TaskRun | `taskrun.Controller` | `List(ctx, ListFilter) ([]Run, error)` | 列出任务运行 |
| TaskRun | `taskrun.Controller` | `Get(ctx, runID string) (*Run, error)` | 获取单个任务运行 |
| TaskRun | `taskrun.Controller` | `Cancel(ctx, runID string) (*Run, bool, error)` | 取消任务运行 |
| TaskRun | `taskrun.Controller` | `Wait(ctx, runID string) (*Run, error)` | 等待任务完成 |
| TaskRun | `taskrun.Observer` | `OnRunUpdate(ctx, Run)` | 生命周期观察 |
| TaskRun | `inprocess.Store` | `Load(ctx) ([]Run, error)` / `Save(ctx, []Run) error` | 持久化接口 |
| TaskRun | `inprocess.Finalizer` | `FinalizeRun(ctx, Run) map[string]string` | 终止元数据附加 |
| Todo | `tool.CallableTool` | `Call(ctx, jsonArgs) (any, error)` | LLM 可调用的待办工具 |
| ToolPipe | `extension.Extension` | `Name() string` / `Register(r *Registry)` | 扩展注册接口 |
| ToolPipe | `Op` | `Apply(ctx, input string) (string, error)` | 过滤操作接口 |
| Codex | `agent.Agent` | `Run(ctx, *Invocation) (<-chan *event.Event, error)` | Agent 标准接口 |
| Codex | `commandRunner` | `Run(ctx, command) ([]byte, []byte, error)` | CLI 命令执行抽象 |
| Dify | `agent.Agent` | 同上 | Agent 标准接口 |
| Dify | `DifyEventConverter` | `ConvertToEvent(...)` / `ConvertStreamingToEvent(...)` | Dify 响应→框架事件转换 |
| Dify | `DifyRequestConverter` | `ConvertToDifyRequest(ctx, *Invocation, bool) (*ChatMessageRequest, error)` | Chatflow 请求构建 |
| Dify | `DifyWorkflowRequestConverter` | `ConvertToWorkflowRequest(ctx, *Invocation) (WorkflowRequest, error)` | Workflow 请求构建 |
| N8N | `agent.Agent` | 同上 | Agent 标准接口 |
| N8N | `RequestConverter` | `ConvertToN8nRequest(ctx, *Invocation) (map[string]any, error)` | 请求转换 |
| N8N | `ResponseConverter` | `ConvertToEvent(...)` / `ConvertStreamingToEvent(...)` | 响应转换 |

### 1.2 关键类型

| 模块 | 类型 | 说明 |
|------|------|------|
| TaskRun | `Status` | 7 种状态：queued/running/finalizing/canceling/completed/failed/canceled，有 `IsTerminal()` |
| TaskRun | `Run` | 持久化控制面视图：ID/OwnerUserID/ParentSessionID/ChildSessionID/AgentName/Task/Status/Summary/Result/Error/Progress/Metadata/时间戳 |
| TaskRun | `SpawnRequest` | 新建任务请求：ID/OwnerUserID/ParentSessionID/AppName/AgentName/Task/Timeout/RuntimeState/RunOptions/RunContext/InjectedContextMessages/Metadata |
| TaskRun | `Progress` | 轻量进度：EventCount/ToolCallCount/ToolResultCount/Token 统计/LastEventAt |
| TaskRun | `ListFilter` | 列表过滤：OwnerUserID/ParentSessionID/ParentAppName/Status |
| Todo | `Status` | 3 种状态：pending/in_progress/completed |
| Todo | `Item` | 待办项：Content（祈使句）/ActiveForm（进行时）/Status |
| Todo | `Output` | 工具调用结果：Message（nudge 提示）/Todos（新列表）/OldTodos（旧列表） |
| Todo | `NudgeHook` | `func(ctx, oldTodos, submitted []Item) string` — 策略回调 |
| ToolPipe | `OpType` | 操作类型：grep/head/tail/jq |
| ToolPipe | `Pipeline` | 操作序列：`Ops []Op` + 原始表达式 |
| ToolPipe | `Engine` | 解析和执行过滤表达式，含白名单/大小/超时限制 |
| ToolPipe | `ToolResult` | 过滤结果：Filter/Content/Truncated/TotalBytes/InputTruncated/Error/EmptyReason/OriginalPreview |
| Codex | `command` | CLI 调用描述：bin/args/stdin/env/dir |
| Codex | `RawOutputHookArgs` | 原始输出钩子参数：InvocationID/SessionID/ThreadID/Prompt/Stdout/Stderr/Error |
| Dify | `DifyMode` | 服务模式：chatflow（默认）/workflow |
| Dify | `StreamingRespHandler` | `func(resp *model.Response) (string, error)` — 流式响应处理 |
| N8N | `AuthType` | 认证类型：none/basic/header |
| N8N | `AuthConfig` | 认证配置：Username/Password 或 HeaderName/HeaderValue |

### 1.3 扩展点

| 模块 | 扩展点 | 机制 | 适用场景 |
|------|--------|------|---------|
| TaskRun | `taskrun.Controller` | 接口实现 | 分布式实现替换（外部存储+队列+租约） |
| TaskRun | `inprocess.Store` | 接口实现 | 自定义持久化后端（DB/Redis） |
| TaskRun | `inprocess.Finalizer` | 接口实现 | run 终止前附加自定义 metadata |
| TaskRun | `taskrun.Observer` | 接口实现 | 监听生命周期事件 |
| Todo | `NudgeHook` | 函数回调 | 附加验证提醒、循环检测、token 预算警告 |
| Todo | `WithStateKeyPrefix` | Option 配置 | 自定义 session.State 存储键布局 |
| ToolPipe | `Op` 接口 | 接口实现 | 扩展新过滤操作类型 |
| ToolPipe | `WithToolScope` | Option 配置 | 动态工具匹配（如 MCP ToolSet） |
| ToolPipe | `WithPrompt` | Option 配置 | 完全自定义提示或禁用 |
| Codex | `commandRunner` | 接口实现 | 替换命令执行方式（测试 mock） |
| Codex | `RawOutputHook` | 函数回调 | 观察/处理原始 CLI 输出 |
| Dify | `DifyEventConverter` | 接口实现 | 自定义 Dify 响应→框架事件转换 |
| Dify | `DifyRequestConverter` | 接口实现 | 自定义 Chatflow 请求构建 |
| Dify | `DifyWorkflowRequestConverter` | 接口实现 | 自定义 Workflow 请求构建 |
| Dify | `WithGetDifyClientFunc` | Option 配置 | 动态客户端创建（多租户/动态密钥） |
| N8N | `RequestConverter` | 接口实现 | 自定义 n8n webhook 请求体构建 |
| N8N | `ResponseConverter` | 接口实现 | 自定义 n8n 响应→Event 转换 |
| N8N | `WithGetHTTPClientFunc` | Option 配置 | 动态 HTTP 客户端创建（TLS/代理/超时） |

### 1.4 配置选项

#### TaskRun — `tool/taskrun` 工具层

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithDefaultAgentName(name)` | spawn 未指定 agent 时的默认名称 | — |
| `WithRuntimeState(state)` | 合并静态运行时状态到每个 spawned run | — |
| `WithInjectedContextMessages(messages)` | 追加非持久化上下文消息 | — |
| `WithSessionService(service)` | 启用子任务 transcript 读取 | — |
| `WithParentAppNamePropagation(enabled)` | 复制当前 invocation 的 app name 到 spawned runs | false |
| `WithNestedSpawns(enabled)` | 允许嵌套 task run | false |

#### TaskRun — `inprocess` 实现层

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithStore(store)` | 持久化存储 | MemoryStore |
| `WithObserver(observer)` | 生命周期观察 | — |
| `WithFinalizer(finalizer)` | 终止元数据附加 | — |
| `WithClock(clock)` | 时钟注入 | time.Now |

#### Todo

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithToolName(name)` | 覆盖注册工具名 | `todo_write` |
| `WithDescription(desc)` | 覆盖工具描述 | 框架默认 |
| `WithNudgeMessage(msg)` | 覆盖/禁用默认 nudge 提示 | 框架默认 |
| `WithStateKeyPrefix(prefix)` | 覆盖 session.State 键前缀 | `temp:todos` |
| `WithClearOnAllDone(clear)` | 全部完成后是否清空列表 | true |
| `WithNudgeHook(hook)` | 注册额外策略钩子 | — |

#### ToolPipe

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithToolNames(names...)` | 按名称指定可过滤的工具 | — |
| `WithToolScope(fn)` | 函数式工具选择器 | — |
| `WithFilterField(field)` | 注入到工具 schema 的 JSON 字段名 | `result_filter` |
| `WithAllowedOps(ops...)` | 允许的过滤操作 | grep/head/tail |
| `WithMaxInputBytes(n)` | 过滤引擎最大输入大小 | 2MB |
| `WithMaxOutputBytes(n)` | 过滤后最大输出大小 | 64KB |
| `WithPrompt(prompt)` | 自定义/禁用系统提示注入 | 框架默认 |

#### Codex

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithName(name)` | agent 名称 | — |
| `WithBin(bin)` | Codex CLI 可执行路径 | `codex` |
| `WithGlobalArgs(args...)` | exec 子命令前的根参数 | — |
| `WithExtraArgs(args...)` | exec/resume 后的额外参数 | — |
| `WithEnv(env...)` | 额外环境变量 | — |
| `WithWorkDir(dir)` | 工作目录 | — |
| `WithRawOutputHook(hook)` | 原始 CLI 输出回调 | — |

#### Dify

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithBaseUrl(url)` | Dify 服务基础 URL | — |
| `WithMode(mode)` | chatflow 或 workflow 模式 | chatflow |
| `WithName(name)` | agent 名称（必填） | — |
| `WithDescription(desc)` | agent 描述 | — |
| `WithCustomEventConverter(c)` | 自定义事件转换 | 默认转换器 |
| `WithCustomRequestConverter(c)` | 自定义 Chatflow 请求转换 | 默认转换器 |
| `WithCustomWorkflowConverter(c)` | 自定义 Workflow 请求转换 | 默认转换器 |
| `WithStreamingChannelBufSize(n)` | 流式通道缓冲大小 | 1024 |
| `WithStreamingRespHandler(h)` | 流式响应处理 | — |
| `WithTransferStateKey(keys...)` | 从 session state 传递到 Dify inputs 的键 | — |
| `WithEnableStreaming(bool)` | 显式控制流式模式 | — |
| `WithAutoGenConversationName(bool)` | 自动生成对话名（仅 chatflow） | — |
| `WithGetDifyClientFunc(fn)` | 每次调用动态创建 Dify 客户端 | — |

#### N8N

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithWebhookURL(url)` | n8n webhook URL（必填） | — |
| `WithName(name)` | agent 名称（必填） | — |
| `WithDescription(desc)` | agent 描述 | — |
| `WithAuthType(authType)` | 认证类型 | none |
| `WithAuthConfig(config)` | 认证配置 | — |
| `WithCustomRequestConverter(c)` | 自定义请求转换 | 默认转换器 |
| `WithCustomResponseConverter(c)` | 自定义响应转换 | 默认转换器 |
| `WithEnableStreaming(bool)` | 显式控制流式模式 | — |
| `WithStreamingChannelBufSize(n)` | 流式通道缓冲大小 | — |
| `WithStreamingRespHandler(h)` | 流式响应处理 | — |
| `WithHTTPClient(client)` | 自定义 HTTP 客户端 | 默认 1h 超时 |
| `WithGetHTTPClientFunc(fn)` | 动态创建 HTTP 客户端 | — |
| `WithTransferStateKey(keys...)` | 从 session state 传递到 n8n inputs 的键 | — |

### 1.5 框架内置实现

| 模块 | 实现 | 路径 | 说明 |
|------|------|------|------|
| TaskRun | `inprocess.Service` | `agent/taskrun/inprocess/` | 单进程 Controller 实现，goroutine 启动任务 |
| TaskRun | `inprocess.MemoryStore` | `agent/taskrun/inprocess/` | 内存存储 |
| TaskRun | `inprocess.FileStore` | `agent/taskrun/inprocess/` | JSON 文件存储（原子写入，tmp+rename） |
| TaskRun | `tool/taskrun.Tools` | `tool/taskrun/` | 6 个工具：start/list/get/cancel/wait/read_transcript |
| Todo | `todo.Tool` | `tool/todo/` | 唯一实现，session.State 持久化 |
| ToolPipe | `ToolPipe` | `agent/extension/toolpipe/` | Extension 实现，BeforeModel/BeforeTool/AfterTool 三阶段 |
| ToolPipe | `OpGrep/OpHead/OpTail/OpJQ` | `agent/extension/toolpipe/` | 4 种内置过滤操作 |
| Codex | `codexAgent` | `agent/codex/` | CLI 封装 Agent，JSONL 事件映射 |
| Dify | `DifyAgent` | `agent/dify/` | Dify 平台集成 Agent，chatflow + workflow |
| N8N | `N8nAgent` | `agent/n8n/` | n8n webhook Agent，SSE 流式 |

---

## 二、项目实现现状

### 2.1 框架接口实现情况

| 框架接口/功能 | 项目实现 | 合规性 | 说明 |
|--------------|---------|--------|------|
| `taskrun.Controller` | ❌ 未使用 | ❌ | 项目自建了完全不同的 Graph Task 体系 |
| `tool/taskrun.Tools` | ❌ 未使用 | ❌ | 项目无 Agent 委派式后台运行需求 |
| `todo.Tool` | ✅ 完全使用 | ✅ | `internal/tools/toolset.go` 直接注册 `trpctodo.New()` |
| `todoenforcer.Extension` | ❌ 未使用 | ❌ | 项目未启用 TodoEnforcer 扩展 |
| `toolpipe.ToolPipe` | ❌ 未使用 | ❌ | 项目无工具结果过滤 |
| `codex.Agent` | ❌ 未使用 | ❌ | 项目无 Codex CLI 集成 |
| `dify.Agent` | ❌ 未使用 | ❌ | 项目无 Dify 平台集成 |
| `n8n.Agent` | ❌ 未使用 | ❌ | 项目无 n8n 工作流集成 |

### 2.2 自建功能清单

| 自建功能 | 实现位置 | 替代框架功能 | 自建原因 |
|---------|---------|-------------|---------|
| Graph Task 体系 | `internal/biz/task.go`、`internal/data/task.go`、`internal/service/graph_task_*.go` | 无直接替代 | 框架 TaskRun 是 Agent 委派式后台运行，项目需要图节点任务板（认领/提交/审查/心跳/超时/阻塞），概念完全不同 |
| Cron Task Run | `internal/biz/cron.go`、`internal/cronrunner/execute.go` | 无直接替代 | 定时任务运行，框架无对应功能 |
| Todo 参数守卫 | `internal/agent/tool_todo_args_guard.go` | 无 | 防止 LLM 在非 `todo_write` 工具调用中误注入 `todos` 参数（解决"stuck tool"问题） |
| Todo 结果展平 | `internal/agent/event_projector.go`（`flattenTodoWriteResult`） | 无 | 将 `todo_write` 工具结果展平为人类可读摘要 |
| Todo 前端看板 | `web/src/components/chat/TodoKanbanBoard.vue`、`TodoBoardTabs.vue`、`useTodoBoard.ts`、`todoPresentation.ts` | 无 | 看板式待办 UI |

### 2.3 未使用的框架功能

| 框架功能 | 未使用原因 | 是否需要启用 |
|---------|-----------|-------------|
| `taskrun.Controller` + `tool/taskrun` | 项目自建的 Graph Task 体系与框架的 Agent 委派式 TaskRun 概念不同，无法直接替代 | 评估中（见对齐项 #1） |
| `todoenforcer.Extension` | 项目未启用，但功能有价值——自动在系统提示中注入当前待办列表状态 | 是（见对齐项 #2） |
| `toolpipe.ToolPipe` | 项目未使用工具结果过滤，长工具输出浪费 Token | 是（见对齐项 #3） |
| `codex.Agent` | 项目无 Codex CLI 集成需求 | 否（当前无业务需求） |
| `dify.Agent` | 项目无 Dify 平台集成需求 | 否（当前无业务需求） |
| `n8n.Agent` | 项目无 n8n 工作流集成需求 | 否（当前无业务需求） |

---

## 三、对比分析

### 3.1 框架优势（项目应采纳的）

| # | 框架优势 | 项目现状 | 对齐收益 |
|---|---------|---------|---------|
| 1 | ToolPipe 可降低 50-90% Token 消耗（框架 benchmark 数据） | 长工具输出直接全量返回给 LLM | Token 成本降低，LLM 上下文利用率提升 |
| 2 | TodoEnforcer 自动注入待办状态到系统提示 | Agent 可能忽略当前待办列表 | Agent 行为更一致，减少遗漏 |
| 3 | TaskRun 提供标准化的 Agent 委派后台运行 | 项目无此能力（Graph Task 是不同概念） | 支持 Agent 异步委派任务，增强 Agent 自主性 |
| 4 | Dify/N8N Agent 提供标准化的外部平台集成 | 项目无外部工作流平台集成能力 | 支持与 Dify/n8n 生态对接，扩展 Agent 能力边界 |
| 5 | Codex Agent 提供标准化的 Codex CLI 封装 | 项目无 Codex CLI 集成 | 支持 Codex 代码 Agent 集成 |

### 3.2 项目优势（框架缺失的）

| # | 项目优势 | 框架现状 | 建议处理 |
|---|---------|---------|---------|
| 1 | Graph Task 体系（10 种状态、6 个窄接口、完整生命周期管理） | 框架 TaskRun 仅 7 种状态，无认领/审查/心跳/阻塞等概念 | 保持自建，框架 TaskRun 无法替代 |
| 2 | Cron Task Run（定时任务运行） | 框架无对应功能 | 保持自建 |
| 3 | Todo 参数守卫（防误注入） | 框架无此防护 | 保持自建，考虑贡献回框架 |
| 4 | Todo 前端看板 UI | 框架无前端组件 | 保持自建 |

### 3.3 差异根因分析

| 差异点 | 根因 | 影响范围 |
|--------|------|---------|
| TaskRun 完全自建 | 架构决策——项目需要的是图节点任务板（人机协作），框架提供的是 Agent 委派式后台运行（Agent 自主），概念不同 | `internal/biz/task.go`、`internal/service/graph_task_*.go`、前端任务面板 |
| TodoEnforcer 未启用 | 认知缺失——项目已使用 `tool/todo` 但未发现 `todoenforcer` 扩展 | Agent 系统提示构建 |
| ToolPipe 未使用 | 认知缺失——项目未评估工具结果过滤的收益 | 所有使用长输出工具的 Agent |
| Codex/Dify/N8N 未使用 | 业务需求缺失——项目当前无这些外部平台集成需求 | 无影响 |
| Todo 参数守卫自建 | 框架功能缺失——`tool/todo` 无防护 LLM 误注入 `todos` 参数的机制 | `internal/agent/tool_todo_args_guard.go` |

---

## 四、对齐方案

### 4.1 对齐项清单

| # | 对齐项 | 类型 | 优先级 | 影响范围 | 预期收益 |
|---|--------|------|--------|---------|---------|
| 1 | 启用 ToolPipe Extension | 启用框架功能 | P1 | 所有使用长输出工具的 Agent | Token 消耗降低 50-90% |
| 2 | 启用 TodoEnforcer 扩展 | 启用框架功能 | P2 | 使用 `todo_write` 工具的 Agent | Agent 行为一致性提升 |
| 3 | 评估框架 TaskRun 与 Graph Task 共存方案 | 新增适配层 | P3 | Agent 运行时、Graph 执行引擎 | Agent 获得异步委派能力 |
| 4 | Dify Agent 集成预留 | 启用框架功能 | P4 | Agent 类型注册 | 支持与 Dify 平台对接 |
| 5 | N8N Agent 集成预留 | 启用框架功能 | P4 | Agent 类型注册 | 支持与 n8n 工作流对接 |
| 6 | Codex Agent 集成预留 | 启用框架功能 | P4 | Agent 类型注册 | 支持 Codex CLI Agent |

### 4.2 对齐项详情

#### 对齐项 #1：启用 ToolPipe Extension

**类型**：启用框架功能

**现状**：
- 项目当前：所有工具结果全量返回给 LLM，长输出（如文件读取、命令执行结果）占用大量上下文窗口
- 框架提供能力：`toolpipe.New()` Extension，为指定工具注入 `result_filter` 参数，支持 grep/head/tail/jq 管道过滤，自动截断大输出

**对齐方案**：
1. 在 Agent 构建流程中注册 `toolpipe.New()` Extension
2. 通过 `WithToolNames()` 或 `WithToolScope()` 指定可过滤的工具（如 `read_file`、`execute_command`、`web_fetch` 等）
3. 通过 `WithAllowedOps()` 配置允许的过滤操作（默认 grep/head/tail 即可，jq 按需开启）
4. 评估是否需要调整 `WithMaxInputBytes`/`WithMaxOutputBytes` 默认值

**代码变更范围**：
- 新增：`internal/agent/` 中 ToolPipe Extension 注册逻辑
- 修改：Agent 构建流程（`BuildTRPCLLMAgent` 或 `buildExtensions`）
- 删除：无

**兼容性风险**：
- 工具 schema 变更（注入 `result_filter` 字段），可能影响前端工具参数展示
- 过滤后结果可能丢失 LLM 需要的关键信息，需仔细配置工具白名单

**回退方案**：
- 移除 ToolPipe Extension 注册即可回退，无数据变更

**验证方法**：
- 对比启用前后同一工具调用的 Token 消耗
- 验证 `result_filter` 参数在工具 schema 中正确注入
- 验证过滤后结果不影响 Agent 决策质量

**预期收益**：
- 代码减少：约 0 行（纯配置启用）
- 性能影响：Token 消耗降低 50-90%（框架 benchmark 数据）
- 维护成本：无额外维护负担
- 功能增强：LLM 上下文利用率提升，可处理更大的工具输出

---

#### 对齐项 #2：启用 TodoEnforcer 扩展

**类型**：启用框架功能

**现状**：
- 项目当前：已使用 `todo.Tool`（`todo_write`），但 Agent 可能忽略当前待办列表状态
- 框架提供能力：`todoenforcer` Extension，自动在系统提示中注入当前待办列表状态，强制 Agent 遵循待办计划

**对齐方案**：
1. 在 Agent 构建流程中注册 `todoenforcer` Extension
2. 验证与现有 `todo_write` 工具的集成无冲突
3. 验证与 `tool_todo_args_guard.go` 的兼容性

**代码变更范围**：
- 新增：`internal/agent/` 中 TodoEnforcer Extension 注册逻辑
- 修改：Agent 构建流程
- 删除：无

**兼容性风险**：
- TodoEnforcer 注入的系统提示可能与自定义 instruction 冲突
- 与 `tool_todo_args_guard.go` 可能存在功能重叠，需评估是否可移除守卫

**回退方案**：
- 移除 Extension 注册即可回退

**验证方法**：
- 验证启用后 Agent 在有待办项时优先处理当前 `in_progress` 项
- 验证系统提示中正确注入待办列表状态

**预期收益**：
- 代码减少：约 0 行（纯配置启用）
- 性能影响：系统提示略增（待办列表状态注入）
- 维护成本：无额外维护负担
- 功能增强：Agent 行为一致性提升，减少遗漏待办项

---

#### 对齐项 #3：评估框架 TaskRun 与 Graph Task 共存方案

**类型**：新增适配层

**现状**：
- 项目当前：自建 Graph Task 体系（`internal/biz/task.go`），10 种状态、6 个窄接口、完整生命周期管理（认领/提交/审查/心跳/超时/阻塞），面向图节点人机协作任务
- 框架提供能力：`taskrun.Controller`（7 种状态），Agent 委派式后台运行（spawn 子 Agent 执行任务），面向 Agent 自主异步任务

**对齐方案**：
1. 两者概念不同，**不应替换**，应共存互补
2. 框架 TaskRun 用于 Agent 自主委派后台运行（如"后台执行这个长时间任务"）
3. 项目 Graph Task 继续用于图节点人机协作任务板
4. 在 Agent 构建流程中注册 `tool/taskrun.Tools`，使 LLM 可调用 `start_task_run` 等工具
5. 创建 `inprocess.Service` 实例并注入 Runner

**代码变更范围**：
- 新增：`internal/agent/` 中 TaskRun Service 初始化和工具注册
- 修改：Agent 构建流程（添加 `taskrun` 工具集）、Wire 注入
- 删除：无

**兼容性风险**：
- 新增工具可能与现有工具命名冲突
- TaskRun 的 `inprocess.Service` 需要持久化存储，默认 MemoryStore 重启丢失
- 需要实现 `inprocess.Store` 接口对接项目数据库

**回退方案**：
- 移除 TaskRun 工具注册即可回退

**验证方法**：
- 验证 Agent 可通过 `start_task_run` 工具启动后台任务
- 验证后台任务状态查询/取消/等待功能正常
- 验证与 Graph Task 体系无冲突

**预期收益**：
- 代码减少：约 0 行（新增功能，非替换）
- 性能影响：Agent 获得异步委派能力，主对话不阻塞
- 维护成本：框架维护 TaskRun 逻辑，项目仅需配置
- 功能增强：Agent 可自主启动后台任务，增强自主性

---

#### 对齐项 #4：Dify Agent 集成预留

**类型**：启用框架功能

**现状**：
- 项目当前：无 Dify 平台集成
- 框架提供能力：`dify.Agent`，支持 Chatflow/Workflow 两种模式，流式/非流式，自定义请求/响应转换

**对齐方案**：
1. 在 Agent 类型注册中添加 Dify 类型支持
2. 在 Agent 配置中添加 Dify 相关字段（BaseURL/Mode/APIKey 等）
3. 在 `bizAgentFactoryForKey` 中添加 Dify Agent 构建分支
4. 使用 `WithGetDifyClientFunc` 支持多租户动态密钥

**代码变更范围**：
- 新增：Agent 构建分支（Dify 类型）
- 修改：Agent 配置 Schema、Agent Factory
- 删除：无

**兼容性风险**：
- Dify API 兼容性需持续跟进
- 多租户密钥管理需安全设计

**回退方案**：
- 移除 Dify Agent 类型注册即可回退

**验证方法**：
- 验证 Dify Chatflow/Workflow 两种模式均可正常工作
- 验证流式响应正确映射为框架事件
- 验证多租户密钥动态创建

**预期收益**：
- 代码减少：约 0 行（新增功能）
- 功能增强：支持与 Dify 平台对接，扩展 Agent 能力边界

---

#### 对齐项 #5：N8N Agent 集成预留

**类型**：启用框架功能

**现状**：
- 项目当前：无 n8n 工作流集成
- 框架提供能力：`n8n.Agent`，通过 HTTP Webhook 与 n8n 工作流通信，支持 SSE 流式，多种认证方式

**对齐方案**：
1. 在 Agent 类型注册中添加 N8N 类型支持
2. 在 Agent 配置中添加 N8N 相关字段（WebhookURL/AuthType/AuthConfig 等）
3. 在 `bizAgentFactoryForKey` 中添加 N8N Agent 构建分支

**代码变更范围**：
- 新增：Agent 构建分支（N8N 类型）
- 修改：Agent 配置 Schema、Agent Factory
- 删除：无

**兼容性风险**：
- n8n Webhook 响应格式可能因工作流设计而异，需自定义 ResponseConverter
- SSE 流式解析依赖 n8n 版本兼容性

**回退方案**：
- 移除 N8N Agent 类型注册即可回退

**验证方法**：
- 验证 N8N Webhook 调用正常
- 验证 SSE 流式响应正确映射
- 验证认证方式（None/Basic/Header）均正常

**预期收益**：
- 代码减少：约 0 行（新增功能）
- 功能增强：支持与 n8n 工作流对接，扩展 Agent 自动化能力

---

#### 对齐项 #6：Codex Agent 集成预留

**类型**：启用框架功能

**现状**：
- 项目当前：无 Codex CLI 集成
- 框架提供能力：`codex.Agent`，封装本地 Codex CLI，JSONL 事件映射，支持会话恢复

**对齐方案**：
1. 在 Agent 类型注册中添加 Codex 类型支持
2. 在 Agent 配置中添加 Codex 相关字段（Bin/GlobalArgs/ExtraArgs/WorkDir 等）
3. 在 `bizAgentFactoryForKey` 中添加 Codex Agent 构建分支

**代码变更范围**：
- 新增：Agent 构建分支（Codex 类型）
- 修改：Agent 配置 Schema、Agent Factory
- 删除：无

**兼容性风险**：
- 依赖本地 Codex CLI 安装，部署环境需预装
- CLI 版本兼容性需持续跟进

**回退方案**：
- 移除 Codex Agent 类型注册即可回退

**验证方法**：
- 验证 Codex CLI 调用正常
- 验证 JSONL 事件正确映射为框架事件
- 验证会话恢复功能

**预期收益**：
- 代码减少：约 0 行（新增功能）
- 功能增强：支持 Codex 代码 Agent 集成

---

## 五、实施路线

### 5.1 阶段规划

| 阶段 | 对齐项 | 前置依赖 | 预计工作量 |
|------|--------|---------|-----------|
| Phase 1 | #1 ToolPipe Extension | 无 | 小（配置级变更） |
| Phase 2 | #2 TodoEnforcer | #1（Extension 注册机制已验证） | 小（配置级变更） |
| Phase 3 | #3 TaskRun 共存 | 无 | 中（需实现 Store 适配 + 工具注册） |
| Phase 4 | #4 Dify / #5 N8N / #6 Codex | 无 | 中（每个 Agent 类型需独立构建分支 + 配置 Schema） |

### 5.2 风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| ToolPipe 过滤丢失 LLM 需要的关键信息 | 中 | 高 | 先对低风险工具（如 `read_file`）启用，逐步扩展；配置 `WithMaxOutputBytes` 保留足够上下文 |
| TodoEnforcer 注入提示与自定义 instruction 冲突 | 低 | 低 | 测试验证提示注入位置和内容，必要时自定义提示模板 |
| TaskRun Store 适配工作量大 | 中 | 中 | 先使用 FileStore 验证功能，后续再实现 DB Store |
| Dify/N8N API 版本不兼容 | 中 | 中 | 使用 `WithCustomEventConverter`/`WithCustomResponseConverter` 适配 |
| Codex CLI 部署环境依赖 | 高 | 低 | 文档说明部署要求，可选安装 |

---

## 六、附录

### A. 框架示例代码参考（必填）

| 示例 | 路径 | 关键 API | 初始化模式 | 与项目实现差异 |
|------|------|---------|-----------|--------------|
| TaskRun | `examples/taskrun/` | `inprocess.NewService`、`inprocess.NewFileStore`、`svc.Spawn`、`svc.Wait` | 创建 Service + FileStore → Spawn 启动任务 → Wait 等待 | 项目无此用法，自建 Graph Task 体系概念不同 |
| Todo | `examples/todo/` | `todo.New`、`WithClearOnAllDone`、`WithNudgeHook`、`todo.GetTodos` | `todo.New(opts...)` → `llmagent.WithTools([]tool.Tool{todoTool})` | 项目已使用 `todo.New()`，但未启用 NudgeHook 和 TodoEnforcer |
| ToolPipe Interactive | `examples/toolpipe/interactive/` | `toolpipe.New`、`WithToolNames`、`WithAllowedOps` | `toolpipe.New(opts...)` → `runner.WithExtensions(toolpipe)` | 项目未使用 |
| ToolPipe Benchmark | `examples/toolpipe/benchmark/` | A/B 基准测试对比 | 创建两个 Runner（有/无 ToolPipe）对比 Token 消耗 | 项目未使用，benchmark 数据可参考评估收益 |
| Codex | `examples/codex/` | `codex.New`、`WithBin`、`WithGlobalArgs`、`WithExtraArgs`、`WithWorkDir`、`WithRawOutputHook` | `codex.New(opts...)` → `runner.WithAgent(codexAgent)` | 项目未使用 |
| Dify Basic Chat | `examples/dify/basic_chat/` | `dify.New`、`WithGetDifyClientFunc`、`WithBaseUrl`、`WithMode` | 环境变量获取配置 → `dify.New(opts...)` → `runner.WithAgent(difyAgent)` | 项目未使用 |
| Dify Streaming Chat | `examples/dify/streaming_chat/` | 流式 Chatflow 模式 | 同上 + `WithEnableStreaming(true)` | 项目未使用 |
| Dify Advanced Usage | `examples/dify/advanced_usage/` | 自定义 Converter、Workflow 模式 | `WithCustomEventConverter`/`WithCustomRequestConverter`/`WithCustomWorkflowConverter` | 项目未使用 |
| N8N Basic Chat | `examples/n8n/basic_chat/` | `n8n.New`、`WithWebhookURL`、`WithAuthType`、`WithAuthConfig` | 环境变量获取配置 → `n8n.New(opts...)` → `runner.WithAgent(n8nAgent)` | 项目未使用 |
| N8N Streaming Chat | `examples/n8n/streaming_chat/` | SSE 流式模式 | 同上 + `WithEnableStreaming(true)` | 项目未使用 |

### B. 框架文档参考

| 文档 | 路径 |
|------|------|
| TaskRun 类型定义 | `pkg/trpc-agent-go/agent/taskrun/types.go` |
| TaskRun Inprocess 实现 | `pkg/trpc-agent-go/agent/taskrun/inprocess/service.go` |
| TaskRun 工具层 | `pkg/trpc-agent-go/tool/taskrun/tool.go` |
| Todo 工具 | `pkg/trpc-agent-go/tool/todo/todo.go` |
| Todo 选项 | `pkg/trpc-agent-go/tool/todo/options.go` |
| TodoEnforcer 扩展 | `pkg/trpc-agent-go/agent/extension/todoenforcer/` |
| ToolPipe 扩展 | `pkg/trpc-agent-go/agent/extension/toolpipe/toolpipe.go` |
| ToolPipe 选项 | `pkg/trpc-agent-go/agent/extension/toolpipe/option.go` |
| ToolPipe 操作 | `pkg/trpc-agent-go/agent/extension/toolpipe/ops.go` |
| Codex Agent | `pkg/trpc-agent-go/agent/codex/codex_agent.go` |
| Codex 选项 | `pkg/trpc-agent-go/agent/codex/options.go` |
| Dify Agent | `pkg/trpc-agent-go/agent/dify/dify_agent.go` |
| Dify 选项 | `pkg/trpc-agent-go/agent/dify/dify_agent_option.go` |
| Dify 转换器 | `pkg/trpc-agent-go/agent/dify/dify_converter.go` |
| N8N Agent | `pkg/trpc-agent-go/agent/n8n/n8n_agent.go` |
| N8N 选项 | `pkg/trpc-agent-go/agent/n8n/n8n_agent_option.go` |
| N8N 转换器 | `pkg/trpc-agent-go/agent/n8n/n8n_converter.go` |
