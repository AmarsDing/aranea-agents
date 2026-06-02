# 06-Langfuse 可观测性

## 一、需求文档

### 1.1 背景

当前 Aranea-Agents 的可观测性依赖 FlowLog（`internal/event`）和基础日志，缺乏专业的 LLM 可观测性能力：

- **缺乏 Trace 级别追踪**：无法追踪一次 Agent 运行的完整调用链（LLM 调用→工具执行→子 Agent）
- **缺乏 Token 用量统计**：无法按用户/会话/模型统计 Token 消耗
- **缺乏延迟分析**：无法分析 LLM 调用的首 Token 延迟、总延迟分布
- **缺乏成本追踪**：无法计算 LLM 调用的实际成本

框架 `pkg/trpc-agent-go/openclaw/app/langfuse.go` 已提供 Langfuse 集成参考，`pkg/trpc-agent-go/telemetry/langfuse/` 提供 Langfuse SDK 封装。

### 1.2 目标

1. **Langfuse 集成**：集成 Langfuse 平台，实现 LLM 调用的全链路追踪
2. **Trace/Span 模型**：Runner 运行为 Trace，LLM 调用/工具执行为 Span
3. **Baggage 传播**：通过 Baggage 在调用链中传播元数据（userID/sessionID/appName 等）
4. **RunOption 注入**：通过 `buildLangfuseRunOptionResolver` 自动注入 Langfuse RunOption

### 1.3 功能需求

#### P0 — 必须实现

| ID | 需求 | 说明 |
|----|------|------|
| L-P0-1 | Langfuse 初始化 | 使用框架 `langfuse.Start` 初始化 Langfuse 客户端 |
| L-P0-2 | RunOption 注入 | 使用 `buildLangfuseRunOptionResolver` 注入 Trace/Span 配置 |
| L-P0-3 | Baggage 传播 | 通过 Baggage 传播 userID/sessionID/appName/channel/requestID |
| L-P0-4 | 配置驱动 | 通过 `config.yaml` 的 `langfuse.enabled` 开关 |
| L-P0-5 | 条件启用 | `maybeEnableLangfuse` 根据配置决定是否启用 |

#### P1 — 应该实现

| ID | 需求 | 说明 |
|----|------|----------|
| L-P1-1 | Trace 命名 | 自动生成 Trace 名称（格式：`{appName}/{agentName}/{sessionID}`） |
| L-P1-2 | Trace-started 回调 | 通过 `trace-started` 回调记录 Trace 创建事件 |
| L-P1-3 | Token 用量统计 | 自动收集 LLM 调用的 Token 用量并上报 Langfuse |
| L-P1-4 | 优雅关闭 | `langfuseRuntime.Shutdown()` 确保所有 Trace 刷出 |

#### P2 — 可以实现

| ID | 需求 | 说明 |
|----|------|----------|
| L-P2-1 | 自定义元数据 | 支持在 Baggage 中传播自定义业务元数据 |
| L-P2-2 | 采样率配置 | 配置 Langfuse Trace 采样率，避免全量上报 |
| L-P2-3 | Langfuse Dashboard 集成 | 前端嵌入 Langfuse Dashboard 链接 |

### 1.4 非功能需求

| 维度 | 要求 |
|------|------|
| 性能 | Langfuse 上报对主流程延迟增加 < 2%（异步上报） |
| 可靠性 | Langfuse 不可用时不影响主流程 |
| 数据安全 | Langfuse API Key 安全存储，不记录到日志 |
| 可观测性 | Langfuse 连接状态、Trace 数量通过 FlowLog 记录 |
| 优雅关闭 | 进程退出前确保所有 pending Trace 刷出到 Langfuse |

### 1.5 验收标准

1. `langfuse.enabled=true` 时，Agent 运行在 Langfuse 平台可见完整 Trace
2. Trace 包含 LLM 调用 Span，记录模型名称、Token 用量、延迟
3. Baggage 正确传播 userID/sessionID 等元数据
4. Langfuse 不可用时主流程不受影响
5. 进程优雅关闭时所有 Trace 刷出
6. `make wire && make build && make test` 全部通过

---

## 二、设计文档

### 2.1 框架参考

#### langfuseRuntime — `pkg/trpc-agent-go/openclaw/app/langfuse.go`

```go
type langfuseRuntime struct {
    adminStatus         *adminStatus
    runOptionResolver   runner.RunOptionResolver
    shutdown            func()
}

func maybeEnableLangfuse(cfg LangfuseConfig) (*langfuseRuntime, error)
```

#### Baggage 元数据传播

框架通过 Baggage 传播以下元数据：

| Key | 说明 |
|-----|------|
| `userID` | 用户标识 |
| `sessionID` | 会话标识 |
| `appName` | 应用名称 |
| `channel` | 渠道标识 |
| `requestID` | 请求标识 |
| `messageID` | 消息标识 |
| `profileID` | 配置标识 |

#### RunOption Resolver

```go
func buildLangfuseRunOptionResolver(resolverOpts ...langfuse.RunOptionResolverOpt) runner.RunOptionResolver
```

Resolver 在每次 `runner.Run` 时自动注入 Langfuse 相关的 `RunOption`，包括：

- Trace 名称生成
- Trace-started 回调
- Baggage 元数据注入

#### Langfuse SDK — `pkg/trpc-agent-go/telemetry/langfuse/`

```go
func Start(opts ...Option) error

type Option func(*config)

func WithPublicKey(key string) Option
func WithSecretKey(key string) Option
func WithBaseURL(url string) Option
func WithFlushInterval(interval time.Duration) Option
func WithEnabled(enabled bool) Option
```

### 2.2 当前项目现状

当前项目没有 Langfuse 集成。Runner 执行流程中无任何可观测性埋点。

关键集成点：

| 位置 | 当前行为 | 需要添加 |
|------|----------|----------|
| `internal/service/chat.go` | Runner.Run 执行 | 注入 RunOptionResolver |
| `cmd/admin/wire.go` | Wire 装配 | 新增 Langfuse 初始化 |
| `cmd/admin/main.go` | 启动流程 | 新增 Langfuse 优雅关闭 |

### 2.3 架构设计

#### 2.3.1 整体架构

```
config.yaml
  └─ langfuse.enabled: true/false
  └─ langfuse.public_key: pk-***
  └─ langfuse.secret_key: sk-***
  └─ langfuse.base_url: https://cloud.langfuse.com
       │
       ▼
internal/telemetry/langfuse.go  ← 新增：Langfuse 运行时管理
  ├─ maybeEnableLangfuse(cfg) → *langfuseRuntime
  ├─ buildLangfuseRunOptionResolver(...)
  └─ Shutdown()
       │
       ▼
internal/service/chat.go  ← 修改：注入 RunOptionResolver
  └─ runner.Run(ctx, userID, sessionID, msg, runOpts...)
       │
       ▼
cmd/admin/main.go  ← 修改：优雅关闭
  └─ langfuseRuntime.Shutdown()
```

#### 2.3.2 配置结构

在 `internal/conf/conf.proto` 中扩展：

```protobuf
message Langfuse {
  bool enabled = 1;
  string public_key = 2;
  string secret_key = 3;
  string base_url = 4;
  int64 flush_interval_ms = 5;
  bool enabled_sampling = 6;
  double sample_rate = 7;
}
```

#### 2.3.3 Langfuse 运行时

新增 `internal/telemetry/langfuse.go`：

```go
type LangfuseRuntime struct {
    runOptionResolver runner.RunOptionResolver
    shutdown          func()
    enabled           bool
}

func NewLangfuseRuntime(cfg *conf.Langfuse) (*LangfuseRuntime, error) {
    if !cfg.Enabled {
        return &LangfuseRuntime{enabled: false}, nil
    }

    rt, err := maybeEnableLangfuse(LangfuseConfig{
        PublicKey:     cfg.PublicKey,
        SecretKey:     cfg.SecretKey,
        BaseURL:       cfg.BaseUrl,
        FlushInterval: time.Duration(cfg.FlushIntervalMs) * time.Millisecond,
    })
    if err != nil {
        return nil, kerrors.InternalServer("LANGFUSE", err.Error())
    }

    return &LangfuseRuntime{
        runOptionResolver: rt.runOptionResolver,
        shutdown:          rt.shutdown,
        enabled:           true,
    }, nil
}

func (r *LangfuseRuntime) RunOptionResolver() runner.RunOptionResolver {
    return r.runOptionResolver
}

func (r *LangfuseRuntime) Shutdown() {
    if r.shutdown != nil {
        r.shutdown()
    }
}
```

#### 2.3.4 Service 层集成

在 `internal/service/chat.go` 中注入 RunOptionResolver：

```go
type ChatService struct {
    runner              *runner.Runner
    langfuseRuntime     *telemetry.LangfuseRuntime
}

func (s *ChatService) Run(ctx context.Context, req *v1.RunRequest) (*v1.RunResponse, error) {
    var runOpts []runner.RunOption

    if s.langfuseRuntime != nil && s.langfuseRuntime.enabled {
        resolver := s.langfuseRuntime.RunOptionResolver()
        if resolver != nil {
            opts := resolver.Resolve(ctx, req.UserId, req.SessionId)
            runOpts = append(runOpts, opts...)
        }
    }

    return s.runner.Run(ctx, req.UserId, req.SessionId, req.Message, runOpts...)
}
```

#### 2.3.5 Baggage 注入

在 Service 层设置 Baggage 元数据：

```go
func injectBaggage(ctx context.Context, req *v1.RunRequest) context.Context {
    md := map[string]string{
        "userID":    req.UserId,
        "sessionID": req.SessionId,
        "appName":   req.AgentName,
        "channel":   req.Channel,
        "requestID": req.RequestId,
    }
    for k, v := range md {
        ctx = baggage.WithValue(ctx, k, v)
    }
    return ctx
}
```

#### 2.3.6 优雅关闭

在 `cmd/admin/main.go` 中注册优雅关闭：

```go
func main() {
    // ... 现有启动逻辑

    app, cleanup, err := initApp(...)
    defer cleanup()

    // 新增：Langfuse 优雅关闭
    if langfuseRt := app.LangfuseRuntime(); langfuseRt != nil {
        defer langfuseRt.Shutdown()
    }

    // ... 现有运行逻辑
}
```

#### 2.3.7 Wire 注入

`internal/telemetry/provider.go`：

```go
var ProviderSet = wire.NewSet(
    NewLangfuseRuntime,
)
```

`cmd/admin/wire.go` 新增 `telemetry.ProviderSet`。

### 2.4 与框架的集成方式

| 集成点 | 框架包 | 项目适配层 | 说明 |
|--------|--------|-----------|------|
| Langfuse 初始化 | `telemetry/langfuse.Start` | `internal/telemetry/langfuse.go` | 直接使用框架启动函数 |
| RunOption Resolver | `buildLangfuseRunOptionResolver` | 同上 | 框架提供 Resolver 构建函数 |
| Baggage 传播 | 框架内部使用 | `internal/service/chat.go` | Service 层注入 Baggage |
| Trace 命名 | 框架内部生成 | 无需适配 | Resolver 自动生成 |
| Token 统计 | 框架内部收集 | 无需适配 | LLM 调用自动上报 |
| 优雅关闭 | `langfuseRuntime.Shutdown` | `cmd/admin/main.go` | 进程退出前调用 |

**关键原则**：Langfuse 的 Trace/Span 创建、Token 统计、延迟分析全部由框架内部处理。项目只做初始化、RunOption 注入和优雅关闭。框架的 `maybeEnableLangfuse` 模式确保 Langfuse 不可用时不影响主流程。

### 2.5 错误处理

| 场景 | 错误类型 | 处理方式 |
|------|----------|----------|
| Langfuse 初始化失败 | `kerrors.InternalServer("LANGFUSE", ...)` | 启动时 Fail Fast |
| Langfuse API Key 无效 | 启动时连接测试失败 | 启动时 Fail Fast |
| Langfuse 上报失败 | 框架内部重试 | 异步重试，不影响主流程 |
| Langfuse 不可用 | 框架内部降级 | 跳过上报，不影响主流程 |
| Baggage 注入失败 | FlowLog 记录 | 不影响主流程 |
| Shutdown 超时 | FlowLog 记录 | 强制退出 |

---

## 三、开发计划

### 3.1 任务拆解

| # | 任务 | 涉及文件 | 依赖 | 预估 |
|---|------|----------|------|------|
| T1 | 扩展 Proto 配置结构 | `internal/conf/conf.proto` | 无 | 0.5d |
| T2 | 新增 Langfuse 运行时 | `internal/telemetry/langfuse.go` | T1 | 1.5d |
| T3 | 新增 Wire ProviderSet | `internal/telemetry/provider.go` | T2 | 0.5d |
| T4 | Service 层 RunOption 注入 | `internal/service/chat.go` | T2 | 1d |
| T5 | Baggage 注入 | `internal/service/chat.go` | T4 | 0.5d |
| T6 | 优雅关闭 | `cmd/admin/main.go` | T2 | 0.5d |
| T7 | Wire 装配更新 | `cmd/admin/wire.go` | T3 | 0.5d |
| T8 | 集成测试 | `internal/telemetry/langfuse_test.go` | T2-T7 | 1d |
| T9 | `make api && make wire && make build` 验证 | 全局 | T1-T8 | 0.5d |

### 3.2 开发顺序

```
Phase 1 — 基础设施（T1 → T2 → T3）
  ├─ T1: Proto 配置扩展
  ├─ T2: Langfuse 运行时
  └─ T3: Wire ProviderSet

Phase 2 — 集成（T4 → T5 → T6 → T7）
  ├─ T4: Service 层 RunOption 注入
  ├─ T5: Baggage 注入
  ├─ T6: 优雅关闭
  └─ T7: Wire 装配更新

Phase 3 — 验证（T8 → T9）
  ├─ T8: 集成测试
  └─ T9: 全量构建验证
```

### 3.3 验证方案

| 验证项 | 方法 | 通过标准 |
|--------|------|----------|
| Langfuse 初始化 | `go test ./internal/telemetry/... -run TestLangfuseInit -count=1` | 配置启用时成功初始化 |
| RunOption 注入 | `go test ./internal/telemetry/... -run TestRunOptionResolver -count=1` | Resolver 返回有效 RunOption |
| Baggage 传播 | `go test ./internal/telemetry/... -run TestBaggage -count=1` | Context 中包含正确元数据 |
| 优雅关闭 | `go test ./internal/telemetry/... -run TestShutdown -count=1` | Shutdown 不阻塞 |
| 降级模式 | `go test ./internal/telemetry/... -run TestLangfuseDisabled -count=1` | 禁用时主流程正常 |
| Langfuse 平台验证 | 手动触发 Agent 运行，检查 Langfuse Dashboard | Trace/Span 完整可见 |
| 全量构建 | `make api && make wire && make build && make test` | 零错误 |
