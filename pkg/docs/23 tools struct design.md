> **Capability Context 归属**  
> 本文档是 `aranea/docs/0 main design.md` 中 **Capability Context** 的 Tool 执行子系统专题设计。当前代码仍位于 `backend/internal/tools/**`，作为迁移期参考实现；目标落位是 `backend/internal/capability/**`，并通过 `kernel/contracts` 暴露 `ToolResolver` / `ToolExecutor` 等端口。本文中的 registry、executor、middleware、backends、adkbridge、schema、telemetry、storage 均应按主架构「四、模块功能设计」对齐：ADK 类型只出现在桥接层，事件必须脱敏，所有执行必须经 executor + middleware；工具目录、启用状态、调用记录等持久化只能通过 `tools/storage` 的端口访问，legacy `repository` 只保留迁移期委托。

整体的“Tools 模块”设计就需要围绕 Go 的接口抽象和 go-adk 的 SDK 来落地。

面向 go-adk 的完整设计方案，包括 **组件清单、分层架构、Go 接口定义、中间件链实现、与 go-adk 的集成方法**，以及一个生产可用的目录结构。

---

## 一、完整能力清单（与语言无关的核心能力回顾）

1.  工具注册与发现  
2.  输入/输出 Schema（强类型 struct，自动生成 JSON Schema 给 LLM）  
3.  参数校验（validator/v2）  
4.  执行器（同步/异步/流式）  
5.  工具组合与编排（管道、条件、并行）  
6.  人机交互 (Human-in-the-Loop)  
7.  错误处理、重试、熔断、降级  
8.  结果缓存  
9.  会话/上下文状态绑定（从 go-adk 的 context 或 Session 提取信息）  
10. 配置与密钥管理（环境变量/配置中心）  
11. 日志、Trace、Metrics（OpenTelemetry）  
12. Mock 与测试支持  

> **范围说明（Aranea）**：工具清单的**持久化**走后端 SQLite + HTTP CRUD（与前端管理台同步，见 `ToolService` / `tools/storage`），**不**在本文中建模「向 LLM 暴露的任意 SQL 执行工具」。**不**在本文中展开：工具多版本线并存、MCP/HTTP 外部协议适配、独立 ACL/权限子系统；若产品需要，另起文档或对接现有 Aranea 身份与会话层。

---

## 二、整体分层架构（Go 实现版）
┌─────────────────────────────────────────────────┐
│ go-adk Agent / Runner │
│ ● 通过 Tool 接口发现并调用工具 │
├─────────────────────────────────────────────────┤
│ Tool Registry & Adapter │
│ ● 将内部 Tool 统一注册为 go-adk 的 Tool │
│ ● 提供 FunctionTool / StreamingTool 适配器 │
├──────────────────┬──────────────────────────────┤
│ Middleware │ Executor │
│ - Auth（可选，身份/令牌）│ - Sync / Async / Stream │
│ - Validation │ - Error handling │
│ - Cache │ │
│ - Retry/CB │ │
│ - Logging/Trace │ │
├──────────────────┴──────────────────────────────┤
│ Backend Implementations（示例；非穷举） │
│ - 业务工具：天气 API、代码执行、文件读写等 │
│ 工具**元数据**与启用状态：见后端 DB + 前端 CRUD，非本层「SQL 工具」│
├─────────────────────────────────────────────────┤
│ Infrastructure │
│ - SQLite（Aranea 工具表 等）/ Redis - Config / Secrets │
│ - OpenTelemetry │
└─────────────────────────────────────────────────┘

**关键适配点**：go-adk 中的 Tool 是一个接口，通常传入 `function` 和 `parameters_json_schema`。我们需要把自己的 Tool 包装成 go-adk 能识别的形式，同时保留中间件链的执行入口。

---

## 三、Go 目录结构设计

```text
tools/
├── schema/ # 工具输入输出 struct 定义 (model)
│ └── weather.go
├── tooldef/ # 工具抽象接口
│ └── tool.go # Tool 接口，含 Execute/Stream 方法
├── registry/ # 工具注册中心与装饰器
│ └── registry.go
├── middleware/ # 中间件实现
│ ├── middleware.go # Middleware 类型定义
│ ├── chain.go # 链式组装
│ ├── validation.go
│ ├── auth.go
│ ├── cache.go
│ ├── retry.go
│ ├── tracing.go
│ └── approval.go # 人机审批挂起（第十一节）
├── executor/ # 核心执行器
│ └── executor.go # 串联 middleware 链并执行
├── toolctx/ # 加强版 ToolContext（从 go-adk ctx 提取；避免与标准库 context 包名冲突）
│ └── toolctx.go
├── backends/ # 具体**运行时**工具实现（实现 tooldef.Tool；与 DB 中工具行对应）
│ ├── weather.go
│ ├── code_exec.go
│ └── ...
├── adkbridge/ # go-adk 集成适配层
│ ├── adapter.go # FunctionTool / Long-running 分派
│ └── streaming.go # Streaming 工具适配（第十二节）
├── telemetry/ # Trace & Metrics（OpenTelemetry）
│ └── provider.go
├── config/
│ └── config.go
├── mocks/ # Mock 工具实现（用于测试）
│ └── mock_tools.go
└── examples/
    └── usage.go
```

> **包名建议**：实现时将「加强版上下文」放在 **`toolctx`** 包中，避免与标准库 `context` 同名的 `package context`。下文代码片段中的 `import "your-module/tools/toolctx"` 即指该包。

---

## 四、核心抽象与接口定义

### 1. 工具基类接口

```go
// tooldef/tool.go
package tooldef

import (
    "encoding/json"

    "your-module/tools/toolctx"
)

// InputSchema 返回输入参数结构体（用于 JSON Schema 生成）
type InputSchema interface {
    JSONSchema() json.RawMessage
    Validate() error
}

// OutputSchema 输出结构体，通常直接实现 JSON 序列化即可
type OutputSchema interface {
    JSON() ([]byte, error)
}

// Tool 是所有工具需要实现的接口
type Tool interface {
    Name() string
    Description() string
    // InputPrototype 返回一个零值输入 struct 用于 Schema 生成
    InputPrototype() InputSchema

    // Execute 同步/异步执行（返回结果或错误）
    Execute(ctx *toolctx.ToolContext, params InputSchema) (OutputSchema, error)

    // Stream 流式执行，返回 channel。若不需要流式，可返回 nil, error
    // 注意：go-adk 流式需配合 StreamHandler 使用，此处先用 channel 抽象
    Stream(ctx *toolctx.ToolContext, params InputSchema) (<-chan StreamChunk, error)
}

// StreamChunk 流式输出的数据块
type StreamChunk struct {
    Content OutputSchema
    Err     error
    Done    bool
}
```

### 2. ToolContext（加强上下文，从 go-adk context 中穿透）

```go
// toolctx/toolctx.go（或 context.go，但包名为 toolctx）
package toolctx

import (
    "context"
    "go.opentelemetry.io/otel/trace"
)

// ToolContext 嵌入标准 context，用于取消、截止时间与 trace 透传
type ToolContext struct {
    context.Context

    SessionID  string
    UserID     string
    TraceID    string
    Span       trace.Span // OTel span
    AuthToken  string
    StateStore SessionStateStore

    // 审批流：若本调用已持有效审批（如 Resume 路径），可写入 ApprovalID 供
    // ApprovalMiddleware 识别并跳过再次挂起。实现时可配合 StateStore 校验。
    ApprovalID string
}

// SessionStateStore 表示与 go-adk Session 对齐的状态/Artifact 存取能力
type SessionStateStore interface {
    Get(key string) (interface{}, error)
    Set(key string, value interface{}) error
}
```

**要点**：`ToolContext` 由中间件与 `adkbridge` 从 ADK 的 `RunContext` 或 `context.Context` 中构造；`ApprovalID` 为第十一节中「审批后跳过挂起」提供挂点。若与 `go.opentelemetry`, `errors` 等包产生循环依赖，可将 `SessionStateStore` 抽到更小的 `ports` 包。

---

## 五、中间件系统设计

### 1. 中间件类型定义

```go
// middleware/middleware.go
package middleware

import (
    "your-module/tools/tooldef"
    "your-module/tools/toolctx"
)

// Next 代表中间件链中的下一个调用
type Next func(ctx *toolctx.ToolContext, tool tooldef.Tool, params tooldef.InputSchema) (tooldef.OutputSchema, error)

// Middleware 接口
type Middleware interface {
    Run(ctx *toolctx.ToolContext, tool tooldef.Tool, params tooldef.InputSchema, next Next) (tooldef.OutputSchema, error)
}

// MiddlewareFunc 适配器
type MiddlewareFunc func(ctx *toolctx.ToolContext, tool tooldef.Tool, params tooldef.InputSchema, next Next) (tooldef.OutputSchema, error)

func (m MiddlewareFunc) Run(ctx *toolctx.ToolContext, tool tooldef.Tool, params tooldef.InputSchema, next Next) (tooldef.OutputSchema, error) {
    return m(ctx, tool, params, next)
}
```

### 2. 中间件链（洋葱模型）

```go
// middleware/chain.go
package middleware

import (
    "your-module/tools/toolctx"
    "your-module/tools/tooldef"
)

func BuildChain(mws ...Middleware) Middleware {
    return MiddlewareFunc(func(ctx *toolctx.ToolContext, tool tooldef.Tool, params tooldef.InputSchema, finalNext Next) (tooldef.OutputSchema, error) {
        next := finalNext
        for i := len(mws) - 1; i >= 0; i-- {
            mw := mws[i]
            currentNext := next
            next = func(ctx *toolctx.ToolContext, tool tooldef.Tool, params tooldef.InputSchema) (tooldef.OutputSchema, error) {
                return mw.Run(ctx, tool, params, currentNext)
            }
        }
        return next(ctx, tool, params)
    })
}

// FinalExecutor 是最终调用 tool.Execute 的 Next
func FinalExecutor() Next {
    return func(ctx *toolctx.ToolContext, tool tooldef.Tool, params tooldef.InputSchema) (tooldef.OutputSchema, error) {
        return tool.Execute(ctx, params)
    }
}
```

### 3. 示例中间件（参数校验、Tracing）

```go
// middleware/validation.go
package middleware

import (
    "fmt"
    "your-module/tools/tooldef"
    "your-module/tools/toolctx"
)

func ValidationMiddleware() Middleware {
    return MiddlewareFunc(func(ctx *toolctx.ToolContext, tool tooldef.Tool, params tooldef.InputSchema, next Next) (tooldef.OutputSchema, error) {
        if err := params.Validate(); err != nil {
            return nil, fmt.Errorf("tool %s validation failed: %w", tool.Name(), err)
        }
        return next(ctx, tool, params)
    })
}
```

```go
// middleware/tracing.go
package middleware

import (
    "your-module/tools/tooldef"
    "your-module/tools/toolctx"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/trace"
)

func TracingMiddleware() Middleware {
    tracer := otel.Tracer("tools")
    return MiddlewareFunc(func(ctx *toolctx.ToolContext, tool tooldef.Tool, params tooldef.InputSchema, next Next) (output tooldef.OutputSchema, err error) {
        _, span := tracer.Start(ctx, tool.Name(), trace.WithAttributes(
            attribute.String("session.id", ctx.SessionID),
            attribute.String("user.id", ctx.UserID),
        ))
        defer span.End()
        out, err := next(ctx, tool, params)
        if err != nil {
            span.RecordError(err)
        }
        return out, err
    })
}
```

---

## 六、执行器与 go-adk 集成

### 1. 执行器负责运行 Tool + 中间件链

```go
// executor/executor.go
package executor

import (
    "your-module/tools/toolctx"
    "your-module/tools/middleware"
    "your-module/tools/tooldef"
)

type Executor struct {
    mw middleware.Middleware
}

// NewExecutor 对传入的中间件**依次**用 BuildChain 包装；不要在外层再包一层 BuildChain
func NewExecutor(mws ...middleware.Middleware) *Executor {
    return &Executor{mw: middleware.BuildChain(mws...)}
}

// FillStructFromMap 将 LLM/JSON 的 map 填进 InputSchema 的强类型实例；生产可优先 json 往返。
func FillStructFromMap(in tooldef.InputSchema, params map[string]any) error { /* 实现略 */ return nil }

func (e *Executor) Run(ctx *toolctx.ToolContext, tool tooldef.Tool, params map[string]any) (tooldef.OutputSchema, error) {
    input := tool.InputPrototype()
    if err := FillStructFromMap(input, params); err != nil {
        return nil, err
    }
    return e.mw.Run(ctx, tool, input, middleware.FinalExecutor())
}
```

### 2. 与 go-adk 的适配器

go-adk 要求工具满足其 `tool.Tool` 接口，通常使用 `tool.NewFunctionTool` 等构造。下例需与 **`github.com/google/adk-go` 当前版本**中的符号名核对。

```go
// adkbridge/adapter.go
package adkbridge

import (
    "context"
    "encoding/json"
    "your-module/tools/executor"
    "your-module/tools/tooldef"
    "your-module/tools/toolctx"

    "github.com/google/adk-go/tool"
    "github.com/google/adk-go/agent/run"
)

// ToADKTool 将内部 Tool 转为 ADK 可用的 FunctionTool
func ToADKTool(t tooldef.Tool, exec *executor.Executor) *tool.FunctionTool {
    inputSchema := t.InputPrototype().JSONSchema()
    return tool.NewFunctionTool(
        run.NewToolCallback(func(ctx context.Context, args json.RawMessage) (any, error) {
            tc, err := toolctx.FromADKContext(ctx)
            if err != nil {
                return nil, err
            }
            var params map[string]any
            if err := json.Unmarshal(args, &params); err != nil {
                return nil, err
            }
            return exec.Run(tc, t, params)
        }),
        tool.WithName(t.Name()),
        tool.WithDescription(t.Description()),
        tool.WithInputSchema(inputSchema),
    )
}
```

人机审批与长时运行工具见**第十一节**；`ToADKTool` 在实现需按工具类型分派为 `FunctionTool` 或 Long-running 变体，避免与 八、注册流程 冲突。

---

## 七、具体工具实现示例（与 Aranea 数据流区分）

**两条线不要混**：

- **工具元数据（CRUD）**：走 Aranea 后端 SQLite + `/api/v1/tools` 等与**前端**的 HTTP API；存的是 `Tool` 行的名称、描述、JSON 配置、是否启用等，由 `ToolService` 调用 `tools/storage.Store` 实现。legacy `repository` 只做迁移期委托，不再承载 tools SQL。本文**不**把「对应用库执行任意 SQL」建造成暴露给 LLM 的 `tooldef.Tool`。
- **运行时工具（`backends/*`）**：在 Agent 中实际被调用的 `tooldef.Tool` 实现（如天气、文件、子进程等）。装配时可从**同一条** DB 行读配置，再分派到具体 backend；名称/Schema 以持久化与注册表一致为准。

```go
// backends/weather.go — 示例：纯业务实现，不直接等于「DB 一行」
package backends

import (
    "your-module/tools/tooldef"
    "your-module/tools/schema"
    "your-module/tools/toolctx"
)

type WeatherAPI struct{ APIKey string }

func (w *WeatherAPI) Name() string         { return "weather_lookup" }
func (w *WeatherAPI) Description() string   { return "按城市查天气" }
func (w *WeatherAPI) InputPrototype() tooldef.InputSchema { return &schema.WeatherInput{} }

func (w *WeatherAPI) Execute(ctx *toolctx.ToolContext, params tooldef.InputSchema) (tooldef.OutputSchema, error) {
    in := params.(*schema.WeatherInput)
    // 调外部 HTTP 或 SDK；APIKey 来自配置，不必来自 LLM
    // ...
    return &schema.WeatherOutput{}, nil
}
```

装配时（启动或 `ReloadTools`）用 `tools/storage` 中已 CRUD 的工具名与上述实现**绑定**，或按 `tool_key` 做注册表分派即可。

---

## 八、打造完整的初始化与注册流程

`NewExecutor` 接收**多个** `Middleware` 变参，由内部 `BuildChain` 组装；**不要**先 `BuildChain` 再传入（否则等效于对整条链再包一层 `BuildChain`）。若需对外暴露已固定的链，可另增 `NewExecutorWithChain(mw middleware.Middleware)` 仅作薄封装存 `Executor{mw: mw}`。

```go
// main.go 或 agent 构建代码片段
package main

import (
    "your-module/tools/adkbridge"
    "your-module/tools/backends"
    "your-module/tools/executor"
    "your-module/tools/middleware"
    "your-module/tools/registry"
    "github.com/google/adk-go/tool"
)

func buildAgent() {
    // 运行时可从 Aranea tools/storage 中已 CRUD 的工具行驱动注册；此处为最小示例
    weatherTool := &backends.WeatherAPI{APIKey: config.WeatherKey}

    reg := registry.New()
    reg.Register(weatherTool)

    // 变参：由 NewExecutor 内部 BuildChain
    exec := executor.NewExecutor(
        middleware.TracingMiddleware(),
        middleware.AuthMiddleware( /* authProvider */ ),
        middleware.ValidationMiddleware(),
        middleware.CacheMiddleware( /* cache */ ),
        middleware.RetryMiddleware( /* retry config */ ),
    )

    var adkTools []tool.Tool
    for _, t := range reg.List() {
        // 若 t 为 ApprovableLongRunning，在实现中可改为 adkbridge.ToADKFunctionOrLongRunning(t, exec) — 见第十一节
        adkTools = append(adkTools, adkbridge.ToADKTool(t, exec))
    }
    _ = adkTools
}
```

---

## 九、关于 Trace 与可观测性
你已有的 Trace 需求可以完整通过 OpenTelemetry 落地：

在工具调用入口（中间件）创建 Span。

SQLite 调用、HTTP 请求等内部操作可以通过 db.QueryContext(ctx) 和 http.NewRequestWithContext(ctx) 自动传播 Trace Context。

配置 OTLP Exporter 将 Trace 发送到 Jaeger/Zipkin/云平台。

go-adk 本身也支持 OTEL，将 Agent 调用和工具调用串联成一条完整 Trace。

---

## 十、总结

使用 go-adk 时，Tools 模块建设路径可遵循：

1. 定义 `tooldef.Tool` 与 `toolctx.ToolContext`（含 `ApprovalID` 等审批挂点，见十一节）  
2. 实现中间件管道（洋葱模型，统一切面）  
3. 实现具体 Backend（业务 API、子进程等），只关注业务逻辑；与 Aranea 中工具**表** CRUD 的衔接见第七节  
4. 构建 `adkbridge` 适配器，将内部 Tool 映射为 go-adk 的 `FunctionTool` / `StreamingTool` / Long-running 变体（**以依赖版本为准**）  
5. 添加配置、密钥、可选 `Auth` 等生产向中间件  
6. 对接 OpenTelemetry 完成 Tracing 与 Metrics  

本设计与 go-adk 保持松耦合：工具与中间件可独立单测，通过薄适配层与 ADK 对接。

---

## 十一、Human-in-the-Loop（人机交互 / 审批流）实现

> **与 adk-go 版本对齐**  
> 下文中 `NewLongRunningFunctionTool`、`LongRunningFunctionResult`、`PendingApproval`、`Resume` 等**类型与构造函数名**须与 `github.com/google/adk-go` 当前所依赖的 tag/commit **核对**；若符号不存在，以官方示例为准改写，本节保留流程语义。

> **与同步路径的关系**  
> `ToADKTool` 在第八节为简化的 `FunctionTool`；若某工具实现 `ApprovableTool`，应在**注册/装配**时改为 `adkbridge` 的 Long-running 构造（下例 `ToADKTool` / `createFunctionTool` 分派），避免与第十一节实现冲突。  
> **恢复执行**时：应调用 `ApprovableTool.ExecuteWithApproval`（在审批中间件**跳过**且 `StateStore` 已取回 `ApprovalResult` 后），或让 `Executor` 在 `ctx.ApprovalID` 与结果齐备时**仅**走 `ExecuteWithApproval`；**不要**在 Resume 中再次对同一逻辑调用未区分的 `exec.Run` 导致与业务语义不一致（实现时以测试锁定）。

高敏感操作（如删除数据、发送邮件、执行 SQL 写操作）需要人类审批后再继续。在 go-adk 中，可利用 **LongRunning** 变体 + Session Artifact/State 机制实现（具体 API 名依版本而定）。

**整体流程**

- Agent 触发需要审批的工具 → 工具进入「挂起」状态  
- 通过 ADK 侧事件返回「待审批」与 `approval_id`（名称以 SDK 为准）  
- 前端展示审批，用户操作后由 REST 将结果写回 Session  
- 恢复路径读取审批结果，调用 `ExecuteWithApproval` 或等效逻辑，返回最终结果  

### 实现步骤

#### a) 定义 `ApprovableTool`（放在 `tooldef` 包）

```go
// 需要人工审批后执行写操作/高危操作的，实现本接口
type ApprovableTool interface {
    Tool
    RequiresApproval(ctx *toolctx.ToolContext, params InputSchema) bool
    ApprovalDetails(ctx *toolctx.ToolContext, params InputSchema) (ApprovalRequest, error)
    // 审批**通过**后执行；禁止再以 Run → Execute 重复执行业务副作用
    ExecuteWithApproval(ctx *toolctx.ToolContext, params InputSchema, res ApprovalResult) (OutputSchema, error)
}

type ApprovalRequest struct {
    ID          string
    ToolName    string
    Description string
    Parameters  map[string]any
    CreatedAt   time.Time
}

type ApprovalResult struct {
    Approved   bool
    Comment    string
    ApprovedBy string
}
```

#### b) 审批挂起 `ApprovalMiddleware`

- 若 `ctx.ApprovalID != ""` 且已在 `StateStore` 中校验为「本 approval_id 已批准」，**不再挂起**（可交给 `FinalExecutor` 的专门分支，见下节 c）。  
- 若 `RequiresApproval` 为真且尚未有有效批准，则返回 `*PendingApprovalError` 供 `adkbridge` 转为挂起/事件。

```go
// middleware/approval.go
package middleware

import (
    "errors"
    "github.com/google/uuid"
    "your-module/tools/tooldef"
    "your-module/tools/toolctx"
)

func ApprovalMiddleware() Middleware {
    return MiddlewareFunc(func(ctx *toolctx.ToolContext, tool tooldef.Tool, params tooldef.InputSchema, next Next) (tooldef.OutputSchema, error) {
        if ctx != nil && ctx.ApprovalID != "" {
            // 可在此与 StateStore 对账：仅对「已批」的 approval_id 放行
            // 放行业务路径建议走 ExecuteWithApproval，而非重复 next → Execute
            return next(ctx, tool, params)
        }
        at, ok := tool.(tooldef.ApprovableTool)
        if !ok || !at.RequiresApproval(ctx, params) {
            return next(ctx, tool, params)
        }
        req, err := at.ApprovalDetails(ctx, params)
        if err != nil {
            return nil, err
        }
        if req.ID == "" {
            req.ID = uuid.NewString()
        }
        _ = ctx.StateStore.Set("pending_approval:"+req.ID, req)
        return nil, &PendingApprovalError{ApprovalID: req.ID, ToolName: tool.Name(), Request: req}
    })
}

type PendingApprovalError struct {
    ApprovalID string
    ToolName   string
    Request    tooldef.ApprovalRequest
}

func (e *PendingApprovalError) Error() string { return "tool " + e.ToolName + " requires approval: " + e.ApprovalID }
```

**实现注**：上段在 `ApprovalID != ""` 时仍 `next` 进入 `FinalExecutor`；实际应在 **`FinalExecutor` 或专用中间件** 中检测：若工具为 `ApprovableTool` 且已有 `ApprovalResult`，则调用 `ExecuteWithApproval`，**不得**再次走需审批的 `Execute` 路径。

#### c) `adkbridge` 与 Resume

**约定（流程语义）**

- 装配入口使用 **`ToADKFunctionOrLongRunning(t, exec)`**（名可自定）：对 `ApprovableTool` 走 Long-running 构造，否则委托第六节 `ToADKTool`（普通 `FunctionTool`）。  
- **首次执行**：`exec.Run` → 可能返回 `*middleware.PendingApprovalError` → 适配层转为 **挂起/待审批** 事件，并附带 `Resume`。  
- **Resume**：从 `StateStore` 取 `approval_result:{approval_id}`；若拒绝则直接返回错误；若批准则 **`ExecuteWithApproval`**（不再次走会触发审批的 `Execute` 路径）。`ctx.ApprovalID` 与审计字段写入应在此时完成。

**以下为一版完整可讨论伪代码**（`LongRunningResultT`、`NewLongRunningFunctionTool`、`Pending`/`Resume` 的字段与回调形参**须**与 `github.com/google/adk-go` 当前包 `tool` / `run` 对齐后替换，注释中标「adk:」处即待核对点）。

```go
// adkbridge/longrunning.go — 讨论稿，与 adk-go 版本绑定时逐符号替换
package adkbridge

import (
    "context"
    "encoding/json"
    "errors"

    "your-module/tools/executor"
    "your-module/tools/middleware"
    "your-module/tools/tooldef"
    "your-module/tools/toolctx"

    "github.com/google/adk-go/tool" // 实际子路径以 go.mod 为准
)

// ToADKFunctionOrLongRunning 在 Agent 装配处替代裸用 ToADKTool
func ToADKFunctionOrLongRunning(t tooldef.Tool, exec *executor.Executor) tool.Tool {
    if _, ok := t.(tooldef.ApprovableTool); ok {
        return createLongRunningADKTool(t, exec)
    }
    return ToADKTool(t, exec) // 第六节
}

// createLongRunningADKTool 包装「可审批」工具；adk: 以下返回类型在 SDK 中可能是 tool.Tool 或专用于 long-running 的接口
func createLongRunningADKTool(t tooldef.Tool, exec *executor.Executor) tool.Tool {
    at := t.(tooldef.ApprovableTool)
    return mustLongRunning(
        // adk: 构造器名可能是 NewLongRunningFunctionTool 或其它；见官方示例
        newLongRunningFunctionToolLike(func(ctx context.Context, args json.RawMessage) (any, error) {
            tc, err := toolctx.FromADKContext(ctx)
            if err != nil {
                return nil, err
            }
            var params map[string]any
            if err := json.Unmarshal(args, &params); err != nil {
                return nil, err
            }

            out, err := exec.Run(tc, t, params)
            if err != nil {
                var perr *middleware.PendingApprovalError
                if errors.As(err, &perr) {
                    return newPendingLongRunningResult(perr, args, t, at, exec), nil
                }
                return nil, err
            }

            // 本次调用未触发审批（例如 RequiresApproval 为 false）
            return out, nil
        }, t),
    )
}

// newLongRunningFunctionToolLike 为占位：实现里应换为 adk 真实回调 + tool.WithName/WithDescription/WithInputSchema
func newLongRunningFunctionToolLike(
    fn func(context.Context, json.RawMessage) (any, error),
    t tooldef.Tool,
) tool.Tool {
    _ = fn
    _ = t
    // return tool.NewLongRunningFunctionTool(回调, tool.WithName(t.Name()), ...) // adk: 实参
    return nil
}

func mustLongRunning(x tool.Tool) tool.Tool { return x }

// newPendingLongRunningResult 将 PendingApproval 与 Resume 打包；结构体名/字段为语义占位
func newPendingLongRunningResult(
    perr *middleware.PendingApprovalError,
    args json.RawMessage,
    t tooldef.Tool,
    at tooldef.ApprovableTool,
    exec *executor.Executor,
) any {
    // adk: 若 SDK 使用 *LongRunningFunctionResult{ Pending, Resume }
    return map[string]any{
        "pending_approval": true, // 仅供示意；真实为 SDK 的 Pending 描述对象
        "resume":           buildResume(perr, args, t, at, exec),
    }
}

// buildResume 用户在前端点「同意」后，由 Runner/SDK 在适当时机调用
func buildResume(
    perr *middleware.PendingApprovalError,
    args json.RawMessage,
    t tooldef.Tool,
    at tooldef.ApprovableTool,
    exec *executor.Executor,
) func(context.Context) (any, error) {
    return func(ctx context.Context) (any, error) {
        tc, err := toolctx.FromADKContext(ctx)
        if err != nil {
            return nil, err
        }
        key := "approval_result:" + perr.ApprovalID
        raw, err := tc.StateStore.Get(key)
        if err != nil {
            return nil, err
        }
        res, ok := raw.(tooldef.ApprovalResult)
        if !ok {
            return nil, errors.New("approval: invalid result type in state")
        }
        if !res.Approved {
            return nil, errors.New("approval rejected")
        }

        in := t.InputPrototype()
        if err := unmarshalParamsIntoInput(args, in); err != nil {
            return nil, err
        }
        tc.ApprovalID = perr.ApprovalID

        // 业务副作用只在此处发生一次
        return at.ExecuteWithApproval(tc, in, res)
    }
}

// 优先 json 二遍到 InputPrototype 的实参
func unmarshalParamsIntoInput(args json.RawMessage, in tooldef.InputSchema) error {
    return json.Unmarshal(args, in) // 若 InputSchema 为具体 struct 指针；否则复用 executor.FillStructFromMap
}
```

**讨论要点（评审时可对照实现）**

1. 若 adk 的 `Resume` **不**经过你们的 `Executor`，则 **`ValidationMiddleware` 等不会在 Resume 中执行**；需要在 `ExecuteWithApproval` 内补校验，或在 `buildResume` 中显式调用 `Validate()`。  
2. 若希望 Resume 也走同一套中间件，可改为 `exec.RunWithMode(tc, t, params, RunApproved)` 由 `Executor` 在链尾**只**调 `ExecuteWithApproval`（需在 `tooldef` / `Executor` 增加枚举或上下文标记）。  
3. `StateStore` 的键与 TTL、与** REST 写审批**（第十一节 d）的幂等性，应在同一张「状态契约」文档中固定。

#### d) 审批 REST 与产品路径

- 事件驱动 UI 展示 `approval_id`；用户决策后，后端把 `ApprovalResult` 写入与 Session 共享的 `StateStore` / Artifact。  
- Aranea 可对接现有会话语义（如 `/api/v1/...`），与 ADK 的「恢复 Runner」方式以你们进程内 `sessionService` 为准；**在文档中只约定键名** `approval_result:{id}` 与**幂等写**。

---

## 十二、流式工具与 SSE 对接

> **与 adk-go 版本对齐**  
> `NewStreamingTool`、`StreamingResult`、`StreamHandler`、事件类型等须与**当前** `adk-go` 核对；`model.ToolEvent` 在下列片段中为**占位名**，应替换为 SDK 中实际消息类型。

> **与同步中间件**  
> 下例在适配器里**直接**调用 `t.Stream(...)`，不经过与 `Execute` 相同的 `Middleware` 链。若需鉴权/审计/限流与同步一致，应：在 `adkbridge` 内复用与 `Run` 相同的前置检查，或后续引入 `StreamMiddleware` 并在适配器最外层包装。

> **与 Aranea 对话 API**  
> 若全站已存在聊天流式端点（例如 `/api/v1/chat/messages/stream`），SSE 的落点**优先复用**该端点，将 Agent/Runner 事件与工具流式事件合流；下例 `api/sse.go` 为原理示意，非必须新建平行路由。

对需要进度的长任务，可用「内部 `Stream` + channel + ADK Streaming 适配 + 对外 SSE」组合。

### 核心组件

- 工具 `Stream` `yield` 回调
- ADK 侧将 chunk 变成流事件（类型依 SDK）  
- HTTP **SSE** 将 Runner 或网关事件推到浏览器  

### a) 流式实现示例

```go
// backends/stream_example.go
package backends

import (
    "bufio"
    "os/exec"
    "your-module/tools/tooldef"
    "your-module/tools/toolctx"
    "your-module/tools/schema"
)

type StreamCodeExec struct{}

func (t *StreamCodeExec) Name() string { return "code_exec_stream" }
func (t *StreamCodeExec) InputPrototype() tooldef.InputSchema { return &StreamCodeInput{} }
func (t *StreamCodeExec) Description() string { return "流式执行代码并返回实时输出" }

func (t *StreamCodeExec) Stream(ctx *toolctx.ToolContext, params tooldef.InputSchema) (<-chan tooldef.StreamChunk, error) {
    input := params.(*StreamCodeInput)
    ch := make(chan tooldef.StreamChunk, 10)
    go func() {
        defer close(ch)
        cmd := exec.CommandContext(ctx, "python", "-c", input.Code)
        stdout, err := cmd.StdoutPipe()
        if err != nil { ch <- tooldef.StreamChunk{Err: err}; return }
        if err := cmd.Start(); err != nil { ch <- tooldef.StreamChunk{Err: err}; return }
        s := bufio.NewScanner(stdout)
        for s.Scan() {
            ch <- tooldef.StreamChunk{Content: &schema.StreamOutput{Data: s.Text()}, Done: false}
            select { case <-ctx.Done(): return; default: }
        }
        _ = cmd.Wait()
        ch <- tooldef.StreamChunk{Content: &schema.StreamOutput{Data: "[DONE]"}, Done: true}
    }()
    return ch, nil
}
```

> `Execute` 可返回 `nil, fmt.Errorf("stream only")` 或提供空实现，视团队约定。

### b) `adkbridge` 流式适配（伪代码）

```go
// adkbridge/streaming.go — 伪代码，签名以 adk-go 为准
func ToADKStreamTool(t tooldef.Tool, exec *executor.Executor) /* tool.StreamingTool 或 tool.Tool */ {
    _ = exec
    // 1) FromADKContext → *toolctx.ToolContext
    // 2) FillStructFromMap(InputPrototype(), params)
    // 3) ch, err := t.Stream(tc, input)
    // 4) 用 yield 迭代 ch，映射为 SDK 要求的事件类型（占位 model.ToolEvent）
    // return tool.NewStreamingTool(...)
    return nil
}
```

### c) SSE 原理示例

```go
// api/sse.go — 原理示意；生产请与会话流式 API 合并
func AgentChatSSE(w http.ResponseWriter, r *http.Request) {
    flusher, _ := w.(http.Flusher)
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    _ = r.URL.Query().Get("msg")
    // runCh, err := runner.Run(ctx, agentName, sessionID, userMsg)
    // for ev := range runCh { ... }
    _ = flusher
}
```

前端可用 `EventSource` 订阅 SSE；**若**与 Aranea 聊天流式端点合并，则保持**事件 schema 一致**（工具 partial 输出与 assistant 消息区分字段）。

---

## 十三、Human-in-the-Loop 与流式小结

- **审批**：`ApprovableTool` + `ApprovalMiddleware` + Long-running 适配；结果经 `StateStore`/Artifact 回传；**恢复**时优先 `ExecuteWithApproval`，避免与首次 `Execute` 重复副作用。  
- **流式**：`<-chan StreamChunk` → ADK Streaming 适配 → 与 Agent 事件同向进入 **SSE** 或既有流式 HTTP；**不**经过与 `Execute` 相同中间件时，须在适配器或独立 `StreamMiddleware` 中补齐安全与审计。

---

## 十四、（说明）Cursor / IDE 里「执行终端命令」与本文「沙箱」不是同一回事

- **Cursor（及 VS Code 系）内置终端**：Agent/用户触发的 shell 命令一般在**本机**由系统 shell（如 PowerShell、bash）启动子进程执行，拥有**当前用户**在 OS 下的权限；**不是**容器级或虚拟机级隔离的「强沙箱」。不同产品版本可能增加**提示、确认、忽略文件**等策略，但不改变「与本地开发环境同一安全域」的基本事实。
- **本文 `backends/code_exec`、流式子进程示例**：指 **Aranea / go-adk 运行时**里由 `exec.CommandContext` 等启动的进程，其隔离程度取决于**你们**在服务端或本机 agent 进程里如何配置（工作目录、环境变量、资源上限、是否单独用户/容器）。与 Cursor 聊天里点「Run」跑 `npm test` **无直接同一套实现**。
- 若要对「LLM 可调用的命令执行」做强隔离，需在**产品侧**另做设计（专用 worker、容器、禁止网络、只读根文件系统等），**不能**假设与 IDE 终端或本设计文档中的示例自动等同。