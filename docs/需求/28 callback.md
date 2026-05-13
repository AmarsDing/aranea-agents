# M19: Callback 回调 — 详细需求

> 对标 `pkg/trpc-agent-go/agent/callbacks.go` + `model/callbacks.go` + `tool/callbacks.go`，实现全链路回调钩子。

---

## 1. 现状分析

项目无 Callback 回调机制。Agent/Model/Tool 的执行过程无法被拦截、修改或增强。

---

## 2. trpc 框架参照

### Agent Callbacks

```
pkg/trpc-agent-go/agent/callbacks.go
```

```go
type Callbacks struct {
    BeforeAgent []BeforeAgentCallbackStructured
    AfterAgent  []AfterAgentCallbackStructured
}

type BeforeAgentCallbackStructured = func(ctx context.Context, args *BeforeAgentArgs) (*BeforeAgentResult, error)

type BeforeAgentArgs struct {
    Invocation *agent.Invocation
}

type BeforeAgentResult struct {
    Context        context.Context
    CustomResponse *model.Response  // 非nil则跳过Agent执行
}

type AfterAgentCallbackStructured = func(ctx context.Context, args *AfterAgentArgs) (*AfterAgentResult, error)

type AfterAgentArgs struct {
    Invocation       *agent.Invocation
    FullResponseEvent *event.Event
    RunErr           error
}

type AfterAgentResult struct {
    Context        context.Context
    CustomResponse *model.Response  // 非nil则替换Agent响应
}
```

### Model Callbacks

```
pkg/trpc-agent-go/model/callbacks.go
```

```go
type Callbacks struct {
    BeforeModel []BeforeModelCallback
    AfterModel  []AfterModelCallback
}

type BeforeModelCallback = func(ctx context.Context, request *Request) (*Request, error)
type AfterModelCallback = func(ctx context.Context, response *Response) (*Response, error)
```

### Tool Callbacks

```
pkg/trpc-agent-go/tool/callbacks.go
```

```go
type Callbacks struct {
    BeforeTool []BeforeToolCallback
    AfterTool  []AfterToolCallback
}

type BeforeToolCallback = func(ctx context.Context, toolName string, args []byte) ([]byte, error)
type AfterToolCallback = func(ctx context.Context, toolName string, result any) (any, error)
```

### PluginManager

```go
type PluginManager interface {
    AgentCallbacks() *agent.Callbacks
    ModelCallbacks() *model.Callbacks
    ToolCallbacks() *tool.Callbacks
    OnEvent(ctx context.Context, invocation *Invocation, e *event.Event) (*event.Event, error)
    Close(ctx context.Context) error
}
```

---

## 3. 需求清单

### 3.1 Agent Callbacks

**需求**：Agent 执行前后可注入回调

**实现要点**：
- 在 `BuildTRPCLLMAgent` 中通过 `WithAgentCallbacks` 注入
- `BeforeAgent`：可修改 Invocation 或跳过执行
- `AfterAgent`：可修改响应或替换响应

**典型用途**：
- 审计日志：记录 Agent 调用
- 权限检查：BeforeAgent 中检查权限
- 响应过滤：AfterAgent 中过滤敏感信息

**验收标准**：Agent 执行前后回调正确触发

### 3.2 Model Callbacks

**需求**：LLM 调用前后可注入回调

**实现要点**：
- 在 `BuildTRPCLLMAgent` 中通过 `WithModelCallbacks` 注入
- `BeforeModel`：可修改请求（如注入 system prompt）
- `AfterModel`：可修改响应（如内容过滤）

**典型用途**：
- Token 计费：BeforeModel 中记录请求 token
- 内容安全：AfterModel 中过滤不安全内容
- 请求增强：BeforeModel 中注入上下文

**验收标准**：LLM 调用前后回调正确触发

### 3.3 Tool Callbacks

**需求**：Tool 调用前后可注入回调

**实现要点**：
- 在 `BuildTRPCLLMAgent` 中通过 `WithToolCallbacks` 注入
- `BeforeTool`：可修改参数或拒绝调用
- `AfterTool`：可修改结果或记录日志

**典型用途**：
- 工具权限：BeforeTool 中检查调用权限
- 执行日志：AfterTool 中记录工具调用
- 参数校验：BeforeTool 中校验参数

**验收标准**：Tool 调用前后回调正确触发

### 3.4 PluginManager 集成

**需求**：Runner 级别的统一回调管理

**实现要点**：
- 新建 `internal/plugin/trpc/manager.go`
- 实现 `agent.PluginManager` 接口
- 在 `NewTRPCRunner` 中通过 `WithPlugins` 注入
- PluginManager 聚合 Agent/Model/Tool 三层回调

**验收标准**：PluginManager 统一管理三层回调

### 3.5 OnEvent 事件回调

**需求**：每个事件可被回调处理

**实现要点**：
- PluginManager 的 `OnEvent` 方法
- 可修改事件内容
- 可过滤事件

**典型用途**：
- 事件审计：记录所有事件
- 事件转换：修改事件格式
- 事件过滤：过滤敏感事件

**验收标准**：事件流经 OnEvent 回调正确处理

### 3.6 回调链顺序

**需求**：多个回调按顺序执行

**实现要点**：
- 回调按注册顺序执行
- Before 回调链：前一个的输出是后一个的输入
- After 回调链：前一个的输出是后一个的输入
- 任一回调返回错误则中断

**验收标准**：回调按注册顺序执行，错误正确传播

### 3.7 产品层回调（超越层）

**需求**：产品层可配置回调规则

**实现要点**：
- 数据库存储回调规则
- 规则引擎：条件匹配 → 执行动作
- 动作类型：日志/通知/拦截/修改
- 支持热更新

**验收标准**：产品层可配置回调规则，无需修改代码

---

## 4. 涉及文件

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/plugin/trpc/manager.go` | 新建 | PluginManager 实现 |
| `internal/plugin/trpc/agent_callbacks.go` | 新建 | Agent 回调 |
| `internal/plugin/trpc/model_callbacks.go` | 新建 | Model 回调 |
| `internal/plugin/trpc/tool_callbacks.go` | 新建 | Tool 回调 |
| `internal/agent/trpc_build.go` | 修改 | 注入回调 |
| `internal/agent/trpc_runtime.go` | 修改 | 注入 PluginManager |
| `internal/biz/hook.go` | 修改 | 产品层回调规则 |

---

## 5. 验收标准总览

1. Agent 执行前后回调正确触发
2. LLM 调用前后回调正确触发
3. Tool 调用前后回调正确触发
4. PluginManager 统一管理三层回调
5. 事件流经 OnEvent 回调正确处理
6. 回调按注册顺序执行，错误正确传播
7. 产品层可配置回调规则（超越层）
