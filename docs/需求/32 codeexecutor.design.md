# CodeExecutor 代码执行模块 — 实现设计文档

> 对应需求：`32 codeexecutor.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

安全代码执行环境：本地执行、E2B 沙箱、Jupyter 内核、Container 隔离。对标 trpc-agent-go `codeexecutor` 包。

核心设计原则：
- 直接复用 `pkg/trpc-agent-go/codeexecutor` 的 `CodeExecutor` 接口，不自定义 Biz 层执行器接口
- 通过适配层将各执行器实例注册到 `CodeExecutorUsecase`，由 Usecase 根据 Agent 配置分发
- `WorkspaceRegistry` 直接使用框架提供的 `codeexecutor.NewWorkspaceRegistry()`
- Agent 级别通过 `AgentRuntimeSettings.CodeExecutorType` 字段配置执行器类型

---

## 二、Proto 层

### 2.1 新增文件：`api/kratos/code_executor/v1/code_executor.proto`

```protobuf
syntax = "proto3";

package api.kratos.code_executor.v1;

option go_package = "aranea-agents/api/kratos/code_executor/v1;v1";

import "google/api/annotations.proto";
import "google/protobuf/empty.proto";

service CodeExecutorService {
  rpc ExecuteCode(ExecuteCodeRequest) returns (ExecuteCodeResponse) {
    option (google.api.http) = { post: "/v1/code-executor/execute" body: "*" };
  }
  rpc ListExecutors(ListExecutorsRequest) returns (ListExecutorsResponse) {
    option (google.api.http) = { get: "/v1/code-executor/executors" };
  }
  rpc GetExecutorStatus(GetExecutorStatusRequest) returns (ExecutorStatus) {
    option (google.api.http) = { get: "/v1/code-executor/executors/{type}/status" };
  }
  rpc UpdateExecutorConfig(UpdateExecutorConfigRequest) returns (ExecutorConfig) {
    option (google.api.http) = { patch: "/v1/code-executor/executors/{type}" body: "*" };
  }
  rpc ListWorkspaces(ListWorkspacesRequest) returns (ListWorkspacesResponse) {
    option (google.api.http) = { get: "/v1/code-executor/workspaces" };
  }
  rpc CleanupWorkspace(CleanupWorkspaceRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/code-executor/workspaces/{session_id}" };
  }
  rpc ListExecutionHistory(ListExecutionHistoryRequest) returns (ListExecutionHistoryResponse) {
    option (google.api.http) = { get: "/v1/code-executor/executions" };
  }
  rpc GetExecution(GetExecutionRequest) returns (CodeExecution) {
    option (google.api.http) = { get: "/v1/code-executor/executions/{id}" };
  }
}

message ExecuteCodeRequest {
  string agent_id = 1;
  string session_id = 2;
  repeated CodeBlock code_blocks = 3;
  string executor_type = 4; // "local"/"e2b"/"jupyter"/"container"，空则用 Agent 配置
  map<string, string> input_files = 5; // filename → content
  repeated InputSpec inputs = 6; // artifact:// host:// 等
  OutputSpec output_spec = 7;
  int32 timeout_seconds = 8;
}

message CodeBlock {
  string code = 1;
  string language = 2; // "python"/"javascript"/"go"/"bash"
}

message InputSpec {
  string from = 1; // artifact://name@version, host://abs/path, workspace://rel/path
  string to = 2;
  string mode = 3; // "link"/"copy"
}

message OutputSpec {
  repeated string globs = 1;
  int32 max_files = 2;
  int64 max_file_bytes = 3;
  int64 max_total_bytes = 4;
  bool save = 5;
  string name_template = 6;
  bool inline = 7;
}

message ExecuteCodeResponse {
  string execution_id = 1;
  string output = 2;
  repeated OutputFile output_files = 3;
  string executor_type = 4;
  int32 duration_ms = 5;
  string error = 6;
}

message OutputFile {
  string name = 1;
  string content = 2;
  string mime_type = 3;
  int64 size_bytes = 4;
  bool truncated = 5;
}

message ListExecutorsRequest {}

message ListExecutorsResponse {
  repeated ExecutorConfig executors = 1;
}

message GetExecutorStatusRequest {
  string type = 1;
}

message ExecutorStatus {
  string type = 1;
  bool available = 2;
  string status_message = 3;
  string version = 4;
  map<string, string> capabilities = 5; // isolation, network_allowed, streaming 等
}

message UpdateExecutorConfigRequest {
  string type = 1;
  string config_json = 2;
  bool is_default = 3;
}

message ExecutorConfig {
  string type = 1;
  string config_json = 2;
  bool is_default = 3;
  bool available = 4;
  string status = 5; // "active"/"unavailable"/"misconfigured"
  string updated_at = 6;
}

message ListWorkspacesRequest {
  string agent_id = 1;
  int32 limit = 2;
  int32 offset = 3;
}

message ListWorkspacesResponse {
  repeated WorkspaceInfo workspaces = 1;
  int32 total = 2;
}

message WorkspaceInfo {
  string session_id = 1;
  string path = 2;
  string executor_type = 3;
  string created_at = 4;
  int64 disk_usage_bytes = 5;
}

message CleanupWorkspaceRequest {
  string session_id = 1;
}

message ListExecutionHistoryRequest {
  string agent_id = 1;
  string session_id = 2;
  string status = 3; // "success"/"failure"/"timeout"
  int32 limit = 4;
  int32 offset = 5;
}

message ListExecutionHistoryResponse {
  repeated CodeExecution executions = 1;
  int32 total = 2;
}

message GetExecutionRequest {
  string id = 1;
}

message CodeExecution {
  string id = 1;
  string session_id = 2;
  string agent_id = 3;
  string executor_type = 4;
  repeated CodeBlock code_blocks = 5;
  string stdout = 6;
  string stderr = 7;
  int32 exit_code = 8;
  repeated OutputFile output_files = 9;
  int32 duration_ms = 10;
  string status = 11; // "success"/"failure"/"timeout"/"running"
  string error = 12;
  string created_at = 13;
}
```

---

## 三、Biz 层

### 3.1 领域模型

```go
// internal/biz/code_executor.go

type CodeExecutionRecord struct {
    ID           string
    SessionID    string
    AgentID      string
    ExecutorType string
    CodeBlocks   []CodeBlockEntry
    Stdout       string
    Stderr       string
    ExitCode     int32
    OutputFiles  []OutputFileEntry
    DurationMs   int32
    Status       string
    Error        string
    CreatedAt    string
}

type CodeBlockEntry struct {
    Code     string
    Language string
}

type OutputFileEntry struct {
    Name      string
    Content   string
    MIMEType  string
    SizeBytes int64
    Truncated bool
}

type ExecutorConfigEntry struct {
    Type       string
    ConfigJSON string
    IsDefault  bool
    Available  bool
    Status     string
    UpdatedAt  string
}

type ExecutorStatusInfo struct {
    Type           string
    Available      bool
    StatusMessage  string
    Version        string
    Capabilities   map[string]string
}

type WorkspaceInfoEntry struct {
    SessionID     string
    Path          string
    ExecutorType  string
    CreatedAt     string
    DiskUsageBytes int64
}
```

### 3.2 Usecase

```go
// internal/biz/code_executor.go

type CodeExecutorUsecase struct {
    registry   map[string]codeexecutor.CodeExecutor // key: "local"/"e2b"/"jupyter"/"container"
    wsRegistry *codeexecutor.WorkspaceRegistry
    repo       CodeExecutorRepo
    agents     AgentRepository
}

func NewCodeExecutorUsecase(
    registry map[string]codeexecutor.CodeExecutor,
    wsRegistry *codeexecutor.WorkspaceRegistry,
    repo CodeExecutorRepo,
    agents AgentRepository,
) *CodeExecutorUsecase {
    return &CodeExecutorUsecase{
        registry:   registry,
        wsRegistry: wsRegistry,
        repo:       repo,
        agents:     agents,
    }
}

func (uc *CodeExecutorUsecase) Execute(ctx context.Context, agentID, sessionID string, blocks []CodeBlockEntry, executorType string, inputFiles map[string]string, outputSpec *OutputSpecEntry) (*CodeExecutionRecord, error) {
    if executorType == "" {
        settings, err := uc.agents.GetAgentRuntimeSettings(ctx, agentID)
        if err != nil {
            executorType = "local"
        } else {
            executorType = settings.CodeExecutorType
            if executorType == "" {
                executorType = "local"
            }
        }
    }
    exec, ok := uc.registry[executorType]
    if !ok {
        return nil, errors.BadRequest("CODE_EXECUTOR", "executor not found: %s", executorType)
    }
    var trpcBlocks []codeexecutor.CodeBlock
    for _, b := range blocks {
        trpcBlocks = append(trpcBlocks, codeexecutor.CodeBlock{Code: b.Code, Language: b.Language})
    }
    input := codeexecutor.CodeExecutionInput{
        CodeBlocks:  trpcBlocks,
        ExecutionID: sessionID,
    }
    result, err := exec.ExecuteCode(ctx, input)
    if err != nil {
        rec := &CodeExecutionRecord{
            ID:           uuid.NewString(),
            SessionID:    sessionID,
            AgentID:      agentID,
            ExecutorType: executorType,
            CodeBlocks:   blocks,
            Status:       "failure",
            Error:        err.Error(),
            CreatedAt:    time.Now().Format(time.RFC3339),
        }
        uc.repo.InsertRecord(ctx, rec)
        return rec, err
    }
    var files []OutputFileEntry
    for _, f := range result.OutputFiles {
        files = append(files, OutputFileEntry{
            Name:      f.Name,
            Content:   f.Content,
            MIMEType:  f.MIMEType,
            SizeBytes: f.SizeBytes,
            Truncated: f.Truncated,
        })
    }
    rec := &CodeExecutionRecord{
        ID:           uuid.NewString(),
        SessionID:    sessionID,
        AgentID:      agentID,
        ExecutorType: executorType,
        CodeBlocks:   blocks,
        Stdout:       result.Output,
        OutputFiles:  files,
        Status:       "success",
        CreatedAt:    time.Now().Format(time.RFC3339),
    }
    uc.repo.InsertRecord(ctx, rec)
    return rec, nil
}

func (uc *CodeExecutorUsecase) ListExecutors(ctx context.Context) ([]ExecutorConfigEntry, error) {
    return uc.repo.ListConfigs(ctx)
}

func (uc *CodeExecutorUsecase) GetStatus(ctx context.Context, executorType string) (*ExecutorStatusInfo, error) {
    exec, ok := uc.registry[executorType]
    if !ok {
        return nil, errors.NotFound("CODE_EXECUTOR", "executor not found: %s", executorType)
    }
    info := &ExecutorStatusInfo{
        Type:      executorType,
        Available: true,
    }
    if ep, ok := exec.(codeexecutor.EngineProvider); ok {
        eng := ep.Engine()
        if eng != nil {
            caps := eng.Describe()
            info.Capabilities = map[string]string{
                "isolation":       caps.Isolation,
                "network_allowed": fmt.Sprintf("%v", caps.NetworkAllowed),
                "streaming":       fmt.Sprintf("%v", caps.Streaming),
                "max_disk_bytes":  fmt.Sprintf("%d", caps.MaxDiskBytes),
            }
        }
    }
    return info, nil
}

func (uc *CodeExecutorUsecase) UpdateConfig(ctx context.Context, cfg ExecutorConfigEntry) (ExecutorConfigEntry, error) {
    return uc.repo.UpdateConfig(ctx, cfg)
}

func (uc *CodeExecutorUsecase) ListWorkspaces(ctx context.Context, agentID string, limit, offset int) ([]WorkspaceInfoEntry, int, error) {
    return uc.repo.ListWorkspaces(ctx, agentID, limit, offset)
}

func (uc *CodeExecutorUsecase) CleanupWorkspace(ctx context.Context, sessionID string) error {
    return uc.repo.CleanupWorkspace(ctx, sessionID)
}

func (uc *CodeExecutorUsecase) ListHistory(ctx context.Context, agentID, sessionID, status string, limit, offset int) ([]CodeExecutionRecord, int, error) {
    return uc.repo.ListRecords(ctx, agentID, sessionID, status, limit, offset)
}

func (uc *CodeExecutorUsecase) GetExecution(ctx context.Context, id string) (*CodeExecutionRecord, error) {
    return uc.repo.GetRecord(ctx, id)
}
```

### 3.3 Repo 接口

```go
// internal/biz/code_executor.go

type CodeExecutorRepo interface {
    InsertRecord(ctx context.Context, r *CodeExecutionRecord) error
    GetRecord(ctx context.Context, id string) (*CodeExecutionRecord, error)
    ListRecords(ctx context.Context, agentID, sessionID, status string, limit, offset int) ([]CodeExecutionRecord, int, error)
    ListConfigs(ctx context.Context) ([]ExecutorConfigEntry, error)
    UpdateConfig(ctx context.Context, cfg ExecutorConfigEntry) (ExecutorConfigEntry, error)
    ListWorkspaces(ctx context.Context, agentID string, limit, offset int) ([]WorkspaceInfoEntry, int, error)
    CleanupWorkspace(ctx context.Context, sessionID string) error
}
```

### 3.4 AgentRuntimeSettings 扩展

在 `internal/biz/agent_types.go` 的 `AgentRuntimeSettings` 中新增：

```go
CodeExecutorType string // "local"/"e2b"/"jupyter"/"container"，默认 "local"
```

在 `internal/biz/agent_defaults.go` 的 `DefaultAgentRuntimeSettings()` 中新增：

```go
CodeExecutorType: "local",
```

---

## 四、Data 层

### 4.1 Ent Schema 扩展

在 `internal/data/ent/schema/agent_runtime_setting.go` 的 Fields 中新增：

```go
field.String("code_executor_type").Default("local"),
```

### 4.2 新增 Ent Schema：`internal/data/ent/schema/code_execution.go`

```go
package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/dialect/entsql"
    "entgo.io/ent/schema"
    "entgo.io/ent/schema/field"
    "entgo.io/ent/schema/index"
)

type CodeExecution struct {
    ent.Schema
}

func (CodeExecution) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "code_executions"},
    }
}

func (CodeExecution) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").MaxLen(64).Unique().Immutable(),
        field.String("session_id").MaxLen(256).Optional(),
        field.String("agent_id").MaxLen(256).Optional(),
        field.String("executor_type").MaxLen(32).Default("local"),
        field.Text("code_blocks_json").Default("[]"),
        field.Text("stdout").Default(""),
        field.Text("stderr").Default(""),
        field.Int32("exit_code").Default(0),
        field.Text("output_files_json").Default("[]"),
        field.Int32("duration_ms").Default(0),
        field.String("status").MaxLen(32).Default("pending"),
        field.Text("error").Default(""),
        field.String("created_at").Default(""),
    }
}

func (CodeExecution) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("agent_id"),
        index.Fields("session_id"),
        index.Fields("status"),
        index.Fields("created_at"),
    }
}
```

### 4.3 新增 Ent Schema：`internal/data/ent/schema/executor_config.go`

```go
package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/dialect/entsql"
    "entgo.io/ent/schema"
    "entgo.io/ent/schema/field"
)

type ExecutorConfig struct {
    ent.Schema
}

func (ExecutorConfig) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "executor_configs"},
    }
}

func (ExecutorConfig) Fields() []ent.Field {
    return []ent.Field{
        field.String("type").MaxLen(32).Unique().Immutable(),
        field.Text("config_json").Default("{}"),
        field.Bool("is_default").Default(false),
        field.Bool("available").Default(false),
        field.String("status").MaxLen(32).Default("unconfigured"),
        field.String("updated_at").Default(""),
    }
}
```

### 4.4 Repo 实现：`internal/data/code_executor.go`

```go
package data

import (
    "context"
    "encoding/json"
    "time"

    "aranea-agents/internal/biz"
    "aranea-agents/internal/data/ent/codeexecution"
    "aranea-agents/internal/data/ent/executorconfig"

    "entgo.io/ent/dialect/sql"
)

type codeExecutorRepo struct {
    data *Data
}

func NewCodeExecutorRepo(data *Data) biz.CodeExecutorRepo {
    return &codeExecutorRepo{data: data}
}

func (r *codeExecutorRepo) InsertRecord(ctx context.Context, rec *biz.CodeExecutionRecord) error {
    blocksJSON, _ := json.Marshal(rec.CodeBlocks)
    filesJSON, _ := json.Marshal(rec.OutputFiles)
    _, err := r.data.db.CodeExecution.Create().
        SetID(rec.ID).
        SetSessionID(rec.SessionID).
        SetAgentID(rec.AgentID).
        SetExecutorType(rec.ExecutorType).
        SetCodeBlocksJSON(string(blocksJSON)).
        SetStdout(rec.Stdout).
        SetStderr(rec.Stderr).
        SetExitCode(rec.ExitCode).
        SetOutputFilesJSON(string(filesJSON)).
        SetDurationMs(rec.DurationMs).
        SetStatus(rec.Status).
        SetError(rec.Error).
        SetCreatedAt(rec.CreatedAt).
        Save(ctx)
    return err
}

func (r *codeExecutorRepo) GetRecord(ctx context.Context, id string) (*biz.CodeExecutionRecord, error) {
    e, err := r.data.db.CodeExecution.Get(ctx, id)
    if err != nil {
        return nil, err
    }
    return entCodeExecutionToBiz(e), nil
}

func (r *codeExecutorRepo) ListRecords(ctx context.Context, agentID, sessionID, status string, limit, offset int) ([]biz.CodeExecutionRecord, int, error) {
    query := r.data.db.CodeExecution.Query()
    if agentID != "" {
        query = query.Where(codeexecution.AgentIDEQ(agentID))
    }
    if sessionID != "" {
        query = query.Where(codeexecution.SessionIDEQ(sessionID))
    }
    if status != "" {
        query = query.Where(codeexecution.StatusEQ(status))
    }
    total, err := query.Count(ctx)
    if err != nil {
        return nil, 0, err
    }
    items, err := query.
        Order(codeexecution.ByCreatedAt(sql.OrderDesc)).
        Limit(limit).
        Offset(offset).
        All(ctx)
    if err != nil {
        return nil, 0, err
    }
    result := make([]biz.CodeExecutionRecord, len(items))
    for i, e := range items {
        result[i] = *entCodeExecutionToBiz(e)
    }
    return result, total, nil
}

func (r *codeExecutorRepo) ListConfigs(ctx context.Context) ([]biz.ExecutorConfigEntry, error) {
    items, err := r.data.db.ExecutorConfig.Query().All(ctx)
    if err != nil {
        return nil, err
    }
    result := make([]biz.ExecutorConfigEntry, len(items))
    for i, e := range items {
        result[i] = biz.ExecutorConfigEntry{
            Type:       e.Type,
            ConfigJSON: e.ConfigJSON,
            IsDefault:  e.IsDefault,
            Available:  e.Available,
            Status:     e.Status,
            UpdatedAt:  e.UpdatedAt,
        }
    }
    return result, nil
}

func (r *codeExecutorRepo) UpdateConfig(ctx context.Context, cfg biz.ExecutorConfigEntry) (biz.ExecutorConfigEntry, error) {
    now := time.Now().Format(time.RFC3339)
    _, err := r.data.db.ExecutorConfig.Create().
        SetType(cfg.Type).
        SetConfigJSON(cfg.ConfigJSON).
        SetIsDefault(cfg.IsDefault).
        SetAvailable(cfg.Available).
        SetStatus(cfg.Status).
        SetUpdatedAt(now).
        OnConflictColumns(executorconfig.FieldType).
        UpdateNewValues().
        Save(ctx)
    if err != nil {
        return biz.ExecutorConfigEntry{}, err
    }
    cfg.UpdatedAt = now
    return cfg, nil
}

func (r *codeExecutorRepo) ListWorkspaces(ctx context.Context, agentID string, limit, offset int) ([]biz.WorkspaceInfoEntry, int, error) {
    return []biz.WorkspaceInfoEntry{}, 0, nil
}

func (r *codeExecutorRepo) CleanupWorkspace(ctx context.Context, sessionID string) error {
    return nil
}

func entCodeExecutionToBiz(e *ent.CodeExecution) *biz.CodeExecutionRecord {
    var blocks []biz.CodeBlockEntry
    _ = json.Unmarshal([]byte(e.CodeBlocksJSON), &blocks)
    var files []biz.OutputFileEntry
    _ = json.Unmarshal([]byte(e.OutputFilesJSON), &files)
    return &biz.CodeExecutionRecord{
        ID:           e.ID,
        SessionID:    e.SessionID,
        AgentID:      e.AgentID,
        ExecutorType: e.ExecutorType,
        CodeBlocks:   blocks,
        Stdout:       e.Stdout,
        Stderr:       e.Stderr,
        ExitCode:     e.ExitCode,
        OutputFiles:  files,
        DurationMs:   e.DurationMs,
        Status:       e.Status,
        Error:        e.Error,
        CreatedAt:    e.CreatedAt,
    }
}
```

### 4.5 执行器适配层

**关键设计**：项目直接使用 `pkg/trpc-agent-go/codeexecutor` 包的接口和实现，通过适配层注册到 Usecase。

#### `internal/codeexecutor/registry.go`

```go
package codeexecutor

import (
    "sync"

    "trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

type ExecutorRegistry struct {
    mu      sync.RWMutex
    entries map[string]codeexecutor.CodeExecutor
    wsReg   *codeexecutor.WorkspaceRegistry
}

func NewExecutorRegistry(wsReg *codeexecutor.WorkspaceRegistry) *ExecutorRegistry {
    return &ExecutorRegistry{
        entries: make(map[string]codeexecutor.CodeExecutor),
        wsReg:   wsReg,
    }
}

func (r *ExecutorRegistry) Register(typ string, exec codeexecutor.CodeExecutor) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.entries[typ] = exec
}

func (r *ExecutorRegistry) Get(typ string) (codeexecutor.CodeExecutor, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    e, ok := r.entries[typ]
    return e, ok
}

func (r *ExecutorRegistry) All() map[string]codeexecutor.CodeExecutor {
    r.mu.RLock()
    defer r.mu.RUnlock()
    cp := make(map[string]codeexecutor.CodeExecutor, len(r.entries))
    for k, v := range r.entries {
        cp[k] = v
    }
    return cp
}

func (r *ExecutorRegistry) WorkspaceRegistry() *codeexecutor.WorkspaceRegistry {
    return r.wsReg
}
```

#### `internal/codeexecutor/local.go`

```go
package codeexecutor

import (
    localexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor/local"
)

func NewLocalExecutor(workDir string) codeexecutor.CodeExecutor {
    opts := []localexec.CodeExecutorOption{
        localexec.WithCleanTempFiles(true),
    }
    if workDir != "" {
        opts = append(opts, localexec.WithWorkDir(workDir))
    }
    return localexec.New(opts...)
}
```

#### `internal/codeexecutor/e2b.go`

```go
package codeexecutor

import (
    "context"
    "fmt"

    e2bexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor/e2b"
    "trpc.group/trpc-go/trpc-agent-go/codeexecutor"

    "aranea-agents/internal/conf"
)

func NewE2BExecutor(cfg *conf.E2BConfig) (codeexecutor.CodeExecutor, error) {
    if cfg == nil || cfg.ApiKey == "" {
        return nil, fmt.Errorf("E2B API key not configured")
    }
    opts := []e2bexec.Option{
        e2bexec.WithAPIKey(cfg.ApiKey),
    }
    if cfg.TimeoutSeconds > 0 {
        opts = append(opts, e2bexec.WithTimeout(time.Duration(cfg.TimeoutSeconds)*time.Second))
    }
    return e2bexec.New(opts...), nil
}
```

#### `internal/codeexecutor/jupyter.go`

```go
package codeexecutor

import (
    "fmt"

    jupyterexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor/jupyter"
    "trpc.group/trpc-go/trpc-agent-go/codeexecutor"

    "aranea-agents/internal/conf"
)

func NewJupyterExecutor(cfg *conf.JupyterConfig) (codeexecutor.CodeExecutor, error) {
    if cfg == nil || cfg.BaseUrl == "" {
        return nil, fmt.Errorf("Jupyter base URL not configured")
    }
    opts := []jupyterexec.Option{
        jupyterexec.WithBaseURL(cfg.BaseUrl),
    }
    if cfg.Token != "" {
        opts = append(opts, jupyterexec.WithToken(cfg.Token))
    }
    return jupyterexec.New(opts...), nil
}
```

#### `internal/codeexecutor/container.go`

```go
package codeexecutor

import (
    "fmt"

    containerexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor/container"
    "trpc.group/trpc-go/trpc-agent-go/codeexecutor"

    "aranea-agents/internal/conf"
)

func NewContainerExecutor(cfg *conf.ContainerConfig) (codeexecutor.CodeExecutor, error) {
    if cfg == nil {
        return nil, fmt.Errorf("container config not provided")
    }
    opts := []containerexec.Option{}
    if cfg.DockerFilePath != "" {
        opts = append(opts, containerexec.WithDockerFilePath(cfg.DockerFilePath))
    }
    if cfg.Image != "" {
        opts = append(opts, containerexec.WithImage(cfg.Image))
    }
    if cfg.ContainerName != "" {
        opts = append(opts, containerexec.WithContainerName(cfg.ContainerName))
    }
    return containerexec.New(opts...), nil
}
```

### 4.6 配置结构：`internal/conf/code_executor.proto`

```protobuf
message CodeExecutorConfig {
  string default_type = 1; // "local"/"e2b"/"jupyter"/"container"
  E2BConfig e2b = 2;
  JupyterConfig jupyter = 3;
  ContainerConfig container = 4;
  LocalConfig local = 5;
}

message LocalConfig {
  string work_dir = 1;
  int32 timeout_seconds = 2;
}

message E2BConfig {
  string api_key = 1;
  int32 timeout_seconds = 2;
  string template_id = 3;
}

message JupyterConfig {
  string base_url = 1;
  string token = 2;
  string kernel_spec = 3; // "python3"/"javascript"
}

message ContainerConfig {
  string docker_file_path = 1;
  string image = 2;
  string container_name = 3;
  string host = 4;
}
```

### 4.7 初始化 Provider：`internal/codeexecutor/provider.go`

```go
package codeexecutor

import (
    "fmt"
    "log"

    "trpc.group/trpc-go/trpc-agent-go/codeexecutor"

    "aranea-agents/internal/conf"
)

func InitRegistry(cfg *conf.CodeExecutorConfig) *ExecutorRegistry {
    wsReg := codeexecutor.NewWorkspaceRegistry()
    reg := NewExecutorRegistry(wsReg)

    localCfg := cfg.GetLocal()
    workDir := ""
    if localCfg != nil {
        workDir = localCfg.WorkDir
    }
    localExec := NewLocalExecutor(workDir)
    reg.Register("local", localExec)

    e2bExec, err := NewE2BExecutor(cfg.GetE2B())
    if err != nil {
        log.Printf("[codeexecutor] E2B not available: %v", err)
    } else {
        reg.Register("e2b", e2bExec)
    }

    jupyterExec, err := NewJupyterExecutor(cfg.GetJupyter())
    if err != nil {
        log.Printf("[codeexecutor] Jupyter not available: %v", err)
    } else {
        reg.Register("jupyter", jupyterExec)
    }

    containerExec, err := NewContainerExecutor(cfg.GetContainer())
    if err != nil {
        log.Printf("[codeexecutor] Container not available: %v", err)
    } else {
        reg.Register("container", containerExec)
    }

    return reg
}
```

### 4.8 Agent 构建集成：修改 `internal/agent/trpc_build.go`

修改 `buildSkillDeps` 函数，根据 Agent 配置选择执行器：

```go
func buildSkillDeps(ctx context.Context, deps TRPCBuilderDeps) (trpcskill.Repository, trpcskill.VisibilityFilter, codeexecutor.CodeExecutor, error) {
    slugs, err := deps.SkillUC.ListEnabledPublishedSkillKeys(ctx)
    if err != nil || len(slugs) == 0 {
        return nil, nil, nil, err
    }

    rootDir := skillstorage.ResolveRoot()
    if deps.Sys != nil {
        if st, e := deps.Sys.Get(ctx); e == nil {
            rootDir = skillstorage.ResolveRootWithPlatform(st.RootDirectory)
        }
    }

    repo, err := skilltrpc.NewFSRepositoryAdapter(rootDir)
    if err != nil {
        return nil, nil, nil, err
    }

    allowSet := strutil.SliceToSet(slugs)
    filter := func(_ context.Context, summary trpcskill.Summary) bool {
        name := strings.TrimSpace(strings.ToLower(summary.Name))
        return allowSet[name]
    }

    var exec codeexecutor.CodeExecutor
    if deps.ExecutorReg != nil {
        executorType := "local"
        if deps.Agent != nil && deps.Agent.Settings != nil && deps.Agent.Settings.CodeExecutorType != "" {
            executorType = deps.Agent.Settings.CodeExecutorType
        }
        if e, ok := deps.ExecutorReg.Get(executorType); ok {
            exec = e
        }
    }
    if exec == nil {
        exec = skilltrpc.NewLocalExecutor(rootDir)
    }

    return repo, filter, exec, nil
}
```

在 `TRPCBuilderDeps` 中新增：

```go
ExecutorReg *codeexecutor.ExecutorRegistry
```

---

## 五、Service 层

### 5.1 `internal/service/code_executor.go`

```go
package service

import (
    "context"

    v1 "aranea-agents/api/kratos/code_executor/v1"
    "aranea-agents/internal/biz"
)

type CodeExecutorService struct {
    v1.UnimplementedCodeExecutorServiceServer
    uc *biz.CodeExecutorUsecase
}

func NewCodeExecutorService(uc *biz.CodeExecutorUsecase) *CodeExecutorService {
    return &CodeExecutorService{uc: uc}
}

func (s *CodeExecutorService) ExecuteCode(ctx context.Context, req *v1.ExecuteCodeRequest) (*v1.ExecuteCodeResponse, error) {
    var blocks []biz.CodeBlockEntry
    for _, b := range req.CodeBlocks {
        blocks = append(blocks, biz.CodeBlockEntry{Code: b.Code, Language: b.Language})
    }
    var outputSpec *biz.OutputSpecEntry
    if req.OutputSpec != nil {
        outputSpec = &biz.OutputSpecEntry{
            Globs:         req.OutputSpec.Globs,
            MaxFiles:      int(req.OutputSpec.MaxFiles),
            MaxFileBytes:  req.OutputSpec.MaxFileBytes,
            MaxTotalBytes: req.OutputSpec.MaxTotalBytes,
            Save:          req.OutputSpec.Save,
            NameTemplate:  req.OutputSpec.NameTemplate,
            Inline:        req.OutputSpec.Inline,
        }
    }
    rec, err := s.uc.Execute(ctx, req.AgentId, req.SessionId, blocks, req.ExecutorType, req.InputFiles, outputSpec)
    if err != nil {
        return nil, err
    }
    return codeExecutionRecordToProtoResponse(rec), nil
}

func (s *CodeExecutorService) ListExecutors(ctx context.Context, req *v1.ListExecutorsRequest) (*v1.ListExecutorsResponse, error) {
    configs, err := s.uc.ListExecutors(ctx)
    if err != nil {
        return nil, err
    }
    var executors []*v1.ExecutorConfig
    for _, c := range configs {
        executors = append(executors, &v1.ExecutorConfig{
            Type:       c.Type,
            ConfigJson: c.ConfigJSON,
            IsDefault:  c.IsDefault,
            Available:  c.Available,
            Status:     c.Status,
            UpdatedAt:  c.UpdatedAt,
        })
    }
    return &v1.ListExecutorsResponse{Executors: executors}, nil
}

func (s *CodeExecutorService) GetExecutorStatus(ctx context.Context, req *v1.GetExecutorStatusRequest) (*v1.ExecutorStatus, error) {
    info, err := s.uc.GetStatus(ctx, req.Type)
    if err != nil {
        return nil, err
    }
    return &v1.ExecutorStatus{
        Type:           info.Type,
        Available:      info.Available,
        StatusMessage:  info.StatusMessage,
        Version:        info.Version,
        Capabilities:   info.Capabilities,
    }, nil
}

func (s *CodeExecutorService) UpdateExecutorConfig(ctx context.Context, req *v1.UpdateExecutorConfigRequest) (*v1.ExecutorConfig, error) {
    cfg, err := s.uc.UpdateConfig(ctx, biz.ExecutorConfigEntry{
        Type:       req.Type,
        ConfigJSON: req.ConfigJson,
        IsDefault:  req.IsDefault,
    })
    if err != nil {
        return nil, err
    }
    return &v1.ExecutorConfig{
        Type:       cfg.Type,
        ConfigJson: cfg.ConfigJSON,
        IsDefault:  cfg.IsDefault,
        Available:  cfg.Available,
        Status:     cfg.Status,
        UpdatedAt:  cfg.UpdatedAt,
    }, nil
}

func (s *CodeExecutorService) ListWorkspaces(ctx context.Context, req *v1.ListWorkspacesRequest) (*v1.ListWorkspacesResponse, error) {
    items, total, err := s.uc.ListWorkspaces(ctx, req.AgentId, int(req.Limit), int(req.Offset))
    if err != nil {
        return nil, err
    }
    var ws []*v1.WorkspaceInfo
    for _, w := range items {
        ws = append(ws, &v1.WorkspaceInfo{
            SessionId:     w.SessionID,
            Path:          w.Path,
            ExecutorType:  w.ExecutorType,
            CreatedAt:     w.CreatedAt,
            DiskUsageBytes: w.DiskUsageBytes,
        })
    }
    return &v1.ListWorkspacesResponse{Workspaces: ws, Total: int32(total)}, nil
}

func (s *CodeExecutorService) CleanupWorkspace(ctx context.Context, req *v1.CleanupWorkspaceRequest) (*emptypb.Empty, error) {
    err := s.uc.CleanupWorkspace(ctx, req.SessionId)
    if err != nil {
        return nil, err
    }
    return &emptypb.Empty{}, nil
}

func (s *CodeExecutorService) ListExecutionHistory(ctx context.Context, req *v1.ListExecutionHistoryRequest) (*v1.ListExecutionHistoryResponse, error) {
    items, total, err := s.uc.ListHistory(ctx, req.AgentId, req.SessionId, req.Status, int(req.Limit), int(req.Offset))
    if err != nil {
        return nil, err
    }
    var execs []*v1.CodeExecution
    for _, r := range items {
        execs = append(execs, codeExecutionRecordToProto(&r))
    }
    return &v1.ListExecutionHistoryResponse{Executions: execs, Total: int32(total)}, nil
}

func (s *CodeExecutorService) GetExecution(ctx context.Context, req *v1.GetExecutionRequest) (*v1.CodeExecution, error) {
    rec, err := s.uc.GetExecution(ctx, req.Id)
    if err != nil {
        return nil, err
    }
    return codeExecutionRecordToProto(rec), nil
}

func codeExecutionRecordToProto(r *biz.CodeExecutionRecord) *v1.CodeExecution {
    var blocks []*v1.CodeBlock
    for _, b := range r.CodeBlocks {
        blocks = append(blocks, &v1.CodeBlock{Code: b.Code, Language: b.Language})
    }
    var files []*v1.OutputFile
    for _, f := range r.OutputFiles {
        files = append(files, &v1.OutputFile{
            Name:      f.Name,
            Content:   f.Content,
            MimeType:  f.MIMEType,
            SizeBytes: f.SizeBytes,
            Truncated: f.Truncated,
        })
    }
    return &v1.CodeExecution{
        Id:           r.ID,
        SessionId:    r.SessionID,
        AgentId:      r.AgentID,
        ExecutorType: r.ExecutorType,
        CodeBlocks:   blocks,
        Stdout:       r.Stdout,
        Stderr:       r.Stderr,
        ExitCode:     r.ExitCode,
        OutputFiles:  files,
        DurationMs:   r.DurationMs,
        Status:       r.Status,
        Error:        r.Error,
        CreatedAt:    r.CreatedAt,
    }
}

func codeExecutionRecordToProtoResponse(r *biz.CodeExecutionRecord) *v1.ExecuteCodeResponse {
    var files []*v1.OutputFile
    for _, f := range r.OutputFiles {
        files = append(files, &v1.OutputFile{
            Name:      f.Name,
            Content:   f.Content,
            MimeType:  f.MIMEType,
            SizeBytes: f.SizeBytes,
            Truncated: f.Truncated,
        })
    }
    return &v1.ExecuteCodeResponse{
        ExecutionId:  r.ID,
        Output:       r.Stdout,
        OutputFiles:  files,
        ExecutorType: r.ExecutorType,
        DurationMs:   r.DurationMs,
        Error:        r.Error,
    }
}
```

---

## 六、Wire 注入

### 6.1 新增 Provider

```
internal/codeexecutor/provider.go  → InitRegistry (读取配置初始化执行器注册表)
internal/data/code_executor.go     → NewCodeExecutorRepo
internal/biz/code_executor.go      → NewCodeExecutorUsecase
internal/service/code_executor.go  → NewCodeExecutorService
```

### 6.2 Wire 集成

在 `cmd/admin/wire.go` 中：

```go
// 新增 provider
codeexecutor.InitRegistry,
data.NewCodeExecutorRepo,
biz.NewCodeExecutorUsecase,
service.NewCodeExecutorService,
```

在 `TRPCBuilderDeps` 中注入 `ExecutorReg`。

### 6.3 Kratos Server 注册

在 `internal/server/http.go` 中注册：

```go
codeExecutorSvc := service.NewCodeExecutorService(wireApp.CodeExecutorUsecase)
v1.RegisterCodeExecutorServiceHTTPServer(srv, codeExecutorSvc)
```

---

## 七、Web 前端设计

### 7.1 页面与组件

#### 新增页面：`web/src/pages/CodeExecutorPage.vue`

代码执行管理页面，包含执行器状态、工作区管理、执行历史三个 Tab。

#### 新增组件目录：`web/src/components/codeexecutor/`

| 组件 | 说明 |
|------|------|
| `ExecutorStatusCard.vue` | 单个执行器状态卡片，显示类型、可用性、能力 |
| `ExecutorConfigDialog.vue` | 执行器配置编辑弹窗（E2B API Key、Jupyter URL 等） |
| `ExecutorTypeSelector.vue` | 执行器类型选择器（嵌入 Agent 设置页） |
| `WorkspaceListTable.vue` | 工作区列表表格 |
| `ExecutionHistoryTable.vue` | 执行历史列表表格 |
| `ExecutionDetailDrawer.vue` | 执行详情抽屉（代码、输出、产出物） |
| `CodeResultBlock.vue` | 代码执行结果展示块（嵌入 Chat 消息流） |
| `OutputFilePreview.vue` | 产出物文件预览（图片/PDF/文本） |

### 7.2 Feature 模块：`web/src/features/codeexecutor/`

#### `api.ts`

```typescript
import { createCodeExecutorService } from "../../services";

export interface CodeBlock {
  code: string;
  language: string;
}

export interface InputSpec {
  from: string;
  to: string;
  mode: string;
}

export interface OutputSpec {
  globs: string[];
  max_files: number;
  max_file_bytes: number;
  max_total_bytes: number;
  save: boolean;
  name_template: string;
  inline: boolean;
}

export interface OutputFile {
  name: string;
  content: string;
  mime_type: string;
  size_bytes: number;
  truncated: boolean;
}

export interface ExecutorConfig {
  type: string;
  config_json: string;
  is_default: boolean;
  available: boolean;
  status: string;
  updated_at: string;
}

export interface ExecutorStatus {
  type: string;
  available: boolean;
  status_message: string;
  version: string;
  capabilities: Record<string, string>;
}

export interface CodeExecutionRecord {
  id: string;
  session_id: string;
  agent_id: string;
  executor_type: string;
  code_blocks: CodeBlock[];
  stdout: string;
  stderr: string;
  exit_code: number;
  output_files: OutputFile[];
  duration_ms: number;
  status: string;
  error: string;
  created_at: string;
}

export interface WorkspaceInfo {
  session_id: string;
  path: string;
  executor_type: string;
  created_at: string;
  disk_usage_bytes: number;
}

export async function executeCode(req: {
  agent_id: string;
  session_id: string;
  code_blocks: CodeBlock[];
  executor_type?: string;
  input_files?: Record<string, string>;
  inputs?: InputSpec[];
  output_spec?: OutputSpec;
  timeout_seconds?: number;
}): Promise<{
  execution_id: string;
  output: string;
  output_files: OutputFile[];
  executor_type: string;
  duration_ms: number;
  error: string;
}> {
  const svc = createCodeExecutorService();
  const res = await svc.ExecuteCode({
    agentId: req.agent_id,
    sessionId: req.session_id,
    codeBlocks: req.code_blocks,
    executorType: req.executor_type ?? "",
    inputFiles: req.input_files,
    inputs: req.inputs,
    outputSpec: req.output_spec,
    timeoutSeconds: req.timeout_seconds ?? 0,
  });
  return {
    execution_id: res.executionId,
    output: res.output,
    output_files: (res.outputFiles ?? []).map(normalizeOutputFile),
    executor_type: res.executorType,
    duration_ms: res.durationMs,
    error: res.error,
  };
}

export async function listExecutors(): Promise<ExecutorConfig[]> {
  const svc = createCodeExecutorService();
  const res = await svc.ListExecutors({});
  return (res.executors ?? []).map(normalizeExecutorConfig);
}

export async function getExecutorStatus(type: string): Promise<ExecutorStatus> {
  const svc = createCodeExecutorService();
  const res = await svc.GetExecutorStatus({ type });
  return {
    type: res.type,
    available: res.available,
    status_message: res.statusMessage,
    version: res.version,
    capabilities: res.capabilities ?? {},
  };
}

export async function updateExecutorConfig(
  type: string,
  req: { config_json: string; is_default: boolean }
): Promise<ExecutorConfig> {
  const svc = createCodeExecutorService();
  const res = await svc.UpdateExecutorConfig({
    type,
    configJson: req.config_json,
    isDefault: req.is_default,
  });
  return normalizeExecutorConfig(res);
}

export async function listWorkspaces(
  agentId: string,
  limit = 20,
  offset = 0
): Promise<{ items: WorkspaceInfo[]; total: number }> {
  const svc = createCodeExecutorService();
  const res = await svc.ListWorkspaces({ agentId, limit, offset });
  return {
    items: (res.workspaces ?? []).map(normalizeWorkspaceInfo),
    total: res.total,
  };
}

export async function cleanupWorkspace(sessionId: string): Promise<void> {
  const svc = createCodeExecutorService();
  await svc.CleanupWorkspace({ sessionId });
}

export async function listExecutionHistory(req: {
  agent_id?: string;
  session_id?: string;
  status?: string;
  limit?: number;
  offset?: number;
}): Promise<{ items: CodeExecutionRecord[]; total: number }> {
  const svc = createCodeExecutorService();
  const res = await svc.ListExecutionHistory({
    agentId: req.agent_id ?? "",
    sessionId: req.session_id ?? "",
    status: req.status ?? "",
    limit: req.limit ?? 20,
    offset: req.offset ?? 0,
  });
  return {
    items: (res.executions ?? []).map(normalizeCodeExecution),
    total: res.total,
  };
}

export async function getExecution(id: string): Promise<CodeExecutionRecord> {
  const svc = createCodeExecutorService();
  const res = await svc.GetExecution({ id });
  return normalizeCodeExecution(res);
}

function normalizeOutputFile(f: any): OutputFile {
  return {
    name: f.name ?? "",
    content: f.content ?? "",
    mime_type: f.mimeType ?? "",
    size_bytes: Number(f.sizeBytes ?? 0),
    truncated: f.truncated ?? false,
  };
}

function normalizeExecutorConfig(c: any): ExecutorConfig {
  return {
    type: c.type ?? "",
    config_json: c.configJson ?? "{}",
    is_default: c.isDefault ?? false,
    available: c.available ?? false,
    status: c.status ?? "unknown",
    updated_at: c.updatedAt ?? "",
  };
}

function normalizeWorkspaceInfo(w: any): WorkspaceInfo {
  return {
    session_id: w.sessionId ?? "",
    path: w.path ?? "",
    executor_type: w.executorType ?? "",
    created_at: w.createdAt ?? "",
    disk_usage_bytes: Number(w.diskUsageBytes ?? 0),
  };
}

function normalizeCodeExecution(e: any): CodeExecutionRecord {
  return {
    id: e.id ?? "",
    session_id: e.sessionId ?? "",
    agent_id: e.agentId ?? "",
    executor_type: e.executorType ?? "",
    code_blocks: (e.codeBlocks ?? []).map((b: any) => ({
      code: b.code ?? "",
      language: b.language ?? "",
    })),
    stdout: e.stdout ?? "",
    stderr: e.stderr ?? "",
    exit_code: Number(e.exitCode ?? 0),
    output_files: (e.outputFiles ?? []).map(normalizeOutputFile),
    duration_ms: Number(e.durationMs ?? 0),
    status: e.status ?? "unknown",
    error: e.error ?? "",
    created_at: e.createdAt ?? "",
  };
}
```

#### `types.ts`

```typescript
export type {
  CodeBlock,
  InputSpec,
  OutputSpec,
  OutputFile,
  ExecutorConfig,
  ExecutorStatus,
  CodeExecutionRecord,
  WorkspaceInfo,
} from "./api";
```

### 7.3 Agent 设置页集成

在 `AgentSettingsPage.vue` 中新增"代码执行"Tab：

- **执行器类型选择**：`ExecutorTypeSelector.vue` 下拉选择 `local`/`e2b`/`jupyter`/`container`
- 选择后自动保存到 `AgentRuntimeSettings.code_executor_type`
- 显示当前执行器状态（可用/不可用）

### 7.4 Chat 消息集成

在 `ChatMessagePanel.vue` 中，当消息包含代码执行结果时：

- 使用 `CodeResultBlock.vue` 展示：
  - 代码块（语法高亮）
  - 执行输出（stdout/stderr）
  - 产出物文件列表（可点击预览/下载）
  - 执行耗时、退出码
  - 执行器类型标签

### 7.5 系统设置集成

在 `SystemSettingsPage.vue` 中新增"代码执行器"区域：

- 各执行器状态一览（Local/E2B/Jupyter/Container）
- 配置编辑（API Key、URL、镜像等）
- 默认执行器选择
- 工作区管理（查看活跃工作区、清理）

### 7.6 路由

在 `web/src/router/routes.ts` 中新增：

```typescript
{
  path: "/code-executor",
  name: "CodeExecutor",
  component: () => import("pages/CodeExecutorPage.vue"),
  meta: { title: "代码执行", icon: "terminal", requiresAuth: true },
},
```

### 7.7 导航

在 `web/src/config/sideNav.ts` 中新增"代码执行"导航项，放在"工具"和"Skills"之间。

---

## 八、数据库迁移

### 8.1 新增表

```sql
CREATE TABLE IF NOT EXISTS code_executions (
    id TEXT PRIMARY KEY,
    session_id TEXT,
    agent_id TEXT,
    executor_type TEXT NOT NULL DEFAULT 'local',
    code_blocks_json TEXT NOT NULL DEFAULT '[]',
    stdout TEXT NOT NULL DEFAULT '',
    stderr TEXT NOT NULL DEFAULT '',
    exit_code INTEGER NOT NULL DEFAULT 0,
    output_files_json TEXT NOT NULL DEFAULT '[]',
    duration_ms INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending',
    error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_code_executions_agent_id ON code_executions(agent_id);
CREATE INDEX IF NOT EXISTS idx_code_executions_session_id ON code_executions(session_id);
CREATE INDEX IF NOT EXISTS idx_code_executions_status ON code_executions(status);
CREATE INDEX IF NOT EXISTS idx_code_executions_created_at ON code_executions(created_at);

CREATE TABLE IF NOT EXISTS executor_configs (
    type TEXT PRIMARY KEY,
    config_json TEXT NOT NULL DEFAULT '{}',
    is_default INTEGER NOT NULL DEFAULT 0,
    available INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'unconfigured',
    updated_at TEXT NOT NULL DEFAULT ''
);
```

### 8.2 Agent Runtime Settings 扩展

```sql
ALTER TABLE agent_runtime_settings ADD COLUMN code_executor_type TEXT NOT NULL DEFAULT 'local';
```

---

## 九、实现顺序

1. **Phase 1**：Ent Schema + 数据库迁移 + Biz 模型 + Repo 接口
2. **Phase 2**：执行器适配层（Local/E2B/Jupyter/Container）+ Registry
3. **Phase 3**：Biz Usecase + Service 层 + Wire 注入
4. **Phase 4**：Agent 构建集成（`trpc_build.go` 修改）
5. **Phase 5**：Web 前端（Agent 设置集成 + 系统设置 + 执行历史）
6. **Phase 6**：Chat 消息集成（CodeResultBlock）
