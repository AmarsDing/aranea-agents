# 04-OpenAI 兼容 API

## 一、需求文档

### 1.1 背景

Aranea-Agents 当前仅通过 Kratos HTTP/gRPC 接口对外提供服务，使用自定义的 Proto 契约。这导致：

- **生态割裂**：无法与 OpenAI 生态工具（如 ChatGPT 前端、LangChain、AutoGen）直接对接
- **迁移成本高**：从 OpenAI 迁移到 Aranea 的用户需要重写客户端代码
- **缺乏标准协议**：无法利用 OpenAI API 的流式响应、函数调用等标准能力

框架 `pkg/trpc-agent-go/server/openai/` 已提供完整的 OpenAI 兼容 API Server 实现：

| 组件 | 文件路径 | 职责 |
|------|----------|------|
| Server | `server/openai/server.go` | HTTP 服务器，处理 Chat Completions 请求 |
| Options | `server/openai/options.go` | 配置选项（BasePath/Path/SessionService/Agent/Runner/ModelName/AppName） |
| Converter | `server/openai/converter.go` | OpenAI 请求/响应格式转换 |

### 1.2 目标

1. **OpenAI 兼容端点**：提供 `/v1/chat/completions` 端点，兼容 OpenAI Chat Completions API
2. **流式响应**：支持 SSE（Server-Sent Events）流式输出
3. **非流式响应**：支持标准 JSON 响应
4. **Session 管理**：自动创建/复用 Session，支持多轮对话
5. **Kratos 集成**：将 OpenAI 兼容 Server 嵌入 Kratos HTTP Server

### 1.3 功能需求

#### P0 — 必须实现

| ID | 需求 | 说明 |
|----|------|------|
| O-P0-1 | Chat Completions 端点 | 提供 `POST /v1/chat/completions`，兼容 OpenAI API 格式 |
| O-P0-2 | 流式响应 | 支持 `stream: true`，返回 SSE 格式的增量响应 |
| O-P0-3 | 非流式响应 | 支持 `stream: false`，返回完整 JSON 响应 |
| O-P0-4 | Runner 集成 | 使用框架 `openai.New(openai.WithRunner(...))` 创建 Server |
| O-P0-5 | Kratos HTTP Server 挂载 | 将 OpenAI Server 的 Handler 挂载到 Kratos HTTP Server |

#### P1 — 应该实现

| ID | 需求 | 说明 |
|----|------|----------|
| O-P1-1 | Model 列表端点 | 提供 `GET /v1/models`，返回可用模型列表 |
| O-P1-2 | Session 自动管理 | 通过 `openai.WithSessionService(...)` 自动创建/复用 Session |
| O-P1-3 | 多 Agent 路由 | 根据请求中的 model 字段路由到不同 Agent |
| O-P1-4 | 认证中间件 | Bearer Token 认证，兼容 OpenAI API Key 格式 |

#### P2 — 可以实现

| ID | 需求 | 说明 |
|----|------|----------|
| O-P2-1 | Function Calling | 支持 OpenAI 格式的函数调用（映射到 Agent Tools） |
| O-P2-2 | Embeddings 端点 | 提供 `POST /v1/embeddings`，返回文本嵌入向量 |
| O-P2-3 | 速率限制 | 按 API Key 限流 |

### 1.4 非功能需求

| 维度 | 要求 |
|------|------|
| 性能 | 流式首 Token 延迟 < 500ms |
| 兼容性 | 兼容 OpenAI API v1 格式，可被 LangChain/OpenAI SDK 直接调用 |
| 可靠性 | SSE 连接断开时正确清理资源 |
| 安全性 | 支持 Bearer Token 认证，默认拒绝未认证请求 |
| 可观测性 | 请求计数、延迟、错误率通过 Prometheus 暴露 |

### 1.5 验收标准

1. 使用 OpenAI Python SDK 能成功调用 `/v1/chat/completions`
2. `stream: true` 返回 SSE 格式增量响应，客户端可逐 Token 接收
3. `stream: false` 返回完整 JSON 响应，格式与 OpenAI 一致
4. 多轮对话通过 Session 自动管理上下文
5. `make wire && make build && make test` 全部通过

---

## 二、设计文档

### 2.1 框架参考

#### Server 结构体 — `pkg/trpc-agent-go/server/openai/server.go`

```go
type Server struct {
    basePath       string
    path           string
    handler        http.Handler
    sessionService session.Service
    runner         *runner.Runner
    agent          agent.Agent
    modelName      string
    converter      *converter
}

func New(opts ...Option) (*Server, error)
```

核心方法：

```go
func (s *Server) Handler() http.Handler
func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request)
func (s *Server) handleStreaming(ctx context.Context, w http.ResponseWriter, req openAIRequest, runOpts ...runner.RunOption)
func (s *Server) handleNonStreaming(ctx context.Context, req openAIRequest, runOpts ...runner.RunOption) (*openAIResponse, error)
```

#### Options — `pkg/trpc-agent-go/server/openai/options.go`

```go
type Option func(*Server) error

func WithBasePath(path string) Option
func WithPath(path string) Option
func WithSessionService(svc session.Service) Option
func WithAgent(a agent.Agent) Option
func WithRunner(r *runner.Runner) Option
func WithModelName(name string) Option
func WithAppName(name string) Option
```

**关键约束**：`WithAgent` 和 `WithRunner` 至少提供一个。如果提供 `WithRunner`，Server 使用 `runner.Run(ctx, userID, sessionID, userMessage, runOpts...)` 执行请求。

#### Converter — `pkg/trpc-agent-go/server/openai/converter.go`

```go
type openAIRequest struct {
    Model       string          `json:"model"`
    Messages    []openAIMessage `json:"messages"`
    Stream      bool            `json:"stream"`
    Temperature *float64        `json:"temperature,omitempty"`
    MaxTokens   *int            `json:"max_tokens,omitempty"`
}

type openAIMessage struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}
```

#### 流式处理

`handleStreaming` 方法：

1. 设置 SSE 响应头：`Content-Type: text/event-stream`、`Cache-Control: no-cache`、`Connection: keep-alive`
2. 调用 `runner.Run()` 获取事件流
3. 逐事件写入 SSE 格式：`data: {json}\n\n`
4. 结束标记：`data: [DONE]\n\n`

#### 非流式处理

`handleNonStreaming` 方法：

1. 调用 `runner.Run()` 聚合所有事件
2. 组装完整 `openAIResponse` 返回

### 2.2 当前项目现状

当前 Server 层在 `internal/server/` 下注册 Kratos HTTP/gRPC 服务：

| 文件 | 职责 |
|------|------|
| `server.go` | HTTP/gRPC Server 创建 |
| `middleware.go` | 中间件注册 |

当前仅注册了 Kratos 自身的 HTTP 路由，无 OpenAI 兼容端点。

Service 层（`internal/service/`）已实现 `runner.Runner` 装配，可直接提供给 OpenAI Server 使用。

### 2.3 架构设计

#### 2.3.1 整体架构

```
Kratos HTTP Server
  ├─ /api/v1/...          ← Kratos 原有路由
  └─ /v1/chat/completions ← OpenAI 兼容路由（新增）
       │
       ▼
internal/server/openai.go  ← 新增：OpenAI Server 初始化
  └─ openai.New(
       openai.WithRunner(runner),
       openai.WithSessionService(sessionSvc),
       openai.WithModelName("aranea"),
       openai.WithBasePath("/v1"),
     )
       │
       ▼
pkg/trpc-agent-go/server/openai  ← 框架提供完整实现
```

#### 2.3.2 Server 初始化

新增 `internal/server/openai.go`：

```go
type OpenAIServerConfig struct {
    Enabled    bool
    BasePath   string
    Path       string
    ModelName  string
    AppName    string
}

func NewOpenAIHandler(
    cfg *OpenAIServerConfig,
    runner *runner.Runner,
    sessionSvc session.Service,
) (http.Handler, error) {
    srv, err := openai.New(
        openai.WithRunner(runner),
        openai.WithSessionService(sessionSvc),
        openai.WithModelName(cfg.ModelName),
        openai.WithAppName(cfg.AppName),
        openai.WithBasePath(cfg.BasePath),
        openai.WithPath(cfg.Path),
    )
    if err != nil {
        return nil, kerrors.InternalServer("OPENAI", err.Error())
    }
    return srv.Handler(), nil
}
```

#### 2.3.3 Kratos HTTP Server 挂载

在 `internal/server/server.go` 中挂载 OpenAI Handler：

```go
func NewHTTPServer(
    c *conf.Server,
    // ... 现有依赖
    openAIHandler http.Handler,
) *http.Server {
    srv := http.NewServer(c.Http)

    // 现有路由注册
    v1.RegisterChatHTTPServer(srv, chatSvc)

    // OpenAI 兼容路由
    if openAIHandler != nil {
        srv.Handle("/v1/", openAIHandler)
    }

    return srv
}
```

#### 2.3.4 配置结构

在 `internal/conf/conf.proto` 中扩展：

```protobuf
message OpenAI {
  bool enabled = 1;
  string base_path = 2;
  string path = 3;
  string model_name = 4;
  string app_name = 5;
  bool auth_enabled = 6;
  repeated string api_keys = 7;
}
```

#### 2.3.5 认证中间件

P1 阶段新增 Bearer Token 认证：

```go
func OpenAIAuthMiddleware(apiKeys []string) func(http.Handler) http.Handler {
    keySet := make(map[string]struct{}, len(apiKeys))
    for _, k := range apiKeys {
        keySet[k] = struct{}{}
    }
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
            if _, ok := keySet[token]; !ok {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

#### 2.3.6 Wire 注入

`internal/server/provider.go` 新增：

```go
var ProviderSet = wire.NewSet(
    NewHTTPServer,
    NewGRPCServer,
    NewOpenAIHandler,
)
```

`NewOpenAIHandler` 在 `openai.enabled=false` 时返回 `nil`，Kratos Server 检查 `nil` 跳过挂载。

### 2.4 与框架的集成方式

| 集成点 | 框架包 | 项目适配层 | 说明 |
|--------|--------|-----------|------|
| OpenAI Server | `server/openai` | `internal/server/openai.go` | 直接使用 `openai.New()` 创建 |
| Runner 执行 | `runner.Runner` | `internal/service` 提供 | 复用现有 Runner 装配 |
| Session 管理 | `session.Service` | `internal/session` 提供 | 复用现有 Session 服务 |
| HTTP Handler | `Server.Handler()` | Kratos HTTP Server 挂载 | 框架返回标准 `http.Handler` |
| SSE 流式 | 框架内部实现 | 无需适配 | `handleStreaming` 开箱即用 |
| 请求转换 | `converter` | 框架内部使用 | OpenAI ↔ Agent 格式转换 |

**关键原则**：OpenAI API 的请求解析、响应格式化、SSE 流式输出全部由框架处理。项目只做 Server 创建、配置注入、Kratos 路由挂载。

### 2.5 错误处理

| 场景 | 错误类型 | 处理方式 |
|------|----------|----------|
| OpenAI Server 创建失败 | `kerrors.InternalServer("OPENAI", ...)` | 启动时 Fail Fast |
| Runner 为 nil | `kerrors.InternalServer("OPENAI", ...)` | 启动时 Fail Fast |
| 认证失败 | HTTP 401 Unauthorized | 返回标准 OpenAI 错误格式 |
| Runner 执行失败 | 框架内部处理 | 返回 OpenAI 格式错误响应 |
| SSE 连接断开 | 框架内部清理 | context 取消，资源释放 |
| 请求格式错误 | HTTP 400 Bad Request | 返回 OpenAI 格式错误响应 |

---

## 三、开发计划

### 3.1 任务拆解

| # | 任务 | 涉及文件 | 依赖 | 预估 |
|---|------|----------|------|------|
| T1 | 扩展 Proto 配置结构 | `internal/conf/conf.proto` | 无 | 0.5d |
| T2 | 新增 OpenAI Server 初始化 | `internal/server/openai.go` | T1 | 1d |
| T3 | Kratos HTTP Server 挂载 | `internal/server/server.go` | T2 | 0.5d |
| T4 | Wire ProviderSet 适配 | `internal/server/provider.go` | T2 | 0.5d |
| T5 | 认证中间件 | `internal/server/openai_auth.go` | T1 | 0.5d |
| T6 | 集成测试 | `internal/server/openai_test.go` | T2, T3 | 1.5d |
| T7 | `make api && make wire && make build` 验证 | 全局 | T1-T5 | 0.5d |

### 3.2 开发顺序

```
Phase 1 — 配置与核心（T1 → T2）
  ├─ T1: Proto 配置扩展
  └─ T2: OpenAI Server 初始化

Phase 2 — 集成（T3 → T4 → T5）
  ├─ T3: Kratos HTTP Server 挂载
  ├─ T4: Wire ProviderSet 适配
  └─ T5: 认证中间件

Phase 3 — 验证（T6 → T7）
  ├─ T6: 集成测试
  └─ T7: 全量构建验证
```

### 3.3 验证方案

| 验证项 | 方法 | 通过标准 |
|--------|------|----------|
| 非流式请求 | `curl -X POST /v1/chat/completions -d '{"model":"aranea","messages":[{"role":"user","content":"hello"}]}'` | 返回完整 JSON 响应 |
| 流式请求 | `curl -N /v1/chat/completions -d '{"model":"aranea","messages":[{"role":"user","content":"hello"}],"stream":true}'` | 返回 SSE 增量响应 |
| OpenAI SDK 兼容 | 使用 OpenAI Python SDK 调用 | 成功完成对话 |
| 认证拦截 | 不带 Bearer Token 请求 | 返回 401 |
| 配置关闭 | `openai.enabled=false` | OpenAI 路由不可访问 |
| 全量构建 | `make api && make wire && make build && make test` | 零错误 |
