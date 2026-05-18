# CodeExecutor 代码执行模块 — 实现设计文档

> **对应需求**：[32 codeexecutor.md](./32%20codeexecutor.md)
> **开发计划**：[32-codeexecutor-development.md](./32-codeexecutor-development.md)
> **遵循规范**：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

安全代码执行环境：本地子进程、Docker 容器、E2B 云端沙箱、Jupyter 内核。对标 `pkg/trpc-agent-go/codeexecutor` 包。

核心设计原则：

- 项目自定义 `Executor` 接口（`internal/agent/codeexecutor`）负责底层执行，通过适配层转换为框架 `codeexecutor.CodeExecutor` 接口
- 适配层位于 `internal/skill/trpc/executor.go`，将项目执行器适配为框架接口供 Skill 工具使用
- 框架 `codeexecutor` 包提供完整的 Workspace / Interactive / Artifact 生态，项目按需集成
- Agent 级别通过 `AgentRuntimeSettings.CodeExecutorType` 字段配置执行器类型（待实现）

---

## 二、架构方案

### 2.1 当前架构

```
trpc_build.go
  └── buildSkillDeps()
        └── skilltrpc.NewExecutor(backend, workDir)
              ├── backend == "docker"
              │     └── dockerExecutorAdapter
              │           └── internal/agent/codeexecutor.DockerExecutor
              │                 └── docker run --rm (one-shot container)
              └── backend != "docker" (default)
                    └── trpclocal.New() (框架 local 执行器)
                          └── 子进程执行

Skill 工具路径:
  trpcllmagent.WithCodeExecutor(exec) → skill_run 工具使用框架 CodeExecutor 接口

环境变量控制:
  CODE_EXECUTOR_BACKEND = "docker" | "" (默认 local)
  CODE_EXECUTOR_DOCKER_IMAGE = 自定义镜像
  CODE_EXECUTOR_TIMEOUT = 超时时间
```

### 2.2 目标架构

```
trpc_build.go
  └── buildSkillDeps()
        └── ExecutorRegistry.Get(agentSettings.CodeExecutorType)
              ├── "local"    → trpclocal.New() (框架 local)
              ├── "docker"   → dockerExecutorAdapter (项目 DockerExecutor)
              ├── "e2b"      → e2bAdapter (框架 e2b.CodeExecutor)
              ├── "jupyter"  → jupyterAdapter (框架 jupyter.CodeExecutor)
              └── "container"→ containerAdapter (框架 container.CodeExecutor)

配置优先级:
  1. AgentRuntimeSettings.CodeExecutorType (Agent 级别)
  2. CODE_EXECUTOR_BACKEND 环境变量 (全局回退)
  3. "local" (系统默认)

Workspace 生态:
  WorkspaceRegistry → Session 级工作区复用
  WorkspaceFS       → 输入文件准备 + 输出文件收集
  ArtifactService   → 产出物自动持久化

Interactive 生态:
  InteractiveProgramRunner → 多轮交互式执行
  ProgramSession           → 状态保持 + 输入/输出/终止
```

### 2.3 双接口关系

项目存在两层执行器接口，各司其职：

| 接口 | 位置 | 职责 | 消费方 |
|------|------|------|--------|
| `Executor` | `internal/agent/codeexecutor` | 底层代码执行（语言→进程→结果） | Docker 适配器 |
| `codeexecutor.CodeExecutor` | `pkg/trpc-agent-go/codeexecutor` | 框架标准接口（CodeBlock→Output+Files） | Skill 工具、LLMAgent |

适配层 `dockerExecutorAdapter` 将项目 `Executor` 接口转换为框架 `CodeExecutor` 接口：

```
codeexecutor.CodeExecutor.ExecuteCode(input)
  → dockerExecutorAdapter.ExecuteCode()
    → 遍历 input.CodeBlocks
    → DockerExecutor.Run(ctx, block.Language, block.Code, timeout)
    → 拼接 stdout/stderr → CodeExecutionResult.Output
```

---

## 三、接口设计

### 3.1 项目 Executor 接口（已实现）

```go
type Executor interface {
    Run(ctx context.Context, language, code string, timeout time.Duration) (Result, error)
}

type Result struct {
    Stdout      string
    Stderr      string
    ExitCode    int
    TimedOut    bool
    OOM         bool
    ArtifactDir string
}
```

### 3.2 框架 CodeExecutor 接口（pkg/trpc-agent-go）

```go
type CodeExecutor interface {
    ExecuteCode(context.Context, CodeExecutionInput) (CodeExecutionResult, error)
    CodeBlockDelimiter() CodeBlockDelimiter
}

type CodeExecutionInput struct {
    CodeBlocks  []CodeBlock
    ExecutionID string
}

type CodeExecutionResult struct {
    Output      string
    OutputFiles []File
}
```

### 3.3 框架 Workspace 生态接口

```go
type WorkspaceManager interface {
    CreateWorkspace(ctx context.Context, execID string, pol WorkspacePolicy) (Workspace, error)
    Cleanup(ctx context.Context, ws Workspace) error
}

type WorkspaceFS interface {
    PutFiles(ctx context.Context, ws Workspace, files []PutFile) error
    StageDirectory(ctx context.Context, ws Workspace, src, to string, opt StageOptions) error
    Collect(ctx context.Context, ws Workspace, patterns []string) ([]File, error)
    StageInputs(ctx context.Context, ws Workspace, specs []InputSpec) error
    CollectOutputs(ctx context.Context, ws Workspace, spec OutputSpec) (OutputManifest, error)
}

type ProgramRunner interface {
    RunProgram(ctx context.Context, ws Workspace, spec RunProgramSpec) (RunResult, error)
}

type Engine interface {
    Manager() WorkspaceManager
    FS() WorkspaceFS
    Runner() ProgramRunner
    Describe() Capabilities
}

type EngineProvider interface {
    Engine() Engine
}
```

### 3.4 框架 Interactive 接口

```go
type InteractiveProgramRunner interface {
    StartProgram(ctx context.Context, ws Workspace, spec InteractiveProgramSpec) (ProgramSession, error)
}

type ProgramSession interface {
    ID() string
    Poll(limit *int) ProgramPoll
    Log(offset *int, limit *int) ProgramLog
    Write(data string, newline bool) error
    Kill(grace time.Duration) error
    Close() error
}
```

### 3.5 框架 WorkspaceRegistry

```go
type WorkspaceRegistry struct { ... }

func NewWorkspaceRegistry() *WorkspaceRegistry

func (r *WorkspaceRegistry) Acquire(ctx context.Context, m WorkspaceManager, id string) (Workspace, error)
```

### 3.6 框架 Artifact 集成

```go
func WithArtifactService(ctx context.Context, svc artifact.Service) context.Context
func SaveArtifactHelper(ctx context.Context, filename string, data []byte, mime string) (int, error)
func LoadArtifactHelper(ctx context.Context, name string, version *int) ([]byte, string, int, error)
```

### 3.7 框架 Env 注入

```go
type RunEnvProvider func(ctx context.Context) map[string]string

func NewEnvInjectingCodeExecutor(exec CodeExecutor, provider RunEnvProvider) CodeExecutor
```

---

## 四、适配层设计

### 4.1 当前适配层（已实现）

`internal/skill/trpc/executor.go` 提供两个构造函数：

| 函数 | 返回 | 说明 |
|------|------|------|
| `NewLocalExecutor(workDir)` | `codeexecutor.CodeExecutor` | 框架 local 执行器 |
| `NewExecutor(backend, workDir)` | `codeexecutor.CodeExecutor` | 根据 backend 选择执行器 |

`dockerExecutorAdapter` 适配逻辑：

- `CodeBlockDelimiter()` → 返回标准 markdown 三反引号分隔符
- `ExecuteCode()` → 遍历 CodeBlocks，逐个调用 `DockerExecutor.Run()`，拼接输出

### 4.2 目标适配层

新增适配器将框架执行器统一注册到 `ExecutorRegistry`：

| 适配器 | 框架执行器 | 适配方式 |
|--------|-----------|----------|
| `dockerExecutorAdapter` | 项目 `DockerExecutor` | 已实现，直接复用 |
| `e2bAdapter` | 框架 `e2b.CodeExecutor` | 直接使用，无需适配（已实现 `CodeExecutor` 接口） |
| `jupyterAdapter` | 框架 `jupyter.CodeExecutor` | 直接使用，无需适配 |
| `containerAdapter` | 框架 `container.CodeExecutor` | 直接使用，无需适配 |

`ExecutorRegistry` 设计：

```go
type ExecutorRegistry struct {
    entries map[string]codeexecutor.CodeExecutor
    wsReg   *codeexecutor.WorkspaceRegistry
}

func (r *ExecutorRegistry) Register(typ string, exec codeexecutor.CodeExecutor)
func (r *ExecutorRegistry) Get(typ string) (codeexecutor.CodeExecutor, bool)
```

---

## 五、配置设计

### 5.1 Agent 级别配置

在 `AgentRuntimeSettings` 中新增字段：

```
CodeExecutorType string  // "local"/"docker"/"e2b"/"jupyter"/"container"，默认 "local"
```

在 `agent_runtime_settings` Ent Schema 中新增：

```
field.String("code_executor_type").Default("local")
```

在 `agent_runtime_settings` 表中新增列：

```sql
ALTER TABLE agent_runtime_settings ADD COLUMN code_executor_type TEXT NOT NULL DEFAULT 'local';
```

### 5.2 Docker 后端配置

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `Image` | `python:3.11-slim` | 执行镜像 |
| `Network` | `none` | 网络模式 |
| `CPUQuota` | `50000` | CPU 配额（微秒/周期，50%） |
| `MemoryBytes` | `268435456` | 内存限制（256 MiB） |
| `TmpSize` | `128m` | /tmp tmpfs 大小 |
| `PullPolicy` | `missing` | 镜像拉取策略 |
| `WorkspaceMount` | `""` | 宿主工作区只读挂载路径 |

环境变量覆盖：

| 环境变量 | 说明 |
|----------|------|
| `CODE_EXECUTOR_BACKEND` | 全局执行器后端选择 |
| `CODE_EXECUTOR_DOCKER_IMAGE` | Docker 镜像覆盖 |
| `CODE_EXECUTOR_TIMEOUT` | 执行超时覆盖 |

### 5.3 E2B 后端配置

| 配置项 | 环境变量 | 说明 |
|--------|----------|------|
| API Key | `E2B_API_KEY` | E2B API 密钥 |
| Domain | — | E2B 域名（默认 `e2b.app`） |
| Template | — | 沙箱模板（默认 `code-interpreter-v1`） |
| Sandbox Timeout | — | 沙箱生命周期 |
| Execution Timeout | — | 单次代码执行超时 |

### 5.4 Jupyter 后端配置

| 配置项 | 说明 |
|--------|------|
| IP | Jupyter 服务器地址 |
| Port | Jupyter 服务器端口 |
| Token | 认证令牌 |
| KernelName | 内核名称 |
| StartTimeout | 启动超时 |

### 5.5 Container 后端配置

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| Image | `python:3.9-slim` | Docker 镜像 |
| NetworkMode | `none` | 网络模式 |
| AutoRemove | `true` | 自动移除容器 |
| Privileged | `false` | 非特权模式 |

---

## 六、安全设计

### 6.1 Docker 后端安全控制

| 控制 | 实现方式 |
|------|----------|
| 无网络 | `--network none` |
| 只读根文件系统 | `--read-only` |
| 内存上限 | `--memory` + `--memory-swap`（相等 → 禁用 swap） |
| CPU 上限 | `--cpus=0.5` |
| 临时容器 | `--rm` — 运行后移除容器 |
| 超时 | `context.WithTimeout` + `--stop-timeout` |
| 临时文件系统 | `--tmpfs /tmp:size=128m` |

### 6.2 回退策略

Docker 不可用时回退到 `LocalExecutor` 并发出告警日志。未来可扩展 `firecracker` 或 `nsjail` 轻量 VM 后端。

---

## 七、可观测性设计

### 7.1 Prometheus 指标

| 指标 | 标签 | 说明 |
|------|------|------|
| `aranea_codeexec_runs_total` | `kind`, `status` | 总执行次数（success/error/timeout/oom） |
| `aranea_codeexec_duration_seconds` | `kind` | 执行时长直方图 |
| `aranea_codeexec_oom_total` | `kind` | OOM kill 计数 |

### 7.2 框架 Engine 能力描述

`EngineProvider.Engine().Describe()` 返回 `Capabilities`：

| 字段 | 说明 |
|------|------|
| `Isolation` | 隔离级别描述 |
| `NetworkAllowed` | 是否允许网络 |
| `ReadOnlyMount` | 是否支持只读挂载 |
| `Streaming` | 是否支持流式输出 |
| `MaxDiskBytes` | 最大磁盘使用 |

---

## 八、数据模型

### 8.1 已有模型

`internal/agent/codeexecutor/executor.go` 中的类型：

| 类型 | 用途 |
|------|------|
| `Result` | 执行结果（Stdout/Stderr/ExitCode/TimedOut/OOM/ArtifactDir） |
| `LocalConfig` | 本地执行器配置 |
| `DockerConfig` | Docker 执行器配置 |

### 8.2 待新增模型

#### AgentRuntimeSettings 扩展

在 `internal/biz/agent_types.go` 的 `AgentRuntimeSettings` 中新增：

```
CodeExecutorType string  // 执行器类型选择
```

在 `internal/biz/agent_defaults.go` 的 `DefaultAgentRuntimeSettings()` 中新增：

```
CodeExecutorType: "local",
```

#### Ent Schema 扩展

在 `internal/data/ent/schema/agent_runtime_setting.go` 的 Fields 中新增：

```
field.String("code_executor_type").Default("local")
```

#### SQL 迁移

```sql
ALTER TABLE agent_runtime_settings ADD COLUMN code_executor_type TEXT NOT NULL DEFAULT 'local';
```

---

## 九、涉及文件

| 文件 | 状态 | 说明 |
|------|------|------|
| `internal/agent/codeexecutor/executor.go` | ✅ 已实现 | 项目 Executor 接口 + Local/Docker 实现 |
| `internal/agent/codeexecutor/executor_test.go` | ✅ 已实现 | 基础测试 |
| `internal/skill/trpc/executor.go` | ✅ 已实现 | 适配层：项目 Executor → 框架 CodeExecutor |
| `internal/skill/trpc/tools.go` | ✅ 已实现 | Skill 工具集构建 |
| `internal/agent/trpc_build.go` | 🟡 需修改 | 接入 ExecutorRegistry + Agent 配置 |
| `internal/biz/agent_types.go` | ❌ 待新增 | CodeExecutorType 字段 |
| `internal/biz/agent_defaults.go` | ❌ 待新增 | CodeExecutorType 默认值 |
| `internal/data/ent/schema/agent_runtime_setting.go` | ❌ 待新增 | code_executor_type 列 |
| `internal/agent/codeexecutor/registry.go` | ❌ 待新建 | ExecutorRegistry |
| `internal/agent/codeexecutor/e2b_adapter.go` | ❌ 待新建 | E2B 执行器适配 |
| `internal/agent/codeexecutor/jupyter_adapter.go` | ❌ 待新建 | Jupyter 执行器适配 |
| `internal/agent/codeexecutor/container_adapter.go` | ❌ 待新建 | Container 执行器适配 |
