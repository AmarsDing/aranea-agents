# 05-DebugRecorder 调试录制

## 一、需求文档

### 1.1 背景

当前 Aranea-Agents 缺乏 Agent 运行过程的调试录制能力。当 Agent 执行出现异常时，开发者只能通过日志排查，存在以下问题：

- **日志碎片化**：日志分散在多个模块，难以还原完整执行链路
- **缺乏结构化记录**：日志为非结构化文本，无法程序化分析
- **无法回放**：无法按时间线回放 Agent 的完整执行过程
- **缺乏 LLM 交互记录**：无法查看发送给 LLM 的完整请求和响应

框架 `pkg/trpc-agent-go/openclaw/internal/debugrecorder/` 已提供完整的调试录制实现：

| 组件 | 文件路径 | 职责 |
|------|----------|------|
| Recorder | `debugrecorder/recorder.go` | 录制器主结构，管理 Trace 生命周期 |
| Trace | `debugrecorder/recorder.go` | 单次请求的录制实例 |
| Context 传播 | `debugrecorder/recorder.go` | `WithTrace`/`TraceFromContext`/`WithRecorder`/`RecorderFromContext` |

### 1.2 目标

1. **结构化录制**：对 Agent 运行的完整生命周期进行结构化录制
2. **Trace 级别隔离**：每次请求一个 Trace，包含完整的执行链路
3. **LLM 交互记录**：记录发送给 LLM 的请求和收到的响应
4. **安全模式**：支持 `full`/`safe` 两种模式，`safe` 模式脱敏敏感信息
5. **持久化存储**：录制结果持久化到文件系统，支持后续分析

### 1.3 功能需求

#### P0 — 必须实现

| ID | 需求 | 说明 |
|----|------|------|
| D-P0-1 | Recorder 初始化 | 使用框架 `debugrecorder.New(dir, mode)` 创建 Recorder |
| D-P0-2 | Trace 生命周期 | 每次 Runner.Run 创建 Trace，结束时 Close |
| D-P0-3 | 事件录制 | 通过 `Trace.Record(kind, payload)` 记录关键事件 |
| D-P0-4 | Context 传播 | 通过 `WithTrace`/`TraceFromContext` 在调用链中传播 Trace |
| D-P0-5 | 配置驱动 | 通过 `config.yaml` 的 `debug_recorder.enabled` 开关 |

#### P1 — 应该实现

| ID | 需求 | 说明 |
|----|------|------|
| D-P1-1 | LLM 请求录制 | 使用 `RecordModelRequest` 记录 LLM API 调用 |
| D-P1-2 | Runner 事件录制 | 使用 `KindRunnerEvent` 记录 Runner 层事件 |
| D-P1-3 | 安全模式 | `safe` 模式下脱敏 API Key、用户输入等敏感信息 |
| D-P1-4 | Gzip 压缩 | Trace 结束时自动 Gzip 压缩 `events.jsonl` |

#### P2 — 可以实现

| ID | 需求 | 说明 |
|----|------|------|
| D-P2-1 | 录制回放 UI | 前端展示录制回放时间线 |
| D-P2-2 | Blob 存储 | 使用 `StoreBlob` 存储大体积附件（如图片、文件） |
| D-P2-3 | 录制清理策略 | 自动清理过期的录制文件 |

### 1.4 非功能需求

| 维度 | 要求 |
|------|------|
| 性能 | 录制操作对主流程延迟增加 < 5% |
| 可靠性 | 录制失败不影响主流程执行 |
| 存储 | 单次 Trace 文件大小 < 10MB（safe 模式），< 50MB（full 模式） |
| 安全 | `safe` 模式下不记录 API Key、Token 等敏感信息 |
| 可观测性 | 录制文件数量、总大小通过 FlowLog 记录 |

### 1.5 验收标准

1. `debug_recorder.enabled=true` 时，每次 Runner.Run 产生一个 Trace 目录
2. Trace 目录包含 `meta.json`（元数据）和 `events.jsonl`（事件流）
3. LLM 请求和响应被完整记录
4. `safe` 模式下 API Key 等敏感信息被脱敏
5. 录制失败不阻塞主流程
6. `make wire && make build && make test` 全部通过

---

## 二、设计文档

### 2.1 框架参考

#### Recorder 结构体 — `pkg/trpc-agent-go/openclaw/internal/debugrecorder/recorder.go`

```go
type Recorder struct {
    dir  string
    mode Mode
}

type Mode string

const (
    ModeFull Mode = "full"
    ModeSafe Mode = "safe"
)

func New(dir string, mode Mode) (*Recorder, error)
```

#### Trace 结构体

```go
type Trace struct {
    dir       string
    metaFile  *os.File
    eventFile *os.File
    mu        sync.Mutex
}

type TraceStart struct {
    SessionID string
    UserID    string
    AgentName string
    Input     string
    Timestamp time.Time
}

type TraceEnd struct {
    Output   string
    Error    string
    Duration time.Duration
}

func (r *Recorder) Start(start TraceStart) (*Trace, error)
func (t *Trace) Record(kind string, payload any) error
func (t *Trace) RecordText(text string) error
func (t *Trace) RecordError(err error) error
func (t *Trace) StoreBlob(name string, data []byte) error
func (t *Trace) Close(end TraceEnd) error
```

#### Context 传播

```go
func WithTrace(ctx context.Context, trace *Trace) context.Context
func TraceFromContext(ctx context.Context) *Trace
func WithRecorder(ctx context.Context, recorder *Recorder) context.Context
func RecorderFromContext(ctx context.Context) *Recorder
```

#### 事件类型常量

```go
const (
    KindTraceStart  = "trace_start"
    KindTraceEnd    = "trace_end"
    KindText        = "text"
    KindError       = "error"
    KindGatewayReq  = "gateway_req"
    KindGatewayRsp  = "gateway_rsp"
    KindModelReq    = "model_req"
    KindRunnerEvent = "runner_event"
)
```

#### RecordModelRequest

```go
func RecordModelRequest(ctx context.Context, req ModelRequest) error
```

在 Context 中查找 Trace，如果存在则记录 `KindModelReq` 事件。

### 2.2 当前项目现状

当前项目没有调试录制功能。Runner 执行流程中无任何录制点。

关键集成点：

| 位置 | 当前行为 | 需要添加 |
|------|----------|----------|
| `internal/service/chat.go` | Runner.Run 执行 | 创建 Trace，注入 Context |
| `internal/agent/` | Agent 构建 | 无需修改（通过 Context 传播） |
| `internal/provider/` | LLM 调用 | 调用 `RecordModelRequest` |
| `internal/event/` | FlowLog 记录 | 可选：同时记录到 Trace |

### 2.3 架构设计

#### 2.3.1 整体架构

```
config.yaml
  └─ debug_recorder.enabled: true/false
  └─ debug_recorder.dir: /data/debug
  └─ debug_recorder.mode: full/safe
       │
       ▼
internal/debug/recorder.go  ← 新增：Recorder 工厂 + 生命周期管理
  └─ NewRecorder(cfg) → *debugrecorder.Recorder
       │
       ▼
internal/service/chat.go  ← 修改：Runner.Run 前后创建/关闭 Trace
  ├─ trace := recorder.Start(...)
  ├─ ctx = debugrecorder.WithTrace(ctx, trace)
  ├─ ctx = debugrecorder.WithRecorder(ctx, recorder)
  ├─ runner.Run(ctx, ...)
  └─ trace.Close(...)

internal/provider/  ← 修改：LLM 调用时记录 ModelRequest
  └─ debugrecorder.RecordModelRequest(ctx, req)
```

#### 2.3.2 配置结构

在 `internal/conf/conf.proto` 中扩展：

```protobuf
message DebugRecorder {
  bool enabled = 1;
  string dir = 2;
  string mode = 3;  // full | safe
  int32 max_trace_size_mb = 4;
  int32 retention_days = 5;
}
```

#### 2.3.3 Recorder 工厂

新增 `internal/debug/recorder.go`：

```go
type RecorderFactory struct {
    cfg *conf.DebugRecorder
}

func NewRecorderFactory(cfg *conf.DebugRecorder) (*RecorderFactory, error) {
    if !cfg.Enabled {
        return nil, nil
    }
    if err := os.MkdirAll(cfg.Dir, 0755); err != nil {
        return nil, kerrors.InternalServer("DEBUG", err.Error())
    }
    return &RecorderFactory{cfg: cfg}, nil
}

func (f *RecorderFactory) NewRecorder() (*debugrecorder.Recorder, error) {
    return debugrecorder.New(f.cfg.Dir, debugrecorder.Mode(f.cfg.Mode))
}
```

#### 2.3.4 Service 层集成

在 `internal/service/chat.go` 的 Runner.Run 调用前后添加录制逻辑：

```go
func (s *ChatService) Run(ctx context.Context, req *v1.RunRequest) (*v1.RunResponse, error) {
    var trace *debugrecorder.Trace
    if s.recorder != nil {
        rec, _ := s.recorder.NewRecorder()
        if rec != nil {
            trace, _ = rec.Start(debugrecorder.TraceStart{
                SessionID: req.SessionId,
                UserID:    req.UserId,
                AgentName: req.AgentName,
                Input:     req.Message,
                Timestamp: time.Now(),
            })
            if trace != nil {
                ctx = debugrecorder.WithTrace(ctx, trace)
                ctx = debugrecorder.WithRecorder(ctx, rec)
            }
        }
    }

    result, err := s.runner.Run(ctx, ...)

    if trace != nil {
        end := debugrecorder.TraceEnd{Duration: time.Since(startTime)}
        if err != nil {
            end.Error = err.Error()
        }
        trace.Close(end)
    }

    return result, err
}
```

#### 2.3.5 Provider 层集成

在 `internal/provider/` 的 LLM 调用中添加 `RecordModelRequest`：

```go
func (p *Provider) Call(ctx context.Context, req model.Request) (model.Response, error) {
    debugrecorder.RecordModelRequest(ctx, debugrecorder.ModelRequest{
        Provider: p.Name(),
        Model:    req.Model,
        Messages: req.Messages,
    })

    resp, err := p.client.Call(ctx, req)

    if trace := debugrecorder.TraceFromContext(ctx); trace != nil {
        trace.Record("model_rsp", map[string]any{
            "model":   req.Model,
            "content": resp.Content,
            "usage":   resp.Usage,
        })
    }

    return resp, err
}
```

#### 2.3.6 Wire 注入

`internal/debug/provider.go`：

```go
var ProviderSet = wire.NewSet(
    NewRecorderFactory,
)
```

`internal/service/service.go` 新增 `RecorderFactory` 依赖。

#### 2.3.7 安全模式

`safe` 模式下的脱敏规则：

| 数据类型 | 脱敏方式 |
|----------|----------|
| API Key | 替换为 `sk-***...***`（保留前3后3字符） |
| Bearer Token | 替换为 `Bearer ***` |
| 用户输入 | 截断到前 100 字符 |
| LLM 响应 | 截断到前 500 字符 |

### 2.4 与框架的集成方式

| 集成点 | 框架包 | 项目适配层 | 说明 |
|--------|--------|-----------|------|
| Recorder 创建 | `debugrecorder.New(dir, mode)` | `internal/debug/recorder.go` | 直接使用框架构造函数 |
| Trace 生命周期 | `Recorder.Start`/`Trace.Close` | `internal/service/chat.go` | Service 层管理 Trace 生命周期 |
| 事件录制 | `Trace.Record`/`RecordText`/`RecordError` | Service/Provider 层 | 在关键节点调用 |
| ModelRequest | `RecordModelRequest` | `internal/provider/` | LLM 调用时记录 |
| Context 传播 | `WithTrace`/`WithRecorder` | Service 层注入 | 框架提供标准 Context 传播 |
| Gzip 压缩 | `Trace.Close` 内部 | 框架自动处理 | 无需额外配置 |

**关键原则**：录制逻辑通过 Context 传播，不修改框架内部代码。所有录制点都在项目适配层（Service/Provider），框架的 `RecordModelRequest` 等辅助函数自动从 Context 获取 Trace。

### 2.5 错误处理

| 场景 | 错误类型 | 处理方式 |
|------|----------|----------|
| Recorder 初始化失败 | `kerrors.InternalServer("DEBUG", ...)` | 启动时 Fail Fast |
| Trace.Start 失败 | FlowLog 记录，跳过录制 | 不影响主流程 |
| Trace.Record 失败 | FlowLog 记录，跳过该事件 | 不影响主流程 |
| Trace.Close 失败 | FlowLog 记录 | 不影响主流程 |
| 目录权限不足 | `kerrors.InternalServer("DEBUG", ...)` | 启动时 Fail Fast |
| 磁盘空间不足 | FlowLog 记录，停止录制 | 不影响主流程 |

**核心原则**：录制是旁路操作，任何录制失败都不影响主业务流程。

---

## 三、开发计划

### 3.1 任务拆解

| # | 任务 | 涉及文件 | 依赖 | 预估 |
|---|------|----------|------|------|
| T1 | 扩展 Proto 配置结构 | `internal/conf/conf.proto` | 无 | 0.5d |
| T2 | 新增 Recorder 工厂 | `internal/debug/recorder.go` | T1 | 1d |
| T3 | 新增 Wire ProviderSet | `internal/debug/provider.go` | T2 | 0.5d |
| T4 | Service 层集成 | `internal/service/chat.go` | T2 | 1d |
| T5 | Provider 层集成 | `internal/provider/` | T2 | 1d |
| T6 | 安全模式脱敏 | `internal/debug/sanitizer.go` | T2 | 0.5d |
| T7 | 集成测试 | `internal/debug/recorder_test.go` | T2-T6 | 1d |
| T8 | `make api && make wire && make build` 验证 | 全局 | T1-T7 | 0.5d |

### 3.2 开发顺序

```
Phase 1 — 基础设施（T1 → T2 → T3）
  ├─ T1: Proto 配置扩展
  ├─ T2: Recorder 工厂
  └─ T3: Wire ProviderSet

Phase 2 — 集成（T4 → T5）
  ├─ T4: Service 层 Trace 生命周期
  └─ T5: Provider 层 ModelRequest 记录

Phase 3 — 安全与验证（T6 → T7 → T8）
  ├─ T6: 安全模式脱敏
  ├─ T7: 集成测试
  └─ T8: 全量构建验证
```

### 3.3 验证方案

| 验证项 | 方法 | 通过标准 |
|--------|------|----------|
| Trace 创建 | `go test ./internal/debug/... -run TestTraceCreate -count=1` | Trace 目录包含 meta.json + events.jsonl |
| 事件录制 | `go test ./internal/debug/... -run TestTraceRecord -count=1` | events.jsonl 包含录制的事件 |
| LLM 请求记录 | `go test ./internal/debug/... -run TestModelRequest -count=1` | KindModelReq 事件被记录 |
| 安全模式 | `go test ./internal/debug/... -run TestSafeMode -count=1` | API Key 被脱敏 |
| Gzip 压缩 | `go test ./internal/debug/... -run TestTraceGzip -count=1` | Close 后 events.jsonl.gz 存在 |
| 录制失败不影响主流程 | `go test ./internal/debug/... -run TestRecorderFailure -count=1` | 录制失败时主流程正常完成 |
| 全量构建 | `make api && make wire && make build && make test` | 零错误 |
