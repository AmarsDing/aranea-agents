# CodeExecutor 代码执行模块 — 实现设计文档

> 对应需求：`32 codeexecutor.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

安全代码执行环境：本地执行、E2B 沙箱、Jupyter 内核、Container 隔离。对标 trpc-agent-go `codeexecutor` 包。

---

## 二、Proto 层

### 2.1 待新增

```protobuf
service CodeExecutorService {
  rpc ExecuteCode(ExecuteCodeRequest) returns (ExecuteCodeResponse) {
    option (google.api.http) = { post: "/v1/code-executor/execute" body: "*" };
  }
  rpc ListExecutors(ListExecutorsRequest) returns (ListExecutorsResponse) {
    option (google.api.http) = { get: "/v1/code-executor/executors" };
  }
  rpc GetExecutorConfig(GetExecutorConfigRequest) returns (ExecutorConfig) {
    option (google.api.http) = { get: "/v1/code-executor/executors/{id}" };
  }
  rpc UpdateExecutorConfig(UpdateExecutorConfigRequest) returns (ExecutorConfig) {
    option (google.api.http) = { patch: "/v1/code-executor/executors/{id}" body: "*" };
  }
}
```

---

## 三、Biz 层

### 3.1 领域模型

```go
type CodeExecution struct {
    ID          string
    SessionID   string
    AgentID     string
    Code        string
    Language    string  // "python"/"javascript"/"go"
    ExecutorType string // "local"/"e2b"/"jupyter"/"container"
    Stdout      string
    Stderr      string
    ExitCode    int32
    Artifacts   []string
    DurationMs  int32
    Status      string
    CreatedAt   string
}

type ExecutorConfig struct {
    ID           string
    Type         string  // "local"/"e2b"/"jupyter"/"container"
    ConfigJSON   string
    IsDefault    bool
    Status       string
}
```

### 3.2 Usecase

```go
type CodeExecutorUsecase struct {
    executors map[string]CodeExecutor
}

func (uc *CodeExecutorUsecase) Execute(ctx, req CodeExecution) (CodeExecution, error)
func (uc *CodeExecutorUsecase) ListConfigs(ctx) ([]ExecutorConfig, error)
func (uc *CodeExecutorUsecase) UpdateConfig(ctx, cfg ExecutorConfig) (ExecutorConfig, error)
```

### 3.3 执行器接口

```go
type CodeExecutor interface {
    Type() string
    Execute(ctx, code string, language string, files map[string]string) (*ExecutionResult, error)
    IsAvailable() bool
}

type ExecutionResult struct {
    Stdout    string
    Stderr    string
    ExitCode  int32
    Artifacts map[string][]byte
    DurationMs int32
}
```

---

## 四、Data 层

### 4.1 执行器实现

```go
// internal/codeexecutor/local/executor.go
type LocalExecutor struct {
    timeout time.Duration
    workDir string
}

// internal/codeexecutor/e2b/executor.go
type E2BExecutor struct {
    apiKey  string
    client  *e2b.Client
}

// internal/codeexecutor/jupyter/executor.go
type JupyterExecutor struct {
    baseURL string
    token   string
}

// internal/codeexecutor/container/executor.go
type ContainerExecutor struct {
    dockerClient *docker.Client
    image       string
}
```

### 4.2 WorkspaceRegistry（待实现）

```go
// internal/codeexecutor/workspace.go
type WorkspaceRegistry struct {
    mu        sync.RWMutex
    workspaces map[string]*Workspace
}

func (r *WorkspaceRegistry) GetOrCreate(ctx, sessionID string) (*Workspace, error)
func (r *WorkspaceRegistry) Cleanup(ctx, sessionID string) error
```

---

## 五、Service 层

```go
func (s *CodeExecutorService) ExecuteCode(ctx, req) (*ExecuteCodeResponse, error)
func (s *CodeExecutorService) ListExecutors(ctx, req) (*ListExecutorsResponse, error)
func (s *CodeExecutorService) GetExecutorConfig(ctx, req) (*ExecutorConfig, error)
func (s *CodeExecutorService) UpdateExecutorConfig(ctx, req) (*ExecutorConfig, error)
```

---

## 六、Wire 注入

待新增：
```
data.ProviderSet → NewCodeExecutorRepo
biz.ProviderSet → NewCodeExecutorUsecase
service.ProviderSet → NewCodeExecutorService
codeexecutor.ProviderSet → NewLocalExecutor, NewE2BExecutor
```

---

## 七、Web 前端设计

### 7.1 组件

**CodeExecutorConfig.vue**：执行器配置面板（嵌入系统设置）

**CodeResultBlock.vue**：代码执行结果展示（嵌入 Chat 消息）

### 7.2 API

```typescript
export async function executeCode(req: ExecuteCodeRequest): Promise<ExecuteCodeResponse>
export async function listExecutors(): Promise<ExecutorConfig[]>
export async function updateExecutorConfig(id: string, req: UpdateConfigRequest): Promise<ExecutorConfig>
```
