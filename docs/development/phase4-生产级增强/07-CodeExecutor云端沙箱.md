# 07-CodeExecutor 云端沙箱

## 一、需求文档

### 1.1 背景

当前 Aranea-Agents 的代码执行能力在 `internal/agent/codeexecutor/` 中实现，支持三种后端：

| 后端 | 文件 | 特性 |
|------|------|------|
| `LocalExecutor` | `executor.go` | 本地子进程执行，无隔离 |
| `DockerExecutor` | `docker_adapter.go` | 一次性容器执行，资源限制 |
| E2B（部分集成） | `factory.go` | 已有 `e2bexec.New` 调用，但未完整集成 |

当前存在的问题：

- **E2B 集成不完整**：`Factory` 中已有 E2B 初始化代码，但未与框架 `codeexecutor/e2b` 完整对齐
- **缺乏 Jupyter 支持**：无法使用 Jupyter Kernel Gateway 执行代码
- **缺乏 Workspace 操作**：无法在沙箱中进行文件上传/下载
- **缺乏框架标准接口**：未使用框架 `codeexecutor.CodeExecutor` 标准接口

框架 `pkg/trpc-agent-go/codeexecutor/` 已提供完整的代码执行标准接口和两种云端沙箱实现：

| 组件 | 文件路径 | 核心接口/结构 |
|------|----------|--------------|
| CodeExecutor 接口 | `codeexecutor/codeexecutor.go` | `ExecuteCode` + `CodeBlockDelimiter` |
| E2B 实现 | `codeexecutor/e2b/e2b.go` | 云端沙箱，支持 Workspace 操作 |
| Jupyter 实现 | `codeexecutor/jupyter/jupyter.go` | Jupyter Kernel Gateway 子进程 |

### 1.2 目标

1. **框架接口对齐**：将现有 `Executor` 接口迁移到框架 `codeexecutor.CodeExecutor` 标准接口
2. **E2B 完整集成**：使用框架 `codeexecutor/e2b` 包，完整支持云端沙箱执行
3. **Jupyter 集成**：使用框架 `codeexecutor/jupyter` 包，支持 Jupyter Kernel Gateway
4. **Workspace 操作**：支持沙箱内的文件上传/下载/目录操作
5. **统一工厂**：重构 `Factory`，统一 Local/Docker/E2B/Jupyter 四种后端

### 1.3 功能需求

#### P0 — 必须实现

| ID | 需求 | 说明 |
|----|------|------|
| C-P0-1 | 框架 CodeExecutor 接口适配 | 将现有 `Executor` 接口适配为 `codeexecutor.CodeExecutor` |
| C-P0-2 | E2B 完整集成 | 使用框架 `e2b.New(opts...)` 创建 E2B CodeExecutor |
| C-P0-3 | 配置驱动后端选择 | 通过 `config.yaml` 的 `code_executor.backend` 选择 local/docker/e2b/jupyter |
| C-P0-4 | Factory 重构 | 统一 Local/Docker/E2B/Jupyter 四种后端的创建和选择 |

#### P1 — 应该实现

| ID | 需求 | 说明 |
|----|------|----------|
| C-P1-1 | Jupyter 集成 | 使用框架 `jupyter.New(opts...)` 创建 Jupyter CodeExecutor |
| C-P1-2 | Workspace 操作 | 通过 E2B 的 Workspace 方法支持文件操作 |
| C-P1-3 | CodeBlockDelimiter | 实现框架 `CodeBlockDelimiter()` 方法，支持代码块分隔 |
| C-P1-4 | 可用性探测 | 启动时探测 E2B/Jupyter 可用性，不可用时自动降级 |

#### P2 — 可以实现

| ID | 需求 | 说明 |
|----|------|----------|
| C-P2-1 | 沙箱复用 | E2B 沙箱跨请求复用，减少启动延迟 |
| C-P2-2 | 执行超时配置 | 按语言配置不同的执行超时 |
| C-P2-3 | 执行指标 | 执行次数、延迟、成功率通过 Prometheus 暴露 |

### 1.4 非功能需求

| 维度 | 要求 |
|------|------|
| 性能 | E2B 沙箱启动 < 3s，代码执行延迟取决于沙箱 |
| 安全 | E2B/Jupyter 沙箱完全隔离，不影响宿主机 |
| 可靠性 | E2B 不可用时自动降级到 Docker/Local |
| 兼容性 | 现有 `Executor` 接口调用方不受影响 |
| 可观测性 | 执行次数、延迟、错误通过 FlowLog 记录 |

### 1.5 验收标准

1. `code_executor.backend=e2b` 配置下，代码在 E2B 云端沙箱中执行
2. `code_executor.backend=jupyter` 配置下，代码在 Jupyter Kernel Gateway 中执行
3. E2B/Jupyter 不可用时自动降级到 Docker/Local
4. `CodeBlockDelimiter()` 返回正确的代码块分隔符
5. 现有代码执行功能不受影响
6. `make wire && make build && make test` 全部通过

---

## 二、设计文档

### 2.1 框架参考

#### CodeExecutor 接口 — `pkg/trpc-agent-go/codeexecutor/codeexecutor.go`

```go
type CodeExecutor interface {
    ExecuteCode(ctx context.Context, input CodeExecutionInput) (CodeExecutionResult, error)
    CodeBlockDelimiter() CodeBlockDelimiter
}
```

#### 核心类型

```go
type CodeExecutionInput struct {
    Code       string
    Language   string
    Files      []File
    Timeout    time.Duration
    WorkDir    string
}

type CodeExecutionResult struct {
    Stdout     string
    Stderr     string
    ExitCode   int
    Error      string
    Files      []File
    CodeBlocks []CodeBlock
}

type CodeBlock struct {
    Code     string
    Language string
}

type CodeBlockDelimiter struct {
    Open  string
    Close string
}

type File struct {
    Name    string
    Content []byte
}
```

#### E2B 实现 — `pkg/trpc-agent-go/codeexecutor/e2b/e2b.go`

```go
type CodeExecutor struct { ... }

func New(opts ...Option) (*CodeExecutor, error)
```

选项：

```go
func WithAPIKey(key string) Option
func WithAccessToken(token string) Option
func WithDomain(domain string) Option
func WithDebug(debug bool) Option
func WithTemplate(template string) Option
func WithSandboxTimeout(timeout time.Duration) Option
func WithRequestTimeout(timeout time.Duration) Option
func WithExecutionTimeout(timeout time.Duration) Option
func WithEnvVars(vars map[string]string) Option
func WithMetadata(metadata map[string]string) Option
func WithHTTPClient(client *http.Client) Option
func WithHeaders(headers map[string]string) Option
func WithSandboxID(id string) Option
func WithLanguage(lang string) Option
func WithSandboxRunBase(base string) Option
```

Workspace 操作（E2B 额外实现）：

```go
func (e *CodeExecutor) CreateWorkspace(ctx context.Context) (string, error)
func (e *CodeExecutor) Cleanup(ctx context.Context, sandboxID string) error
func (e *CodeExecutor) PutFiles(ctx context.Context, sandboxID string, files []File) error
func (e *CodeExecutor) PutDirectory(ctx context.Context, sandboxID string, src, dst string) error
func (e *CodeExecutor) RunProgram(ctx context.Context, sandboxID, cmd string) (string, error)
func (e *CodeExecutor) Collect(ctx context.Context, sandboxID, path string) ([]byte, error)
func (e *CodeExecutor) StageInputs(ctx context.Context, sandboxID string, files []File) error
func (e *CodeExecutor) CollectOutputs(ctx context.Context, sandboxID string) ([]File, error)
func (e *CodeExecutor) ExecuteInline(ctx context.Context, sandboxID, code string) (string, error)
func (e *CodeExecutor) Engine() string
```

#### Jupyter 实现 — `pkg/trpc-agent-go/codeexecutor/jupyter/jupyter.go`

```go
type CodeExecutor struct { ... }

func New(opts ...Option) (*CodeExecutor, error)
```

选项：

```go
func WithIP(ip string) Option
func WithPort(port int) Option
func WithToken(token string) Option
func WithKernelName(name string) Option
func WithLogFile(path string) Option
func WithLogLevel(level string) Option
func WithStartTimeout(timeout time.Duration) Option
func WithWaitReadyTimeout(timeout time.Duration) Option
```

Jupyter CodeExecutor 启动 `jupyter kernelgateway` 子进程，通过 HTTP API 与内核通信执行代码。

### 2.2 当前项目现状

当前代码执行在 `internal/agent/codeexecutor/` 目录下：

| 文件 | 职责 |
|------|------|
| `executor.go` | `Executor` 接口 + `LocalExecutor` + `DockerExecutor` + `Result` 类型 |
| `factory.go` | `Factory` 结构体，按可用性选择后端 |
| `docker_adapter.go` | Docker 容器执行适配 |
| `docker_fallback.go` | Docker 不可用时的降级逻辑 |
| `docker_probe.go` | Docker 可用性探测 |
| `capabilities.go` | 执行能力声明 |
| `config.go` | 执行器配置 |
| `language.go` | 语言支持定义 |
| `metrics_executor.go` | 指标装饰器 |
| `output_files.go` | 输出文件处理 |

当前 `Executor` 接口：

```go
type Executor interface {
    Run(ctx context.Context, language, code string, timeout time.Duration) (Result, error)
}
```

当前 `Result` 类型：

```go
type Result struct {
    Stdout      string
    Stderr      string
    ExitCode    int
    TimedOut    bool
    OOM         bool
    ArtifactDir string
}
```

当前 `Factory` 已有 E2B 初始化代码：

```go
e2bexec.New(e2bexec.WithAPIKey(apiKey))
```

但未与框架 `codeexecutor.CodeExecutor` 接口对齐。

### 2.3 架构设计

#### 2.3.1 整体架构

```
config.yaml
  └─ code_executor.backend: local | docker | e2b | jupyter
       │
       ▼
internal/agent/codeexecutor/factory.go  ← 重构：统一四后端
  ├─ localFactory    → LocalExecutor (现有)
  ├─ dockerFactory   → DockerExecutor (现有)
  ├─ e2bFactory      → e2b.New(...) → codeexecutor.CodeExecutor (新增)
  └─ jupyterFactory  → jupyter.New(...) → codeexecutor.CodeExecutor (新增)
       │
       ▼
internal/agent/codeexecutor/adapter.go  ← 新增：接口适配
  └─ codeExecutorAdapter: Executor → codeexecutor.CodeExecutor
  └─ executorAdapter: codeexecutor.CodeExecutor → Executor
```

#### 2.3.2 配置结构

在 `internal/conf/conf.proto` 中扩展：

```protobuf
message CodeExecutor {
  string backend = 1;  // local | docker | e2b | jupyter
  E2BExecutor e2b = 2;
  JupyterExecutor jupyter = 3;
  DockerExecutor docker = 4;
  int64 default_timeout_ms = 5;
}

message E2BExecutor {
  string api_key = 1;
  string access_token = 2;
  string domain = 3;
  string template = 4;
  int64 sandbox_timeout_ms = 5;
  int64 execution_timeout_ms = 6;
  map<string, string> env_vars = 7;
}

message JupyterExecutor {
  string ip = 1;
  int32 port = 2;
  string token = 3;
  string kernel_name = 4;
  int64 start_timeout_ms = 5;
}
```

#### 2.3.3 接口适配

新增 `internal/agent/codeexecutor/adapter.go`：

```go
type codeExecutorAdapter struct {
    executor codeexecutor.CodeExecutor
}

func (a *codeExecutorAdapter) Run(ctx context.Context, language, code string, timeout time.Duration) (Result, error) {
    input := codeexecutor.CodeExecutionInput{
        Code:     code,
        Language: language,
        Timeout:  timeout,
    }
    result, err := a.executor.ExecuteCode(ctx, input)
    if err != nil {
        return Result{}, err
    }
    return Result{
        Stdout:   result.Stdout,
        Stderr:   result.Stderr,
        ExitCode: result.ExitCode,
    }, nil
}

type executorAdapter struct {
    executor Executor
    delimiter codeexecutor.CodeBlockDelimiter
}

func (a *executorAdapter) ExecuteCode(ctx context.Context, input codeexecutor.CodeExecutionInput) (codeexecutor.CodeExecutionResult, error) {
    result, err := a.executor.Run(ctx, input.Language, input.Code, input.Timeout)
    if err != nil {
        return codeexecutor.CodeExecutionResult{}, err
    }
    return codeexecutor.CodeExecutionResult{
        Stdout:   result.Stdout,
        Stderr:   result.Stderr,
        ExitCode: result.ExitCode,
    }, nil
}

func (a *executorAdapter) CodeBlockDelimiter() codeexecutor.CodeBlockDelimiter {
    return a.delimiter
}
```

#### 2.3.4 Factory 重构

重构 `internal/agent/codeexecutor/factory.go`：

```go
type Factory struct {
    cfg        *conf.CodeExecutor
    local      Executor
    docker     Executor
    e2b        codeexecutor.CodeExecutor
    jupyter    codeexecutor.CodeExecutor
}

func (f *Factory) Resolve(ctx context.Context, agentType, workDir string) (Executor, error) {
    switch f.cfg.Backend {
    case "e2b":
        if f.e2b != nil {
            return &codeExecutorAdapter{executor: f.e2b}, nil
        }
        return f.fallbackToDocker(ctx, workDir)
    case "jupyter":
        if f.jupyter != nil {
            return &codeExecutorAdapter{executor: f.jupyter}, nil
        }
        return f.fallbackToLocal(workDir)
    case "docker":
        return f.resolveDocker(ctx, workDir)
    default:
        return f.resolveLocal(workDir)
    }
}

func (f *Factory) ResolveCodeExecutor(ctx context.Context) (codeexecutor.CodeExecutor, error) {
    switch f.cfg.Backend {
    case "e2b":
        return f.e2b, nil
    case "jupyter":
        return f.jupyter, nil
    default:
        return &executorAdapter{executor: f.local, delimiter: defaultDelimiter()}, nil
    }
}
```

#### 2.3.5 E2B 初始化

```go
func newE2BExecutor(cfg *conf.E2BExecutor) (codeexecutor.CodeExecutor, error) {
    opts := []e2b.Option{
        e2b.WithAPIKey(cfg.ApiKey),
    }
    if cfg.AccessToken != "" {
        opts = append(opts, e2b.WithAccessToken(cfg.AccessToken))
    }
    if cfg.Domain != "" {
        opts = append(opts, e2b.WithDomain(cfg.Domain))
    }
    if cfg.Template != "" {
        opts = append(opts, e2b.WithTemplate(cfg.Template))
    }
    if cfg.SandboxTimeoutMs > 0 {
        opts = append(opts, e2b.WithSandboxTimeout(time.Duration(cfg.SandboxTimeoutMs)*time.Millisecond))
    }
    if cfg.ExecutionTimeoutMs > 0 {
        opts = append(opts, e2b.WithExecutionTimeout(time.Duration(cfg.ExecutionTimeoutMs)*time.Millisecond))
    }
    if len(cfg.EnvVars) > 0 {
        opts = append(opts, e2b.WithEnvVars(cfg.EnvVars))
    }
    return e2b.New(opts...)
}
```

#### 2.3.6 Jupyter 初始化

```go
func newJupyterExecutor(cfg *conf.JupyterExecutor) (codeexecutor.CodeExecutor, error) {
    opts := []jupyter.Option{}
    if cfg.Ip != "" {
        opts = append(opts, jupyter.WithIP(cfg.Ip))
    }
    if cfg.Port > 0 {
        opts = append(opts, jupyter.WithPort(int(cfg.Port)))
    }
    if cfg.Token != "" {
        opts = append(opts, jupyter.WithToken(cfg.Token))
    }
    if cfg.KernelName != "" {
        opts = append(opts, jupyter.WithKernelName(cfg.KernelName))
    }
    if cfg.StartTimeoutMs > 0 {
        opts = append(opts, jupyter.WithStartTimeout(time.Duration(cfg.StartTimeoutMs)*time.Millisecond))
    }
    return jupyter.New(opts...)
}
```

#### 2.3.7 Wire 注入

`internal/agent/codeexecutor/provider.go`：

```go
var ProviderSet = wire.NewSet(
    NewFactory,
)
```

`NewFactory` 根据配置创建对应的执行器实例。

### 2.4 与框架的集成方式

| 集成点 | 框架包 | 项目适配层 | 说明 |
|--------|--------|-----------|------|
| CodeExecutor 接口 | `codeexecutor.CodeExecutor` | `adapter.go` | 双向适配：框架接口 ↔ 项目接口 |
| E2B 执行器 | `codeexecutor/e2b` | `factory.go` | 直接使用 `e2b.New(opts...)` |
| Jupyter 执行器 | `codeexecutor/jupyter` | `factory.go` | 直接使用 `jupyter.New(opts...)` |
| CodeBlockDelimiter | `codeexecutor.CodeBlockDelimiter` | `adapter.go` | 适配器实现 `CodeBlockDelimiter()` |
| Workspace 操作 | `e2b.CodeExecutor` 扩展方法 | P1 阶段 | 文件上传/下载/目录操作 |
| 执行输入/输出 | `CodeExecutionInput`/`CodeExecutionResult` | `adapter.go` | 类型转换 |

**关键原则**：框架的 E2B/Jupyter 实现直接作为后端使用，项目通过适配器桥接框架 `CodeExecutor` 接口和现有 `Executor` 接口。不复制框架的沙箱管理、内核通信等内部逻辑。

### 2.5 错误处理

| 场景 | 错误类型 | 处理方式 |
|------|----------|----------|
| E2B API Key 无效 | `kerrors.InternalServer("CODE_EXECUTOR", ...)` | 启动时 Fail Fast |
| E2B 沙箱启动失败 | 降级到 Docker/Local | FlowLog 记录降级事件 |
| Jupyter 进程启动失败 | 降级到 Local | FlowLog 记录降级事件 |
| 代码执行超时 | `Result{TimedOut: true}` | 返回超时结果 |
| 代码执行 OOM | `Result{OOM: true}` | 返回 OOM 结果 |
| Docker 不可用 | 降级到 Local | FlowLog 记录降级事件 |
| Workspace 操作失败 | `kerrors.InternalServer("CODE_EXECUTOR", ...)` | 返回错误 |

---

## 三、开发计划

### 3.1 任务拆解

| # | 任务 | 涉及文件 | 依赖 | 预估 |
|---|------|----------|------|------|
| T1 | 扩展 Proto 配置结构 | `internal/conf/conf.proto` | 无 | 0.5d |
| T2 | 新增接口适配器 | `internal/agent/codeexecutor/adapter.go` | 无 | 1d |
| T3 | E2B 执行器初始化 | `internal/agent/codeexecutor/e2b_factory.go` | T1 | 1d |
| T4 | Jupyter 执行器初始化 | `internal/agent/codeexecutor/jupyter_factory.go` | T1 | 1d |
| T5 | Factory 重构 | `internal/agent/codeexecutor/factory.go` | T2, T3, T4 | 1.5d |
| T6 | 可用性探测 | `internal/agent/codeexecutor/e2b_probe.go`, `jupyter_probe.go` | T3, T4 | 0.5d |
| T7 | Wire ProviderSet 适配 | `internal/agent/codeexecutor/provider.go` | T5 | 0.5d |
| T8 | 集成测试 | `internal/agent/codeexecutor/e2b_test.go`, `jupyter_test.go` | T3-T7 | 1d |
| T9 | `make api && make wire && make build` 验证 | 全局 | T1-T8 | 0.5d |

### 3.2 开发顺序

```
Phase 1 — 基础设施（T1 → T2）
  ├─ T1: Proto 配置扩展
  └─ T2: 接口适配器

Phase 2 — 执行器实现（T3 → T4 → T6）
  ├─ T3: E2B 执行器初始化
  ├─ T4: Jupyter 执行器初始化
  └─ T6: 可用性探测

Phase 3 — 工厂重构（T5 → T7）
  ├─ T5: Factory 统一四后端
  └─ T7: Wire ProviderSet 适配

Phase 4 — 验证（T8 → T9）
  ├─ T8: 集成测试
  └─ T9: 全量构建验证
```

### 3.3 验证方案

| 验证项 | 方法 | 通过标准 |
|--------|------|----------|
| E2B 代码执行 | `go test ./internal/agent/codeexecutor/... -run TestE2BExecute -count=1` | 代码在 E2B 沙箱中执行并返回结果 |
| Jupyter 代码执行 | `go test ./internal/agent/codeexecutor/... -run TestJupyterExecute -count=1` | 代码在 Jupyter 内核中执行并返回结果 |
| 接口适配 | `go test ./internal/agent/codeexecutor/... -run TestAdapter -count=1` | 双向适配正确转换类型 |
| 降级逻辑 | `go test ./internal/agent/codeexecutor/... -run TestFallback -count=1` | E2B 不可用时降级到 Docker |
| CodeBlockDelimiter | `go test ./internal/agent/codeexecutor/... -run TestDelimiter -count=1` | 返回正确的分隔符 |
| 现有功能回归 | `go test ./internal/agent/codeexecutor/... -count=1` | Local/Docker 执行器不受影响 |
| 全量构建 | `make api && make wire && make build && make test` | 零错误 |
